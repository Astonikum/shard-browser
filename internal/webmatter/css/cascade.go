package css

import (
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/shard-browser/shard/internal/webmatter/dom"
)

// Cascade applies CSS rules to the DOM tree and computes styles.
type Cascade struct {
	userAgentRules []Rule
	userRules      []Rule
}

// NewCascade creates a new style cascade.
func NewCascade() *Cascade {
	c := &Cascade{}
	ua := Parse(userAgentStylesheet)
	c.userAgentRules = ua.Rules
	return c
}

// Apply computes and assigns styles to every node in the DOM tree.
func (c *Cascade) Apply(doc *dom.Document, userStylesheets []string) {
	// Parse user stylesheets
	c.userRules = nil
	for _, css := range userStylesheets {
		ss := Parse(css)
		c.userRules = append(c.userRules, ss.Rules...)
	}

	// Process inline styles from HTML attributes
	// Walk and compute styles
	root := &doc.Node
	c.computeNode(root, nil, doc)
}

func (c *Cascade) computeNode(n *dom.Node, parent *dom.Node, doc *dom.Document) {
	style := dom.DefaultComputedStyle()

	// Inherit from parent
	if parent != nil && parent.ComputedStyle != nil {
		style.InheritFrom(parent.ComputedStyle)
	}

	// Apply UA rules
	if n.Type == dom.NodeTypeElement {
		style.Display = defaultDisplay(n.TagName)
	}

	// Collect matched declarations
	type matchedDecl struct {
		decl        Declaration
		specificity [3]int
		order       int
		important   bool
		origin      int // 0=UA, 1=user
	}

	var matched []matchedDecl
	order := 0

	applyRules := func(rules []Rule, origin int) {
		for _, rule := range rules {
			for _, sel := range rule.Selectors {
				if matchesSelector(n, sel) {
					for _, decl := range rule.Declarations {
						matched = append(matched, matchedDecl{
							decl:        decl,
							specificity: sel.Specificity,
							order:       order,
							important:   decl.Important,
							origin:      origin,
						})
						order++
					}
					break // only apply once per rule even with multiple selectors
				}
			}
			// Handle @media etc.
			for _, sub := range rule.SubRules {
				// TODO: check media condition
				for _, sel := range sub.Selectors {
					if matchesSelector(n, sel) {
						for _, decl := range sub.Declarations {
							matched = append(matched, matchedDecl{
								decl:        decl,
								specificity: sel.Specificity,
								order:       order,
								important:   decl.Important,
								origin:      origin,
							})
							order++
						}
						break
					}
				}
			}
		}
	}

	applyRules(c.userAgentRules, 0)
	applyRules(c.userRules, 1)

	// Apply inline style
	if n.Type == dom.NodeTypeElement {
		inlineStyle := n.GetAttr("style")
		if inlineStyle != "" {
			for _, decl := range ParseDeclarations(inlineStyle) {
				matched = append(matched, matchedDecl{
					decl:        decl,
					specificity: [3]int{1, 0, 0}, // inline has very high specificity
					order:       order,
					important:   decl.Important,
					origin:      2,
				})
				order++
			}
		}
	}

	// Sort: normal declarations by (origin, specificity, order)
	// Important declarations override normal
	// Simplified cascade: later wins for same specificity
	sortDeclarations(matched)

	// Apply declarations to style
	for _, m := range matched {
		applyDeclaration(style, m.decl, parent)
	}

	// Post-process
	if style.LineHeight == 0 {
		style.LineHeight = style.FontSize * 1.2
	}

	n.ComputedStyle = style

	// Recurse into children
	for _, child := range n.Children {
		c.computeNode(child, n, doc)
	}
}

func sortDeclarations(decls []matchedDecl) {
	// Stable sort: important > normal; higher origin > lower; higher specificity > lower; later order > earlier
	for i := 1; i < len(decls); i++ {
		for j := i; j > 0; j-- {
			a, b := decls[j-1], decls[j]
			if declLess(a, b) {
				decls[j-1], decls[j] = decls[j], decls[j-1]
			} else {
				break
			}
		}
	}
}

func declLess(a, b matchedDecl) bool {
	// important always beats normal
	if a.important != b.important {
		return !a.important // non-important comes before important
	}
	// higher origin wins
	if a.origin != b.origin {
		return a.origin < b.origin
	}
	// higher specificity wins
	for i := 0; i < 3; i++ {
		if a.specificity[i] != b.specificity[i] {
			return a.specificity[i] < b.specificity[i]
		}
	}
	// later order wins
	return a.order < b.order
}

// matchesSelector checks if a node matches a selector.
func matchesSelector(n *dom.Node, sel Selector) bool {
	if n.Type != dom.NodeTypeElement {
		return false
	}
	if len(sel.Parts) == 0 {
		return false
	}
	return matchSelectorParts(n, sel.Parts, len(sel.Parts)-1)
}

func matchSelectorParts(n *dom.Node, parts []SelectorPart, idx int) bool {
	if idx < 0 {
		return true
	}
	part := parts[idx]
	if !matchSimplePart(n, part) {
		return false
	}
	if idx == 0 {
		return true
	}
	prevPart := parts[idx-1]
	switch part.Combinator {
	case CombinatorNone, CombinatorDescendant:
		// Check ancestors
		anc := n.Parent
		for anc != nil {
			if matchSimplePart(anc, prevPart) {
				if matchSelectorParts(anc, parts, idx-1) {
					return true
				}
			}
			anc = anc.Parent
		}
		return false
	case CombinatorChild:
		if n.Parent == nil {
			return false
		}
		return matchSelectorParts(n.Parent, parts, idx-1)
	case CombinatorAdjacent:
		sibling := n.PrevSibling()
		if sibling == nil {
			return false
		}
		// Skip text nodes
		for sibling != nil && sibling.Type != dom.NodeTypeElement {
			sibling = sibling.PrevSibling()
		}
		if sibling == nil {
			return false
		}
		return matchSelectorParts(sibling, parts, idx-1)
	case CombinatorSibling:
		sibling := n.PrevSibling()
		for sibling != nil {
			if sibling.Type == dom.NodeTypeElement {
				if matchSelectorParts(sibling, parts, idx-1) {
					return true
				}
			}
			sibling = sibling.PrevSibling()
		}
		return false
	}
	return false
}

func matchSimplePart(n *dom.Node, part SelectorPart) bool {
	if n.Type != dom.NodeTypeElement {
		return false
	}
	if part.Tag != "" && part.Tag != "*" && n.TagName != part.Tag {
		return false
	}
	if part.ID != "" && n.ID() != part.ID {
		return false
	}
	for _, cls := range part.Classes {
		if !n.HasClass(cls) {
			return false
		}
	}
	for _, attr := range part.Attrs {
		if !matchAttr(n, attr) {
			return false
		}
	}
	for _, pseudo := range part.PseudoClass {
		if !matchPseudo(n, pseudo) {
			return false
		}
	}
	return true
}

func matchAttr(n *dom.Node, attr AttrSelector) bool {
	val := n.GetAttr(attr.Name)
	if attr.Op == "" {
		return n.HasAttr(attr.Name)
	}
	switch attr.Op {
	case "=":
		return val == attr.Value
	case "~=":
		for _, word := range strings.Fields(val) {
			if word == attr.Value {
				return true
			}
		}
		return false
	case "|=":
		return val == attr.Value || strings.HasPrefix(val, attr.Value+"-")
	case "^=":
		return strings.HasPrefix(val, attr.Value)
	case "$=":
		return strings.HasSuffix(val, attr.Value)
	case "*=":
		return strings.Contains(val, attr.Value)
	}
	return false
}

func matchPseudo(n *dom.Node, pseudo string) bool {
	switch pseudo {
	case "first-child":
		return isFirstChild(n)
	case "last-child":
		return isLastChild(n)
	case "first-of-type":
		return isFirstOfType(n)
	case "last-of-type":
		return isLastOfType(n)
	case "only-child":
		return isFirstChild(n) && isLastChild(n)
	case "only-of-type":
		return isFirstOfType(n) && isLastOfType(n)
	case "empty":
		return len(n.Children) == 0
	case "root":
		return n.Parent == nil || n.Parent.Type == dom.NodeTypeDocument
	case "link", "any-link":
		return n.TagName == "a" && n.HasAttr("href")
	case "hover", "focus", "active", "visited":
		return false // dynamic states not supported yet
	case "checked":
		return n.HasAttr("checked")
	case "disabled":
		return n.HasAttr("disabled")
	case "enabled":
		return !n.HasAttr("disabled")
	}
	return false
}

func isFirstChild(n *dom.Node) bool {
	if n.Parent == nil {
		return true
	}
	for _, child := range n.Parent.Children {
		if child.Type == dom.NodeTypeElement {
			return child == n
		}
	}
	return false
}

func isLastChild(n *dom.Node) bool {
	if n.Parent == nil {
		return true
	}
	for i := len(n.Parent.Children) - 1; i >= 0; i-- {
		child := n.Parent.Children[i]
		if child.Type == dom.NodeTypeElement {
			return child == n
		}
	}
	return false
}

func isFirstOfType(n *dom.Node) bool {
	if n.Parent == nil {
		return true
	}
	for _, child := range n.Parent.Children {
		if child.Type == dom.NodeTypeElement && child.TagName == n.TagName {
			return child == n
		}
	}
	return false
}

func isLastOfType(n *dom.Node) bool {
	if n.Parent == nil {
		return true
	}
	for i := len(n.Parent.Children) - 1; i >= 0; i-- {
		child := n.Parent.Children[i]
		if child.Type == dom.NodeTypeElement && child.TagName == n.TagName {
			return child == n
		}
	}
	return false
}

// applyDeclaration applies a single CSS declaration to a computed style.
func applyDeclaration(s *dom.ComputedStyle, d Declaration, parent *dom.Node) {
	v := strings.TrimSpace(d.Value)
	if v == "" {
		return
	}

	prop := d.Property

	// Handle shorthand properties first
	switch prop {
	case "margin":
		vals := parseBoxShorthand(v, s.FontSize)
		s.MarginTop = vals[0]
		s.MarginRight = vals[1]
		s.MarginBottom = vals[2]
		s.MarginLeft = vals[3]
		return
	case "padding":
		vals := parseBoxShorthand(v, s.FontSize)
		s.PaddingTop = vals[0]
		s.PaddingRight = vals[1]
		s.PaddingBottom = vals[2]
		s.PaddingLeft = vals[3]
		return
	case "border":
		parseBorderShorthand(s, v)
		return
	case "border-top":
		parseBorderSideShorthand(s, v, "top")
		return
	case "border-right":
		parseBorderSideShorthand(s, v, "right")
		return
	case "border-bottom":
		parseBorderSideShorthand(s, v, "bottom")
		return
	case "border-left":
		parseBorderSideShorthand(s, v, "left")
		return
	case "border-width":
		vals := parseBoxShorthand(v, s.FontSize)
		s.BorderTopWidth = vals[0].Amount
		s.BorderRightWidth = vals[1].Amount
		s.BorderBottomWidth = vals[2].Amount
		s.BorderLeftWidth = vals[3].Amount
		return
	case "border-style":
		parts := strings.Fields(v)
		sides := expandBoxShorthandStrings(parts)
		s.BorderTopStyle = sides[0]
		s.BorderRightStyle = sides[1]
		s.BorderBottomStyle = sides[2]
		s.BorderLeftStyle = sides[3]
		return
	case "border-color":
		parts := splitByCommaRespectingParens(v)
		colors := expandBoxShorthandColors(parts)
		s.BorderTopColor = colors[0]
		s.BorderRightColor = colors[1]
		s.BorderBottomColor = colors[2]
		s.BorderLeftColor = colors[3]
		return
	case "border-radius":
		vals := parseBoxShorthand(v, s.FontSize)
		s.BorderTopLeftRadius = vals[0].Amount
		s.BorderTopRightRadius = vals[1].Amount
		s.BorderBottomRightRadius = vals[2].Amount
		s.BorderBottomLeftRadius = vals[3].Amount
		return
	case "background":
		parseBackgroundShorthand(s, v)
		return
	case "font":
		parseFontShorthand(s, v)
		return
	case "flex":
		parseFlexShorthand(s, v)
		return
	case "list-style":
		parts := strings.Fields(v)
		for _, p := range parts {
			switch p {
			case "none":
				s.ListStyleType = "none"
			case "disc", "circle", "square", "decimal",
				"lower-roman", "upper-roman", "lower-alpha", "upper-alpha":
				s.ListStyleType = p
			case "inside", "outside":
				s.ListStylePosition = p
			}
		}
		return
	case "outline":
		// simplified
		s.OutlineStyle = "solid"
		return
	case "transition", "animation", "transform":
		// Not yet implemented
		return
	}

	// Individual properties
	switch prop {
	case "display":
		if v != "inherit" {
			s.Display = v
		}
	case "visibility":
		s.Visibility = v
	case "color":
		if c, ok := parseColor(v); ok {
			s.Color = c
		}
	case "background-color":
		if c, ok := parseColor(v); ok {
			s.BackgroundColor = c
		}
	case "opacity":
		var f float64
		parseFloat(v, &f)
		s.Opacity = math.Max(0, math.Min(1, f))
	case "font-size":
		s.FontSize = parseFontSize(v, s.FontSize, parentFontSize(parent))
	case "font-weight":
		s.FontWeight = parseFontWeight(v, s.FontWeight)
	case "font-style":
		s.FontStyle = v
	case "font-family":
		s.FontFamily = parseFontFamily(v)
	case "text-align":
		s.TextAlign = v
	case "text-decoration":
		s.TextDecoration = v
	case "text-transform":
		s.TextTransform = v
	case "line-height":
		if v == "normal" {
			s.LineHeight = 0
		} else if strings.HasSuffix(v, "px") {
			var f float64
			parseFloat(v[:len(v)-2], &f)
			s.LineHeight = f
		} else {
			var f float64
			parseFloat(v, &f)
			if f > 0 {
				s.LineHeight = f * s.FontSize
			}
		}
	case "letter-spacing":
		s.LetterSpacing = parseLength(v, s.FontSize)
	case "white-space":
		s.WhiteSpace = v
	case "word-wrap", "overflow-wrap":
		s.WordWrap = v
	case "width":
		s.Width = parseDimension(v, s.FontSize)
	case "height":
		s.Height = parseDimension(v, s.FontSize)
	case "min-width":
		s.MinWidth = parseDimension(v, s.FontSize)
	case "max-width":
		s.MaxWidth = parseDimension(v, s.FontSize)
	case "min-height":
		s.MinHeight = parseDimension(v, s.FontSize)
	case "max-height":
		s.MaxHeight = parseDimension(v, s.FontSize)
	case "margin-top":
		s.MarginTop = parseDimension(v, s.FontSize)
	case "margin-right":
		s.MarginRight = parseDimension(v, s.FontSize)
	case "margin-bottom":
		s.MarginBottom = parseDimension(v, s.FontSize)
	case "margin-left":
		s.MarginLeft = parseDimension(v, s.FontSize)
	case "padding-top":
		s.PaddingTop = parseDimension(v, s.FontSize)
	case "padding-right":
		s.PaddingRight = parseDimension(v, s.FontSize)
	case "padding-bottom":
		s.PaddingBottom = parseDimension(v, s.FontSize)
	case "padding-left":
		s.PaddingLeft = parseDimension(v, s.FontSize)
	case "border-top-width":
		s.BorderTopWidth = parseLength(v, s.FontSize)
	case "border-right-width":
		s.BorderRightWidth = parseLength(v, s.FontSize)
	case "border-bottom-width":
		s.BorderBottomWidth = parseLength(v, s.FontSize)
	case "border-left-width":
		s.BorderLeftWidth = parseLength(v, s.FontSize)
	case "border-top-color":
		if c, ok := parseColor(v); ok {
			s.BorderTopColor = c
		}
	case "border-right-color":
		if c, ok := parseColor(v); ok {
			s.BorderRightColor = c
		}
	case "border-bottom-color":
		if c, ok := parseColor(v); ok {
			s.BorderBottomColor = c
		}
	case "border-left-color":
		if c, ok := parseColor(v); ok {
			s.BorderLeftColor = c
		}
	case "border-top-style":
		s.BorderTopStyle = v
	case "border-right-style":
		s.BorderRightStyle = v
	case "border-bottom-style":
		s.BorderBottomStyle = v
	case "border-left-style":
		s.BorderLeftStyle = v
	case "border-top-left-radius":
		s.BorderTopLeftRadius = parseLength(v, s.FontSize)
	case "border-top-right-radius":
		s.BorderTopRightRadius = parseLength(v, s.FontSize)
	case "border-bottom-right-radius":
		s.BorderBottomRightRadius = parseLength(v, s.FontSize)
	case "border-bottom-left-radius":
		s.BorderBottomLeftRadius = parseLength(v, s.FontSize)
	case "box-sizing":
		s.BoxSizing = v
	case "position":
		s.Position = v
	case "top":
		s.Top = parseDimension(v, s.FontSize)
	case "right":
		s.Right = parseDimension(v, s.FontSize)
	case "bottom":
		s.Bottom = parseDimension(v, s.FontSize)
	case "left":
		s.Left = parseDimension(v, s.FontSize)
	case "z-index":
		var f float64
		parseFloat(v, &f)
		s.ZIndex = int(f)
	case "float":
		s.Float = v
	case "clear":
		s.Clear = v
	case "overflow":
		s.Overflow = v
		s.OverflowX = v
		s.OverflowY = v
	case "overflow-x":
		s.OverflowX = v
	case "overflow-y":
		s.OverflowY = v
	case "flex-direction":
		s.FlexDirection = v
	case "flex-wrap":
		s.FlexWrap = v
	case "justify-content":
		s.JustifyContent = v
	case "align-items":
		s.AlignItems = v
	case "align-content":
		s.AlignContent = v
	case "align-self":
		s.AlignSelf = v
	case "flex-grow":
		var f float64
		parseFloat(v, &f)
		s.FlexGrow = f
	case "flex-shrink":
		var f float64
		parseFloat(v, &f)
		s.FlexShrink = f
	case "flex-basis":
		s.FlexBasis = parseDimension(v, s.FontSize)
	case "order":
		var f float64
		parseFloat(v, &f)
		s.Order = int(f)
	case "gap", "grid-gap":
		s.Gap = parseLength(v, s.FontSize)
	case "list-style-type":
		s.ListStyleType = v
	case "list-style-position":
		s.ListStylePosition = v
	case "cursor":
		s.Cursor = v
	case "content":
		s.Content = v
	}
}

// --- Value Parsing ---

func parseDimension(v string, fontSize float64) dom.Value {
	switch strings.TrimSpace(v) {
	case "auto":
		return dom.Auto
	case "none":
		return dom.Value{Kind: dom.ValueNone}
	case "inherit":
		return dom.Auto
	case "0":
		return dom.Px(0)
	}
	return dom.Px(parseLength(v, fontSize))
}

func parseLength(v string, fontSize float64) float64 {
	v = strings.TrimSpace(v)
	if v == "0" {
		return 0
	}
	if strings.HasSuffix(v, "px") {
		var f float64
		parseFloat(v[:len(v)-2], &f)
		return f
	}
	if strings.HasSuffix(v, "em") {
		var f float64
		parseFloat(v[:len(v)-2], &f)
		return f * fontSize
	}
	if strings.HasSuffix(v, "rem") {
		var f float64
		parseFloat(v[:len(v)-3], &f)
		return f * 16 // root em = 16px default
	}
	if strings.HasSuffix(v, "pt") {
		var f float64
		parseFloat(v[:len(v)-2], &f)
		return f * (4.0 / 3.0) // 1pt = 4/3px
	}
	if strings.HasSuffix(v, "vw") {
		var f float64
		parseFloat(v[:len(v)-2], &f)
		return f * 12 // assume 1200px viewport
	}
	if strings.HasSuffix(v, "vh") {
		var f float64
		parseFloat(v[:len(v)-2], &f)
		return f * 8 // assume 800px viewport
	}
	if strings.HasSuffix(v, "%") {
		// percentage needs parent context; return raw for now
		var f float64
		parseFloat(v[:len(v)-1], &f)
		return f // caller must handle %
	}
	// Try parsing as a plain number
	var f float64
	parseFloat(v, &f)
	return f
}

func parseFontSize(v string, current, parentSize float64) float64 {
	switch v {
	case "xx-small":
		return 9
	case "x-small":
		return 10
	case "small":
		return 13
	case "medium":
		return 16
	case "large":
		return 18
	case "x-large":
		return 24
	case "xx-large":
		return 32
	case "smaller":
		return current * 0.833
	case "larger":
		return current * 1.2
	case "inherit":
		return parentSize
	}
	if strings.HasSuffix(v, "%") {
		var f float64
		parseFloat(v[:len(v)-1], &f)
		return parentSize * f / 100
	}
	if strings.HasSuffix(v, "em") {
		var f float64
		parseFloat(v[:len(v)-2], &f)
		return parentSize * f
	}
	return parseLength(v, current)
}

func parseFontWeight(v string, current int) int {
	switch v {
	case "normal":
		return 400
	case "bold":
		return 700
	case "bolder":
		if current < 400 {
			return 400
		} else if current < 700 {
			return 700
		}
		return 900
	case "lighter":
		if current > 700 {
			return 700
		} else if current > 400 {
			return 400
		}
		return 100
	case "inherit":
		return current
	}
	n, err := strconv.Atoi(v)
	if err == nil {
		return n
	}
	return current
}

func parseFontFamily(v string) []string {
	parts := splitByCommaRespectingParens(v)
	var families []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"'")
		if p != "" {
			families = append(families, p)
		}
	}
	return families
}

func parseBoxShorthand(v string, fontSize float64) [4]dom.Value {
	parts := strings.Fields(v)
	var vals [4]dom.Value
	switch len(parts) {
	case 1:
		d := parseDimension(parts[0], fontSize)
		vals = [4]dom.Value{d, d, d, d}
	case 2:
		tb := parseDimension(parts[0], fontSize)
		lr := parseDimension(parts[1], fontSize)
		vals = [4]dom.Value{tb, lr, tb, lr}
	case 3:
		t := parseDimension(parts[0], fontSize)
		lr := parseDimension(parts[1], fontSize)
		b := parseDimension(parts[2], fontSize)
		vals = [4]dom.Value{t, lr, b, lr}
	case 4:
		vals = [4]dom.Value{
			parseDimension(parts[0], fontSize),
			parseDimension(parts[1], fontSize),
			parseDimension(parts[2], fontSize),
			parseDimension(parts[3], fontSize),
		}
	}
	return vals
}

func expandBoxShorthandStrings(parts []string) [4]string {
	var vals [4]string
	switch len(parts) {
	case 1:
		vals = [4]string{parts[0], parts[0], parts[0], parts[0]}
	case 2:
		vals = [4]string{parts[0], parts[1], parts[0], parts[1]}
	case 3:
		vals = [4]string{parts[0], parts[1], parts[2], parts[1]}
	case 4:
		vals = [4]string{parts[0], parts[1], parts[2], parts[3]}
	}
	return vals
}

func expandBoxShorthandColors(parts []string) [4]dom.Color {
	var vals [4]dom.Color
	switch len(parts) {
	case 1:
		c, _ := parseColor(parts[0])
		vals = [4]dom.Color{c, c, c, c}
	case 2:
		c0, _ := parseColor(parts[0])
		c1, _ := parseColor(parts[1])
		vals = [4]dom.Color{c0, c1, c0, c1}
	case 3:
		c0, _ := parseColor(parts[0])
		c1, _ := parseColor(parts[1])
		c2, _ := parseColor(parts[2])
		vals = [4]dom.Color{c0, c1, c2, c1}
	case 4:
		c0, _ := parseColor(parts[0])
		c1, _ := parseColor(parts[1])
		c2, _ := parseColor(parts[2])
		c3, _ := parseColor(parts[3])
		vals = [4]dom.Color{c0, c1, c2, c3}
	}
	return vals
}

func parseBorderShorthand(s *dom.ComputedStyle, v string) {
	parts := strings.Fields(v)
	for _, p := range parts {
		if isLengthStr(p) {
			w := parseLength(p, s.FontSize)
			s.BorderTopWidth = w
			s.BorderRightWidth = w
			s.BorderBottomWidth = w
			s.BorderLeftWidth = w
		} else if isBorderStyle(p) {
			s.BorderTopStyle = p
			s.BorderRightStyle = p
			s.BorderBottomStyle = p
			s.BorderLeftStyle = p
		} else if c, ok := parseColor(p); ok {
			s.BorderTopColor = c
			s.BorderRightColor = c
			s.BorderBottomColor = c
			s.BorderLeftColor = c
		}
	}
}

func parseBorderSideShorthand(s *dom.ComputedStyle, v, side string) {
	parts := strings.Fields(v)
	for _, p := range parts {
		if isLengthStr(p) {
			w := parseLength(p, s.FontSize)
			switch side {
			case "top":
				s.BorderTopWidth = w
			case "right":
				s.BorderRightWidth = w
			case "bottom":
				s.BorderBottomWidth = w
			case "left":
				s.BorderLeftWidth = w
			}
		} else if isBorderStyle(p) {
			switch side {
			case "top":
				s.BorderTopStyle = p
			case "right":
				s.BorderRightStyle = p
			case "bottom":
				s.BorderBottomStyle = p
			case "left":
				s.BorderLeftStyle = p
			}
		} else if c, ok := parseColor(p); ok {
			switch side {
			case "top":
				s.BorderTopColor = c
			case "right":
				s.BorderRightColor = c
			case "bottom":
				s.BorderBottomColor = c
			case "left":
				s.BorderLeftColor = c
			}
		}
	}
}

func parseBackgroundShorthand(s *dom.ComputedStyle, v string) {
	if v == "none" || v == "transparent" {
		s.BackgroundColor = dom.Transparent
		return
	}
	// Try to find color
	parts := splitByCommaRespectingParens(v)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if c, ok := parseColor(p); ok {
			s.BackgroundColor = c
		}
	}
	if c, ok := parseColor(v); ok {
		s.BackgroundColor = c
	}
}

func parseFontShorthand(s *dom.ComputedStyle, v string) {
	// Simplified: font: style weight size/line-height family
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return
	}
	i := 0
	// optional style
	if parts[i] == "italic" || parts[i] == "oblique" {
		s.FontStyle = parts[i]
		i++
	}
	// optional weight
	if i < len(parts) {
		if w := parseFontWeight(parts[i], s.FontWeight); w != s.FontWeight {
			s.FontWeight = w
			i++
		} else if parts[i] == "bold" || parts[i] == "normal" || parts[i] == "bolder" || parts[i] == "lighter" {
			s.FontWeight = parseFontWeight(parts[i], s.FontWeight)
			i++
		}
	}
	// size/line-height
	if i < len(parts) {
		sizeStr := parts[i]
		if idx := strings.Index(sizeStr, "/"); idx != -1 {
			s.FontSize = parseFontSize(sizeStr[:idx], s.FontSize, 16)
			lh := sizeStr[idx+1:]
			var f float64
			parseFloat(lh, &f)
			if f > 0 {
				s.LineHeight = f * s.FontSize
			}
		} else {
			s.FontSize = parseFontSize(sizeStr, s.FontSize, 16)
		}
		i++
	}
	// family
	if i < len(parts) {
		familyStr := strings.Join(parts[i:], " ")
		s.FontFamily = parseFontFamily(familyStr)
	}
}

func parseFlexShorthand(s *dom.ComputedStyle, v string) {
	switch v {
	case "none":
		s.FlexGrow = 0
		s.FlexShrink = 0
		s.FlexBasis = dom.Auto
		return
	case "auto":
		s.FlexGrow = 1
		s.FlexShrink = 1
		s.FlexBasis = dom.Auto
		return
	}
	parts := strings.Fields(v)
	switch len(parts) {
	case 1:
		var f float64
		parseFloat(parts[0], &f)
		s.FlexGrow = f
		s.FlexShrink = 1
		s.FlexBasis = dom.Px(0)
	case 2:
		var grow, shrink float64
		parseFloat(parts[0], &grow)
		parseFloat(parts[1], &shrink)
		s.FlexGrow = grow
		s.FlexShrink = shrink
		s.FlexBasis = dom.Px(0)
	case 3:
		var grow, shrink float64
		parseFloat(parts[0], &grow)
		parseFloat(parts[1], &shrink)
		s.FlexGrow = grow
		s.FlexShrink = shrink
		s.FlexBasis = parseDimension(parts[2], s.FontSize)
	}
}

func isBorderStyle(v string) bool {
	switch v {
	case "none", "hidden", "dotted", "dashed", "solid", "double",
		"groove", "ridge", "inset", "outset":
		return true
	}
	return false
}

func isLengthStr(v string) bool {
	if v == "0" || v == "thin" || v == "medium" || v == "thick" {
		return true
	}
	return strings.HasSuffix(v, "px") || strings.HasSuffix(v, "em") ||
		strings.HasSuffix(v, "rem") || strings.HasSuffix(v, "pt")
}

// parseColor parses a CSS color value.
func parseColor(v string) (dom.Color, bool) {
	v = strings.TrimSpace(v)
	if v == "transparent" {
		return dom.Transparent, true
	}
	if v == "currentcolor" || v == "inherit" {
		return dom.Color{}, false
	}

	// Hex colors
	if strings.HasPrefix(v, "#") {
		return parseHexColor(v[1:])
	}

	// rgb() / rgba()
	if strings.HasPrefix(v, "rgb(") || strings.HasPrefix(v, "rgba(") {
		return parseRGBAFunc(v)
	}

	// hsl() / hsla()
	if strings.HasPrefix(v, "hsl(") || strings.HasPrefix(v, "hsla(") {
		return parseHSLAFunc(v)
	}

	// Named colors
	if c, ok := namedColors[v]; ok {
		return c, true
	}

	return dom.Color{}, false
}

func parseHexColor(hex string) (dom.Color, bool) {
	switch len(hex) {
	case 3:
		r := hexByte(hex[0], hex[0])
		g := hexByte(hex[1], hex[1])
		b := hexByte(hex[2], hex[2])
		return dom.Color{R: r, G: g, B: b, A: 255}, true
	case 4:
		r := hexByte(hex[0], hex[0])
		g := hexByte(hex[1], hex[1])
		b := hexByte(hex[2], hex[2])
		a := hexByte(hex[3], hex[3])
		return dom.Color{R: r, G: g, B: b, A: a}, true
	case 6:
		r := hexByte(hex[0], hex[1])
		g := hexByte(hex[2], hex[3])
		b := hexByte(hex[4], hex[5])
		return dom.Color{R: r, G: g, B: b, A: 255}, true
	case 8:
		r := hexByte(hex[0], hex[1])
		g := hexByte(hex[2], hex[3])
		b := hexByte(hex[4], hex[5])
		a := hexByte(hex[6], hex[7])
		return dom.Color{R: r, G: g, B: b, A: a}, true
	}
	return dom.Color{}, false
}

func hexByte(hi, lo byte) uint8 {
	return uint8(hexVal2(hi)<<4 | hexVal2(lo))
}

func hexVal2(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func parseRGBAFunc(v string) (dom.Color, bool) {
	inner := extractFuncArgs(v)
	parts := splitArgs(inner)
	if len(parts) < 3 {
		return dom.Color{}, false
	}
	r := parseColorComponent(parts[0], 255)
	g := parseColorComponent(parts[1], 255)
	b := parseColorComponent(parts[2], 255)
	a := uint8(255)
	if len(parts) >= 4 {
		var af float64
		parseFloat(strings.TrimSpace(parts[3]), &af)
		a = uint8(af * 255)
	}
	return dom.Color{R: r, G: g, B: b, A: a}, true
}

func parseHSLAFunc(v string) (dom.Color, bool) {
	inner := extractFuncArgs(v)
	parts := splitArgs(inner)
	if len(parts) < 3 {
		return dom.Color{}, false
	}
	var h, s, l float64
	parseFloat(strings.TrimSpace(parts[0]), &h)
	sPart := strings.TrimSuffix(strings.TrimSpace(parts[1]), "%")
	lPart := strings.TrimSuffix(strings.TrimSpace(parts[2]), "%")
	parseFloat(sPart, &s)
	parseFloat(lPart, &l)
	s /= 100
	l /= 100
	a := 1.0
	if len(parts) >= 4 {
		parseFloat(strings.TrimSpace(parts[3]), &a)
	}
	r, g, b := hslToRGB(h, s, l)
	return dom.Color{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: uint8(a * 255),
	}, true
}

func hslToRGB(h, s, l float64) (float64, float64, float64) {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	if s == 0 {
		return l, l, l
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	hue2rgb := func(p, q, t float64) float64 {
		if t < 0 {
			t += 1
		}
		if t > 1 {
			t -= 1
		}
		if t < 1.0/6 {
			return p + (q-p)*6*t
		}
		if t < 1.0/2 {
			return q
		}
		if t < 2.0/3 {
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	h /= 360
	return hue2rgb(p, q, h+1.0/3), hue2rgb(p, q, h), hue2rgb(p, q, h-1.0/3)
}

func parseColorComponent(s string, max float64) uint8 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		var f float64
		parseFloat(s[:len(s)-1], &f)
		return uint8(f / 100 * max)
	}
	var f float64
	parseFloat(s, &f)
	if f > 255 {
		f = 255
	}
	return uint8(f)
}

func extractFuncArgs(v string) string {
	start := strings.Index(v, "(")
	end := strings.LastIndex(v, ")")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return v[start+1 : end]
}

func splitArgs(s string) []string {
	return splitByCommaRespectingParens(s)
}

func splitByCommaRespectingParens(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, ch := range s {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if start <= len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

func defaultDisplay(tag string) string {
	switch tag {
	case "div", "p", "h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "dl", "dt", "dd", "blockquote", "pre",
		"address", "article", "aside", "footer", "header",
		"hgroup", "main", "nav", "section", "figure",
		"figcaption", "details", "summary", "dialog",
		"fieldset", "form", "hr", "table", "caption",
		"thead", "tbody", "tfoot":
		return "block"
	case "li":
		return "list-item"
	case "tr":
		return "table-row"
	case "td", "th":
		return "table-cell"
	case "colgroup":
		return "table-column-group"
	case "col":
		return "table-column"
	case "head", "script", "style", "title", "meta", "link",
		"base", "noscript", "template":
		return "none"
	case "span", "a", "em", "strong", "b", "i", "u", "s",
		"small", "big", "abbr", "cite", "code", "dfn",
		"kbd", "samp", "var", "mark", "bdi", "bdo",
		"sub", "sup", "time", "wbr", "label":
		return "inline"
	case "img", "input", "button", "select", "textarea",
		"video", "audio", "canvas", "svg", "iframe",
		"object", "embed":
		return "inline-block"
	case "br":
		return "inline"
	default:
		return "inline"
	}
}

func parentFontSize(parent *dom.Node) float64 {
	if parent == nil || parent.ComputedStyle == nil {
		return 16
	}
	return parent.ComputedStyle.FontSize
}

// GoImageColor converts a dom.Color to a Go image/color.
func GoImageColor(c dom.Color) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

// namedColors maps CSS color names to dom.Color values.
var namedColors = map[string]dom.Color{
	"aliceblue":            {240, 248, 255, 255},
	"antiquewhite":         {250, 235, 215, 255},
	"aqua":                 {0, 255, 255, 255},
	"aquamarine":           {127, 255, 212, 255},
	"azure":                {240, 255, 255, 255},
	"beige":                {245, 245, 220, 255},
	"bisque":               {255, 228, 196, 255},
	"black":                {0, 0, 0, 255},
	"blanchedalmond":       {255, 235, 205, 255},
	"blue":                 {0, 0, 255, 255},
	"blueviolet":           {138, 43, 226, 255},
	"brown":                {165, 42, 42, 255},
	"burlywood":            {222, 184, 135, 255},
	"cadetblue":            {95, 158, 160, 255},
	"chartreuse":           {127, 255, 0, 255},
	"chocolate":            {210, 105, 30, 255},
	"coral":                {255, 127, 80, 255},
	"cornflowerblue":       {100, 149, 237, 255},
	"cornsilk":             {255, 248, 220, 255},
	"crimson":              {220, 20, 60, 255},
	"cyan":                 {0, 255, 255, 255},
	"darkblue":             {0, 0, 139, 255},
	"darkcyan":             {0, 139, 139, 255},
	"darkgoldenrod":        {184, 134, 11, 255},
	"darkgray":             {169, 169, 169, 255},
	"darkgreen":            {0, 100, 0, 255},
	"darkgrey":             {169, 169, 169, 255},
	"darkkhaki":            {189, 183, 107, 255},
	"darkmagenta":          {139, 0, 139, 255},
	"darkolivegreen":       {85, 107, 47, 255},
	"darkorange":           {255, 140, 0, 255},
	"darkorchid":           {153, 50, 204, 255},
	"darkred":              {139, 0, 0, 255},
	"darksalmon":           {233, 150, 122, 255},
	"darkseagreen":         {143, 188, 143, 255},
	"darkslateblue":        {72, 61, 139, 255},
	"darkslategray":        {47, 79, 79, 255},
	"darkslategrey":        {47, 79, 79, 255},
	"darkturquoise":        {0, 206, 209, 255},
	"darkviolet":           {148, 0, 211, 255},
	"deeppink":             {255, 20, 147, 255},
	"deepskyblue":          {0, 191, 255, 255},
	"dimgray":              {105, 105, 105, 255},
	"dimgrey":              {105, 105, 105, 255},
	"dodgerblue":           {30, 144, 255, 255},
	"firebrick":            {178, 34, 34, 255},
	"floralwhite":          {255, 250, 240, 255},
	"forestgreen":          {34, 139, 34, 255},
	"fuchsia":              {255, 0, 255, 255},
	"gainsboro":            {220, 220, 220, 255},
	"ghostwhite":           {248, 248, 255, 255},
	"gold":                 {255, 215, 0, 255},
	"goldenrod":            {218, 165, 32, 255},
	"gray":                 {128, 128, 128, 255},
	"green":                {0, 128, 0, 255},
	"greenyellow":          {173, 255, 47, 255},
	"grey":                 {128, 128, 128, 255},
	"honeydew":             {240, 255, 240, 255},
	"hotpink":              {255, 105, 180, 255},
	"indianred":            {205, 92, 92, 255},
	"indigo":               {75, 0, 130, 255},
	"ivory":                {255, 255, 240, 255},
	"khaki":                {240, 230, 140, 255},
	"lavender":             {230, 230, 250, 255},
	"lavenderblush":        {255, 240, 245, 255},
	"lawngreen":            {124, 252, 0, 255},
	"lemonchiffon":         {255, 250, 205, 255},
	"lightblue":            {173, 216, 230, 255},
	"lightcoral":           {240, 128, 128, 255},
	"lightcyan":            {224, 255, 255, 255},
	"lightgoldenrodyellow": {250, 250, 210, 255},
	"lightgray":            {211, 211, 211, 255},
	"lightgreen":           {144, 238, 144, 255},
	"lightgrey":            {211, 211, 211, 255},
	"lightpink":            {255, 182, 193, 255},
	"lightsalmon":          {255, 160, 122, 255},
	"lightseagreen":        {32, 178, 170, 255},
	"lightskyblue":         {135, 206, 250, 255},
	"lightslategray":       {119, 136, 153, 255},
	"lightslategrey":       {119, 136, 153, 255},
	"lightsteelblue":       {176, 196, 222, 255},
	"lightyellow":          {255, 255, 224, 255},
	"lime":                 {0, 255, 0, 255},
	"limegreen":            {50, 205, 50, 255},
	"linen":                {250, 240, 230, 255},
	"magenta":              {255, 0, 255, 255},
	"maroon":               {128, 0, 0, 255},
	"mediumaquamarine":     {102, 205, 170, 255},
	"mediumblue":           {0, 0, 205, 255},
	"mediumorchid":         {186, 85, 211, 255},
	"mediumpurple":         {147, 112, 219, 255},
	"mediumseagreen":       {60, 179, 113, 255},
	"mediumslateblue":      {123, 104, 238, 255},
	"mediumspringgreen":    {0, 250, 154, 255},
	"mediumturquoise":      {72, 209, 204, 255},
	"mediumvioletred":      {199, 21, 133, 255},
	"midnightblue":         {25, 25, 112, 255},
	"mintcream":            {245, 255, 250, 255},
	"mistyrose":            {255, 228, 225, 255},
	"moccasin":             {255, 228, 181, 255},
	"navajowhite":          {255, 222, 173, 255},
	"navy":                 {0, 0, 128, 255},
	"oldlace":              {253, 245, 230, 255},
	"olive":                {128, 128, 0, 255},
	"olivedrab":            {107, 142, 35, 255},
	"orange":               {255, 165, 0, 255},
	"orangered":            {255, 69, 0, 255},
	"orchid":               {218, 112, 214, 255},
	"palegoldenrod":        {238, 232, 170, 255},
	"palegreen":            {152, 251, 152, 255},
	"paleturquoise":        {175, 238, 238, 255},
	"palevioletred":        {219, 112, 147, 255},
	"papayawhip":           {255, 239, 213, 255},
	"peachpuff":            {255, 218, 185, 255},
	"peru":                 {205, 133, 63, 255},
	"pink":                 {255, 192, 203, 255},
	"plum":                 {221, 160, 221, 255},
	"powderblue":           {176, 224, 230, 255},
	"purple":               {128, 0, 128, 255},
	"rebeccapurple":        {102, 51, 153, 255},
	"red":                  {255, 0, 0, 255},
	"rosybrown":            {188, 143, 143, 255},
	"royalblue":            {65, 105, 225, 255},
	"saddlebrown":          {139, 69, 19, 255},
	"salmon":               {250, 128, 114, 255},
	"sandybrown":           {244, 164, 96, 255},
	"seagreen":             {46, 139, 87, 255},
	"seashell":             {255, 245, 238, 255},
	"sienna":               {160, 82, 45, 255},
	"silver":               {192, 192, 192, 255},
	"skyblue":              {135, 206, 235, 255},
	"slateblue":            {106, 90, 205, 255},
	"slategray":            {112, 128, 144, 255},
	"slategrey":            {112, 128, 144, 255},
	"snow":                 {255, 250, 250, 255},
	"springgreen":          {0, 255, 127, 255},
	"steelblue":            {70, 130, 180, 255},
	"tan":                  {210, 180, 140, 255},
	"teal":                 {0, 128, 128, 255},
	"thistle":              {216, 191, 216, 255},
	"tomato":               {255, 99, 71, 255},
	"turquoise":            {64, 224, 208, 255},
	"violet":               {238, 130, 238, 255},
	"wheat":                {245, 222, 179, 255},
	"white":                {255, 255, 255, 255},
	"whitesmoke":           {245, 245, 245, 255},
	"yellow":               {255, 255, 0, 255},
	"yellowgreen":          {154, 205, 50, 255},
}

// userAgentStylesheet is the browser default stylesheet.
const userAgentStylesheet = `
html, body { display: block; }
body { margin: 8px; }

h1 { display: block; font-size: 2em; font-weight: bold; margin-top: 0.67em; margin-bottom: 0.67em; }
h2 { display: block; font-size: 1.5em; font-weight: bold; margin-top: 0.75em; margin-bottom: 0.75em; }
h3 { display: block; font-size: 1.17em; font-weight: bold; margin-top: 0.83em; margin-bottom: 0.83em; }
h4 { display: block; font-size: 1em; font-weight: bold; margin-top: 1.12em; margin-bottom: 1.12em; }
h5 { display: block; font-size: 0.83em; font-weight: bold; margin-top: 1.5em; margin-bottom: 1.5em; }
h6 { display: block; font-size: 0.75em; font-weight: bold; margin-top: 1.67em; margin-bottom: 1.67em; }

p { display: block; margin-top: 1em; margin-bottom: 1em; }
blockquote { display: block; margin-top: 1em; margin-bottom: 1em; margin-left: 40px; margin-right: 40px; }
pre { display: block; font-family: monospace; white-space: pre; margin-top: 1em; margin-bottom: 1em; }

ul, ol { display: block; margin-top: 1em; margin-bottom: 1em; padding-left: 40px; }
li { display: list-item; }
dl { display: block; margin-top: 1em; margin-bottom: 1em; }
dt { display: block; font-weight: bold; }
dd { display: block; margin-left: 40px; }

a { color: #0000ee; text-decoration: underline; cursor: pointer; }
a:visited { color: #551a8b; }
a:hover { text-decoration: underline; }

strong, b { font-weight: bold; }
em, i { font-style: italic; }
small { font-size: 0.833em; }
big { font-size: 1.17em; }
sub { font-size: 0.83em; }
sup { font-size: 0.83em; }
code, kbd, samp, tt { font-family: monospace; }
u { text-decoration: underline; }
s, strike, del { text-decoration: line-through; }

hr { display: block; margin-top: 0.5em; margin-bottom: 0.5em; border-top-width: 1px; border-top-style: solid; border-top-color: #888888; }

table { display: table; border-collapse: separate; }
thead { display: table-header-group; }
tbody { display: table-row-group; }
tfoot { display: table-footer-group; }
tr { display: table-row; }
td, th { display: table-cell; padding-top: 1px; padding-right: 1px; padding-bottom: 1px; padding-left: 1px; }
th { font-weight: bold; text-align: center; }
caption { display: table-caption; text-align: center; }

head, script, style, title, meta, link, base, noscript { display: none; }

input, button, select, textarea { display: inline-block; }
button { cursor: pointer; }
img { display: inline-block; }
video, audio, canvas { display: inline-block; }

mark { background-color: yellow; color: black; }
abbr { text-decoration: underline dotted; }
cite { font-style: italic; }
q { quotes: '"' '"' "'" "'"; }

figure { display: block; margin-top: 1em; margin-bottom: 1em; margin-left: 40px; margin-right: 40px; }
figcaption { display: block; }

article, aside, details, footer, header, hgroup, main, nav, section, summary {
    display: block;
}

address { display: block; font-style: italic; }
div { display: block; }
span { display: inline; }
`
