# domainr

A CLI tool to check domain name availability via Namecheap. Uses Playwright to scrape search results and displays availability with pricing.

## Requirements

- Go 1.23+
- Playwright browsers installed (`go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps chromium`)

## Install

```sh
go install github.com/jpoz/domainr@latest
```

## Usage

```sh
domainr <domain> [domain...]
```

Check one or more domains:

```sh
domainr example.com example.io example.dev
```

### Flags

- `-visible` — Show the browser window (useful for debugging)

## Example

```
$ domainr coolproject.com coolproject.io coolproject.dev

  coolproject.com   Taken
  coolproject.io    Available  $29.98/yr
  coolproject.dev   Available  $12.98/yr
```
