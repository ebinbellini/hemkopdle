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

	// pageSize is the largest page the category search endpoint accepts. It
	// caps whatever is requested (size=1000 still returns 100), so asking for
	// more only wastes a request.
	pageSize = 100

	// maxCategoryPages is a safety net against a broken/looping pagination
	// response. The biggest real category (Skafferi) needs 31 pages.
	maxCategoryPages = 200
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
	Pagination struct {
		PageSize             int `json:"pageSize"`
		CurrentPage          int `json:"currentPage"`
		NumberOfPages        int `json:"numberOfPages"`
		TotalNumberOfResults int `json:"totalNumberOfResults"`
	} `json:"pagination"`
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

// FetchCategoryRawData requests one page of the JSON search response for a
// category and decodes it into a CategoryRawData.
func FetchCategoryRawData(hc *http.Client, cat *server.Category, page int) (*CategoryRawData, error) {
	url := fmt.Sprintf("%s?size=%d&page=%d",
		strings.Replace(cat.URL, sortimentPrefix, baseURL+"/c/", 1), pageSize, page)

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

// FindProductsInCategory fetches every page of a category and returns all of
// its products. A category is a whole subtree of the category tree, so this is
// where the bulk of the catalogue comes from: the old code only ever read the
// first 30 results of each category.
func FindProductsInCategory(hc *http.Client, cat *server.Category) ([]server.Product, error) {
	products := make([]server.Product, 0, pageSize)
	seen := make(map[string]bool, pageSize)

	for page := 0; ; page++ {
		if page >= maxCategoryPages {
			fmt.Printf("Stopping at %d pages for category %s\n", page, cat.URL)
			break
		}

		raw, err := FetchCategoryRawData(hc, cat, page)
		if err != nil {
			if page > 0 {
				// Keep what we already have instead of losing the whole
				// category because of one bad page.
				fmt.Printf("Page %d of %s failed, keeping %d products: %v\n", page, cat.URL, len(products), err)
				break
			}
			return nil, err
		}

		for _, product := range ExtractProducts(raw, cat) {
			if product.ID != "" && seen[product.ID] {
				continue
			}
			seen[product.ID] = true
			products = append(products, product)
		}

		// numberOfPages is authoritative; an empty page guards against it
		// being wrong (or missing on a category with no results).
		lastPage := raw.Pagination.NumberOfPages - 1
		if len(raw.Results) == 0 || page >= lastPage {
			break
		}
	}

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

	// Key the map by the full slug path. Keying by the top level name made
	// every subcategory of a group overwrite the previous one, so only 20 of
	// the 516 categories ever survived and the product pool stayed tiny.
	failed := 0
	for _, cat := range categories {
		key := strings.Join(cat.Slugs, "/")
		fmt.Println("Scraping category:", cat.URL, "Name:", cat.Names, "Slug:", cat.Slugs)
		products, err := FindProductsInCategory(client, &cat)
		if err != nil {
			// One bad category must not discard an otherwise good crawl.
			failed++
			fmt.Printf("Skipping category %s: %v\n", cat.URL, err)
			continue
		}
		cat.Products = products
		categoryMap[key] = cat
	}

	if len(categoryMap) == 0 {
		return fmt.Errorf("every category failed to scrape (%d tried)", failed)
	}
	if failed > 0 {
		fmt.Printf("%d of %d categories failed, keeping the %d that worked\n",
			failed, len(categories), len(categoryMap))
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

// readCategoryCache loads the category map from disk.
func readCategoryCache() (map[string]server.Category, error) {
	file, err := os.Open(categoryCacheFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	categoryMap := make(map[string]server.Category)
	if err := gob.NewDecoder(file).Decode(&categoryMap); err != nil {
		return nil, err
	}
	if len(categoryMap) == 0 {
		return nil, fmt.Errorf("cache holds no categories")
	}

	return categoryMap, nil
}

func LoadOrScrape() {
	categoryMap, err := readCategoryCache()
	if err == nil {
		server.SetCategoryMap(categoryMap)
		fmt.Printf("Loaded category map from disk (%d categories)\n", len(categoryMap))
		return
	}

	// A missing or unreadable cache is treated like a fresh installation, so
	// the visit window is bypassed once here as well.
	fmt.Println("No usable category map on disk, scraping outside the visit window is allowed on startup... (" + err.Error() + ")")
	if err := ScrapeAtStartup(); err != nil {
		fmt.Println("Error scraping categories:", err)
	}
}
