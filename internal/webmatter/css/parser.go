package css

import (
	"strings"
)

// Stylesheet is a parsed CSS stylesheet.
type Stylesheet struct {
	Rules []Rule
}

// Rule is a CSS rule (selector + declarations).
type Rule struct {
	Selectors    []Selector    // list of selectors separated by commas
	Declarations []Declaration // CSS declarations
	AtRule       string        // if it's an at-rule (@media, @font-face, etc.)
	Conditions   string        // at-rule condition
	SubRules     []Rule        // nested rules for @media etc.
}

// Declaration is a CSS property: value pair.
type Declaration struct {
	Property  string
	Value     string // raw value string
	Important bool
}

// Selector is a parsed CSS selector.
type Selector struct {
	Parts      []SelectorPart
	Specificity [3]int // [0]=id, [1]=class/attr/pseudo, [2]=element
}

// SelectorCombinator describes how a selector part relates to the previous.
type SelectorCombinator int

const (
	CombinatorDescendant SelectorCombinator = iota // space
	CombinatorChild                                 // >
	CombinatorAdjacent                              // +
	CombinatorSibling                               // ~
	CombinatorNone                                  // first part (no combinator)
)

// SelectorPart is a single simple selector component.
type SelectorPart struct {
	Combinator  SelectorCombinator
	Tag         string // element type selector (or * for any)
	ID          string // #id
	Classes     []string
	Attrs       []AttrSelector
	PseudoClass []string // :hover, :first-child, etc.
	PseudoElem  string   // ::before, ::after, etc.
}

// AttrSelector is an attribute selector [attr=value].
type AttrSelector struct {
	Name     string
	Op       string // =, ~=, |=, ^=, $=, *=, or "" for existence
	Value    string
	CaseFlag bool // i flag
}

// Parser parses CSS.
type Parser struct {
	toks []Tok
	pos  int
}

// Parse parses a CSS stylesheet string.
func Parse(input string) *Stylesheet {
	p := &Parser{
		toks: NewTokenizer(input).All(),
	}
	return p.parseStylesheet()
}

// ParseDeclarations parses inline CSS declarations (style="...").
func ParseDeclarations(input string) []Declaration {
	p := &Parser{
		toks: NewTokenizer(input).All(),
	}
	return p.parseDeclarationList()
}

func (p *Parser) current() Tok {
	if p.pos >= len(p.toks) {
		return Tok{Type: TokEOF}
	}
	return p.toks[p.pos]
}

func (p *Parser) advance() Tok {
	t := p.current()
	if p.pos < len(p.toks) {
		p.pos++
	}
	return t
}

func (p *Parser) skipWS() {
	for p.pos < len(p.toks) && p.toks[p.pos].Type == TokWhitespace {
		p.pos++
	}
}

func (p *Parser) parseStylesheet() *Stylesheet {
	ss := &Stylesheet{}
	for {
		p.skipWS()
		if p.current().Type == TokEOF {
			break
		}
		if p.current().Type == TokAtKeyword {
			rule := p.parseAtRule()
			if rule != nil {
				ss.Rules = append(ss.Rules, *rule)
			}
			continue
		}
		if p.current().Type == TokCDO || p.current().Type == TokCDC {
			p.advance()
			continue
		}
		rule := p.parseQualifiedRule()
		if rule != nil {
			ss.Rules = append(ss.Rules, *rule)
		}
	}
	return ss
}

func (p *Parser) parseAtRule() *Rule {
	kwTok := p.advance() // consume @keyword
	keyword := kwTok.Value

	rule := &Rule{AtRule: keyword}

	// Collect prelude until { or ;
	var prelude strings.Builder
	for {
		tok := p.current()
		if tok.Type == TokEOF || tok.Type == TokSemicolon {
			p.advance()
			return rule // non-block at-rule
		}
		if tok.Type == TokLBrace {
			p.advance() // consume {
			break
		}
		prelude.WriteString(tok.Value)
		p.advance()
	}
	rule.Conditions = strings.TrimSpace(prelude.String())

	// Parse block (either rules or declarations depending on at-rule type)
	switch keyword {
	case "media", "supports", "document", "layer":
		for {
			p.skipWS()
			if p.current().Type == TokRBrace || p.current().Type == TokEOF {
				p.advance()
				break
			}
			if p.current().Type == TokAtKeyword {
				sub := p.parseAtRule()
				if sub != nil {
					rule.SubRules = append(rule.SubRules, *sub)
				}
				continue
			}
			sub := p.parseQualifiedRule()
			if sub != nil {
				rule.SubRules = append(rule.SubRules, *sub)
			}
		}
	default:
		// Skip block
		depth := 1
		for p.pos < len(p.toks) && depth > 0 {
			tok := p.advance()
			if tok.Type == TokLBrace {
				depth++
			} else if tok.Type == TokRBrace {
				depth--
			}
		}
	}
	return rule
}

func (p *Parser) parseQualifiedRule() *Rule {
	rule := &Rule{}

	// Parse selectors (prelude)
	var prelude []Tok
	for {
		tok := p.current()
		if tok.Type == TokEOF {
			return nil
		}
		if tok.Type == TokLBrace {
			p.advance()
			break
		}
		prelude = append(prelude, tok)
		p.advance()
	}

	// Parse selector list from prelude
	rule.Selectors = parseSelectors(prelude)

	// Parse declarations
	rule.Declarations = p.parseDeclarationList()

	return rule
}

func (p *Parser) parseDeclarationList() []Declaration {
	var decls []Declaration
	for {
		p.skipWS()
		tok := p.current()
		if tok.Type == TokRBrace || tok.Type == TokEOF {
			if tok.Type == TokRBrace {
				p.advance()
			}
			break
		}
		if tok.Type == TokSemicolon {
			p.advance()
			continue
		}
		if tok.Type == TokIdent {
			decl := p.parseDeclaration()
			if decl != nil {
				decls = append(decls, *decl)
			}
			continue
		}
		// Skip unknown token
		p.advance()
	}
	return decls
}

func (p *Parser) parseDeclaration() *Declaration {
	nameTok := p.advance() // consume property name
	p.skipWS()

	if p.current().Type != TokColon {
		// Skip until semicolon or end
		p.skipUntilSemiOrEnd()
		return nil
	}
	p.advance() // consume ':'
	p.skipWS()

	// Collect value tokens until ; or }
	var valueParts []string
	important := false
	for {
		tok := p.current()
		if tok.Type == TokSemicolon || tok.Type == TokRBrace || tok.Type == TokEOF {
			break
		}
		p.advance()
		valueParts = append(valueParts, tok.Value)
	}

	// Check for !important
	value := strings.TrimSpace(strings.Join(valueParts, ""))
	if strings.HasSuffix(strings.ToLower(value), "!important") {
		important = true
		value = strings.TrimSpace(value[:len(value)-len("!important")])
	}
	// Also check "! important" pattern
	if idx := strings.LastIndex(strings.ToLower(value), "!"); idx != -1 {
		rest := strings.TrimSpace(value[idx+1:])
		if rest == "important" {
			important = true
			value = strings.TrimSpace(value[:idx])
		}
	}

	return &Declaration{
		Property:  strings.ToLower(strings.TrimSpace(nameTok.Value)),
		Value:     strings.TrimSpace(value),
		Important: important,
	}
}

func (p *Parser) skipUntilSemiOrEnd() {
	for {
		tok := p.current()
		if tok.Type == TokSemicolon || tok.Type == TokRBrace || tok.Type == TokEOF {
			break
		}
		p.advance()
	}
}

// parseSelectors parses a comma-separated selector list.
func parseSelectors(toks []Tok) []Selector {
	// Split by comma (TokComma)
	var groups [][]Tok
	cur := []Tok{}
	for _, tok := range toks {
		if tok.Type == TokComma {
			groups = append(groups, cur)
			cur = []Tok{}
		} else {
			cur = append(cur, tok)
		}
	}
	groups = append(groups, cur)

	var selectors []Selector
	for _, group := range groups {
		sel := parseSelector(group)
		selectors = append(selectors, sel)
	}
	return selectors
}

// parseSelector parses a single compound selector.
func parseSelector(toks []Tok) Selector {
	sel := Selector{}
	pos := 0

	skipWSToks := func() {
		for pos < len(toks) && toks[pos].Type == TokWhitespace {
			pos++
		}
	}

	current := SelectorPart{
		Combinator: CombinatorNone,
		Tag:        "",
	}
	first := true

	flush := func() {
		if !first || current.Tag != "" || current.ID != "" || len(current.Classes) > 0 {
			sel.Parts = append(sel.Parts, current)
		}
		first = false
	}

	for pos < len(toks) {
		tok := toks[pos]

		if tok.Type == TokWhitespace {
			// Check next non-WS token for combinator
			pos++
			nextPos := pos
			for nextPos < len(toks) && toks[nextPos].Type == TokWhitespace {
				nextPos++
			}
			if nextPos >= len(toks) {
				break
			}
			next := toks[nextPos]
			if next.Type == TokDelim && (next.Value == ">" || next.Value == "+" || next.Value == "~") {
				// Combinator follows, handled below
				continue
			}
			// Descendant combinator
			flush()
			current = SelectorPart{Combinator: CombinatorDescendant}
			first = false
			continue
		}

		if tok.Type == TokDelim {
			switch tok.Value {
			case ">":
				pos++
				skipWSToks()
				flush()
				current = SelectorPart{Combinator: CombinatorChild}
				first = false
				continue
			case "+":
				pos++
				skipWSToks()
				flush()
				current = SelectorPart{Combinator: CombinatorAdjacent}
				first = false
				continue
			case "~":
				pos++
				skipWSToks()
				flush()
				current = SelectorPart{Combinator: CombinatorSibling}
				first = false
				continue
			case "*":
				current.Tag = "*"
				pos++
				continue
			case ".":
				pos++
				if pos < len(toks) && toks[pos].Type == TokIdent {
					current.Classes = append(current.Classes, toks[pos].Value)
					pos++
				}
				continue
			}
		}

		if tok.Type == TokIdent {
			if first || current.Tag == "" {
				current.Tag = tok.Value
			}
			pos++
			continue
		}

		if tok.Type == TokHash {
			current.ID = tok.Value
			pos++
			continue
		}

		if tok.Type == TokColon {
			pos++
			if pos < len(toks) {
				if toks[pos].Type == TokColon {
					// ::pseudo-element
					pos++
					if pos < len(toks) && toks[pos].Type == TokIdent {
						current.PseudoElem = toks[pos].Value
						pos++
					}
				} else if toks[pos].Type == TokIdent {
					current.PseudoClass = append(current.PseudoClass, toks[pos].Value)
					pos++
				} else if toks[pos].Type == TokFunction {
					// :nth-child(n) etc.
					fnName := toks[pos].Value
					pos++
					// skip until )
					var args strings.Builder
					for pos < len(toks) && toks[pos].Type != TokRParen {
						args.WriteString(toks[pos].Value)
						pos++
					}
					if pos < len(toks) {
						pos++ // consume )
					}
					current.PseudoClass = append(current.PseudoClass, fnName+"("+args.String()+")")
				}
			}
			continue
		}

		if tok.Type == TokLBracket {
			pos++
			attr := parseAttrSelector(toks, &pos)
			current.Attrs = append(current.Attrs, attr)
			continue
		}

		pos++
	}

	flush()

	// Calculate specificity
	for _, part := range sel.Parts {
		if part.ID != "" {
			sel.Specificity[0]++
		}
		sel.Specificity[1] += len(part.Classes) + len(part.Attrs) + len(part.PseudoClass)
		if part.Tag != "" && part.Tag != "*" {
			sel.Specificity[2]++
		}
		if part.PseudoElem != "" {
			sel.Specificity[2]++
		}
	}

	return sel
}

func parseAttrSelector(toks []Tok, pos *int) AttrSelector {
	attr := AttrSelector{}
	// skip ws
	for *pos < len(toks) && toks[*pos].Type == TokWhitespace {
		*pos++
	}
	if *pos < len(toks) && toks[*pos].Type == TokIdent {
		attr.Name = toks[*pos].Value
		*pos++
	}
	// skip ws
	for *pos < len(toks) && toks[*pos].Type == TokWhitespace {
		*pos++
	}
	if *pos < len(toks) && toks[*pos].Type == TokRBracket {
		*pos++
		return attr
	}
	// operator
	if *pos < len(toks) {
		tok := toks[*pos]
		switch {
		case tok.Type == TokDelim && tok.Value == "=":
			attr.Op = "="
			*pos++
		case tok.Type == TokDelim && tok.Value == "~":
			*pos++
			if *pos < len(toks) && toks[*pos].Type == TokDelim && toks[*pos].Value == "=" {
				attr.Op = "~="
				*pos++
			}
		case tok.Type == TokDelim && tok.Value == "|":
			*pos++
			if *pos < len(toks) && toks[*pos].Type == TokDelim && toks[*pos].Value == "=" {
				attr.Op = "|="
				*pos++
			}
		case tok.Type == TokDelim && tok.Value == "^":
			*pos++
			if *pos < len(toks) && toks[*pos].Type == TokDelim && toks[*pos].Value == "=" {
				attr.Op = "^="
				*pos++
			}
		case tok.Type == TokDelim && tok.Value == "$":
			*pos++
			if *pos < len(toks) && toks[*pos].Type == TokDelim && toks[*pos].Value == "=" {
				attr.Op = "$="
				*pos++
			}
		case tok.Type == TokDelim && tok.Value == "*":
			*pos++
			if *pos < len(toks) && toks[*pos].Type == TokDelim && toks[*pos].Value == "=" {
				attr.Op = "*="
				*pos++
			}
		}
	}
	// skip ws
	for *pos < len(toks) && toks[*pos].Type == TokWhitespace {
		*pos++
	}
	// value
	if *pos < len(toks) {
		tok := toks[*pos]
		if tok.Type == TokString || tok.Type == TokIdent {
			attr.Value = tok.Value
			*pos++
		}
	}
	// skip to ]
	for *pos < len(toks) && toks[*pos].Type != TokRBracket {
		*pos++
	}
	if *pos < len(toks) {
		*pos++ // consume ]
	}
	return attr
}
