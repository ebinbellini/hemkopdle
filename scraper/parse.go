package scraper

import (
	"regexp"
	"strconv"
	"strings"
)

// hemkop.se renders prices in Swedish notation, e.g. "24,50/st" or "49,95/kg".
var priceRe = regexp.MustCompile(`^([0-9]+(?:[.,][0-9]+)?)\s*(?:/\s*(.+))?$`)

// parsePrice turns "24,50/st" into (24.5, "st").
func parsePrice(s string) (float64, string) {
	m := priceRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, ""
	}
	return parseDecimal(m[1]), strings.TrimSpace(m[2])
}

// amountRe matches a number followed by a unit, tolerating decorations such
// as "ca: 850g", "2p/350g" or "3dl".
//
// The alternation is ordered so that longer units win: "kg" before "g" and
// "ml"/"cl"/"dl" before "l".
var amountRe = regexp.MustCompile(`(?i)([0-9]+(?:[.,][0-9]+)?)\s*(kg|ml|cl|dl|g|l|st|p)\b`)

// countUnits are units that describe a number of items rather than a
// measurable size. A weight/volume unit is always preferred over these.
var countUnits = map[string]bool{"st": true, "p": true}

// parseAmount best-effort extracts the package size from the displayed
// string. It returns (0, "") when nothing sensible can be found.
//
// Multi-number values such as "500/200g" (net/drained weight) or "2p/350g"
// resolve to the first weight or volume match, which is the interpretation
// used for storage.
func parseAmount(raw string) (float64, string) {
	matches := amountRe.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return 0, ""
	}
	pick := matches[0]
	for _, m := range matches {
		if !countUnits[strings.ToLower(m[2])] {
			pick = m
			break
		}
	}
	return parseDecimal(pick[1]), strings.ToLower(pick[2])
}

// compareRe pulls the value and unit out of the clean comparison-price label
// "Jmf pris 49,00 kr/l".
var compareRe = regexp.MustCompile(`(?i)jmf pris\s*([0-9]+(?:[.,][0-9]+)?)\s*kr\s*/\s*(\w+)`)

// compareWordsRe is the fallback for the verbose variant
// "Jämförelsepris 49,00 kronor per liter".
var compareWordsRe = regexp.MustCompile(`(?i)j[äa]mf[öo]relsepris\s*([0-9]+(?:[.,][0-9]+)?)\s*kronor per (\w+)`)

var compareWordUnits = map[string]string{
	"kilo":  "kg",
	"liter": "l",
	"styck": "st",
}

// parseCompare returns the normalised comparison price and unit.
func parseCompare(presentation, verbose string) (float64, string) {
	if m := compareRe.FindStringSubmatch(presentation); m != nil {
		return parseDecimal(m[1]), strings.ToLower(m[2])
	}
	if m := compareWordsRe.FindStringSubmatch(verbose); m != nil {
		return parseDecimal(m[1]), compareWordUnits[strings.ToLower(m[2])]
	}
	return 0, ""
}

// productIDRe extracts the article number and sell unit from the tail of a
// product URL, e.g. ".../Olivolja-Classico-101290879_ST" -> 101290879_ST.
var productIDRe = regexp.MustCompile(`-(\d{5,})_([A-Za-z]+)$`)

// parseProductID returns the article id including its sell unit suffix.
func parseProductID(href string) string {
	if m := productIDRe.FindStringSubmatch(href); m != nil {
		return m[1] + "_" + strings.ToUpper(m[2])
	}
	// Fall back to the last path segment.
	if i := strings.LastIndex(href, "/"); i >= 0 {
		return href[i+1:]
	}
	return href
}

// parseDecimal handles both "24,50" and "24.50".
func parseDecimal(s string) float64 {
	f, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	if err != nil {
		return 0
	}
	return f
}

// humanizeSlug is the fallback display name for a category level when the
// page breadcrumb could not be read: "saser-och-grytbaser" ->
// "Saser och grytbaser".
func humanizeSlug(slug string) string {
	s := strings.ReplaceAll(slug, "-", " ")
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
