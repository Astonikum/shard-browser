// Package network implements the HTTP(S) networking layer for WebMatter.
package network

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Resource is a fetched network resource.
type Resource struct {
	URL         string
	ContentType string
	Body        []byte
	StatusCode  int
	Headers     http.Header
	Err         error
}

// IsHTML reports whether the resource is an HTML document.
func (r *Resource) IsHTML() bool {
	ct := strings.ToLower(r.ContentType)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml")
}

// IsCSS reports whether the resource is a CSS stylesheet.
func (r *Resource) IsCSS() bool {
	ct := strings.ToLower(r.ContentType)
	return strings.Contains(ct, "text/css")
}

// Text returns the body as a string.
func (r *Resource) Text() string {
	return string(r.Body)
}

// cacheEntry is an entry in the resource cache.
type cacheEntry struct {
	resource  *Resource
	expiresAt time.Time
}

// Client is an HTTP client with caching.
type Client struct {
	httpClient *http.Client
	cache      map[string]*cacheEntry
	mu         sync.RWMutex
	userAgent  string
}

// NewClient creates a new network client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		cache:     make(map[string]*cacheEntry),
		userAgent: "Shard/1.0 WebMatter/1.0 (compatible)",
	}
}

// Fetch fetches a URL and returns the resource.
func (c *Client) Fetch(rawURL string) *Resource {
	// Check cache
	c.mu.RLock()
	entry, ok := c.cache[rawURL]
	c.mu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.resource
	}

	// Handle special URLs
	if strings.HasPrefix(rawURL, "about:") {
		return c.fetchAbout(rawURL)
	}
	if strings.HasPrefix(rawURL, "data:") {
		return c.fetchData(rawURL)
	}

	// HTTP/HTTPS fetch
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return &Resource{URL: rawURL, Err: err}
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Resource{URL: rawURL, Err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return &Resource{URL: rawURL, Err: err}
	}

	resource := &Resource{
		URL:         resp.Request.URL.String(), // final URL after redirects
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
		StatusCode:  resp.StatusCode,
		Headers:     resp.Header,
	}

	// Cache successful responses
	if resp.StatusCode == 200 {
		ttl := parseCacheControl(resp.Header.Get("Cache-Control"))
		if ttl > 0 {
			c.mu.Lock()
			c.cache[rawURL] = &cacheEntry{
				resource:  resource,
				expiresAt: time.Now().Add(ttl),
			}
			c.mu.Unlock()
		}
	}

	return resource
}

// FetchCSS fetches a CSS stylesheet relative to a base URL.
func (c *Client) FetchCSS(href, baseURL string) *Resource {
	resolved := resolveURL(href, baseURL)
	if resolved == "" {
		return &Resource{URL: href, Err: fmt.Errorf("could not resolve URL")}
	}
	return c.Fetch(resolved)
}

// ResolveURL resolves a URL relative to a base URL.
func ResolveURL(href, baseURL string) string {
	return resolveURL(href, baseURL)
}

func resolveURL(href, baseURL string) string {
	if href == "" {
		return baseURL
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") ||
		strings.HasPrefix(href, "about:") || strings.HasPrefix(href, "data:") {
		return href
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func (c *Client) fetchAbout(rawURL string) *Resource {
	switch rawURL {
	case "about:blank":
		return &Resource{
			URL:         rawURL,
			ContentType: "text/html",
			Body:        []byte(newTabPage),
			StatusCode:  200,
		}
	case "about:newtab":
		return &Resource{
			URL:         rawURL,
			ContentType: "text/html",
			Body:        []byte(newTabPage),
			StatusCode:  200,
		}
	default:
		return &Resource{
			URL:         rawURL,
			ContentType: "text/html",
			Body:        []byte("<html><body><p>Unknown about: URL</p></body></html>"),
			StatusCode:  200,
		}
	}
}

func (c *Client) fetchData(rawURL string) *Resource {
	// data:mime;base64,data or data:mime,data
	rest := rawURL[5:] // after "data:"
	comma := strings.Index(rest, ",")
	if comma == -1 {
		return &Resource{URL: rawURL, Err: fmt.Errorf("invalid data URL")}
	}
	meta := rest[:comma]
	data := rest[comma+1:]
	parts := strings.Split(meta, ";")
	mime := "text/plain"
	if len(parts) > 0 && parts[0] != "" {
		mime = parts[0]
	}
	isBase64 := len(parts) > 1 && parts[len(parts)-1] == "base64"
	_ = isBase64
	// For simplicity, just return as text (skip base64 decode for now)
	return &Resource{
		URL:         rawURL,
		ContentType: mime,
		Body:        []byte(data),
		StatusCode:  200,
	}
}

func parseCacheControl(cc string) time.Duration {
	if cc == "" {
		return 0
	}
	parts := strings.Split(cc, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "max-age=") {
			var secs int64
			for _, ch := range p[8:] {
				if ch >= '0' && ch <= '9' {
					secs = secs*10 + int64(ch-'0')
				} else {
					break
				}
			}
			if secs > 0 {
				return time.Duration(secs) * time.Second
			}
		}
		if p == "no-store" || p == "no-cache" {
			return 0
		}
	}
	return 5 * time.Minute // default cache for 5 minutes
}

// ClearCache clears the network cache.
func (c *Client) ClearCache() {
	c.mu.Lock()
	c.cache = make(map[string]*cacheEntry)
	c.mu.Unlock()
}

// newTabPage is the HTML for the new tab page.
const newTabPage = `<!DOCTYPE html>
<html>
<head>
<title>New Tab</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
    background-color: #f9f9f9;
    font-family: sans-serif;
    display: flex;
    flex-direction: column;
    align-items: center;
    padding-top: 80px;
    min-height: 100vh;
}
.logo {
    font-size: 48px;
    font-weight: bold;
    color: #1a73e8;
    margin-bottom: 32px;
    letter-spacing: -2px;
}
.logo span { color: #e8431a; }
.search-container {
    width: 600px;
    max-width: 90%;
}
.search-box {
    width: 100%;
    padding: 12px 20px;
    font-size: 18px;
    border: 2px solid #dadce0;
    border-radius: 24px;
    background: white;
    color: #202124;
}
.shortcuts {
    display: flex;
    margin-top: 40px;
    gap: 16px;
    flex-wrap: wrap;
    justify-content: center;
    max-width: 600px;
}
.shortcut {
    display: flex;
    flex-direction: column;
    align-items: center;
    width: 96px;
    padding: 12px 8px;
    border-radius: 12px;
    cursor: pointer;
    color: #3c4043;
    font-size: 12px;
    text-decoration: none;
}
.shortcut:hover { background: #f1f3f4; }
.shortcut-icon {
    width: 48px;
    height: 48px;
    border-radius: 50%;
    margin-bottom: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 24px;
    font-weight: bold;
}
.footer {
    position: fixed;
    bottom: 20px;
    color: #5f6368;
    font-size: 12px;
}
</style>
</head>
<body>
<div class="logo">Sh<span>a</span>rd</div>
<div class="search-container">
    <input class="search-box" type="text" placeholder="Search or enter web address">
</div>
<div class="shortcuts">
    <a class="shortcut" href="https://github.com">
        <div class="shortcut-icon" style="background:#24292e; color:white">G</div>
        GitHub
    </a>
    <a class="shortcut" href="https://google.com">
        <div class="shortcut-icon" style="background:#4285f4; color:white">G</div>
        Google
    </a>
    <a class="shortcut" href="https://wikipedia.org">
        <div class="shortcut-icon" style="background:#f8f8f8; color:#202122">W</div>
        Wikipedia
    </a>
    <a class="shortcut" href="https://news.ycombinator.com">
        <div class="shortcut-icon" style="background:#ff6600; color:white">Y</div>
        Hacker News
    </a>
</div>
<div class="footer">Shard Browser &mdash; Powered by WebMatter Engine</div>
</body>
</html>`
