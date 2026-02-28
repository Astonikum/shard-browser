package layout

import (
	"strings"

	"github.com/shard-browser/shard/internal/webmatter/dom"
)

// Viewport is the dimensions of the browser viewport.
type Viewport struct {
	Width  float64
	Height float64
}

// Engine is the layout engine.
type Engine struct {
	viewport Viewport
}

// NewEngine creates a new layout engine.
func NewEngine(vp Viewport) *Engine {
	return &Engine{viewport: vp}
}

// Layout computes layout for the document and returns the root box.
func (e *Engine) Layout(doc *dom.Document) *Box {
	root := &doc.Node
	if doc.Root != nil {
		root = doc.Root
	}

	rootBox := e.buildBox(root)
	if rootBox == nil {
		return nil
	}

	// The initial containing block
	rootBox.Width = e.viewport.Width
	rootBox.X = 0
	rootBox.Y = 0

	e.layoutBlock(rootBox, e.viewport.Width)

	return rootBox
}

// buildBox constructs a Box tree from the DOM, respecting display types.
func (e *Engine) buildBox(node *dom.Node) *Box {
	if node.Type == dom.NodeTypeText {
		if strings.TrimSpace(node.Data) == "" {
			return nil
		}
		b := &Box{Node: node, BType: BoxInline, IsAnon: true}
		return b
	}

	if node.Type != dom.NodeTypeElement && node.Type != dom.NodeTypeDocument {
		return nil
	}

	s := node.ComputedStyle
	if s == nil {
		s = dom.DefaultComputedStyle()
	}

	btype := displayToBoxType(s.Display)
	if btype == BoxNone {
		return nil
	}

	b := newBox(node)
	b.BType = btype

	// Build children
	for _, child := range node.Children {
		childBox := e.buildBox(child)
		if childBox == nil {
			continue
		}
		b.Children = append(b.Children, childBox)
	}

	// Normalize: if a block box contains mixed inline and block children,
	// wrap anonymous block boxes around inline sequences
	if btype == BoxBlock || btype == BoxListItem {
		b.Children = normalizeChildren(b.Children)
	}

	return b
}

func displayToBoxType(display string) BoxType {
	switch display {
	case "block":
		return BoxBlock
	case "inline":
		return BoxInline
	case "inline-block":
		return BoxInlineBlock
	case "flex", "inline-flex":
		return BoxFlex
	case "list-item":
		return BoxListItem
	case "table":
		return BoxTable
	case "table-row":
		return BoxTableRow
	case "table-cell":
		return BoxTableCell
	case "none":
		return BoxNone
	default:
		return BoxInline
	}
}

// normalizeChildren wraps runs of inline children in anonymous block boxes
// when a block container has mixed inline/block content.
func normalizeChildren(children []*Box) []*Box {
	hasBlock := false
	for _, c := range children {
		if isBlockLevel(c) {
			hasBlock = true
			break
		}
	}
	if !hasBlock {
		return children
	}

	var result []*Box
	var inlineRun []*Box

	flushInline := func() {
		if len(inlineRun) > 0 {
			anon := &Box{BType: BoxBlock, IsAnon: true, Children: inlineRun}
			result = append(result, anon)
			inlineRun = nil
		}
	}

	for _, c := range children {
		if isBlockLevel(c) {
			flushInline()
			result = append(result, c)
		} else {
			inlineRun = append(inlineRun, c)
		}
	}
	flushInline()
	return result
}

func isBlockLevel(b *Box) bool {
	switch b.BType {
	case BoxBlock, BoxListItem, BoxFlex, BoxTable, BoxAnonymousBlock:
		return true
	}
	return false
}

// ---- BLOCK LAYOUT ----

// layoutBlock lays out a block-level box within the given available width.
func (e *Engine) layoutBlock(b *Box, availWidth float64) {
	s := getStyle(b)

	// Apply padding
	b.PaddingTop = resolveLength(s.PaddingTop, availWidth, s.FontSize)
	b.PaddingRight = resolveLength(s.PaddingRight, availWidth, s.FontSize)
	b.PaddingBottom = resolveLength(s.PaddingBottom, availWidth, s.FontSize)
	b.PaddingLeft = resolveLength(s.PaddingLeft, availWidth, s.FontSize)

	// Apply margin
	b.MarginTop = resolveLength(s.MarginTop, availWidth, s.FontSize)
	b.MarginRight = resolveLength(s.MarginRight, availWidth, s.FontSize)
	b.MarginBottom = resolveLength(s.MarginBottom, availWidth, s.FontSize)
	b.MarginLeft = resolveLength(s.MarginLeft, availWidth, s.FontSize)

	// Width
	contentWidth := availWidth - b.MarginLeft - b.MarginRight - b.BorderLeft - b.BorderRight - b.PaddingLeft - b.PaddingRight
	if !s.Width.IsAuto() {
		w := resolveLength(s.Width, availWidth, s.FontSize)
		if s.BoxSizing == "border-box" {
			w = w - b.PaddingLeft - b.PaddingRight - b.BorderLeft - b.BorderRight
		}
		contentWidth = w
	}
	if contentWidth < 0 {
		contentWidth = 0
	}
	// Apply min/max width
	minW := resolveLength(s.MinWidth, availWidth, s.FontSize)
	if contentWidth < minW {
		contentWidth = minW
	}
	if !s.MaxWidth.IsNone() {
		maxW := resolveLength(s.MaxWidth, availWidth, s.FontSize)
		if contentWidth > maxW {
			contentWidth = maxW
		}
	}
	b.Width = contentWidth

	// Layout children
	switch b.BType {
	case BoxFlex:
		e.layoutFlex(b, contentWidth)
	default:
		e.layoutBlockChildren(b, contentWidth)
	}

	// Height
	if !s.Height.IsAuto() {
		h := resolveLength(s.Height, 0, s.FontSize)
		if s.BoxSizing == "border-box" {
			h = h - b.PaddingTop - b.PaddingBottom - b.BorderTop - b.BorderBottom
		}
		b.Height = h
	}
	// Apply min/max height
	if !s.MinHeight.IsNone() {
		minH := resolveLength(s.MinHeight, 0, s.FontSize)
		if b.Height < minH {
			b.Height = minH
		}
	}
	if !s.MaxHeight.IsNone() {
		maxH := resolveLength(s.MaxHeight, 0, s.FontSize)
		if b.Height > maxH {
			b.Height = maxH
		}
	}
}

// layoutBlockChildren lays out children in block flow.
func (e *Engine) layoutBlockChildren(b *Box, contentWidth float64) {
	curY := 0.0

	for _, child := range b.Children {
		childStyle := getStyle(child)

		// Margin collapsing (simplified: just add margin)
		childMarginTop := resolveLength(childStyle.MarginTop, contentWidth, childStyle.FontSize)

		child.X = 0
		child.Y = curY + childMarginTop

		if isBlockLevel(child) {
			e.layoutBlock(child, contentWidth)
		} else if child.BType == BoxInlineBlock {
			e.layoutInlineBlock(child, contentWidth)
		} else {
			e.layoutInlineContainer(child, contentWidth)
		}

		childMarginBottom := resolveLength(childStyle.MarginBottom, contentWidth, childStyle.FontSize)
		curY = child.Y + child.BorderBoxHeight() + childMarginBottom
	}

	if b.Node == nil || getStyle(b).Height.IsAuto() {
		b.Height = curY
	}
}

// layoutInlineBlock lays out an inline-block.
func (e *Engine) layoutInlineBlock(b *Box, availWidth float64) {
	s := getStyle(b)
	b.PaddingTop = resolveLength(s.PaddingTop, availWidth, s.FontSize)
	b.PaddingRight = resolveLength(s.PaddingRight, availWidth, s.FontSize)
	b.PaddingBottom = resolveLength(s.PaddingBottom, availWidth, s.FontSize)
	b.PaddingLeft = resolveLength(s.PaddingLeft, availWidth, s.FontSize)
	b.MarginTop = resolveLength(s.MarginTop, availWidth, s.FontSize)
	b.MarginRight = resolveLength(s.MarginRight, availWidth, s.FontSize)
	b.MarginBottom = resolveLength(s.MarginBottom, availWidth, s.FontSize)
	b.MarginLeft = resolveLength(s.MarginLeft, availWidth, s.FontSize)

	var contentWidth float64
	if !s.Width.IsAuto() {
		contentWidth = resolveLength(s.Width, availWidth, s.FontSize)
		if s.BoxSizing == "border-box" {
			contentWidth = contentWidth - b.PaddingLeft - b.PaddingRight - b.BorderLeft - b.BorderRight
		}
	} else {
		contentWidth = availWidth - b.MarginLeft - b.MarginRight - b.BorderLeft - b.BorderRight - b.PaddingLeft - b.PaddingRight
	}
	if contentWidth < 0 {
		contentWidth = 0
	}
	b.Width = contentWidth

	e.layoutBlockChildren(b, contentWidth)

	if !s.Height.IsAuto() {
		b.Height = resolveLength(s.Height, 0, s.FontSize)
	}
}

// layoutInlineContainer lays out inline-level children in line boxes.
func (e *Engine) layoutInlineContainer(b *Box, contentWidth float64) {
	s := getStyle(b)
	b.PaddingTop = resolveLength(s.PaddingTop, contentWidth, s.FontSize)
	b.PaddingRight = resolveLength(s.PaddingRight, contentWidth, s.FontSize)
	b.PaddingBottom = resolveLength(s.PaddingBottom, contentWidth, s.FontSize)
	b.PaddingLeft = resolveLength(s.PaddingLeft, contentWidth, s.FontSize)
	b.MarginTop = resolveLength(s.MarginTop, contentWidth, s.FontSize)
	b.MarginRight = resolveLength(s.MarginRight, contentWidth, s.FontSize)
	b.MarginBottom = resolveLength(s.MarginBottom, contentWidth, s.FontSize)
	b.MarginLeft = resolveLength(s.MarginLeft, contentWidth, s.FontSize)
	b.Width = contentWidth

	b.Height = e.layoutLines(b, contentWidth)
}

// layoutLines lays out inline content into line boxes and returns total height.
func (e *Engine) layoutLines(b *Box, maxWidth float64) float64 {
	lineHeight := getLineHeight(b)
	curY := 0.0

	var currentLine []*Box
	var lineWidth float64

	flushLine := func() {
		if len(currentLine) == 0 {
			return
		}
		x := 0.0
		for _, lb := range currentLine {
			lb.X = x
			lb.Y = curY
			x += lb.Width
		}
		b.Lines = append(b.Lines, LineBox{
			Boxes:  currentLine,
			Width:  lineWidth,
			Height: lineHeight,
		})
		curY += lineHeight
		currentLine = nil
		lineWidth = 0
	}

	for _, child := range b.Children {
		if child.Node != nil && child.Node.Type == dom.NodeTypeText {
			words := breakIntoWords(child.Node.Data)
			charWidth := getCharWidth(b)
			for _, word := range words {
				wordWidth := float64(len([]rune(word))) * charWidth
				if lineWidth+wordWidth > maxWidth && lineWidth > 0 {
					flushLine()
				}
				wbox := &Box{
					Node:   child.Node,
					BType:  BoxInline,
					Width:  wordWidth,
					Height: lineHeight,
				}
				currentLine = append(currentLine, wbox)
				lineWidth += wordWidth
			}
		} else if child.BType == BoxInlineBlock {
			e.layoutInlineBlock(child, maxWidth)
			if lineWidth+child.MarginBoxWidth() > maxWidth && lineWidth > 0 {
				flushLine()
			}
			currentLine = append(currentLine, child)
			lineWidth += child.MarginBoxWidth()
		} else {
			// Inline element: recurse into children
			for _, grandChild := range child.Children {
				b.Children = append(b.Children, grandChild)
			}
		}
	}
	flushLine()

	return curY
}

// ---- FLEX LAYOUT ----

func (e *Engine) layoutFlex(b *Box, contentWidth float64) {
	s := getStyle(b)
	b.PaddingTop = resolveLength(s.PaddingTop, contentWidth, s.FontSize)
	b.PaddingRight = resolveLength(s.PaddingRight, contentWidth, s.FontSize)
	b.PaddingBottom = resolveLength(s.PaddingBottom, contentWidth, s.FontSize)
	b.PaddingLeft = resolveLength(s.PaddingLeft, contentWidth, s.FontSize)

	isRow := s.FlexDirection == "row" || s.FlexDirection == "row-reverse" || s.FlexDirection == ""

	gap := s.Gap

	// First pass: layout all children with their intrinsic sizes
	var children []*Box
	for _, child := range b.Children {
		if child.BType != BoxNone {
			children = append(children, child)
		}
	}

	if len(children) == 0 {
		b.Height = 0
		return
	}

	// Calculate flex basis for each child
	type flexItem struct {
		box        *Box
		basis      float64
		grow       float64
		shrink     float64
		minContent float64
	}

	items := make([]flexItem, len(children))
	totalBasis := 0.0
	totalGrow := 0.0
	totalGaps := gap * float64(len(children)-1)

	for i, child := range children {
		cs := getStyle(child)
		var basis float64
		if !cs.FlexBasis.IsAuto() {
			basis = resolveLength(cs.FlexBasis, contentWidth, cs.FontSize)
		} else if !cs.Width.IsAuto() {
			basis = resolveLength(cs.Width, contentWidth, cs.FontSize)
		} else {
			basis = 0 // will be determined by content
		}
		items[i] = flexItem{
			box:    child,
			basis:  basis,
			grow:   cs.FlexGrow,
			shrink: cs.FlexShrink,
		}
		totalBasis += basis
		totalGrow += cs.FlexGrow
	}

	if isRow {
		// Distribute free space
		freeSpace := contentWidth - totalBasis - totalGaps
		curX := 0.0
		maxHeight := 0.0

		for i, item := range items {
			child := item.box
			cs := getStyle(child)

			extra := 0.0
			if totalGrow > 0 && freeSpace > 0 {
				extra = freeSpace * (item.grow / totalGrow)
			}
			childWidth := item.basis + extra

			e.layoutBlock(child, childWidth)
			child.Width = childWidth

			child.X = curX + child.MarginLeft
			child.Y = child.MarginTop
			curX += childWidth + child.MarginLeft + child.MarginRight

			if i < len(items)-1 {
				curX += gap
			}

			h := child.BorderBoxHeight() + cs.MarginTop + cs.MarginBottom + child.PaddingTop + child.PaddingBottom
			if h > maxHeight {
				maxHeight = child.Height + child.PaddingTop + child.PaddingBottom + child.BorderTop + child.BorderBottom
			}
			_ = h
			if child.Height > maxHeight {
				maxHeight = child.Height
			}
		}

		// Align items
		for _, item := range items {
			cs := getStyle(item.box)
			alignSelf := cs.AlignSelf
			if alignSelf == "auto" {
				alignSelf = s.AlignItems
			}
			switch alignSelf {
			case "stretch", "":
				item.box.Height = maxHeight - item.box.PaddingTop - item.box.PaddingBottom - item.box.BorderTop - item.box.BorderBottom
			case "flex-end":
				item.box.Y = maxHeight - item.box.BorderBoxHeight()
			case "center":
				item.box.Y = (maxHeight - item.box.BorderBoxHeight()) / 2
			}
		}

		if s.Height.IsAuto() {
			b.Height = maxHeight
		}
	} else {
		// Column layout
		curY := 0.0
		maxWidth := 0.0

		for i, item := range items {
			child := item.box
			cs := getStyle(child)
			_ = cs

			e.layoutBlock(child, contentWidth)

			child.X = child.MarginLeft
			child.Y = curY + child.MarginTop
			curY += child.BorderBoxHeight() + child.MarginTop + child.MarginBottom

			if i < len(items)-1 {
				curY += gap
			}

			if child.Width > maxWidth {
				maxWidth = child.Width
			}
		}

		if s.Height.IsAuto() {
			b.Height = curY
		}
		_ = maxWidth
	}
}

// ---- HELPERS ----

func getStyle(b *Box) *dom.ComputedStyle {
	if b.Node == nil || b.Node.ComputedStyle == nil {
		return dom.DefaultComputedStyle()
	}
	return b.Node.ComputedStyle
}

func resolveLength(v dom.Value, containerWidth float64, fontSize float64) float64 {
	switch v.Kind {
	case dom.ValueAuto:
		return 0
	case dom.ValueNone:
		return 0
	case dom.ValueLength:
		switch v.Unit {
		case "px", "":
			return v.Amount
		case "em":
			return v.Amount * fontSize
		case "rem":
			return v.Amount * 16
		case "pt":
			return v.Amount * (4.0 / 3.0)
		default:
			return v.Amount
		}
	case dom.ValuePercentage:
		return v.Amount / 100 * containerWidth
	}
	return 0
}

func getLineHeight(b *Box) float64 {
	s := getStyle(b)
	if s.LineHeight > 0 {
		return s.LineHeight
	}
	return s.FontSize * 1.4
}

func getCharWidth(b *Box) float64 {
	s := getStyle(b)
	// Approximate: average char width is ~0.6 * fontSize
	return s.FontSize * 0.6
}

func breakIntoWords(text string) []string {
	// Split text into words preserving spaces as part of next word
	var words []string
	var cur strings.Builder
	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if cur.Len() > 0 {
				words = append(words, cur.String()+" ")
				cur.Reset()
			}
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}
