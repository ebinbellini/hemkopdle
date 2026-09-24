package scraper

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// Client owns one headless browser tab that is reused for every page.
type Client struct {
	ctx     context.Context
	cleanup []func()
	delay   time.Duration
}

// Options configures the browser.
type Options struct {
	// ChromePath overrides browser discovery (e.g. "/usr/bin/chromium").
	ChromePath string
	// Headful runs a visible browser, which is much easier to debug.
	Headful bool
	// Delay is the pause inserted between page loads. hemkop.se/robots.txt
	// asks for a crawl-delay of 10 seconds, so that is the default.
	Delay time.Duration
}

// New launches the browser and returns a client. Call Close when done.
func New(opts Options) (*Client, error) {
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserAgent(userAgent),
		chromedp.WindowSize(1440, 1000),
		// The automation banner and the "Chrome is being controlled" bar
		// shift the layout around; neither is useful here.
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
	)

	if opts.Headful {
		allocOpts = append(allocOpts, chromedp.Flag("headless", false))
	} else {
		allocOpts = append(allocOpts, chromedp.Flag("headless", "new"))
	}
	if opts.ChromePath != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(opts.ChromePath))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	tabCtx, cancelTab := chromedp.NewContext(allocCtx)

	// Start the browser eagerly so a missing binary fails here rather than
	// on the first navigation.
	if err := chromedp.Run(tabCtx); err != nil {
		cancelTab()
		cancelAlloc()
		return nil, fmt.Errorf("start browser: %w", err)
	}

	return &Client{
		ctx:     tabCtx,
		cleanup: []func(){cancelTab, cancelAlloc},
		delay:   opts.Delay,
	}, nil
}

// Close shuts the browser down.
func (c *Client) Close() {
	for _, fn := range c.cleanup {
		fn()
	}
	c.cleanup = nil
}

// waitForProducts blocks until the client-rendered product grid has settled.
//
// The grid is hydrated from the network after first paint, so there is no
// server-rendered node to wait on. Instead the card count is polled until it
// stops changing.
func (c *Client) waitForProducts(ctx context.Context, timeout time.Duration) (int, error) {
	const cardCountJS = `document.querySelectorAll('a[href^="/produkt/"]').length`

	deadline := time.Now().Add(timeout)
	last, stableSince := -1, time.Now()

	for {
		var n int
		if err := chromedp.Run(c.ctx, chromedp.Evaluate(cardCountJS, &n)); err != nil {
			return 0, err
		}

		if n != last {
			last, stableSince = n, time.Now()
		}
		if n > 0 && time.Since(stableSince) > 1200*time.Millisecond {
			return n, nil
		}
		if time.Now().After(deadline) {
			if n > 0 {
				return n, nil
			}
			return 0, fmt.Errorf("product grid did not render within %s", timeout)
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// dismissCookieBanner clicks "Avvisa alla" if the consent dialog is up. It is
// best effort: a failure just means the banner stays, which does not affect
// the DOM we read.
func (c *Client) dismissCookieBanner() {
	const js = `(() => {
		const labels = ['Avvisa alla', 'Acceptera cookies'];
		const btn = Array.from(document.querySelectorAll('button'))
			.find(b => labels.includes((b.textContent || '').trim()));
		if (btn) { btn.click(); return true; }
		return false;
	})()`

	var clicked bool
	_ = chromedp.Run(c.ctx, chromedp.Evaluate(js, &clicked))
}
