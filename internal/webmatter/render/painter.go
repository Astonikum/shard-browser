// Package render implements the painting/rendering stage of the WebMatter engine.
package render

import (
	"image"
	"image/color"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/shard-browser/shard/internal/webmatter/dom"
	"github.com/shard-browser/shard/internal/webmatter/layout"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Painter renders a layout tree into an image.
type Painter struct {
	img     *image.RGBA
	width   int
	height  int
	scrollY float64
}

// NewPainter creates a new painter for the given dimensions.
func NewPainter(width, height int) *Painter {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	return &Painter{img: img, width: width, height: height}
}

// SetScroll sets the scroll offset.
func (p *Painter) SetScroll(scrollY float64) {
	p.scrollY = scrollY
}

// Paint renders the layout tree and returns the rendered image.
func (p *Painter) Paint(root *layout.Box) *image.RGBA {
	// Fill background white
	fillRect(p.img, 0, 0, p.width, p.height, color.RGBA{255, 255, 255, 255})

	if root != nil {
		p.paintBox(root, 0, -p.scrollY)
	}

	return p.img
}

func (p *Painter) paintBox(b *layout.Box, offsetX, offsetY float64) {
	if b == nil {
		return
	}

	s := getStyle(b)
	if s != nil && s.Visibility == "hidden" {
		return
	}

	bx := offsetX + b.BorderX()
	by := offsetY + b.BorderY()
	bw := b.BorderBoxWidth()
	bh := b.BorderBoxHeight()

	cx := offsetX + b.ContentX()
	cy := offsetY + b.ContentY()

	// Clip to visible area (optimization)
	if by+bh < 0 || by > float64(p.height) {
		// still paint children since they might overflow
	}

	// 1. Paint background
	if s != nil && !s.BackgroundColor.IsTransparent() {
		fillRectF(p.img, bx, by, bw, bh, domColorToRGBA(s.BackgroundColor))
	}

	// 2. Paint borders
	if s != nil {
		p.paintBorders(s, bx, by, bw, bh)
	}

	// 3. Paint content
	switch b.BType {
	case layout.BoxInline, layout.BoxAnonymousInline:
		if b.Node != nil && b.Node.Type == dom.NodeTypeText {
			p.paintText(b, s, cx, cy)
		}
	default:
		// Paint list marker
		if b.BType == layout.BoxListItem && s != nil && s.ListStyleType != "none" {
			p.paintListMarker(s, bx, by, bh)
		}

		// Paint text children (from line boxes)
		if len(b.Lines) > 0 {
			for _, line := range b.Lines {
				for _, lb := range line.Boxes {
					if lb.Node != nil && lb.Node.Type == dom.NodeTypeText {
						ls := getInlineStyle(lb)
						p.paintTextAt(lb.Node.Data, ls, cx+lb.X, cy+lb.Y)
					}
				}
			}
		}

		// Recurse children
		for _, child := range b.Children {
			p.paintBox(child, cx, cy)
		}
	}

	// 4. Paint outline (if any)
	if s != nil && s.OutlineWidth > 0 && s.OutlineStyle != "none" {
		// simple outline
		strokeRectF(p.img, bx-s.OutlineWidth, by-s.OutlineWidth,
			bw+s.OutlineWidth*2, bh+s.OutlineWidth*2,
			s.OutlineWidth, domColorToRGBA(s.OutlineColor))
	}
}

func (p *Painter) paintBorders(s *dom.ComputedStyle, x, y, w, h float64) {
	// Top border
	if s.BorderTopWidth > 0 && s.BorderTopStyle != "none" && s.BorderTopStyle != "" {
		fillRectF(p.img, x, y, w, s.BorderTopWidth, domColorToRGBA(s.BorderTopColor))
	}
	// Bottom border
	if s.BorderBottomWidth > 0 && s.BorderBottomStyle != "none" && s.BorderBottomStyle != "" {
		fillRectF(p.img, x, y+h-s.BorderBottomWidth, w, s.BorderBottomWidth, domColorToRGBA(s.BorderBottomColor))
	}
	// Left border
	if s.BorderLeftWidth > 0 && s.BorderLeftStyle != "none" && s.BorderLeftStyle != "" {
		fillRectF(p.img, x, y, s.BorderLeftWidth, h, domColorToRGBA(s.BorderLeftColor))
	}
	// Right border
	if s.BorderRightWidth > 0 && s.BorderRightStyle != "none" && s.BorderRightStyle != "" {
		fillRectF(p.img, x+w-s.BorderRightWidth, y, s.BorderRightWidth, h, domColorToRGBA(s.BorderRightColor))
	}
}

func (p *Painter) paintText(b *layout.Box, s *dom.ComputedStyle, x, y float64) {
	if b.Node == nil {
		return
	}
	text := b.Node.Data
	if s == nil {
		s = dom.DefaultComputedStyle()
	}
	p.paintTextAt(text, s, x, y)
}

func (p *Painter) paintTextAt(text string, s *dom.ComputedStyle, x, y float64) {
	if text == "" || s == nil {
		return
	}

	// Apply text transform
	switch s.TextTransform {
	case "uppercase":
		text = strings.ToUpper(text)
	case "lowercase":
		text = strings.ToLower(text)
	case "capitalize":
		text = capitalizeWords(text)
	}

	textColor := domColorToRGBA(s.Color)
	face := basicfont.Face7x13

	// Draw text using basicfont
	// basicfont is 7px wide, 13px tall
	// We scale based on fontSize
	scaleX := s.FontSize / 13.0
	scaleY := s.FontSize / 13.0
	_ = scaleY

	// Draw each character
	cx := x
	cy := y + s.FontSize // baseline offset

	for _, r := range text {
		// basicfont is fixed 7x13
		charWidth := float64(face.Advance) * scaleX

		// Draw the character glyph scaled
		dr := &font.Drawer{
			Dst:  p.img,
			Src:  image.NewUniform(textColor),
			Face: face,
			Dot:  fixed.P(int(cx), int(cy)),
		}
		dr.DrawString(string(r))
		cx += charWidth
	}

	// Text decoration
	lineY := cy
	switch s.TextDecoration {
	case "underline":
		fillRectF(p.img, x, lineY+1, cx-x, 1, textColor)
	case "line-through":
		fillRectF(p.img, x, y+s.FontSize/2, cx-x, 1, textColor)
	case "overline":
		fillRectF(p.img, x, y, cx-x, 1, textColor)
	}
}

func (p *Painter) paintListMarker(s *dom.ComputedStyle, x, y, h float64) {
	cy := y + h/2
	cx := x - 15
	c := domColorToRGBA(s.Color)

	switch s.ListStyleType {
	case "disc":
		drawFilledCircle(p.img, cx, cy, 3, c)
	case "circle":
		drawCircleOutline(p.img, cx, cy, 3, c)
	case "square":
		fillRectF(p.img, cx-3, cy-3, 6, 6, c)
	}
}

// ---- Drawing Primitives ----

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	bounds := img.Bounds()
	x0 := clampInt(x, bounds.Min.X, bounds.Max.X)
	y0 := clampInt(y, bounds.Min.Y, bounds.Max.Y)
	x1 := clampInt(x+w, bounds.Min.X, bounds.Max.X)
	y1 := clampInt(y+h, bounds.Min.Y, bounds.Max.Y)
	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			img.SetRGBA(px, py, blendOver(img.RGBAAt(px, py), c))
		}
	}
}

func fillRectF(img *image.RGBA, x, y, w, h float64, c color.RGBA) {
	fillRect(img, int(math.Round(x)), int(math.Round(y)), int(math.Round(w)), int(math.Round(h)), c)
}

func strokeRectF(img *image.RGBA, x, y, w, h, lineWidth float64, c color.RGBA) {
	lw := int(math.Round(lineWidth))
	if lw < 1 {
		lw = 1
	}
	fillRectF(img, x, y, w, float64(lw), c)           // top
	fillRectF(img, x, y+h-float64(lw), w, float64(lw), c) // bottom
	fillRectF(img, x, y, float64(lw), h, c)            // left
	fillRectF(img, x+w-float64(lw), y, float64(lw), h, c) // right
}

func drawFilledCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	bounds := img.Bounds()
	for py := int(cy - r - 1); py <= int(cy+r+1); py++ {
		for px := int(cx - r - 1); px <= int(cx+r+1); px++ {
			if !image.Pt(px, py).In(bounds) {
				continue
			}
			dx := float64(px) - cx
			dy := float64(py) - cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(px, py, blendOver(img.RGBAAt(px, py), c))
			}
		}
	}
}

func drawCircleOutline(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	bounds := img.Bounds()
	for py := int(cy - r - 2); py <= int(cy+r+2); py++ {
		for px := int(cx - r - 2); px <= int(cx+r+2); px++ {
			if !image.Pt(px, py).In(bounds) {
				continue
			}
			dx := float64(px) - cx
			dy := float64(py) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist >= r-0.5 && dist <= r+0.5 {
				img.SetRGBA(px, py, blendOver(img.RGBAAt(px, py), c))
			}
		}
	}
}

// blendOver alpha-composites src over dst.
func blendOver(dst, src color.RGBA) color.RGBA {
	if src.A == 255 {
		return src
	}
	if src.A == 0 {
		return dst
	}
	a := uint32(src.A)
	ia := 255 - a
	r := (uint32(src.R)*a + uint32(dst.R)*ia) / 255
	g := (uint32(src.G)*a + uint32(dst.G)*ia) / 255
	b := (uint32(src.B)*a + uint32(dst.B)*ia) / 255
	na := uint32(src.A) + uint32(dst.A)*(255-uint32(src.A))/255
	if na > 255 {
		na = 255
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(na)}
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func domColorToRGBA(c dom.Color) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

func getStyle(b *layout.Box) *dom.ComputedStyle {
	if b.Node == nil {
		return dom.DefaultComputedStyle()
	}
	if b.Node.ComputedStyle == nil {
		return dom.DefaultComputedStyle()
	}
	return b.Node.ComputedStyle
}

func getInlineStyle(b *layout.Box) *dom.ComputedStyle {
	if b.Node != nil && b.Node.Parent != nil && b.Node.Parent.ComputedStyle != nil {
		return b.Node.Parent.ComputedStyle
	}
	return getStyle(b)
}

func capitalizeWords(s string) string {
	var result strings.Builder
	capNext := true
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == ' ' || r == '\t' || r == '\n' {
			capNext = true
			result.WriteRune(r)
		} else if capNext {
			result.WriteString(strings.ToUpper(string(r)))
			capNext = false
		} else {
			result.WriteRune(r)
		}
		i += size
	}
	return result.String()
}

// MeasureText measures the rendered width of a text string with given style.
func MeasureText(text string, s *dom.ComputedStyle) float64 {
	if s == nil {
		s = dom.DefaultComputedStyle()
	}
	face := basicfont.Face7x13
	scaleX := s.FontSize / 13.0
	return float64(len([]rune(text))) * float64(face.Advance) * scaleX
}
