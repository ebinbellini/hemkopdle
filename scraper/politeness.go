package scraper

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// hemkop.se/robots.txt asks for a ten second crawl delay and allows visits
// between 04:00 and 08:45 UTC. The values are kept here in one place so the
// startup scrape, the scheduled scrape and the HTTP client cannot drift apart.
const (
	robotsCrawlDelay       = 10 * time.Second
	robotsVisitStartMinute = 4 * 60
	robotsVisitEndMinute   = 8*60 + 45
)

// ErrOutsideVisitTime is returned by the scheduled scrape when the current time
// is outside hemkop.se's visit window. The first startup scrape deliberately
// bypasses that window so a new installation still gets products.
var ErrOutsideVisitTime = errors.New("outside hemkop.se visit time (04:00-08:45 UTC)")

// withinVisitTime reports whether now is inside robots.txt's visit window.
// The window is inclusive at the start and exclusive at the end.
func withinVisitTime(now time.Time) bool {
	utc := now.UTC()
	minute := utc.Hour()*60 + utc.Minute()
	return minute >= robotsVisitStartMinute && minute < robotsVisitEndMinute
}

func ensureVisitTime(now time.Time) error {
	if withinVisitTime(now) {
		return nil
	}
	return fmt.Errorf("%w, current time is %s UTC", ErrOutsideVisitTime, now.UTC().Format("15:04"))
}

// crawlDelay parses the configured delay
func crawlDelay() time.Duration {
	raw := strings.TrimSpace(os.Getenv("SCRAPE_DELAY"))
	if raw == "" {
		return robotsCrawlDelay
	}

	delay, err := time.ParseDuration(raw)
	if err != nil {
		return robotsCrawlDelay
	}
	return delay
}

// politeTransport spaces every outgoing request by at least delay. Mutexes are
// held while waiting and while recording the start time, so concurrent callers
// cannot start two requests in the same slot.
type politeTransport struct {
	base  http.RoundTripper
	delay time.Duration

	mu   sync.Mutex
	last time.Time
}

func (t *politeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if !t.last.IsZero() {
		wait := t.delay - time.Since(t.last)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-request.Context().Done():
				timer.Stop()
				t.mu.Unlock()
				return nil, request.Context().Err()
			}
		}
	}

	t.last = time.Now()
	t.mu.Unlock()

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(request)
}

// sharedCrawlTransport is used by every scrape. Keeping one transport means the
// crawl delay also applies between two consecutive scrapes, not just between
// requests inside one scrape.
var sharedCrawlTransport = &politeTransport{base: http.DefaultTransport}

// newCrawlClient returns an HTTP client whose transport enforces the crawl
// delay between all requests. The timeout applies to the request itself, so the
// delay is added to it: otherwise a long configured crawl delay would make the
// client time out while waiting politely.
func newCrawlClient(timeout, delay time.Duration) *http.Client {
	if delay < 0 {
		delay = 0
	}
	if timeout > 0 {
		timeout += delay
	}

	sharedCrawlTransport.mu.Lock()
	sharedCrawlTransport.delay = delay
	sharedCrawlTransport.mu.Unlock()

	return &http.Client{
		Timeout:   timeout,
		Transport: sharedCrawlTransport,
	}
}
