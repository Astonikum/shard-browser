// Package browser implements the Shard browser application layer.
package browser

import (
	"image"
	"sync"

	"github.com/shard-browser/shard/internal/webmatter"
)

// TabState describes the loading state of a tab.
type TabState int

const (
	TabIdle     TabState = iota
	TabLoading
	TabError
)

// HistoryEntry is a URL in the tab's navigation history.
type HistoryEntry struct {
	URL   string
	Title string
}

// Tab represents a browser tab.
type Tab struct {
	ID      int
	URL     string
	Title   string
	State   TabState
	Favicon string // URL of favicon

	page    *webmatter.Page
	engine  *webmatter.Engine
	history []HistoryEntry
	histIdx int // current position in history

	mu sync.RWMutex

	// Callbacks (set by UI)
	OnLoadStart  func()
	OnLoadFinish func(title string)
	OnLoadError  func(err error)
	OnImageReady func(img *image.RGBA)
}

var tabCounter int

// NewTab creates a new browser tab.
func NewTab(engine *webmatter.Engine) *Tab {
	tabCounter++
	return &Tab{
		ID:      tabCounter,
		Title:   "New Tab",
		URL:     "about:blank",
		State:   TabIdle,
		engine:  engine,
		histIdx: -1,
	}
}

// Navigate navigates the tab to a URL.
func (t *Tab) Navigate(url string) {
	if url == "" {
		url = "about:blank"
	}
	url = normalizeURL(url)

	t.mu.Lock()
	t.URL = url
	t.State = TabLoading
	t.mu.Unlock()

	if t.OnLoadStart != nil {
		t.OnLoadStart()
	}

	go func() {
		page := t.engine.Navigate(url)

		t.mu.Lock()
		t.page = page
		t.URL = page.URL
		title := page.Title
		if title == "" {
			title = url
		}
		t.Title = title
		t.State = TabIdle

		// Add to history
		entry := HistoryEntry{URL: page.URL, Title: title}
		// Truncate forward history
		if t.histIdx < len(t.history)-1 {
			t.history = t.history[:t.histIdx+1]
		}
		t.history = append(t.history, entry)
		t.histIdx = len(t.history) - 1
		t.mu.Unlock()

		if t.OnLoadFinish != nil {
			t.OnLoadFinish(title)
		}
		if t.OnImageReady != nil && page.Image != nil {
			t.OnImageReady(page.Image)
		}
	}()
}

// Reload reloads the current page.
func (t *Tab) Reload() {
	t.mu.RLock()
	url := t.URL
	t.mu.RUnlock()
	t.Navigate(url)
}

// GoBack navigates back in history.
func (t *Tab) GoBack() bool {
	t.mu.Lock()
	if t.histIdx <= 0 {
		t.mu.Unlock()
		return false
	}
	t.histIdx--
	entry := t.history[t.histIdx]
	t.mu.Unlock()
	t.Navigate(entry.URL)
	return true
}

// GoForward navigates forward in history.
func (t *Tab) GoForward() bool {
	t.mu.Lock()
	if t.histIdx >= len(t.history)-1 {
		t.mu.Unlock()
		return false
	}
	t.histIdx++
	entry := t.history[t.histIdx]
	t.mu.Unlock()
	t.Navigate(entry.URL)
	return true
}

// CanGoBack reports whether back navigation is possible.
func (t *Tab) CanGoBack() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.histIdx > 0
}

// CanGoForward reports whether forward navigation is possible.
func (t *Tab) CanGoForward() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.histIdx < len(t.history)-1
}

// GetImage returns the current rendered image of the page.
func (t *Tab) GetImage() *image.RGBA {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.page == nil {
		return nil
	}
	return t.page.Image
}

// GetPage returns the current page.
func (t *Tab) GetPage() *webmatter.Page {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.page
}

// Scroll updates the scroll position and re-renders.
func (t *Tab) Scroll(deltaY float64) *image.RGBA {
	t.mu.Lock()
	if t.page == nil {
		t.mu.Unlock()
		return nil
	}
	page := t.page
	t.mu.Unlock()

	newY := page.ScrollY + deltaY
	if newY < 0 {
		newY = 0
	}
	maxY := page.Height
	if newY > maxY {
		newY = maxY
	}
	return t.engine.RenderWithScroll(page, newY)
}

// FindLink finds a link at the given page coordinates.
func (t *Tab) FindLink(x, y float64) string {
	t.mu.RLock()
	page := t.page
	t.mu.RUnlock()
	if page == nil {
		return ""
	}
	return t.engine.FindLinkAt(page, x, y)
}

// Resize re-renders for a new viewport size.
func (t *Tab) Resize(width, height float64) *image.RGBA {
	t.mu.RLock()
	page := t.page
	t.mu.RUnlock()
	if page == nil {
		return nil
	}
	return t.engine.Resize(page, width, height)
}

// normalizeURL adds a scheme if missing.
func normalizeURL(raw string) string {
	if raw == "" {
		return "about:blank"
	}
	if raw == "about:blank" || raw == "about:newtab" {
		return raw
	}
	if len(raw) > 0 && (raw[0] == '/' || raw[0] == '.') {
		return "file://" + raw
	}
	if len(raw) > 7 && (raw[:7] == "http://" || raw[:8] == "https://") {
		return raw
	}
	if len(raw) > 6 && raw[:5] == "file:" {
		return raw
	}
	if len(raw) > 5 && raw[:5] == "data:" {
		return raw
	}
	// Check if it looks like a domain (contains a dot)
	if containsDot(raw) && !containsSpace(raw) {
		return "https://" + raw
	}
	// Treat as search query
	return "https://www.google.com/search?q=" + encodeQuery(raw)
}

func containsDot(s string) bool {
	for _, ch := range s {
		if ch == '.' {
			return true
		}
	}
	return false
}

func containsSpace(s string) bool {
	for _, ch := range s {
		if ch == ' ' {
			return true
		}
	}
	return false
}

func encodeQuery(s string) string {
	var result []byte
	for _, ch := range []byte(s) {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			result = append(result, ch)
		} else if ch == ' ' {
			result = append(result, '+')
		} else {
			result = append(result, '%', hexChar(ch>>4), hexChar(ch&0xf))
		}
	}
	return string(result)
}

func hexChar(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'a' + n - 10
}
