package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type Product struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Brand        string  `json:"brand,omitempty"`
	URL          string  `json:"url"`
	ImageURL     string  `json:"image_url,omitempty"`
	Price        float64 `json:"price"`
	Currency     string  `json:"currency"`
	PriceUnit    string  `json:"price_unit"`
	Amount       string  `json:"amount,omitempty"`
	ComparePrice float64 `json:"compare_price,omitempty"`
	CompareUnit  string  `json:"compare_unit,omitempty"`
	// Promotion is the campaign label when the item is on offer,
	// e.g. "Klubbpris" or "Alltid bra pris!".
	Promotion    string    `json:"promotion,omitempty"`
	CategoryPath string    `json:"category_path"`
	ScrapedAt    time.Time `json:"scraped_at"`
}

var testProducts = []Product{
	{
		ID:           "101475405_ST",
		Name:         "Test Product 1",
		Brand:        "Brand A",
		URL:          "https://example.com/product1",
		ImageURL:     "https://example.com/product1.jpg",
		Price:        19.99,
		Currency:     "SEK",
		PriceUnit:    "st",
		Amount:       "500ml",
		ComparePrice: 39.98,
		CompareUnit:  "kg",
		Promotion:    "Klubbpris",
		CategoryPath: "Beverages > Soft Drinks > Cola",
		ScrapedAt:    time.Now(),
	},
	{
		ID:           "101475406_ST",
		Name:         "Test Product 2",
		Brand:        "Brand B",
		URL:          "https://example.com/product2",
		ImageURL:     "https://example.com/product2.jpg",
		Price:        29.99,
		Currency:     "SEK",
		PriceUnit:    "st",
		Amount:       "1L",
		ComparePrice: 29.99,
		CompareUnit:  "L",
		Promotion:    "",
		CategoryPath: "Beverages > Juices > Orange Juice",
		ScrapedAt:    time.Now(),
	},
}

func DailyProductHandler(w http.ResponseWriter, r *http.Request) {
	randomIndex := time.Now().Unix() % int64(len(testProducts))
	product := testProducts[randomIndex]

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(product)
	if err != nil {
		http.Error(w, "Failed to encode product data", http.StatusInternalServerError)
		return
	}
	fmt.Printf("Served daily product: %+v\n", product)
}

func Run() {
	// API handlers
	http.HandleFunc("/daily_product/", DailyProductHandler)

	devMode := os.Getenv("DEV_MODE")
	if devMode == "true" {
		// Serve static files in development mode
		staticFiles := http.FileServer(http.Dir("web_static"))
		http.HandleFunc("/", staticFiles.ServeHTTP)
	}

	listenAddr := os.Getenv("ADDR")
	fmt.Println("Starting server on", listenAddr)
	err := http.ListenAndServe(listenAddr, nil)
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
