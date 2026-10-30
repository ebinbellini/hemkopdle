package scraper

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	server "github.com/ebinbellini/hemkopdle/server"
)

const (
	baseURL           = "https://www.hemkop.se"
	sortimentPrefix   = baseURL + "/sortiment/"
	categoryTreePath  = "/leftMenu/categorytree"
	categoryCacheFile = "/tmp/hemkodple_category_map_cache.bin"
)

// categoryNode is one node of the /leftMenu/categorytree response. Only the
// fields we use are modelled; id, category and valid are ignored.
type categoryNode struct {
	Title    string         `json:"title"`
	URL      string         `json:"url"`
	Children []categoryNode `json:"children"`
}

// CategoryRawData mirrors the parts of the category search response we need
type CategoryRawData struct {
	Results []struct {
		Code             string  `json:"code"`
		Name             string  `json:"name"`
		Manufacturer     string  `json:"manufacturer"`
		PriceValue       float64 `json:"priceValue"`
		PriceUnit        string  `json:"priceUnit"`
		DisplayVolume    string  `json:"displayVolume"`
		ComparePriceUnit string  `json:"comparePriceUnit"`
		Image            struct {
			URL string `json:"url"`
		} `json:"image"`
	} `json:"results"`
}

// DiscoverCategories fetches the category tree and flattens it into a sorted
// list of categories, each knowing its own slugs and display names.
func DiscoverCategories(hc *http.Client) ([]server.Category, error) {
	resp, err := hc.Get(baseURL + categoryTreePath)
	if err != nil {
		return nil, fmt.Errorf("category tree: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("category tree: unexpected status %s", resp.Status)
	}

	var root categoryNode
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		return nil, fmt.Errorf("parse category tree: %w", err)
	}

	seen := make(map[string]bool)
	cats := make([]server.Category, 0, 256)

	var walk func(node categoryNode, parentNames []string)
	walk = func(node categoryNode, parentNames []string) {
		path := strings.Trim(node.URL, "/")
		if path == "" || strings.Contains(path, "alla-varor") {
			return
		}

		names := make([]string, 0, len(parentNames)+1)
		names = append(names, parentNames...)
		names = append(names, strings.TrimSpace(node.Title))

		if !seen[path] {
			seen[path] = true
			cats = append(cats, server.Category{
				Slugs: strings.Split(path, "/"),
				Names: names,
				URL:   sortimentPrefix + path,
			})
		}

		for _, child := range node.Children {
			walk(child, names)
		}
	}

	// The root is "Alla varor"; its children are the top-level categories and
	// their URLs already carry the full slug path.
	for _, child := range root.Children {
		walk(child, nil)
	}

	sort.Slice(cats, func(i, j int) bool { return cats[i].URL < cats[j].URL })
	return cats, nil
}

// FetchCategoryRawData requests the JSON search response for a category and
// decodes it into a CategoryRawData.
func FetchCategoryRawData(hc *http.Client, cat *server.Category) (*CategoryRawData, error) {
	url := strings.Replace(cat.URL, sortimentPrefix, baseURL+"/c/", 1) + "?size=30&page=0"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create category request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch category: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetch category: unexpected status %s", resp.Status)
	}

	var raw CategoryRawData
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse category response: %w", err)
	}

	return &raw, nil
}

// ExtractProducts converts the raw search results into server.Product values,
// using the category to fill in the URL and category path.
func ExtractProducts(raw *CategoryRawData, cat *server.Category) []server.Product {
	products := make([]server.Product, 0, len(raw.Results))
	for _, item := range raw.Results {
		products = append(products, server.Product{
			ID:           item.Code,
			Name:         item.Name,
			Brand:        item.Manufacturer,
			URL:          cat.URL,
			ImageURL:     item.Image.URL,
			Price:        item.PriceValue,
			PriceUnit:    item.PriceUnit,
			Amount:       item.DisplayVolume,
			CompareUnit:  item.ComparePriceUnit,
			CategoryPath: strings.Join(cat.Slugs, "/"),
			ScrapedAt:    time.Now(),
		})
	}

	return products
}

// FindProductsInCategory fetches a category's products and returns them.
func FindProductsInCategory(hc *http.Client, cat *server.Category) ([]server.Product, error) {
	raw, err := FetchCategoryRawData(hc, cat)
	if err != nil {
		return nil, err
	}

	products := ExtractProducts(raw, cat)
	fmt.Printf("Found %d products in category: %s\n", len(products), cat.URL)

	return products, nil
}

// Scrape runs a scheduled scrape. It refuses to start outside hemkop.se's
// visit window (04:00-08:45 UTC); the startup path uses ScrapeAtStartup
// instead so a fresh installation can still fetch products immediately. Once a
// run has started it is allowed to finish even if it crosses the end of the
// visit window.
func Scrape() error {
	return scrapeAt(time.Now, false)
}

// ScrapeAtStartup is the one deliberate exception to the visit window: a new
// installation with no cached products is allowed to crawl outside the allowed
// hours. The crawl delay is still enforced.
func ScrapeAtStartup() error {
	return scrapeAt(time.Now, true)
}

// scrapeMu serializes the startup scrape and scheduled scrapes. Without it two
// runs could overlap and make requests closer together than the crawl delay.
var scrapeMu sync.Mutex

func scrapeAt(now func() time.Time, allowOutsideVisitTime bool) error {
	scrapeMu.Lock()
	defer scrapeMu.Unlock()

	// This is the only visit-time check. Once the scrape has started it runs
	// to completion even if it crosses the end of the window.
	if !allowOutsideVisitTime {
		if err := ensureVisitTime(now()); err != nil {
			return err
		}
	}

	delay := crawlDelay()
	fmt.Printf("Crawling hemkop.se with a %s delay\n", delay)
	client := newCrawlClient(30*time.Second, delay)
	categories, err := DiscoverCategories(client)
	categoryMap := make(map[string]server.Category)

	if err != nil {
		return fmt.Errorf("error discovering categories: %w", err)
	}

	for _, cat := range categories {
		fmt.Println("Scraping category:", cat.URL, "Name:", cat.Names, "Slug:", cat.Slugs)
		products, err := FindProductsInCategory(client, &cat)
		if err != nil {
			return fmt.Errorf("error finding products in category %s: %w", cat.Names, err)
		}
		cat.Products = products
		categoryMap[cat.Names[0]] = cat
	}

	// Rebuilding the category map also flattens every product into the pool
	// that "Dagens vara" picks from.
	server.SetCategoryMap(categoryMap)

	// Save the category map to disk
	file, err := os.Create(categoryCacheFile)
	if err != nil {
		return fmt.Errorf("error creating category map file: %w", err)
	}
	defer file.Close()

	err = gob.NewEncoder(file).Encode(categoryMap)
	if err != nil {
		return fmt.Errorf("error writing category map to file: %w", err)
	}

	return nil
}

func LoadOrScrape() {
	// Check if the category map is already stored on disk
	file, err := os.Open(categoryCacheFile)
	if err != nil {
		fmt.Println("Category map not found on disk, scraping outside the visit window is allowed on startup...")
		err = ScrapeAtStartup()
		if err != nil {
			fmt.Println("Error scraping categories:", err)
			return
		}
	} else {
		defer file.Close()
		categoryMap := make(map[string]server.Category)
		err = gob.NewDecoder(file).Decode(&categoryMap)
		if err != nil {
			// Empty or stale cache: rebuild it instead of giving up. A broken
			// cache is treated like a fresh startup, so the visit window is
			// bypassed once here as well.
			fmt.Println("Error loading category map from disk, scraping outside the visit window is allowed on startup...", err)
			err = ScrapeAtStartup()
			if err != nil {
				fmt.Println("Error scraping categories:", err)
			}
			return
		}
		server.SetCategoryMap(categoryMap)
		fmt.Println("Loaded category map from disk")
	}
}
