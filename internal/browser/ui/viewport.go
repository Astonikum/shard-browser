package ui

import (
	"image"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// Viewport is a Fyne widget that displays a rendered web page.
type Viewport struct {
	widget.BaseWidget

	img     *canvas.Image
	mu      sync.RWMutex
	rawImg  *image.RGBA

	// Callbacks
	OnScroll func(deltaY float64)
	OnTap    func(x, y float32)
}

// NewViewport creates a new web content viewport.
func NewViewport() *Viewport {
	v := &Viewport{}
	v.img = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	v.img.FillMode = canvas.ImageFillStretch
	v.img.ScaleMode = canvas.ImageScaleFastest
	v.ExtendBaseWidget(v)
	return v
}

// CreateRenderer implements fyne.Widget.
func (v *Viewport) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(v.img)
}

// SetImage updates the displayed image.
func (v *Viewport) SetImage(img *image.RGBA) {
	v.mu.Lock()
	v.rawImg = img
	v.mu.Unlock()

	if img != nil {
		v.img.Image = img
		v.img.Refresh()
	}
	v.Refresh()
}

// MinSize implements fyne.Widget.
func (v *Viewport) MinSize() fyne.Size {
	return fyne.NewSize(200, 200)
}

// Scrolled implements fyne.Scrollable.
func (v *Viewport) Scrolled(ev *fyne.ScrollEvent) {
	if v.OnScroll != nil {
		v.OnScroll(float64(-ev.Scrolled.DY) * 3)
	}
}

// Tapped implements fyne.Tappable.
func (v *Viewport) Tapped(ev *fyne.PointEvent) {
	if v.OnTap != nil {
		v.OnTap(ev.Position.X, ev.Position.Y)
	}
}

// TappedSecondary implements fyne.SecondaryTappable.
func (v *Viewport) TappedSecondary(_ *fyne.PointEvent) {}

// GetSize returns the current size of the viewport.
func (v *Viewport) GetSize() (float32, float32) {
	s := v.Size()
	return s.Width, s.Height
}
