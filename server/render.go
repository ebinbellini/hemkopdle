package server

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
)

// templateFS holds the HTML templates compiled into the binary, so a deployed
// server always renders exactly the markup it was built with. The templates
// are only static HTML; everything a page needs is passed in as data.
//
//go:embed templates
var templateFS embed.FS

// Page names, used both as template set keys and as file names under
// templates/pages.
const (
	dailyPage = "daily"
	slumpPage = "slump"
	stridPage = "strid"
)

// Mode is one entry in the mode picker in the navbar.
type Mode struct {
	Href  string
	Label string
	Icon  string
}

// modes is shared by every page so the navbar stays identical everywhere. The
// order is the order shown to the player.
var modes = []Mode{
	{Href: "/", Label: "Dagens vara", Icon: "/icons/calendar.svg"},
	{Href: "/slump", Label: "Jag har tur", Icon: "/icons/cart.svg"},
	{Href: "/strid", Label: "Kund mot kund", Icon: "/icons/fight.svg"},
}

// pageData is what the shared layout and the navbar get from every page.
type pageData struct {
	// Title is the document title, ActiveLink the href of the mode to
	// highlight in the navbar.
	Title      string
	ActiveLink string
	Modes      []Mode
}

// productCard is the product a game page shows. The price is not part of it:
// that is what the player guesses.
type productCard struct {
	HasProduct bool
	Name       string
	Brand      string
	Amount     string
	ImageURL   string
}

// gamePageData feeds the game board that both playable modes share. Only the
// intro and the data attributes differ between them.
type gamePageData struct {
	pageData
	Product    productCard
	MaxGuesses int
	Tolerance  string
	// Date is set on the daily page (the day the product belongs to) and
	// ProductID on the slump page (the product being played).
	Date      string
	ProductID string
}

// introPageData feeds the pages that are not playable yet.
type introPageData struct {
	pageData
	Intro *introView
}

// introView is the copy shown on the pages that are not playable yet.
type introView struct {
	Heading string
	Lead    string
	Title   string
	Body    string
}

// pageTemplates parses every page once. A page is the shared layout, navbar and
// partials merged with one file from templates/pages: having a template set per
// page means the "content" block of one page cannot leak into another.
var pageTemplates = sync.OnceValues(func() (map[string]*template.Template, error) {
	shared := []string{
		"templates/layout.html",
		"templates/navbar.html",
		"templates/placeholder.html",
		"templates/product-card.html",
		"templates/guess-board.html",
	}

	pages := make(map[string]*template.Template, 3)
	for _, page := range []string{dailyPage, slumpPage, stridPage} {
		files := append([]string{"templates/pages/" + page + ".html"}, shared...)
		tmpl, err := template.New(page).ParseFS(templateFS, files...)
		if err != nil {
			return nil, fmt.Errorf("parse %q page: %w", page, err)
		}
		pages[page] = tmpl
	}

	return pages, nil
})

// LoadTemplates parses the templates up front so a broken template is reported
// at startup instead of on the first request.
func LoadTemplates() error {
	_, err := pageTemplates()
	return err
}

// renderPage renders a page into a buffer first, so a template error leaves the
// response untouched instead of sending half a document.
func renderPage(w http.ResponseWriter, page string, data any) {
	templates, err := pageTemplates()
	if err != nil {
		log.Printf("Could not load template %q: %v", page, err)
		http.Error(w, "kunde inte rendera sidan", http.StatusInternalServerError)
		return
	}

	tmpl, ok := templates[page]
	if !ok {
		log.Printf("Unknown page %q", page)
		http.Error(w, "kunde inte rendera sidan", http.StatusInternalServerError)
		return
	}

	var body bytes.Buffer
	if err := tmpl.ExecuteTemplate(&body, "layout.html", data); err != nil {
		log.Printf("Could not render %q: %v", page, err)
		http.Error(w, "kunde inte rendera sidan", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := body.WriteTo(w); err != nil {
		log.Printf("Could not write %q response: %v", page, err)
	}
}

// productCardFrom turns a product into the card a game page renders.
func productCardFrom(product Product, ok bool) productCard {
	if !ok {
		return productCard{}
	}

	return productCard{
		HasProduct: true,
		Name:       product.Name,
		Brand:      product.Brand,
		Amount:     product.Amount,
		ImageURL:   product.ImageURL,
	}
}

// DailyPage renders the daily game with the product of the day already in the
// markup, so the player sees the product without waiting for a fetch.
func DailyPage(w http.ResponseWriter, r *http.Request) {
	product, ok := CurrentDailyProduct()

	data := gamePageData{
		pageData: pageData{
			Title:      "Hemköpdle - Dagens vara",
			ActiveLink: "/",
			Modes:      modes,
		},
		Product:    productCardFrom(product, ok),
		MaxGuesses: MaxGuesses,
		Tolerance:  toleranceText(),
	}
	if ok {
		data.Date = todayKey()
	}

	renderPage(w, dailyPage, data)
}

// StridPage is still a placeholder. It shares the layout, the navbar and the
// placeholder partial with the pages that are finished.
func StridPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, stridPage, introPageData{
		pageData: pageData{
			Title:      "Hemköpdle - Kund mot kund",
			ActiveLink: "/strid",
			Modes:      modes,
		},
		Intro: &introView{
			Heading: "Kund mot kund",
			Lead:    "Två kunder, en vara, en vinnare.",
			Title:   "Striden är inte öppnad än",
			Body: "Läget är under uppbyggnad. Här kommer du att möta en annan kund " +
				"öga mot öga och gissa samma vara. Den som kommer närmast priset vinner.",
		},
	})
}
