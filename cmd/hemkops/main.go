// Command hemkops is a proof-of-concept scraper for hemkop.se prices.
//
// It renders a handful of category pages with a real browser (the product
// grid is client-side), extracts the price / amount / category / image for
// each product and prints the rows it would insert into a database.
//
//	# default run: three categories, five products each
//	go run ./cmd/hemkops
//
//	# pick categories by slug substring
//	go run ./cmd/hemkops -match 'skafferi/asien' -match 'mejeri-ost-och-agg/ost'
//
//	# see what is available
//	go run ./cmd/hemkops -list
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ebinbellini/hemkopdle/scraper"
)

// defaultCategories are known-good leaf categories, chosen because between
// them they cover the three shapes the grid produces:
//
//   - sauces, which is the Oyster Sauce example from the brief
//   - oils, a plain per-piece listing
//   - grapes, which mixes per-piece and per-kilo (weight-priced) items
var defaultCategories = []string{
	"skafferi/asien/saser-och-grytbaser",
	"skafferi/olja-och-vinager/olivolja",
	"frukt-och-gront/frukt/druvor",
}

func main() {
	var (
		matches      multiFlag
		perCategory  = flag.Int("products", 5, "max products to read per category (0 = all)")
		delay        = flag.Duration("delay", 10*time.Second, "pause between page loads (robots.txt crawl-delay)")
		renderWait   = flag.Duration("render-timeout", 30*time.Second, "how long to wait for the product grid")
		headful      = flag.Bool("headful", false, "show the browser window (useful for debugging)")
		chromePath   = flag.String("chrome", "", "path to the Chrome/Chromium binary (default: auto-detect)")
		jsonOut      = flag.String("json-out", "", "also write the rows as JSON to this file")
		listOnly     = flag.Bool("list", false, "list matching categories and exit")
		showFullPath = flag.Bool("full-url", false, "print the absolute product URL in the table")
	)
	flag.Var(&matches, "match", "only use categories whose slug path contains this substring (repeatable)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	warnAboutRobots(*delay)

	// 1. Discover the category tree from the public sitemap.
	fmt.Println("→ reading category sitemap…")
	hc := scraper.HTTPClient()
	all, err := scraper.DiscoverCategories(ctx, hc)
	if err != nil {
		fatalf("discover categories: %v", err)
	}
	fmt.Printf("  found %d categories\n", len(all))

	// With no -match flags, scrape the curated defaults rather than the whole
	// tree: there are 528 leaf categories and robots.txt asks for a 10 second
	// crawl-delay, so an unfiltered run would take well over an hour.
	patterns := []string(matches)
	if len(patterns) == 0 && !*listOnly {
		patterns = defaultCategories
	}

	wanted := scraper.FindCategories(all, patterns...)
	if len(wanted) == 0 {
		fatalf("no categories matched %v", patterns)
	}

	if *listOnly {
		for _, c := range wanted {
			fmt.Printf("  %-60s %s\n", strings.Join(c.Slugs, "/"), c.Path())
		}
		return
	}

	// 2. Render each category page and read the grid.
	client, err := scraper.New(scraper.Options{
		ChromePath: *chromePath,
		Headful:    *headful,
		Delay:      *delay,
	})
	if err != nil {
		fatalf("start browser: %v", err)
	}
	defer client.Close()

	var rows []scraper.Product
	for i, cat := range wanted {
		if i > 0 {
			client.Sleep(ctx)
		}

		fmt.Printf("→ scraping /%s\n", strings.Join(cat.Slugs, "/"))
		resolved, products, err := client.ScrapeCategory(ctx, cat, *perCategory, *renderWait)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! %v\n", err)
			continue
		}
		fmt.Printf("  %s — kept %d product(s)\n", resolved.Path(), len(products))
		rows = append(rows, products...)
	}

	if len(rows) == 0 {
		fatalf("no products scraped")
	}

	// 3. Print what would be stored.
	fmt.Printf("\n%d rows ready to insert\n\n", len(rows))
	printTable(rows, *showFullPath)

	if *jsonOut != "" {
		if err := writeJSON(*jsonOut, rows); err != nil {
			fatalf("write %s: %v", *jsonOut, err)
		}
		fmt.Printf("\nwrote %s\n", *jsonOut)
	} else {
		fmt.Println("\nThe same rows as JSON (this is the DB payload):")
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(rows); err != nil {
			fatalf("encode rows: %v", err)
		}
	}
}

func printTable(rows []scraper.Product, fullURL bool) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tBRAND\tAMOUNT\tPRICE\tUNIT\tCATEGORY\tID")
	fmt.Fprintln(w, "\t\t\t\t\t\t")

	for _, p := range rows {
		amount := p.Amount
		if amount == "" {
			// Weight-priced goods have no fixed package size.
			amount = "(by weight)"
		}
		category := p.CategoryPath
		if fullURL {
			category = p.URL
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%.2f\t%s\t%s\t%s\n",
			truncate(p.Name, 42),
			truncate(p.Brand, 18),
			amount,
			p.Price,
			p.PriceUnit,
			category,
			p.ID,
		)
	}
	w.Flush()
}

func writeJSON(path string, rows []scraper.Product) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(rows)
}

// warnAboutRobots surfaces the site's stated crawl policy. The default delay
// already follows the crawl-delay; the visit window is only reported, since
// honouring it would be surprising for an interactive proof of concept.
func warnAboutRobots(delay time.Duration) {
	fmt.Println("note: hemkop.se/robots.txt asks for Crawl-delay: 10 and")
	fmt.Println("      Visit-time: 0400-0845 UTC (06:00-10:45 Swedish time).")

	if delay < 10*time.Second {
		fmt.Printf("      -delay is %s, below the requested crawl-delay.\n", delay)
	}
	now := time.Now().UTC()
	if now.Hour() < 4 || now.Hour() >= 8 || (now.Hour() == 8 && now.Minute() > 45) {
		fmt.Printf("      Current UTC time %s is outside that window.\n", now.Format("15:04"))
	}
	fmt.Println()
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

// multiFlag collects a repeatable string flag.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
