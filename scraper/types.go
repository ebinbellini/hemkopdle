// Package scraper is a proof-of-concept price scraper for hemkop.se.
//
// Everything it produces is shaped as if it were about to be written to a
// database row: one Product per package/article, with its category path
// flattened into the row.
package scraper

import "time"

// Category is one node of the Hemköp "sortiment" tree.
//
// Names/Slugs are ordered from the top level down, so
//
//	Names: ["Skafferi", "Asien", "Såser & grytbaser"]
//	Slugs: ["skafferi", "asien",   "saser-och-grytbaser"]
type Category struct {
	Slugs []string `json:"slugs"`
	Names []string `json:"names"`
	URL   string   `json:"url"`
}

// Path renders the category the way a human would read the breadcrumb,
// e.g. "Skafferi > Asien > Såser & grytbaser".
func (c Category) Path() string {
	if len(c.Names) == 0 {
		return ""
	}
	out := c.Names[0]
	for _, n := range c.Names[1:] {
		out += " > " + n
	}
	return out
}

// Slug returns the deepest (leaf) slug, which is the stable identifier for
// the category regardless of how the display names change.
func (c Category) Slug() string {
	if len(c.Slugs) == 0 {
		return ""
	}
	return c.Slugs[len(c.Slugs)-1]
}

// Product is one row, shaped for insertion into a prices table.
type Product struct {
	ID    string `json:"id"` // e.g. 101475405_ST
	Name  string `json:"name"`
	Brand string `json:"brand,omitempty"`
	URL   string `json:"url"`

	ImageURL string `json:"image_url,omitempty"`

	// Price is the shelf price in PriceUnit terms. PriceUnit is how the
	// product is sold: "st" (per piece) or "kg" (by weight).
	Price     float64 `json:"price"`
	Currency  string  `json:"currency"`
	PriceUnit string  `json:"price_unit"`

	// Amount is the package size exactly as displayed, e.g. "500ml".
	// Empty for goods sold by weight, where there is no fixed size.
	Amount string `json:"amount,omitempty"`
	// AmountValue/AmountUnit are the best-effort parsed form of Amount,
	// e.g. 500 and "ml".
	AmountValue float64 `json:"amount_value,omitempty"`
	AmountUnit  string  `json:"amount_unit,omitempty"`

	// Comparison price, normalised by Hemköp to a per-kg / per-l basis.
	ComparePrice float64 `json:"compare_price,omitempty"`
	CompareUnit  string  `json:"compare_unit,omitempty"`

	// Promotion is the campaign label when the item is on offer,
	// e.g. "Klubbpris" or "Alltid bra pris!".
	Promotion string `json:"promotion,omitempty"`

	// CategoryPath is the flattened breadcrumb, plus the individual levels
	// in case a normalised categories table is wanted later.
	CategoryPath  string   `json:"category_path"`
	CategoryNames []string `json:"category_names"`
	CategorySlugs []string `json:"category_slugs"`

	ScrapedAt time.Time `json:"scraped_at"`
}
