# hemkop.se price scraper (proof of concept)

Scrapes price, package size, category path and image for products on
[hemkop.se](https://www.hemkop.se), shaped as rows ready for a `products`
table.

```bash
go run ./cmd/hemkops                      # 3 curated categories, 5 products each
go run ./cmd/hemkops -list | head -40     # list all 528 categories
go run ./cmd/hemkops -match 'mejeri-ost-och-agg' -products 20
go run ./cmd/hemkops -match 'frukt-och-gront' -json-out rows.json
go run ./cmd/hemkops -headful -match 'skafferi/asien'   # watch it run
```

## What comes back

| column | source | example |
| --- | --- | --- |
| `price` + `price_unit` | `[data-testid="price"]` | `27.84` / `st` |
| `amount` | `[data-testid="display-volume"]` | `200ml` |
| `category_path` | JSON-LD `BreadcrumbList` | `Skafferi > Asien > Såser & grytbaser` |
| `image_url` | card `<img src>` | `https://assets.axfood.se/...` |

Plus `id`, `name`, `brand`, `url`, `promotion`, and the normalised
`compare_price` / `compare_unit` (Hemköp's per-kg / per-l "Jmf pris").

## Why a browser at all

The category page ships an empty shell: `curl` on
`/sortiment/skafferi/asien/saser-och-grytbaser` contains **zero** `/produkt/`
links. The grid is fetched and rendered client-side, so a real engine is
required. `chromedp` drives one reused tab and polls until the card count
stops changing.

## Why the sitemap, not the "Kategorier" menu

The `Kategorier` node in the mega-menu on `/handla` is client-rendered and
styled with generated class hashes, e.g.

```html
<li class="sc-15b2a752-2 sc-8a320d91-5 bLlyDn gSSWfU">Kategorier</li>
```

`sc-8a320d91-5` is a run-time hash that changes on every deploy, so anything
built on it rots silently. `https://www.hemkop.se/sitemap.xml` instead links a
`Category` sitemap holding all 528 category pages, and each URL already encodes
the hierarchy:

```
/sortiment/skafferi/asien/saser-och-grytbaser
```

The real display names (with `å`, `ä`, `ö` and `&`) are read from the
server-rendered `BreadcrumbList` JSON-LD on each category page, so the slugs
stay the stable key while the names stay pretty.

## Selector strategy

Every hook used is `data-testid` or a JSON-LD block. No CSS classes are
referenced. Verified hooks:

```
product-title, display-manufacturer, display-volume,
price, price-text, price-unit, compare-price,
presentation-text, promotion-theme-label, screen-reader-text
```

## Caveats / known limits

- **Weight-priced goods** (`..._KG`) have no fixed size. `amount` is empty and
  `price_unit` is `kg`; the price is per kilo, not per package.
- **Multi-number amounts** such as `500/200g` (net/drained) or `2p/350g`
  resolve to the first weight or volume match, preferring weight over a count
  unit. `amount` always keeps the raw string.
- **Only the first 30 products** are read per category. The grid renders an
  initial batch of 30 and lazy-loads the rest on scroll. Categories that finish
  naturally return fewer (e.g. `skafferi/asien/chutney` → 3, `.../soja` → 24);
  anything that reports exactly 30 is truncated. `-products 0` means
  "everything rendered", which is still capped at 30. Extend `ScrapeCategory`
  with a scroll-and-settle loop for full listings.
- **No store context** is selected, so prices are the site defaults for a
  logged-out visitor. Club prices (`Klubbpris`) appear in `promotion` but are
  not applied to `price`.
- **Images** use the site's `t_200` transform. Swap `t_200` for a larger
  variant before storing if a bigger file is wanted.

## Politeness

`https://www.hemkop.se/robots.txt` asks for:

```
Crawl-delay: 10
Visit-time: 0400-0845      # UTC
```

`-delay` defaults to `10s` and is applied between page loads. The tool warns
when it is raised or when a run happens outside the visit window. Discovery via
the sitemap costs two extra requests; scraping one category costs one page load
regardless of how many products are read from it.
