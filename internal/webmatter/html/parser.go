package html

import (
	"strings"

	"github.com/shard-browser/shard/internal/webmatter/dom"
)

// insertionMode describes the HTML5 tree construction insertion mode.
type insertionMode int

const (
	modeInitial insertionMode = iota
	modeBeforeHTML
	modeBeforeHead
	modeInHead
	modeAfterHead
	modeInBody
	modeText
	modeAfterBody
	modeAfterAfterBody
)

// Parser implements the HTML5 tree construction algorithm.
type Parser struct {
	tokenizer     *Tokenizer
	doc           *dom.Document
	mode          insertionMode
	openElements  []*dom.Node
	headElement   *dom.Node
	formElement   *dom.Node
	scriptContent strings.Builder
	styleContent  strings.Builder
}

// Parse parses an HTML document string and returns a Document.
func Parse(input string) *dom.Document {
	p := &Parser{
		tokenizer: NewTokenizer(input),
		doc:       dom.NewDocument(),
		mode:      modeInitial,
	}
	p.run()
	return p.doc
}

func (p *Parser) run() {
	for {
		tok := p.tokenizer.Next()
		if tok.Type == TokenEOF {
			p.handleEOF()
			break
		}
		p.processToken(tok)
	}
}

func (p *Parser) processToken(tok Token) {
	switch p.mode {
	case modeInitial:
		p.initialMode(tok)
	case modeBeforeHTML:
		p.beforeHTMLMode(tok)
	case modeBeforeHead:
		p.beforeHeadMode(tok)
	case modeInHead:
		p.inHeadMode(tok)
	case modeAfterHead:
		p.afterHeadMode(tok)
	case modeInBody:
		p.inBodyMode(tok)
	case modeText:
		p.textMode(tok)
	case modeAfterBody:
		p.afterBodyMode(tok)
	case modeAfterAfterBody:
		p.inBodyMode(tok)
	}
}

func (p *Parser) initialMode(tok Token) {
	if tok.Type == TokenDoctype {
		p.doc.Node.Data = tok.Data
		p.mode = modeBeforeHTML
		return
	}
	p.mode = modeBeforeHTML
	p.processToken(tok)
}

func (p *Parser) beforeHTMLMode(tok Token) {
	if tok.Type == TokenStartTag && tok.TagName == "html" {
		el := p.createElement(tok)
		p.doc.Node.AppendChild(el)
		p.doc.Root = el
		p.pushOpen(el)
		p.mode = modeBeforeHead
		return
	}
	if tok.Type == TokenChar && isOnlyWhitespace(tok.Data) {
		return
	}
	// Implied <html>
	html := dom.NewElement("html")
	p.doc.Node.AppendChild(html)
	p.doc.Root = html
	p.pushOpen(html)
	p.mode = modeBeforeHead
	p.processToken(tok)
}

func (p *Parser) beforeHeadMode(tok Token) {
	if tok.Type == TokenChar && isOnlyWhitespace(tok.Data) {
		return
	}
	if tok.Type == TokenStartTag && tok.TagName == "head" {
		head := p.createElement(tok)
		p.currentNode().AppendChild(head)
		p.doc.Head = head
		p.headElement = head
		p.pushOpen(head)
		p.mode = modeInHead
		return
	}
	// Implied <head>
	head := dom.NewElement("head")
	p.currentNode().AppendChild(head)
	p.doc.Head = head
	p.headElement = head
	p.pushOpen(head)
	p.mode = modeInHead
	p.processToken(tok)
}

func (p *Parser) inHeadMode(tok Token) {
	if tok.Type == TokenChar && isOnlyWhitespace(tok.Data) {
		p.insertCharacter(tok.Data)
		return
	}
	if tok.Type == TokenComment {
		return
	}
	if tok.Type == TokenStartTag {
		switch tok.TagName {
		case "meta", "link", "base":
			el := p.createElement(tok)
			p.currentNode().AppendChild(el)
			return
		case "title":
			el := p.createElement(tok)
			p.currentNode().AppendChild(el)
			p.pushOpen(el)
			// Read title content
			var titleText strings.Builder
			for {
				t := p.tokenizer.Next()
				if t.Type == TokenEOF || (t.Type == TokenEndTag && t.TagName == "title") {
					break
				}
				if t.Type == TokenChar {
					titleText.WriteString(t.Data)
				}
			}
			p.popOpen()
			text := dom.NewText(strings.TrimSpace(titleText.String()))
			el.AppendChild(text)
			p.doc.Title = text.Data
			return
		case "style":
			// Collect style content
			var css strings.Builder
			for {
				t := p.tokenizer.Next()
				if t.Type == TokenEOF || (t.Type == TokenEndTag && t.TagName == "style") {
					break
				}
				if t.Type == TokenChar {
					css.WriteString(t.Data)
				}
			}
			p.doc.Node.SetAttr("_stylesheet_"+randKey(), css.String())
			return
		case "script":
			// Skip script for now (no JS engine yet)
			for {
				t := p.tokenizer.Next()
				if t.Type == TokenEOF || (t.Type == TokenEndTag && t.TagName == "script") {
					break
				}
			}
			return
		case "noscript", "noframes":
			return
		}
	}
	if tok.Type == TokenEndTag && tok.TagName == "head" {
		p.popOpen()
		p.mode = modeAfterHead
		return
	}
	// Anything else: leave head
	p.popOpen()
	p.mode = modeAfterHead
	p.processToken(tok)
}

var randCounter int

func randKey() string {
	randCounter++
	return strings.Repeat("0", 8-len(itoa(randCounter))) + itoa(randCounter)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func (p *Parser) afterHeadMode(tok Token) {
	if tok.Type == TokenChar && isOnlyWhitespace(tok.Data) {
		p.insertCharacter(tok.Data)
		return
	}
	if tok.Type == TokenComment {
		return
	}
	if tok.Type == TokenStartTag && tok.TagName == "body" {
		body := p.createElement(tok)
		p.currentNode().AppendChild(body)
		p.doc.Body = body
		p.pushOpen(body)
		p.mode = modeInBody
		return
	}
	// Implied <body>
	body := dom.NewElement("body")
	p.currentNode().AppendChild(body)
	p.doc.Body = body
	p.pushOpen(body)
	p.mode = modeInBody
	p.processToken(tok)
}

func (p *Parser) inBodyMode(tok Token) {
	switch tok.Type {
	case TokenChar:
		if tok.Data != "" {
			p.insertCharacter(tok.Data)
		}
		return

	case TokenComment:
		return

	case TokenStartTag:
		p.handleStartTagInBody(tok)

	case TokenEndTag:
		p.handleEndTagInBody(tok)

	case TokenSelfClosingTag:
		p.handleSelfClosingInBody(tok)
	}
}

func (p *Parser) handleStartTagInBody(tok Token) {
	tag := tok.TagName
	switch tag {
	case "html":
		// Merge attributes into <html> element
		return

	case "body":
		return

	case "address", "article", "aside", "blockquote", "center",
		"details", "dialog", "dir", "div", "dl", "fieldset",
		"figcaption", "figure", "footer", "header", "hgroup",
		"main", "menu", "nav", "ol", "p", "section", "summary", "ul":
		p.closeParagraphIfNeeded()
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "h1", "h2", "h3", "h4", "h5", "h6":
		p.closeParagraphIfNeeded()
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "pre", "listing":
		p.closeParagraphIfNeeded()
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "form":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)
		p.formElement = el

	case "li":
		p.closeOpenLI()
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "dt", "dd":
		p.closeOpenDTDD()
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "plaintext":
		p.closeParagraphIfNeeded()
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "button":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "a":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "b", "big", "code", "em", "font", "i", "kbd", "mark",
		"s", "samp", "small", "span", "strike", "strong", "sub",
		"sup", "tt", "u", "var":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "nobr":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "table":
		p.closeParagraphIfNeeded()
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "tr":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "td", "th":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "caption", "col", "colgroup", "tbody", "tfoot", "thead":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "br":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		// br is self-closing

	case "img", "input", "area", "param", "track", "wbr":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		// self-closing: don't push to open elements

	case "hr":
		p.closeParagraphIfNeeded()
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)

	case "textarea":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "select":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "option", "optgroup":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "script":
		// Skip script content
		for {
			t := p.tokenizer.Next()
			if t.Type == TokenEOF || (t.Type == TokenEndTag && t.TagName == "script") {
				break
			}
		}

	case "style":
		var css strings.Builder
		for {
			t := p.tokenizer.Next()
			if t.Type == TokenEOF || (t.Type == TokenEndTag && t.TagName == "style") {
				break
			}
			if t.Type == TokenChar {
				css.WriteString(t.Data)
			}
		}
		p.doc.Node.SetAttr("_stylesheet_"+randKey(), css.String())

	case "template":
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	case "head":
		// Ignore

	case "noscript", "noframes", "noembed":
		// Skip content
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)

	default:
		el := p.createElement(tok)
		p.currentNode().AppendChild(el)
		p.pushOpen(el)
	}
}

func (p *Parser) handleSelfClosingInBody(tok Token) {
	el := p.createElement(tok)
	p.currentNode().AppendChild(el)
}

func (p *Parser) handleEndTagInBody(tok Token) {
	tag := tok.TagName
	switch tag {
	case "body", "html":
		p.mode = modeAfterBody
		return

	case "address", "article", "aside", "blockquote", "button", "center",
		"details", "dialog", "dir", "div", "dl", "fieldset", "figcaption",
		"figure", "footer", "header", "hgroup", "listing", "main", "menu",
		"nav", "ol", "pre", "section", "summary", "ul",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"form", "p", "li", "dt", "dd",
		"table", "tr", "td", "th", "caption", "colgroup", "tbody", "tfoot", "thead",
		"a", "b", "big", "code", "em", "font", "i", "kbd", "mark",
		"s", "samp", "small", "span", "strike", "strong", "sub",
		"sup", "tt", "u", "var", "nobr",
		"template", "select", "option", "optgroup", "textarea":
		p.popUntil(tag)

	default:
		p.popUntil(tag)
	}
}

func (p *Parser) textMode(tok Token) {
	switch tok.Type {
	case TokenChar:
		p.insertCharacter(tok.Data)
	case TokenEndTag:
		p.popOpen()
		p.mode = modeInBody
	}
}

func (p *Parser) afterBodyMode(tok Token) {
	if tok.Type == TokenChar && isOnlyWhitespace(tok.Data) {
		return
	}
	if tok.Type == TokenEndTag && tok.TagName == "html" {
		p.mode = modeAfterAfterBody
		return
	}
	// anything else: go back to in body
	p.mode = modeInBody
	p.processToken(tok)
}

func (p *Parser) handleEOF() {
	// Nothing to do
}

// --- Helpers ---

func (p *Parser) createElement(tok Token) *dom.Node {
	el := dom.NewElement(tok.TagName)
	for k, v := range tok.Attrs {
		el.SetAttr(k, v)
	}
	return el
}

func (p *Parser) currentNode() *dom.Node {
	if len(p.openElements) == 0 {
		return &p.doc.Node
	}
	return p.openElements[len(p.openElements)-1]
}

func (p *Parser) pushOpen(n *dom.Node) {
	p.openElements = append(p.openElements, n)
}

func (p *Parser) popOpen() *dom.Node {
	if len(p.openElements) == 0 {
		return nil
	}
	n := p.openElements[len(p.openElements)-1]
	p.openElements = p.openElements[:len(p.openElements)-1]
	return n
}

func (p *Parser) popUntil(tagName string) {
	for i := len(p.openElements) - 1; i >= 0; i-- {
		if p.openElements[i].TagName == tagName {
			p.openElements = p.openElements[:i]
			return
		}
	}
}

func (p *Parser) insertCharacter(data string) {
	cur := p.currentNode()
	// Merge with existing text node if last child is text
	if len(cur.Children) > 0 {
		last := cur.Children[len(cur.Children)-1]
		if last.Type == dom.NodeTypeText {
			last.Data += data
			return
		}
	}
	cur.AppendChild(dom.NewText(data))
}

func (p *Parser) closeParagraphIfNeeded() {
	for i := len(p.openElements) - 1; i >= 0; i-- {
		el := p.openElements[i]
		if el.TagName == "p" {
			p.openElements = p.openElements[:i]
			return
		}
		if isSpecialElement(el.TagName) {
			return
		}
	}
}

func (p *Parser) closeOpenLI() {
	for i := len(p.openElements) - 1; i >= 0; i-- {
		el := p.openElements[i]
		if el.TagName == "li" {
			p.openElements = p.openElements[:i]
			return
		}
		if isSpecialElement(el.TagName) {
			return
		}
	}
}

func (p *Parser) closeOpenDTDD() {
	for i := len(p.openElements) - 1; i >= 0; i-- {
		el := p.openElements[i]
		if el.TagName == "dt" || el.TagName == "dd" {
			p.openElements = p.openElements[:i]
			return
		}
		if isSpecialElement(el.TagName) {
			return
		}
	}
}

func isSpecialElement(tag string) bool {
	switch tag {
	case "address", "applet", "area", "article", "aside", "base", "basefont",
		"bgsound", "blockquote", "body", "br", "button", "caption", "center",
		"col", "colgroup", "dd", "details", "dir", "div", "dl", "dt", "embed",
		"fieldset", "figcaption", "figure", "footer", "form", "frame",
		"frameset", "h1", "h2", "h3", "h4", "h5", "h6", "head", "header",
		"hgroup", "hr", "html", "iframe", "img", "input", "isindex", "li",
		"link", "listing", "main", "marquee", "menu", "meta", "nav", "noembed",
		"noframes", "noscript", "object", "ol", "p", "param", "plaintext",
		"pre", "script", "section", "select", "source", "style", "summary",
		"table", "tbody", "td", "template", "textarea", "tfoot", "th",
		"thead", "title", "tr", "track", "ul", "wbr", "xmp":
		return true
	}
	return false
}

func isOnlyWhitespace(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' && r != '\f' {
			return false
		}
	}
	return true
}

// CollectStylesheets extracts all stylesheet content from a parsed document.
func CollectStylesheets(doc *dom.Document) []string {
	var sheets []string
	// Collect from document node attributes (style tags collected during parsing)
	for k, v := range doc.Node.Attrs {
		if strings.HasPrefix(k, "_stylesheet_") {
			sheets = append(sheets, v)
		}
	}
	return sheets
}
