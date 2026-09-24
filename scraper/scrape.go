package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// extractJS walks the rendered product grid and returns everything as JSON.
//
// Only data-testid hooks and href/title attributes are used. The site is
// built with styled-components, so every class name is a generated hash
// (e.g. "sc-4082521d-0 ddUTPV") that changes on each deploy, while the
// data-testid values have been stable.
const extractJS = `(() => {
	const txt = (root, sel) => {
		const el = root.querySelector(sel);
		return el ? (el.textContent || '').trim() : '';
	};

	const cards = Array.from(document.querySelectorAll('a[href^="/produkt/"]'));
	const seen = Object.create(null);
	const products = [];

	for (const a of cards) {
		const href = a.getAttribute('href') || '';
		if (!href || seen[href]) continue;
		seen[href] = true;

		// The campaign label sits on a title attribute of an ancestor of
		// the promo icon, so walk up until a title is found.
		let promo = '';
		let node = a.querySelector('[data-testid="promotion-theme-label"]');
		while (node && !promo) {
			promo = node.getAttribute('title') || '';
			node = node.parentElement;
		}

		const img = a.querySelector('img');

		products.push({
			href: href,
			name: txt(a, '[data-testid="product-title"]') || a.getAttribute('title') || '',
			brand: txt(a, '[data-testid="display-manufacturer"]'),
			amount: txt(a, '[data-testid="display-volume"]'),
			price: txt(a, '[data-testid="price"]')
				|| txt(a, '[data-testid="price-text"]')
				|| txt(a, '[data-testid="screen-reader-text"]'),
			price_unit: txt(a, '[data-testid="price-unit"]'),
			compare: txt(a, '[data-testid="compare-price"]'),
			presentation: txt(a, '[data-testid="presentation-text"]'),
			promotion: promo,
			image: img ? (img.getAttribute('src') || '') : ''
		});
	}

	// The breadcrumb is server-rendered as JSON-LD, which gives the real
	// display names ("Såser & grytbaser") for each path level.
	let crumbs = [];
	const scripts = document.querySelectorAll('script[type="application/ld+json"]');
	for (const s of scripts) {
		let parsed;
		try { parsed = JSON.parse(s.textContent); } catch (e) { continue; }
		const list = Array.isArray(parsed) ? parsed : [parsed];
		for (const item of list) {
			if (item && item['@type'] === 'BreadcrumbList' && Array.isArray(item.itemListElement)) {
				crumbs = item.itemListElement.map(e => ({
					name: e.name || '',
					item: e.item || ''
				}));
			}
		}
	}

	// Fallback: the visible breadcrumb nav.
	if (!crumbs.length) {
		const nav = document.querySelector('nav[aria-label="Navigationssökväg"]');
		if (nav) {
			crumbs = Array.from(nav.querySelectorAll('li'))
				.map(li => {
					const link = li.querySelector('a');
					return {
						name: (li.textContent || '').trim(),
						item: link ? (link.getAttribute('href') || '') : ''
					};
				})
				.filter(c => c.name);
		}
	}

	return JSON.stringify({ breadcrumb: crumbs, products: products });
})()`

type rawProduct struct {
	Href         string `json:"href"`
	Name         string `json:"name"`
	Brand        string `json:"brand"`
	Amount       string `json:"amount"`
	Price        string `json:"price"`
	PriceUnit    string `json:"price_unit"`
	Compare      string `json:"compare"`
	Presentation string `json:"presentation"`
	Promotion    string `json:"promotion"`
	Image        string `json:"image"`
}

type rawCrumb struct {
	Name string `json:"name"`
	Item string `json:"item"`
}

type rawPage struct {
	Breadcrumb []rawCrumb   `json:"breadcrumb"`
	Products   []rawProduct `json:"products"`
}

// ScrapeCategory renders one category page and returns the category with its
// real display names plus up to limit products.
//
// A limit of zero means "everything the grid rendered".
func (c *Client) ScrapeCategory(ctx context.Context, cat Category, limit int, renderTimeout time.Duration) (Category, []Product, error) {
	if err := chromedp.Run(c.ctx, chromedp.Navigate(cat.URL)); err != nil {
		return cat, nil, fmt.Errorf("navigate %s: %w", cat.URL, err)
	}

	c.dismissCookieBanner()

	if _, err := c.waitForProducts(ctx, renderTimeout); err != nil {
		return cat, nil, fmt.Errorf("%s: %w", cat.URL, err)
	}

	var payload string
	if err := chromedp.Run(c.ctx, chromedp.Evaluate(extractJS, &payload)); err != nil {
		return cat, nil, fmt.Errorf("extract %s: %w", cat.URL, err)
	}

	var page rawPage
	if err := json.Unmarshal([]byte(payload), &page); err != nil {
		return cat, nil, fmt.Errorf("decode %s: %w", cat.URL, err)
	}

	cat = applyBreadcrumb(cat, page.Breadcrumb)

	products := make([]Product, 0, len(page.Products))
	for _, rp := range page.Products {
		if limit > 0 && len(products) >= limit {
			break
		}
		if rp.Name == "" || rp.Href == "" {
			continue
		}
		products = append(products, rp.toProduct(cat))
	}
	return cat, products, nil
}

// applyBreadcrumb replaces the slug-derived placeholder names with the real
// names from the page, dropping the leading "Start" / "Handla" entries which
// are not part of the category tree.
func applyBreadcrumb(cat Category, crumbs []rawCrumb) Category {
	var names []string
	for _, c := range crumbs {
		n := strings.TrimSpace(c.Name)
		if n == "" || strings.EqualFold(n, "Start") || strings.EqualFold(n, "Handla") {
			continue
		}
		names = append(names, n)
	}
	// Only trust the breadcrumb if it agrees with the URL depth.
	if len(names) == len(cat.Slugs) {
		cat.Names = names
	}
	return cat
}

// toProduct normalises one rendered card into a storable row.
func (rp rawProduct) toProduct(cat Category) Product {
	price, unit := parsePrice(rp.Price)
	if unit == "" {
		unit = strings.TrimPrefix(rp.PriceUnit, "/")
	}
	amountValue, amountUnit := parseAmount(rp.Amount)
	comparePrice, compareUnit := parseCompare(rp.Presentation, rp.Compare)

	return Product{
		ID:            parseProductID(rp.Href),
		Name:          rp.Name,
		Brand:         strings.Trim(strings.TrimSpace(rp.Brand), ","),
		URL:           baseURL + rp.Href,
		ImageURL:      rp.Image,
		Price:         price,
		Currency:      "SEK",
		PriceUnit:     unit,
		Amount:        rp.Amount,
		AmountValue:   amountValue,
		AmountUnit:    amountUnit,
		ComparePrice:  comparePrice,
		CompareUnit:   compareUnit,
		Promotion:     rp.Promotion,
		CategoryPath:  cat.Path(),
		CategoryNames: cat.Names,
		CategorySlugs: cat.Slugs,
		ScrapedAt:     time.Now().UTC().Truncate(time.Second),
	}
}

// Sleep applies the politeness delay requested by robots.txt. It is skipped
// for the first request, so it is called before every request after that.
func (c *Client) Sleep(ctx context.Context) {
	if c.delay <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(c.delay):
	}
}

// Delay reports the configured inter-request pause.
func (c *Client) Delay() time.Duration { return c.delay }
