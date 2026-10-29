package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Brand       string  `json:"brand,omitempty"`
	URL         string  `json:"url"`
	ImageURL    string  `json:"image_url,omitempty"`
	Price       float64 `json:"price"`
	PriceUnit   string  `json:"price_unit"`
	Amount      string  `json:"amount,omitempty"`
	CompareUnit string  `json:"compare_unit,omitempty"`
	// Promotion is the campaign label when the item is on offer,
	// e.g. "Klubbpris" or "Alltid bra pris!".
	Promotion    string    `json:"promotion,omitempty"`
	CategoryPath string    `json:"category_path"`
	ScrapedAt    time.Time `json:"scraped_at"`
}

type Category struct {
	Slugs    []string  `json:"slugs"`
	Names    []string  `json:"names"`
	URL      string    `json:"url"`
	Products []Product `json:"products,omitempty"`
}

// CategoryMap holds every scraped category keyed by its top level name.
// The scraper replaces it through SetCategoryMap.
var CategoryMap = make(map[string]Category)

var categoryMu sync.RWMutex

// SetCategoryMap replaces the scraped categories and rebuilds the flat product
// pool that the product of the day is picked from.
func SetCategoryMap(categories map[string]Category) {
	categoryMu.Lock()
	CategoryMap = categories
	categoryMu.Unlock()

	productCount := 0
	for _, category := range categories {
		productCount += len(category.Products)
	}

	list := make([]Product, 0, productCount)
	for _, category := range categories {
		list = append(list, category.Products...)
	}

	SetProducts(list)
}

func Run() {
	if err := LoadTemplates(); err != nil {
		log.Fatalf("Error loading templates: %v", err)
	}

	mux := http.NewServeMux()

	// Pages rendered from go templates
	mux.HandleFunc("GET /{$}", DailyPage)
	mux.HandleFunc("GET /slump", SlumpPage)
	mux.HandleFunc("GET /strid", StridPage)

	// Game API
	mux.HandleFunc("/api/daily", DailyHandler)
	mux.HandleFunc("/api/daily/guess", GuessHandler)
	mux.HandleFunc("/api/daily/answer", AnswerHandler)

	devMode := os.Getenv("DEV_MODE")
	if devMode == "true" {
		// Serve static files in development mode. The page routes above are
		// more specific, so they still win over this catch-all.
		staticFiles := http.FileServer(http.Dir("web_static"))
		mux.Handle("/", staticFiles)
	}

	listenAddr := os.Getenv("ADDR")
	fmt.Println("Starting server on", listenAddr)
	err := http.ListenAndServe(listenAddr, mux)
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
