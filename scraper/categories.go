package scraper

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	baseURL = "https://www.hemkop.se"

	// sortimentPrefix is the URL space that category pages live under:
	// /sortiment/skafferi/asien/saser-och-grytbaser
	sortimentPrefix = baseURL + "/sortiment/"
)

// userAgent is a plain desktop Chrome UA. The site renders its product grid
// client-side, so a real browser engine is used; this just avoids the
// obvious "headless" tells.
const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"

type sitemapIndex struct {
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

type urlSet struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

// mediasRe normalises the doubly-prefixed sitemap URLs the CMS emits, e.g.
// "https://www.hemkop.sehttps://www.hemkop.se/medias/Category-...".
var mediasRe = regexp.MustCompile(`https://www\.hemkop\.se/medias/[^\s<]+`)

// DiscoverCategories reads the public category sitemap.
//
// The "Kategorier" mega-menu on /handla is rendered client-side and its CSS
// classes are generated hashes, whereas the sitemap is a stable, documented
// index of exactly the same pages. Each sitemap entry already encodes the
// full hierarchy in its path, which is what the Category type stores.
func DiscoverCategories(ctx context.Context, hc *http.Client) ([]Category, error) {
	idxBody, err := httpGet(ctx, hc, baseURL+"/sitemap.xml")
	if err != nil {
		return nil, fmt.Errorf("sitemap index: %w", err)
	}

	var idx sitemapIndex
	if err := xml.Unmarshal(idxBody, &idx); err != nil {
		return nil, fmt.Errorf("parse sitemap index: %w", err)
	}

	var categorySitemap string
	for _, s := range idx.Sitemaps {
		loc := normalizeSitemapLoc(s.Loc)
		if strings.Contains(loc, "/medias/Category-") {
			categorySitemap = loc
			break
		}
	}
	if categorySitemap == "" {
		return nil, fmt.Errorf("no Category sitemap found in sitemap index")
	}

	body, err := httpGet(ctx, hc, categorySitemap)
	if err != nil {
		return nil, fmt.Errorf("category sitemap: %w", err)
	}

	var set urlSet
	if err := xml.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("parse category sitemap: %w", err)
	}

	seen := make(map[string]bool, len(set.URLs))
	cats := make([]Category, 0, len(set.URLs))
	for _, u := range set.URLs {
		loc := strings.TrimSpace(u.Loc)
		if !strings.HasPrefix(loc, sortimentPrefix) || seen[loc] {
			continue
		}
		seen[loc] = true

		slugs := strings.Split(strings.Trim(strings.TrimPrefix(loc, sortimentPrefix), "/"), "/")
		names := make([]string, len(slugs))
		for i, s := range slugs {
			names[i] = humanizeSlug(s)
		}
		cats = append(cats, Category{Slugs: slugs, Names: names, URL: loc})
	}

	sort.Slice(cats, func(i, j int) bool { return cats[i].URL < cats[j].URL })
	return cats, nil
}

// FindCategories returns every category whose path matches any of the given
// substrings, case-insensitively. Matching on the slug path keeps the CLI
// terse: "skafferi/asien".
func FindCategories(all []Category, matches ...string) []Category {
	if len(matches) == 0 {
		return all
	}
	var out []Category
	for _, c := range all {
		slugPath := strings.Join(c.Slugs, "/")
		for _, m := range matches {
			if strings.Contains(slugPath, strings.ToLower(strings.Trim(m, "/"))) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// normalizeSitemapLoc strips the duplicated origin the CMS prepends.
func normalizeSitemapLoc(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := mediasRe.FindString(raw); m != "" {
		return m
	}
	return raw
}

// httpGet performs a single GET with a sane timeout and the shared UA.
func httpGet(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "sv-SE,sv;q=0.9,en;q=0.8")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
	// 4 MiB is far more than these documents need.
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// HTTPClient returns the client used for the non-rendered fetches.
func HTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
