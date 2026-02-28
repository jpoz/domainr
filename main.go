package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorPurple = "\033[35m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

var domainRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z]{2,})+$`)

func main() {
	visible := flag.Bool("visible", false, "Show the browser window (useful for debugging)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: domainr [flags] <domain> [domain...]\n\nCheck domain name availability via Namecheap.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	domains := flag.Args()
	if len(domains) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	for _, d := range domains {
		if !domainRegex.MatchString(d) {
			fmt.Fprintf(os.Stderr, "Invalid domain: %s\n", d)
			os.Exit(1)
		}
	}

	results, err := CheckDomains(domains, !*visible)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	printResults(results)
}

func printResults(results []DomainResult) {
	// Find the longest domain name for alignment
	maxLen := 0
	for _, r := range results {
		if len(r.Domain) > maxLen {
			maxLen = len(r.Domain)
		}
	}

	fmt.Println()
	for _, r := range results {
		padded := r.Domain + strings.Repeat(" ", maxLen-len(r.Domain))
		switch r.Status {
		case StatusAvailable:
			fmt.Printf("  %s%s%s  %s%s Available %s  %s%s%s\n",
				colorBold, padded, colorReset,
				colorGreen, colorBold, colorReset,
				colorDim, r.Price, colorReset)
		case StatusPremium:
			fmt.Printf("  %s%s%s  %s%s Premium   %s\n",
				colorBold, padded, colorReset,
				colorPurple, colorBold, colorReset)
		case StatusTaken:
			fmt.Printf("  %s%s%s  %s%s Taken     %s\n",
				colorBold, padded, colorReset,
				colorRed, colorBold, colorReset)
		default:
			reason := ""
			if r.Reason != "" {
				reason = fmt.Sprintf("  %s(%s)%s", colorDim, r.Reason, colorReset)
			}
			fmt.Printf("  %s%s%s  %s%s Unknown   %s%s\n",
				colorBold, padded, colorReset,
				colorYellow, colorBold, colorReset,
				reason)
		}
	}
	fmt.Println()
}
