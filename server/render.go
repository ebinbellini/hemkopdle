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

// pageData is what the shared layout, the navbar and a page template get.
type pageData struct {
	// Title is the document title, ActiveLink the href of the mode to
	// highlight in the navbar.
	Title      string
	ActiveLink string
	Modes      []Mode

	// Daily is used by the daily game page.
	Daily dailyView
	// MaxGuesses and Tolerance describe the rules in the page instructions.
	MaxGuesses int
	Tolerance  string

	// Intro is used by the placeholder pages.
	Intro *introView
}

// dailyView is the product of the day as the daily game page needs it. The
// price is deliberately not part of it: that is what the player guesses.
type dailyView struct {
	Date       string
	HasProduct bool
	Name       string
	Brand      string
	Amount     string
	ImageURL   string
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
func renderPage(w http.ResponseWriter, page string, data pageData) {
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

// currentDailyView builds the product card for the daily game page. When the
// scraper has not produced a product yet the page is still rendered, and the
// game script picks the product up through /api/daily instead.
func currentDailyView() dailyView {
	product, ok := CurrentDailyProduct()
	if !ok {
		return dailyView{}
	}

	return dailyView{
		Date:       todayKey(),
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
	renderPage(w, dailyPage, pageData{
		Title:      "Hemköpdle - Dagens vara",
		ActiveLink: "/",
		Modes:      modes,
		Daily:      currentDailyView(),
		MaxGuesses: MaxGuesses,
		Tolerance:  toleranceText(),
	})
}

// SlumpPage and StridPage are placeholders for the game modes that are not
// implemented yet. They share the layout, the navbar and the placeholder
// partial with each other.
func SlumpPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, slumpPage, pageData{
		Title:      "Hemköpdle - Jag har tur",
		ActiveLink: "/slump",
		Modes:      modes,
		Intro: &introView{
			Heading: "Jag har tur",
			Lead:    "Här bestämmer slumpen vilken vara du ska prissätta.",
			Title:   "Slumpen är inte laddad än",
			Body: "Läget är under uppbyggnad. Så snart det är klart drar vi en " +
				"slumpad vara ur sortimentet och du får samma gissningsregler som i " +
				"Dagens vara.",
		},
	})
}

func StridPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, stridPage, pageData{
		Title:      "Hemköpdle - Kund mot kund",
		ActiveLink: "/strid",
		Modes:      modes,
		Intro: &introView{
			Heading: "Kund mot kund",
			Lead:    "Två kunder, en vara, en vinnare.",
			Title:   "Striden är inte öppnad än",
			Body: "Läget är under uppbyggnad. Här kommer du att möta en annan kund " +
				"öga mot öga och gissa samma vara. Den som kommer närmast priset vinner.",
		},
	})
}
