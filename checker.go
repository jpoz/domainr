package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

type DomainStatus int

const (
	StatusUnknown DomainStatus = iota
	StatusAvailable
	StatusTaken
)

type DomainResult struct {
	Domain string
	Status DomainStatus
	Price  string
}

var errCloudflareBlocked = errors.New("blocked by Cloudflare challenge")

func CheckDomains(domains []string, headless bool) ([]DomainResult, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("launching playwright: %w", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
		Args:     []string{"--disable-blink-features=AutomationControlled"},
	})
	if err != nil {
		return nil, fmt.Errorf("launching browser: %w", err)
	}
	defer browser.Close()

	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"),
	})
	if err != nil {
		return nil, fmt.Errorf("creating page: %w", err)
	}

	// Hide webdriver property to avoid bot detection
	page.AddInitScript(playwright.Script{
		Content: playwright.String(`Object.defineProperty(navigator, 'webdriver', {get: () => undefined})`),
	})

	// Build a lookup set for requested domains
	wanted := make(map[string]bool)
	for _, d := range domains {
		wanted[strings.ToLower(d)] = true
	}
	found := make(map[string]DomainResult)

	// Search for the first domain — Namecheap shows related TLDs too
	if err := searchWithRetry(page, domains[0], wanted, found); err != nil {
		return nil, fmt.Errorf("searching for %s: %w", domains[0], err)
	}

	// Search individually for any domains not found in the first search
	for _, d := range domains {
		if _, ok := found[strings.ToLower(d)]; ok {
			continue
		}
		// Delay between requests to avoid triggering rate limits
		time.Sleep(1500 * time.Millisecond)

		if err := searchWithRetry(page, d, wanted, found); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to check %s: %v\n", d, err)
		}
	}

	// Build results in original order
	var results []DomainResult
	for _, d := range domains {
		key := strings.ToLower(d)
		if r, ok := found[key]; ok {
			results = append(results, r)
		} else {
			results = append(results, DomainResult{Domain: d, Status: StatusUnknown})
		}
	}

	return results, nil
}

const maxRetries = 3

func searchWithRetry(page playwright.Page, query string, wanted map[string]bool, found map[string]DomainResult) error {
	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			backoff := time.Duration(attempt*3) * time.Second
			fmt.Fprintf(os.Stderr, "Retrying %s in %v (attempt %d/%d)...\n", query, backoff, attempt+1, maxRetries)
			time.Sleep(backoff)
		}

		lastErr = searchAndScrape(page, query, wanted, found)
		if lastErr == nil {
			return nil
		}

		// Only retry on Cloudflare blocks
		if !errors.Is(lastErr, errCloudflareBlocked) {
			return lastErr
		}
	}
	return fmt.Errorf("giving up after %d attempts: %w", maxRetries, lastErr)
}

func searchAndScrape(page playwright.Page, query string, wanted map[string]bool, found map[string]DomainResult) error {
	url := fmt.Sprintf("https://www.namecheap.com/domains/registration/results/?domain=%s", query)

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	}); err != nil {
		return fmt.Errorf("navigating to namecheap: %w", err)
	}

	// Wait for Cloudflare challenge to pass and first result to appear
	err := page.Locator("article[class*='domain-']").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(30000),
	})
	if err != nil {
		// Check if we're stuck on a Cloudflare challenge page
		title, _ := page.Title()
		if strings.Contains(strings.ToLower(title), "just a moment") {
			return fmt.Errorf("%w: page stuck on challenge for %s", errCloudflareBlocked, query)
		}
		return fmt.Errorf("waiting for results for %s (possibly rate limited): %w", query, err)
	}

	// Poll until the article count stabilizes instead of a fixed 2s wait.
	// Checks every 400ms, exits once count is stable for one interval (max ~2s).
	articleLocator := page.Locator("article[class*='domain-']")
	prevCount := 0
	for range 5 {
		time.Sleep(400 * time.Millisecond)
		count, _ := articleLocator.Count()
		if count > 0 && count == prevCount {
			break
		}
		prevCount = count
	}

	return scrapeResults(page, wanted, found)
}

func scrapeResults(page playwright.Page, wanted map[string]bool, found map[string]DomainResult) error {
	articles, err := page.Locator("article[class*='domain-']").All()
	if err != nil {
		return fmt.Errorf("querying results: %w", err)
	}

	for _, article := range articles {
		result, err := parseArticle(article)
		if err != nil {
			continue
		}
		key := strings.ToLower(result.Domain)
		if wanted[key] {
			found[key] = result
		}
	}
	return nil
}

func parseArticle(article playwright.Locator) (DomainResult, error) {
	var result DomainResult

	// Get the domain name from h2 inside .domain-name .name
	nameLocator := article.Locator(".domain-name .name h2")
	count, _ := nameLocator.Count()
	if count == 0 {
		// Fallback: try just h2
		nameLocator = article.Locator("h2")
		count, _ = nameLocator.Count()
		if count == 0 {
			return result, fmt.Errorf("no domain name found")
		}
	}

	name, err := nameLocator.First().TextContent()
	if err != nil {
		return result, fmt.Errorf("getting domain text: %w", err)
	}
	result.Domain = strings.TrimSpace(name)

	// Skip non-domain articles (product-ssl, product-vpn, etc.)
	if result.Domain == "" {
		return result, fmt.Errorf("empty domain name")
	}

	// Determine availability from the article's classes
	classes, err := article.GetAttribute("class")
	if err == nil {
		classList := strings.ToLower(classes)
		if strings.Contains(classList, " available") {
			result.Status = StatusAvailable
		} else if strings.Contains(classList, " unavailable") {
			result.Status = StatusTaken
		}
	}

	// Get price from .price strong
	priceLocator := article.Locator(".price strong")
	priceCount, _ := priceLocator.Count()
	if priceCount > 0 {
		price, err := priceLocator.First().TextContent()
		if err == nil {
			result.Price = strings.TrimSpace(price)
		}
	}

	return result, nil
}
