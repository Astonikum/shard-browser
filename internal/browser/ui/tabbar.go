package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// TabInfo holds display information for a tab.
type TabInfo struct {
	ID    int
	Title string
	URL   string
}

// TabBar is the browser tab bar widget.
type TabBar struct {
	widget.BaseWidget

	tabs      []TabInfo
	activeIdx int

	OnTabSelect func(idx int)
	OnTabClose  func(idx int)
	OnNewTab    func()

	container *fyne.Container
}

// NewTabBar creates a new tab bar.
func NewTabBar() *TabBar {
	tb := &TabBar{activeIdx: -1}
	tb.ExtendBaseWidget(tb)
	return tb
}

// CreateRenderer implements fyne.Widget.
func (tb *TabBar) CreateRenderer() fyne.WidgetRenderer {
	tb.container = container.NewHBox()
	tb.rebuild()
	return widget.NewSimpleRenderer(tb.container)
}

// SetTabs updates the list of tabs.
func (tb *TabBar) SetTabs(tabs []TabInfo, activeIdx int) {
	tb.tabs = tabs
	tb.activeIdx = activeIdx
	tb.rebuild()
	tb.Refresh()
}

// AddTab adds a tab.
func (tb *TabBar) AddTab(info TabInfo) {
	tb.tabs = append(tb.tabs, info)
	tb.activeIdx = len(tb.tabs) - 1
	tb.rebuild()
	tb.Refresh()
}

// UpdateTab updates a tab's info.
func (tb *TabBar) UpdateTab(idx int, info TabInfo) {
	if idx >= 0 && idx < len(tb.tabs) {
		tb.tabs[idx] = info
		tb.rebuild()
		tb.Refresh()
	}
}

// RemoveTab removes a tab.
func (tb *TabBar) RemoveTab(idx int) {
	if idx < 0 || idx >= len(tb.tabs) {
		return
	}
	tb.tabs = append(tb.tabs[:idx], tb.tabs[idx+1:]...)
	if tb.activeIdx >= len(tb.tabs) {
		tb.activeIdx = len(tb.tabs) - 1
	}
	tb.rebuild()
	tb.Refresh()
}

func (tb *TabBar) rebuild() {
	if tb.container == nil {
		return
	}

	var items []fyne.CanvasObject

	for i, tab := range tb.tabs {
		i := i
		tab := tab

		isActive := i == tb.activeIdx

		// Tab background
		var bg color.Color
		if isActive {
			bg = ChromeActiveTab
		} else {
			bg = ChromeTabBG
		}

		tabBG := canvas.NewRectangle(bg)
		tabBG.CornerRadius = 8

		// Tab title (truncated)
		title := tab.Title
		if len([]rune(title)) > 20 {
			runes := []rune(title)
			title = string(runes[:18]) + "…"
		}
		if title == "" {
			title = "New Tab"
		}

		titleLabel := canvas.NewText(title, ChromeDark)
		titleLabel.TextSize = 12
		if isActive {
			titleLabel.TextStyle = fyne.TextStyle{Bold: false}
		}

		// Close button
		closeBtn := widget.NewButtonWithIcon("", theme.WindowCloseIcon(), func() {
			if tb.OnTabClose != nil {
				tb.OnTabClose(i)
			}
		})
		closeBtn.Importance = widget.LowImportance

		// Tab click area
		tapArea := newTapRect(func() {
			if tb.OnTabSelect != nil {
				tb.OnTabSelect(i)
			}
		})

		tabContent := container.New(
			layout.NewHBoxLayout(),
			titleLabel,
			layout.NewSpacer(),
			closeBtn,
		)

		tab_ := container.NewStack(
			tabBG,
			tapArea,
			container.NewPadded(tabContent),
		)
		_ = tab_

		// Simpler approach: use a button-like widget
		var tabWidget fyne.CanvasObject
		if isActive {
			tabWidget = newActiveTab(title, func() {
				if tb.OnTabSelect != nil {
					tb.OnTabSelect(i)
				}
			}, func() {
				if tb.OnTabClose != nil {
					tb.OnTabClose(i)
				}
			})
		} else {
			tabWidget = newInactiveTab(title, func() {
				if tb.OnTabSelect != nil {
					tb.OnTabSelect(i)
				}
			}, func() {
				if tb.OnTabClose != nil {
					tb.OnTabClose(i)
				}
			})
		}
		items = append(items, tabWidget)
	}

	// New tab button
	newTabBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		if tb.OnNewTab != nil {
			tb.OnNewTab()
		}
	})
	newTabBtn.Importance = widget.LowImportance
	items = append(items, newTabBtn)
	items = append(items, layout.NewSpacer())

	tb.container.Objects = items
}

// MinSize implements fyne.Widget.
func (tb *TabBar) MinSize() fyne.Size {
	return fyne.NewSize(100, 36)
}

// newActiveTab creates a visual active tab.
func newActiveTab(title string, onSelect, onClose func()) fyne.CanvasObject {
	bg := canvas.NewRectangle(ChromeActiveTab)
	bg.CornerRadius = 8

	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{}

	closeBtn := widget.NewButtonWithIcon("", theme.WindowCloseIcon(), onClose)
	closeBtn.Importance = widget.LowImportance

	content := container.NewBorder(nil, nil, nil, closeBtn, titleLabel)

	tap := newTapRect(onSelect)

	return container.NewStack(
		bg,
		tap,
		container.New(&tabLayout{}, content),
	)
}

// newInactiveTab creates a visual inactive tab.
func newInactiveTab(title string, onSelect, onClose func()) fyne.CanvasObject {
	bg := canvas.NewRectangle(ChromeTabBG)
	bg.CornerRadius = 6

	titleLabel := canvas.NewText(title, ChromeMedGray)
	titleLabel.TextSize = 12

	closeBtn := widget.NewButtonWithIcon("", theme.WindowCloseIcon(), onClose)
	closeBtn.Importance = widget.LowImportance

	tap := newTapRect(onSelect)

	return container.NewStack(
		bg,
		tap,
		container.NewBorder(nil, nil, nil, closeBtn,
			container.NewPadded(titleLabel)),
	)
}

// tabLayout is a custom layout for tab items.
type tabLayout struct{}

func (l *tabLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	for _, o := range objects {
		o.Resize(fyne.NewSize(containerSize.Width-4, containerSize.Height-4))
		o.Move(fyne.NewPos(2, 2))
	}
}

func (l *tabLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(120, 32)
}

// tapRect is an invisible tappable rectangle.
type tapRect struct {
	widget.BaseWidget
	onTap func()
}

func newTapRect(onTap func()) *tapRect {
	t := &tapRect{onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapRect) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	return widget.NewSimpleRenderer(bg)
}

func (t *tapRect) Tapped(_ *fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *tapRect) MinSize() fyne.Size {
	return fyne.NewSize(0, 0)
}
