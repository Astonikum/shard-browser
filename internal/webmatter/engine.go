// Package webmatter implements the WebMatter web rendering engine.
package webmatter

import (
	"image"
	"strings"
	"sync"

	"github.com/shard-browser/shard/internal/webmatter/css"
	"github.com/shard-browser/shard/internal/webmatter/dom"
	"github.com/shard-browser/shard/internal/webmatter/html"
	"github.com/shard-browser/shard/internal/webmatter/layout"
	"github.com/shard-browser/shard/internal/webmatter/network"
	"github.com/shard-browser/shard/internal/webmatter/render"
)

// Page represents a rendered web page.
type Page struct {
	URL     string
	Title   string
	Doc     *dom.Document
	Root    *layout.Box
	Image   *image.RGBA
	Height  float64 // total page height
	ScrollY float64
}

// Engine is the WebMatter rendering engine.
type Engine struct {
	net      *network.Client
	cascade  *css.Cascade
	mu       sync.Mutex
	viewport layout.Viewport
}

// NewEngine creates a new WebMatter engine instance.
func NewEngine(viewportWidth, viewportHeight float64) *Engine {
	return &Engine{
		net:     network.NewClient(),
		cascade: css.NewCascade(),
		viewport: layout.Viewport{
			Width:  viewportWidth,
			Height: viewportHeight,
		},
	}
}

// SetViewport updates the viewport dimensions.
func (e *Engine) SetViewport(width, height float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.viewport.Width = width
	e.viewport.Height = height
}

// Navigate fetches and renders a URL.
func (e *Engine) Navigate(url string) *Page {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Fetch the page
	resource := e.net.Fetch(url)
	if resource.Err != nil {
		return e.renderErrorPage(url, resource.Err)
	}

	finalURL := resource.URL
	if finalURL == "" {
		finalURL = url
	}

	// Parse HTML
	doc := html.Parse(resource.Text())
	doc.BaseURL = finalURL

	// Collect stylesheets embedded in HTML
	sheets := html.CollectStylesheets(doc)

	// Fetch external CSS
	for _, linkEl := range dom.GetElementsByTagName(&doc.Node, "link") {
		if strings.ToLower(linkEl.GetAttr("rel")) == "stylesheet" {
			href := linkEl.GetAttr("href")
			if href != "" {
				cssRes := e.net.FetchCSS(href, finalURL)
				if cssRes.Err == nil {
					sheets = append(sheets, cssRes.Text())
				}
			}
		}
	}

	// Apply CSS cascade
	e.cascade.Apply(doc, sheets)

	// Layout
	layoutEngine := layout.NewEngine(e.viewport)
	rootBox := layoutEngine.Layout(doc)

	// Determine page height
	var pageHeight float64
	if rootBox != nil {
		pageHeight = rootBox.BorderBoxHeight() + rootBox.PaddingTop + rootBox.PaddingBottom
	}

	// Render
	painter := render.NewPainter(int(e.viewport.Width), int(e.viewport.Height))
	img := painter.Paint(rootBox)

	page := &Page{
		URL:    finalURL,
		Title:  doc.Title,
		Doc:    doc,
		Root:   rootBox,
		Image:  img,
		Height: pageHeight,
	}

	if page.Title == "" {
		page.Title = finalURL
	}

	return page
}

// RenderWithScroll re-renders a page with a new scroll offset.
func (e *Engine) RenderWithScroll(page *Page, scrollY float64) *image.RGBA {
	e.mu.Lock()
	defer e.mu.Unlock()

	page.ScrollY = scrollY

	painter := render.NewPainter(int(e.viewport.Width), int(e.viewport.Height))
	painter.SetScroll(scrollY)
	return painter.Paint(page.Root)
}

// Resize re-lays out a page for a new viewport size.
func (e *Engine) Resize(page *Page, width, height float64) *image.RGBA {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.viewport.Width = width
	e.viewport.Height = height

	layoutEngine := layout.NewEngine(e.viewport)
	page.Root = layoutEngine.Layout(page.Doc)

	var pageHeight float64
	if page.Root != nil {
		pageHeight = page.Root.BorderBoxHeight()
	}
	page.Height = pageHeight

	painter := render.NewPainter(int(width), int(height))
	painter.SetScroll(page.ScrollY)
	return painter.Paint(page.Root)
}

// HitTest finds the DOM node at the given coordinates.
func (e *Engine) HitTest(page *Page, x, y float64) *dom.Node {
	if page == nil || page.Root == nil {
		return nil
	}
	box := page.Root.HitTest(x, y+page.ScrollY)
	if box == nil {
		return nil
	}
	return box.Node
}

// FindLinkAt finds an <a href> ancestor of the node at the given coordinates.
func (e *Engine) FindLinkAt(page *Page, x, y float64) string {
	node := e.HitTest(page, x, y)
	for node != nil {
		if node.TagName == "a" && node.HasAttr("href") {
			href := node.GetAttr("href")
			return network.ResolveURL(href, page.URL)
		}
		node = node.Parent
	}
	return ""
}

func (e *Engine) renderErrorPage(url string, err error) *Page {
	errHTML := `<!DOCTYPE html><html><head><title>Error</title><style>
body { font-family: sans-serif; padding: 40px; background: #fef2f2; }
h1 { color: #dc2626; }
p { color: #374151; }
code { background: #f3f4f6; padding: 2px 6px; border-radius: 4px; font-family: monospace; }
</style></head><body>
<h1>Cannot load page</h1>
<p>Shard could not load <code>` + url + `</code></p>
<p>Error: <code>` + err.Error() + `</code></p>
</body></html>`

	doc := html.Parse(errHTML)
	doc.BaseURL = url

	e.cascade.Apply(doc, nil)

	layoutEngine := layout.NewEngine(e.viewport)
	rootBox := layoutEngine.Layout(doc)

	painter := render.NewPainter(int(e.viewport.Width), int(e.viewport.Height))
	img := painter.Paint(rootBox)

	return &Page{
		URL:   url,
		Title: "Error",
		Doc:   doc,
		Root:  rootBox,
		Image: img,
	}
}
