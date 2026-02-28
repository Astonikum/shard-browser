package browser

import (
	"sync"

	"github.com/shard-browser/shard/internal/webmatter"
)

// Browser is the main browser application controller.
type Browser struct {
	engine     *webmatter.Engine
	tabs       []*Tab
	activeTab  int
	mu         sync.RWMutex

	// Callbacks
	OnTabAdded    func(tab *Tab)
	OnTabRemoved  func(idx int)
	OnTabChanged  func(tab *Tab)
	OnTitleChange func(tab *Tab, title string)
}

// New creates a new Browser.
func New(viewportWidth, viewportHeight float64) *Browser {
	return &Browser{
		engine:    webmatter.NewEngine(viewportWidth, viewportHeight),
		activeTab: -1,
	}
}

// NewTab creates and adds a new tab.
func (b *Browser) NewTab(url string) *Tab {
	tab := NewTab(b.engine)

	b.mu.Lock()
	b.tabs = append(b.tabs, tab)
	idx := len(b.tabs) - 1
	b.activeTab = idx
	b.mu.Unlock()

	// Set callbacks
	tab.OnLoadFinish = func(title string) {
		if b.OnTitleChange != nil {
			b.OnTitleChange(tab, title)
		}
	}

	if b.OnTabAdded != nil {
		b.OnTabAdded(tab)
	}

	if url != "" {
		tab.Navigate(url)
	}

	return tab
}

// CloseTab closes a tab by index.
func (b *Browser) CloseTab(idx int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if idx < 0 || idx >= len(b.tabs) {
		return
	}

	b.tabs = append(b.tabs[:idx], b.tabs[idx+1:]...)

	if b.activeTab >= len(b.tabs) {
		b.activeTab = len(b.tabs) - 1
	}

	if b.OnTabRemoved != nil {
		b.OnTabRemoved(idx)
	}
}

// ActiveTab returns the currently active tab.
func (b *Browser) ActiveTab() *Tab {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.activeTab < 0 || b.activeTab >= len(b.tabs) {
		return nil
	}
	return b.tabs[b.activeTab]
}

// SetActiveTab sets the active tab by index.
func (b *Browser) SetActiveTab(idx int) {
	b.mu.Lock()
	if idx >= 0 && idx < len(b.tabs) {
		b.activeTab = idx
	}
	tab := b.tabs[b.activeTab]
	b.mu.Unlock()

	if b.OnTabChanged != nil {
		b.OnTabChanged(tab)
	}
}

// Tabs returns all tabs.
func (b *Browser) Tabs() []*Tab {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]*Tab, len(b.tabs))
	copy(result, b.tabs)
	return result
}

// TabCount returns the number of open tabs.
func (b *Browser) TabCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.tabs)
}

// ActiveIndex returns the index of the active tab.
func (b *Browser) ActiveIndex() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.activeTab
}

// Engine returns the underlying WebMatter engine.
func (b *Browser) Engine() *webmatter.Engine {
	return b.engine
}

// SetViewport updates the viewport size for the engine.
func (b *Browser) SetViewport(width, height float64) {
	b.engine.SetViewport(width, height)
}
