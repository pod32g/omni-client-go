package omni

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// Target is the scrape health of one target, as reported by /api/v1/targets.
type Target struct {
	Job                string    `json:"job"`
	Instance           string    `json:"instance"`
	ScrapeURL          string    `json:"scrapeUrl"`
	Up                 bool      `json:"up"`
	LastScrape         time.Time `json:"lastScrape"`
	LastError          string    `json:"lastError"`
	LastScrapeDuration float64   `json:"lastScrapeDuration"` // seconds
	SamplesScraped     int       `json:"samplesScraped"`
}

// Series returns the label sets of series matching the given selectors (e.g.
// `up`, `http_requests_total{job="api"}`). A zero start/end lets the server use
// its default window.
func (c *Client) Series(ctx context.Context, matches []string, start, end time.Time) ([]Labels, error) {
	if len(matches) == 0 {
		return nil, fmt.Errorf("omni: at least one match selector is required")
	}
	params := url.Values{"match[]": matches}
	if !start.IsZero() {
		params.Set("start", formatTime(start))
	}
	if !end.IsZero() {
		params.Set("end", formatTime(end))
	}
	var out []Labels
	if err := c.get(ctx, "/api/v1/series", params, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LabelNames returns all label names present in the server's series.
func (c *Client) LabelNames(ctx context.Context) ([]string, error) {
	var out []string
	if err := c.get(ctx, "/api/v1/labels", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LabelValues returns the distinct values of a single label.
func (c *Client) LabelValues(ctx context.Context, name string) ([]string, error) {
	if name == "" {
		return nil, fmt.Errorf("omni: label name is required")
	}
	var out []string
	if err := c.get(ctx, "/api/v1/label/"+url.PathEscape(name)+"/values", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Targets returns the scrape target health.
func (c *Client) Targets(ctx context.Context) ([]Target, error) {
	var out []Target
	if err := c.get(ctx, "/api/v1/targets", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
