// Package layout implements the CSS layout engine for WebMatter.
package layout

import (
	"github.com/shard-browser/shard/internal/webmatter/dom"
)

// BoxType describes how a box participates in layout.
type BoxType int

const (
	BoxBlock       BoxType = iota // block formatting context
	BoxInline                     // inline formatting context
	BoxInlineBlock                // inline-block
	BoxFlex                       // flex container
	BoxFlexItem                   // flex item
	BoxListItem                   // list item (block + marker)
	BoxTableRow
	BoxTableCell
	BoxTable
	BoxAnonymousBlock  // anonymous block box
	BoxAnonymousInline // anonymous inline box
	BoxNone            // display:none
)

// Box is a layout box corresponding to a DOM node.
type Box struct {
	Node    *dom.Node
	BType   BoxType
	IsAnon  bool // anonymous box (no corresponding element)

	// Positioning (relative to parent content area)
	X, Y float64

	// Content dimensions
	Width, Height float64

	// Margin edges (positive = outward)
	MarginTop, MarginRight, MarginBottom, MarginLeft float64

	// Border widths
	BorderTop, BorderRight, BorderBottom, BorderLeft float64

	// Padding
	PaddingTop, PaddingRight, PaddingBottom, PaddingLeft float64

	// Children layout boxes
	Children []*Box

	// Inline layout: baselines, line boxes
	Lines []LineBox

	// For positioned elements
	PositionedParent *Box

	// ScrollOffset for overflow:scroll/auto
	ScrollX, ScrollY float64
}

// LineBox holds a line of inline boxes.
type LineBox struct {
	Boxes  []*Box
	Width  float64
	Height float64
	BaseLine float64
}

// MarginBox returns the total width including margins.
func (b *Box) MarginBoxWidth() float64 {
	return b.MarginLeft + b.BorderLeft + b.PaddingLeft + b.Width +
		b.PaddingRight + b.BorderRight + b.MarginRight
}

// MarginBoxHeight returns the total height including margins.
func (b *Box) MarginBoxHeight() float64 {
	return b.MarginTop + b.BorderTop + b.PaddingTop + b.Height +
		b.PaddingBottom + b.BorderBottom + b.MarginBottom
}

// ContentX returns the left edge of the content area.
func (b *Box) ContentX() float64 {
	return b.X + b.MarginLeft + b.BorderLeft + b.PaddingLeft
}

// ContentY returns the top edge of the content area.
func (b *Box) ContentY() float64 {
	return b.Y + b.MarginTop + b.BorderTop + b.PaddingTop
}

// BorderX returns the left edge of the border area.
func (b *Box) BorderX() float64 {
	return b.X + b.MarginLeft
}

// BorderY returns the top edge of the border area.
func (b *Box) BorderY() float64 {
	return b.Y + b.MarginTop
}

// BorderWidth returns the width including padding and border.
func (b *Box) BorderBoxWidth() float64 {
	return b.BorderLeft + b.PaddingLeft + b.Width + b.PaddingRight + b.BorderRight
}

// BorderBoxHeight returns the height including padding and border.
func (b *Box) BorderBoxHeight() float64 {
	return b.BorderTop + b.PaddingTop + b.Height + b.PaddingBottom + b.BorderBottom
}

// HitTest returns the innermost box at (x,y) relative to this box's origin.
func (b *Box) HitTest(x, y float64) *Box {
	bx := b.BorderX()
	by := b.BorderY()
	bw := b.BorderBoxWidth()
	bh := b.BorderBoxHeight()

	if x < bx || x > bx+bw || y < by || y > by+bh {
		return nil
	}

	// Check children in reverse (top-most first)
	for i := len(b.Children) - 1; i >= 0; i-- {
		child := b.Children[i]
		if hit := child.HitTest(x-b.ContentX(), y-b.ContentY()); hit != nil {
			return hit
		}
	}

	return b
}

// newBox creates a Box for a DOM node with the given computed style.
func newBox(node *dom.Node) *Box {
	b := &Box{Node: node}
	if node == nil || node.ComputedStyle == nil {
		return b
	}
	s := node.ComputedStyle
	b.BorderTop = s.BorderTopWidth
	b.BorderRight = s.BorderRightWidth
	b.BorderBottom = s.BorderBottomWidth
	b.BorderLeft = s.BorderLeftWidth
	return b
}
