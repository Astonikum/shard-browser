package dom

// ComputedStyle holds all resolved CSS properties for a DOM node.
type ComputedStyle struct {
	// Display
	Display    string // block, inline, inline-block, flex, none, list-item, table, table-row, table-cell
	Visibility string // visible, hidden

	// Colors
	Color           Color
	BackgroundColor Color

	// Font
	FontSize   float64  // px
	FontWeight int      // 100-900
	FontStyle  string   // normal, italic, oblique
	FontFamily []string // ordered list of font families

	// Box model
	Width     Value
	Height    Value
	MinWidth  Value
	MaxWidth  Value
	MinHeight Value
	MaxHeight Value

	MarginTop    Value
	MarginRight  Value
	MarginBottom Value
	MarginLeft   Value

	PaddingTop    Value
	PaddingRight  Value
	PaddingBottom Value
	PaddingLeft   Value

	BorderTopWidth    float64
	BorderRightWidth  float64
	BorderBottomWidth float64
	BorderLeftWidth   float64

	BorderTopColor    Color
	BorderRightColor  Color
	BorderBottomColor Color
	BorderLeftColor   Color

	BorderTopStyle    string // none, solid, dashed, dotted, double, groove, ridge, inset, outset
	BorderRightStyle  string
	BorderBottomStyle string
	BorderLeftStyle   string

	BoxSizing string // content-box, border-box

	// Border radius
	BorderTopLeftRadius     float64 // px
	BorderTopRightRadius    float64
	BorderBottomRightRadius float64
	BorderBottomLeftRadius  float64

	// Text
	TextAlign      string  // left, right, center, justify
	TextDecoration string  // none, underline, overline, line-through
	TextTransform  string  // none, uppercase, lowercase, capitalize
	LineHeight     float64 // px (0 = auto = 1.2 * FontSize)
	LetterSpacing  float64 // px
	WhiteSpace     string  // normal, nowrap, pre, pre-wrap, pre-line
	WordWrap       string  // normal, break-word

	// Position
	Position string // static, relative, absolute, fixed, sticky
	Top      Value
	Right    Value
	Bottom   Value
	Left     Value
	ZIndex   int

	// Float & clear
	Float string // left, right, none
	Clear string // left, right, both, none

	// Overflow
	Overflow  string // visible, hidden, scroll, auto
	OverflowX string
	OverflowY string

	// Flex (container)
	FlexDirection  string // row, row-reverse, column, column-reverse
	FlexWrap       string // nowrap, wrap, wrap-reverse
	JustifyContent string // flex-start, flex-end, center, space-between, space-around, space-evenly
	AlignItems     string // stretch, flex-start, flex-end, center, baseline
	AlignContent   string
	Gap            float64 // px, simplified (row-gap = col-gap)

	// Flex (item)
	FlexGrow   float64
	FlexShrink float64
	FlexBasis  Value
	AlignSelf  string
	Order      int

	// List
	ListStyleType     string // disc, circle, square, decimal, none
	ListStylePosition string // inside, outside

	// Other
	Opacity float64
	Cursor  string
	Content string // for ::before/::after

	// Outline
	OutlineWidth float64
	OutlineColor Color
	OutlineStyle string
}

// Color is an RGBA color (0-255 per channel).
type Color struct {
	R, G, B uint8
	A        uint8 // 255 = fully opaque
}

// Transparent is a fully transparent color.
var Transparent = Color{0, 0, 0, 0}

// Black and White convenience colors.
var (
	Black = Color{0, 0, 0, 255}
	White = Color{255, 255, 255, 255}
)

// IsTransparent reports whether the color is fully transparent.
func (c Color) IsTransparent() bool { return c.A == 0 }

// ValueKind identifies the kind of a CSS value.
type ValueKind int

const (
	ValueAuto       ValueKind = iota // auto
	ValueLength                      // numeric + unit
	ValuePercentage                  // numeric + %
	ValueNone                        // none
	ValueInitial                     // initial
	ValueInherit                     // inherit
)

// Value represents a CSS dimension value (width, height, margin, etc.).
type Value struct {
	Kind   ValueKind
	Amount float64
	Unit   string // px, em, rem, vw, vh, %
}

// IsAuto reports whether the value is auto.
func (v Value) IsAuto() bool { return v.Kind == ValueAuto }

// IsNone reports whether the value is none.
func (v Value) IsNone() bool { return v.Kind == ValueNone }

// Px creates a pixel length value.
func Px(amount float64) Value {
	return Value{Kind: ValueLength, Amount: amount, Unit: "px"}
}

// Pct creates a percentage value.
func Pct(amount float64) Value {
	return Value{Kind: ValuePercentage, Amount: amount}
}

// Auto is the auto value.
var Auto = Value{Kind: ValueAuto}

// DefaultComputedStyle returns the browser default computed style.
func DefaultComputedStyle() *ComputedStyle {
	return &ComputedStyle{
		Display:    "inline",
		Visibility: "visible",
		Color:      Black,
		// Background transparent by default
		BackgroundColor: Transparent,
		FontSize:        16,
		FontWeight:      400,
		FontStyle:       "normal",
		FontFamily:      []string{"sans-serif"},
		TextAlign:       "left",
		TextDecoration:  "none",
		TextTransform:   "none",
		WhiteSpace:      "normal",
		WordWrap:        "normal",
		Position:        "static",
		Float:           "none",
		Clear:           "none",
		Overflow:        "visible",
		OverflowX:       "visible",
		OverflowY:       "visible",
		BoxSizing:       "content-box",
		Opacity:         1,
		Cursor:          "auto",
		ListStyleType:   "disc",
		ListStylePosition: "outside",
		FlexDirection:   "row",
		FlexWrap:        "nowrap",
		JustifyContent:  "flex-start",
		AlignItems:      "stretch",
		AlignSelf:       "auto",
		FlexGrow:        0,
		FlexShrink:      1,
		FlexBasis:       Auto,
		Width:           Auto,
		Height:          Auto,
		MinWidth:        Value{Kind: ValueLength, Amount: 0, Unit: "px"},
		MaxWidth:        Value{Kind: ValueNone},
		MinHeight:       Value{Kind: ValueLength, Amount: 0, Unit: "px"},
		MaxHeight:       Value{Kind: ValueNone},
	}
}

// Clone returns a deep copy of the computed style (for inheritance).
func (s *ComputedStyle) Clone() *ComputedStyle {
	if s == nil {
		return DefaultComputedStyle()
	}
	c := *s
	c.FontFamily = make([]string, len(s.FontFamily))
	copy(c.FontFamily, s.FontFamily)
	return &c
}

// InheritableProperties copies inheritable CSS properties from parent to child.
// This is called before applying the child's own rules.
func (child *ComputedStyle) InheritFrom(parent *ComputedStyle) {
	if parent == nil {
		return
	}
	// Inheritable properties per CSS spec:
	child.Color = parent.Color
	child.FontSize = parent.FontSize
	child.FontWeight = parent.FontWeight
	child.FontStyle = parent.FontStyle
	child.FontFamily = parent.FontFamily
	child.TextAlign = parent.TextAlign
	child.TextDecoration = parent.TextDecoration
	child.TextTransform = parent.TextTransform
	child.LineHeight = parent.LineHeight
	child.LetterSpacing = parent.LetterSpacing
	child.WhiteSpace = parent.WhiteSpace
	child.WordWrap = parent.WordWrap
	child.ListStyleType = parent.ListStyleType
	child.ListStylePosition = parent.ListStylePosition
	child.Visibility = parent.Visibility
	child.Cursor = parent.Cursor
}
