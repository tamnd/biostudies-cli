// Package biostudies is the library behind the biostudies command line:
// the HTTP client, request shaping, and the typed data models for EMBL-EBI BioStudies.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package biostudies

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultUserAgent identifies the client to BioStudies.
const DefaultUserAgent = "biostudies-cli/dev (+https://github.com/tamnd/biostudies-cli)"

// Host is the BioStudies site this client talks to.
const Host = "www.ebi.ac.uk"

// baseURL is the root every request is built from.
const baseURL = "https://" + Host + "/biostudies"

// Client talks to BioStudies over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 300ms
// minimum gap between requests, and three retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Retries:   3,
	}
}

// Get fetches rawURL and returns the response body. It paces and retries
// according to the client's settings. The body is read fully and closed here.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire types (unexported) ---

type wireHit struct {
	Accession   string `json:"accession"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Links       int    `json:"links"`
	Files       int    `json:"files"`
	ReleaseDate string `json:"release_date"`
	Views       int    `json:"views"`
	IsPublic    bool   `json:"isPublic"`
}

type wireSearchResp struct {
	TotalHits int       `json:"totalHits"`
	Hits      []wireHit `json:"hits"`
}

type wireAttr struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type wireStudy struct {
	Accno      string     `json:"accno"`
	Attributes []wireAttr `json:"attributes"`
}

// --- public output types ---

// Study is a single EMBL-EBI BioStudies record.
type Study struct {
	ID          string `json:"id"             kit:"id"`
	Title       string `json:"title"`
	Author      string `json:"author,omitempty"`
	Type        string `json:"type,omitempty"`
	Files       int    `json:"files,omitempty"`
	Links       int    `json:"links,omitempty"`
	ReleaseDate string `json:"release_date,omitempty"`
	Views       int    `json:"views,omitempty"`
}

// --- client methods ---

// Search queries the BioStudies search API and returns matching Study records
// along with the total hit count.
func (c *Client) Search(ctx context.Context, query string, limit, page int) ([]*Study, int, error) {
	if limit <= 0 {
		limit = 10
	}
	if page <= 0 {
		page = 1
	}
	params := url.Values{
		"q":        {query},
		"pageSize": {strconv.Itoa(limit)},
		"page":     {strconv.Itoa(page)},
	}
	rawURL := baseURL + "/api/v1/search?" + params.Encode()
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, 0, err
	}
	var ws wireSearchResp
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, 0, fmt.Errorf("search parse: %w", err)
	}
	out := make([]*Study, 0, len(ws.Hits))
	for _, h := range ws.Hits {
		out = append(out, studyFromHit(h))
	}
	return out, ws.TotalHits, nil
}

// GetStudy fetches a single BioStudies record by accession (e.g. "S-BSST3126").
func (c *Client) GetStudy(ctx context.Context, accession string) (*Study, error) {
	rawURL := baseURL + "/api/v1/studies/" + accession
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var ws wireStudy
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, fmt.Errorf("study parse: %w", err)
	}
	id := ws.Accno
	if id == "" {
		id = accession
	}
	title := attrValue(ws.Attributes, "Title")
	return &Study{
		ID:    id,
		Title: title,
	}, nil
}

// searchURL fetches and parses a pre-built search URL. It is the testable core
// of Search; tests point it at an httptest server without touching baseURL.
func (c *Client) searchURL(ctx context.Context, rawURL string) ([]*Study, int, error) {
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, 0, err
	}
	var ws wireSearchResp
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, 0, fmt.Errorf("search parse: %w", err)
	}
	out := make([]*Study, 0, len(ws.Hits))
	for _, h := range ws.Hits {
		out = append(out, studyFromHit(h))
	}
	return out, ws.TotalHits, nil
}

// getStudyURL fetches and parses a pre-built study detail URL. It is the
// testable core of GetStudy; tests point it at an httptest server.
func (c *Client) getStudyURL(ctx context.Context, rawURL string) (*Study, error) {
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var ws wireStudy
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, fmt.Errorf("study parse: %w", err)
	}
	id := ws.Accno
	if id == "" {
		id = rawURL
	}
	title := attrValue(ws.Attributes, "Title")
	return &Study{
		ID:    id,
		Title: title,
	}, nil
}

// --- helpers ---

// attrValue returns the value of the first attribute whose name matches, case-sensitive.
func attrValue(attrs []wireAttr, name string) string {
	for _, a := range attrs {
		if a.Name == name {
			return a.Value
		}
	}
	return ""
}

// studyFromHit converts a search hit to a Study.
func studyFromHit(h wireHit) *Study {
	return &Study{
		ID:          h.Accession,
		Title:       h.Title,
		Author:      h.Author,
		Type:        h.Type,
		Files:       h.Files,
		Links:       h.Links,
		ReleaseDate: h.ReleaseDate,
		Views:       h.Views,
	}
}
