package ui

import (
	"image"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/shard-browser/shard/internal/browser"
)

// Window is the main Shard browser window.
type Window struct {
	app        fyne.App
	win        fyne.Window
	b          *browser.Browser
	tabBar     *TabBar
	toolbar    *Toolbar
	viewport   *Viewport
	statusBar  *StatusBar
	currentTab *browser.Tab
}

// NewWindow creates and returns the main browser window.
func NewWindow() *Window {
	a := app.NewWithID("com.shard.browser")
	a.Settings().SetTheme(&ShardTheme{})

	w := &Window{
		app: a,
	}

	w.win = a.NewWindow("Shard")
	w.win.Resize(fyne.NewSize(1280, 800))
	w.win.CenterOnScreen()

	return w
}

// Run initializes the UI and runs the application.
func (w *Window) Run() {
	w.build()
	w.win.ShowAndRun()
}

func (w *Window) build() {
	// Initialize browser
	winSize := w.win.Canvas().Size()
	viewportW := float64(winSize.Width)
	viewportH := float64(winSize.Height) - 80 // subtract chrome height
	if viewportW <= 0 {
		viewportW = 1280
	}
	if viewportH <= 0 {
		viewportH = 700
	}

	w.b = browser.New(viewportW, viewportH)

	// Tab bar
	w.tabBar = NewTabBar()
	w.tabBar.OnTabSelect = func(idx int) {
		w.b.SetActiveTab(idx)
		tab := w.b.ActiveTab()
		w.setActiveTab(tab)
	}
	w.tabBar.OnTabClose = func(idx int) {
		w.b.CloseTab(idx)
		tabs := w.b.Tabs()
		if len(tabs) == 0 {
			w.b.NewTab("about:blank")
			tabs = w.b.Tabs()
		}
		w.refreshTabBar()
		tab := w.b.ActiveTab()
		if tab != nil {
			w.setActiveTab(tab)
		}
	}
	w.tabBar.OnNewTab = func() {
		tab := w.b.NewTab("about:blank")
		w.refreshTabBar()
		w.setActiveTab(tab)
	}

	// Toolbar
	w.toolbar = NewToolbar()
	w.toolbar.OnNavigate = func(url string) {
		tab := w.b.ActiveTab()
		if tab != nil {
			tab.Navigate(url)
			w.toolbar.SetURL(url)
			w.toolbar.SetLoading(true)
		}
	}
	w.toolbar.OnBack = func() {
		tab := w.b.ActiveTab()
		if tab != nil {
			tab.GoBack()
		}
	}
	w.toolbar.OnForward = func() {
		tab := w.b.ActiveTab()
		if tab != nil {
			tab.GoForward()
		}
	}
	w.toolbar.OnReload = func() {
		tab := w.b.ActiveTab()
		if tab != nil {
			tab.Reload()
		}
	}
	w.toolbar.OnHome = func() {
		tab := w.b.ActiveTab()
		if tab != nil {
			tab.Navigate("about:blank")
		}
	}

	// Viewport
	w.viewport = NewViewport()
	w.viewport.OnScroll = func(deltaY float64) {
		tab := w.b.ActiveTab()
		if tab != nil {
			img := tab.Scroll(deltaY)
			if img != nil {
				w.viewport.SetImage(img)
			}
		}
	}
	w.viewport.OnTap = func(x, y float32) {
		tab := w.b.ActiveTab()
		if tab == nil {
			return
		}
		link := tab.FindLink(float64(x), float64(y))
		if link != "" {
			tab.Navigate(link)
			w.toolbar.SetURL(link)
			w.toolbar.SetLoading(true)
		}
	}

	// Status bar
	w.statusBar = NewStatusBar()

	// Browser callbacks
	w.b.OnTitleChange = func(tab *browser.Tab, title string) {
		if w.b.ActiveTab() == tab {
			w.win.SetTitle(title + " — Shard")
			w.toolbar.SetURL(tab.URL)
			w.toolbar.SetLoading(false)
		}
		w.refreshTabBar()
	}

	// Handle window resize
	w.win.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeyF5:
			if tab := w.b.ActiveTab(); tab != nil {
				tab.Reload()
			}
		case fyne.KeyEscape:
			// stop loading (TODO)
		}
	})

	// Layout
	chrome := container.NewVBox(
		w.buildTabBar(),
		w.buildToolbar(),
	)

	chromeBG := canvas.NewRectangle(ChromeGray)

	chromeStack := container.NewStack(chromeBG, chrome)

	// Main layout
	content := container.NewBorder(
		chromeStack,       // top
		w.buildStatusBar(), // bottom
		nil,
		nil,
		w.viewport, // center (fills remaining space)
	)

	w.win.SetContent(content)

	// Open initial tab
	tab := w.b.NewTab("about:blank")
	w.refreshTabBar()
	w.setActiveTab(tab)

	// Handle resize
	w.win.Canvas().Content().Refresh()
}

func (w *Window) buildTabBar() *fyne.Container {
	tabBG := canvas.NewRectangle(ChromeTabBG)
	return container.NewStack(
		tabBG,
		container.New(&tabBarLayout{height: 36}, w.tabBar),
	)
}

func (w *Window) buildToolbar() *fyne.Container {
	toolBG := canvas.NewRectangle(ChromeGray)
	return container.NewStack(
		toolBG,
		container.New(&toolbarLayout{height: 44}, w.toolbar),
	)
}

func (w *Window) buildStatusBar() *fyne.Container {
	return container.NewStack(
		canvas.NewRectangle(ChromeGray),
		container.NewPadded(w.statusBar),
	)
}

func (w *Window) setActiveTab(tab *browser.Tab) {
	if tab == nil {
		return
	}
	w.currentTab = tab

	// Wire up tab callbacks
	tab.OnLoadStart = func() {
		w.toolbar.SetLoading(true)
		w.statusBar.SetText("Loading " + tab.URL + "...")
	}
	tab.OnLoadFinish = func(title string) {
		w.toolbar.SetLoading(false)
		w.toolbar.SetURL(tab.URL)
		w.win.SetTitle(title + " — Shard")
		w.toolbar.SetBackEnabled(tab.CanGoBack())
		w.toolbar.SetForwardEnabled(tab.CanGoForward())
		w.statusBar.SetText("")
		w.refreshTabBar()

		// Update rendered image
		img := tab.GetImage()
		if img != nil {
			w.viewport.SetImage(img)
		}
	}
	tab.OnLoadError = func(err error) {
		w.toolbar.SetLoading(false)
		w.statusBar.SetText("Error: " + err.Error())
	}
	tab.OnImageReady = func(img *image.RGBA) {
		w.viewport.SetImage(img)
	}

	// Show current tab's content
	img := tab.GetImage()
	if img != nil {
		w.viewport.SetImage(img)
	}
	w.toolbar.SetURL(tab.URL)
	w.toolbar.SetBackEnabled(tab.CanGoBack())
	w.toolbar.SetForwardEnabled(tab.CanGoForward())
	w.toolbar.SetLoading(tab.State == browser.TabLoading)
}

func (w *Window) refreshTabBar() {
	tabs := w.b.Tabs()
	infos := make([]TabInfo, len(tabs))
	for i, tab := range tabs {
		title := tab.Title
		if title == "" {
			title = "New Tab"
		}
		infos[i] = TabInfo{
			ID:    tab.ID,
			Title: title,
			URL:   tab.URL,
		}
	}
	w.tabBar.SetTabs(infos, w.b.ActiveIndex())
}

// tabBarLayout is a custom layout for the tab bar container.
type tabBarLayout struct {
	height float32
}

func (l *tabBarLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	for _, o := range objects {
		o.Resize(fyne.NewSize(containerSize.Width, l.height))
		o.Move(fyne.NewPos(0, 0))
	}
}

func (l *tabBarLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, l.height)
}

// toolbarLayout is a custom layout for the toolbar.
type toolbarLayout struct {
	height float32
}

func (l *toolbarLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	for _, o := range objects {
		o.Resize(fyne.NewSize(containerSize.Width, l.height))
		o.Move(fyne.NewPos(0, 0))
	}
}

func (l *toolbarLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, l.height)
}

// ---- Toolbar ----

// Toolbar is the navigation toolbar widget.
type Toolbar struct {
	widget.BaseWidget

	OnBack     func()
	OnForward  func()
	OnReload   func()
	OnHome     func()
	OnNavigate func(url string)

	backBtn    *widget.Button
	fwdBtn     *widget.Button
	reloadBtn  *widget.Button
	homeBtn    *widget.Button
	urlEntry   *widget.Entry
	menuBtn    *widget.Button
	loading    bool
	container  *fyne.Container
}

// NewToolbar creates a new navigation toolbar.
func NewToolbar() *Toolbar {
	t := &Toolbar{}
	t.ExtendBaseWidget(t)
	return t
}

// CreateRenderer implements fyne.Widget.
func (t *Toolbar) CreateRenderer() fyne.WidgetRenderer {
	t.backBtn = widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if t.OnBack != nil {
			t.OnBack()
		}
	})
	t.backBtn.Importance = widget.LowImportance

	t.fwdBtn = widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() {
		if t.OnForward != nil {
			t.OnForward()
		}
	})
	t.fwdBtn.Importance = widget.LowImportance

	t.reloadBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		if t.OnReload != nil {
			t.OnReload()
		}
	})
	t.reloadBtn.Importance = widget.LowImportance

	t.homeBtn = widget.NewButtonWithIcon("", theme.HomeIcon(), func() {
		if t.OnHome != nil {
			t.OnHome()
		}
	})
	t.homeBtn.Importance = widget.LowImportance

	// URL bar (omnibox)
	t.urlEntry = widget.NewEntry()
	t.urlEntry.PlaceHolder = "Search or enter web address"
	t.urlEntry.OnSubmitted = func(s string) {
		if t.OnNavigate != nil {
			t.OnNavigate(strings.TrimSpace(s))
		}
	}

	// Menu button
	t.menuBtn = widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		// TODO: show menu
	})
	t.menuBtn.Importance = widget.LowImportance

	navBtns := container.NewHBox(
		t.backBtn,
		t.fwdBtn,
		t.reloadBtn,
		t.homeBtn,
	)

	t.container = container.NewBorder(
		nil, nil,
		navBtns,
		t.menuBtn,
		container.NewPadded(t.urlEntry),
	)

	return widget.NewSimpleRenderer(container.NewPadded(t.container))
}

// SetURL sets the URL in the address bar.
func (t *Toolbar) SetURL(url string) {
	if t.urlEntry != nil {
		t.urlEntry.SetText(url)
	}
}

// SetLoading sets the loading state (changes reload button icon).
func (t *Toolbar) SetLoading(loading bool) {
	t.loading = loading
	if t.reloadBtn == nil {
		return
	}
	if loading {
		t.reloadBtn.SetIcon(theme.CancelIcon())
	} else {
		t.reloadBtn.SetIcon(theme.ViewRefreshIcon())
	}
}

// SetBackEnabled enables/disables the back button.
func (t *Toolbar) SetBackEnabled(enabled bool) {
	if t.backBtn == nil {
		return
	}
	if enabled {
		t.backBtn.Enable()
	} else {
		t.backBtn.Disable()
	}
}

// SetForwardEnabled enables/disables the forward button.
func (t *Toolbar) SetForwardEnabled(enabled bool) {
	if t.fwdBtn == nil {
		return
	}
	if enabled {
		t.fwdBtn.Enable()
	} else {
		t.fwdBtn.Disable()
	}
}

// MinSize implements fyne.Widget.
func (t *Toolbar) MinSize() fyne.Size {
	return fyne.NewSize(400, 44)
}

// ---- Status Bar ----

// StatusBar is the browser status bar.
type StatusBar struct {
	widget.BaseWidget
	label *canvas.Text
	text  string
}

// NewStatusBar creates a new status bar.
func NewStatusBar() *StatusBar {
	s := &StatusBar{}
	s.ExtendBaseWidget(s)
	return s
}

// CreateRenderer implements fyne.Widget.
func (s *StatusBar) CreateRenderer() fyne.WidgetRenderer {
	s.label = canvas.NewText(s.text, color.RGBA{R: 0x5F, G: 0x63, B: 0x68, A: 0xFF})
	s.label.TextSize = 11
	return widget.NewSimpleRenderer(s.label)
}

// SetText sets the status bar text.
func (s *StatusBar) SetText(text string) {
	s.text = text
	if s.label != nil {
		s.label.Text = text
		s.label.Refresh()
	}
}

// MinSize implements fyne.Widget.
func (s *StatusBar) MinSize() fyne.Size {
	return fyne.NewSize(0, 18)
}

// ---- Spacer ----

// spacer is a fixed-size spacer (used internally).
type spacer struct {
	size fyne.Size
}

func newSpacer(w, h float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(w, h))
	return r
}

// needed for the layout package import
var _ = layout.NewSpacer
var _ = newSpacer
