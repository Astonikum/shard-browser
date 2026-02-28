// Package html implements an HTML5 tokenizer and parser.
package html

import (
	"strings"
	"unicode"
)

// TokenType identifies the type of an HTML token.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenDoctype
	TokenStartTag
	TokenEndTag
	TokenSelfClosingTag
	TokenChar
	TokenComment
)

// Token is an HTML token.
type Token struct {
	Type        TokenType
	TagName     string
	Attrs       map[string]string
	AttrOrder   []string // preserve attribute order
	Data        string   // text content / comment / doctype name
	SelfClosing bool
}

// tokenizerState is the current state of the tokenizer.
type tokenizerState int

const (
	stateData tokenizerState = iota
	stateTagOpen
	stateEndTagOpen
	stateTagName
	stateBeforeAttrName
	stateAttrName
	stateAfterAttrName
	stateBeforeAttrValue
	stateAttrValueDouble
	stateAttrValueSingle
	stateAttrValueUnquoted
	stateAfterAttrValueQuoted
	stateSelfClosingStartTag
	stateMarkupDeclarationOpen
	stateCommentStart
	stateCommentStartDash
	stateComment
	stateCommentEndDash
	stateCommentEnd
	stateDoctype
	stateBeforeDoctypeName
	stateDoctypeName
	stateScriptData
	stateRawText
	stateRCDATA
)

// Tokenizer tokenizes HTML input.
type Tokenizer struct {
	input  []rune
	pos    int
	state  tokenizerState
	rawTag string // current raw tag (script, style, etc.)

	// current token being built
	current Token
	curAttr string
	curVal  string
}

// NewTokenizer creates a new HTML tokenizer.
func NewTokenizer(input string) *Tokenizer {
	return &Tokenizer{
		input: []rune(input),
		state: stateData,
	}
}

// Next returns the next token.
func (t *Tokenizer) Next() Token {
	for {
		if t.pos >= len(t.input) {
			return Token{Type: TokenEOF}
		}
		tok := t.consume()
		if tok != nil {
			return *tok
		}
	}
}

func (t *Tokenizer) peek() (rune, bool) {
	if t.pos >= len(t.input) {
		return 0, false
	}
	return t.input[t.pos], true
}

func (t *Tokenizer) advance() rune {
	r := t.input[t.pos]
	t.pos++
	return r
}

func (t *Tokenizer) consume() *Token {
	ch, ok := t.peek()
	if !ok {
		return &Token{Type: TokenEOF}
	}

	switch t.state {

	// ----- DATA STATE -----
	case stateData:
		if ch == '<' {
			t.advance()
			t.state = stateTagOpen
			return nil
		}
		if ch == '&' {
			t.advance()
			ref := t.consumeCharRef()
			return &Token{Type: TokenChar, Data: ref}
		}
		t.advance()
		return &Token{Type: TokenChar, Data: string(ch)}

	// ----- SCRIPT DATA -----
	case stateScriptData, stateRawText:
		// Read until </tagName>
		closing := "</" + t.rawTag
		rest := string(t.input[t.pos:])
		idx := strings.Index(strings.ToLower(rest), closing)
		if idx == -1 {
			// Consume everything
			data := string(t.input[t.pos:])
			t.pos = len(t.input)
			if data == "" {
				return &Token{Type: TokenEOF}
			}
			return &Token{Type: TokenChar, Data: data}
		}
		data := rest[:idx]
		t.pos += idx
		t.state = stateData
		if data == "" {
			return nil
		}
		return &Token{Type: TokenChar, Data: data}

	// ----- RCDATA (textarea, title) -----
	case stateRCDATA:
		closing := "</" + t.rawTag
		rest := string(t.input[t.pos:])
		idx := strings.Index(strings.ToLower(rest), closing)
		if idx == -1 {
			data := decodeEntities(string(t.input[t.pos:]))
			t.pos = len(t.input)
			return &Token{Type: TokenChar, Data: data}
		}
		data := decodeEntities(rest[:idx])
		t.pos += idx
		t.state = stateData
		if data == "" {
			return nil
		}
		return &Token{Type: TokenChar, Data: data}

	// ----- TAG OPEN -----
	case stateTagOpen:
		if ch == '!' {
			t.advance()
			t.state = stateMarkupDeclarationOpen
			return nil
		}
		if ch == '/' {
			t.advance()
			t.state = stateEndTagOpen
			return nil
		}
		if unicode.IsLetter(ch) {
			t.current = Token{
				Type:  TokenStartTag,
				Attrs: make(map[string]string),
			}
			t.state = stateTagName
			return nil
		}
		// anything else: emit '<' as char
		t.state = stateData
		return &Token{Type: TokenChar, Data: "<"}

	// ----- END TAG OPEN -----
	case stateEndTagOpen:
		if unicode.IsLetter(ch) {
			t.current = Token{
				Type:  TokenEndTag,
				Attrs: make(map[string]string),
			}
			t.state = stateTagName
			return nil
		}
		t.state = stateData
		return nil

	// ----- TAG NAME -----
	case stateTagName:
		if isWhitespace(ch) {
			t.advance()
			t.state = stateBeforeAttrName
			return nil
		}
		if ch == '/' {
			t.advance()
			t.state = stateSelfClosingStartTag
			return nil
		}
		if ch == '>' {
			t.advance()
			t.state = stateData
			t.setRawTagState()
			tok := t.current
			return &tok
		}
		t.advance()
		t.current.TagName += string(unicode.ToLower(ch))
		return nil

	// ----- BEFORE ATTRIBUTE NAME -----
	case stateBeforeAttrName:
		if isWhitespace(ch) {
			t.advance()
			return nil
		}
		if ch == '/' {
			t.advance()
			t.state = stateSelfClosingStartTag
			return nil
		}
		if ch == '>' {
			t.advance()
			t.state = stateData
			t.setRawTagState()
			tok := t.current
			return &tok
		}
		t.curAttr = ""
		t.curVal = ""
		t.state = stateAttrName
		return nil

	// ----- ATTRIBUTE NAME -----
	case stateAttrName:
		if isWhitespace(ch) {
			t.advance()
			t.saveAttr()
			t.state = stateAfterAttrName
			return nil
		}
		if ch == '/' {
			t.advance()
			t.saveAttr()
			t.state = stateSelfClosingStartTag
			return nil
		}
		if ch == '=' {
			t.advance()
			t.state = stateBeforeAttrValue
			return nil
		}
		if ch == '>' {
			t.advance()
			t.saveAttr()
			t.state = stateData
			t.setRawTagState()
			tok := t.current
			return &tok
		}
		t.advance()
		t.curAttr += string(unicode.ToLower(ch))
		return nil

	// ----- AFTER ATTRIBUTE NAME -----
	case stateAfterAttrName:
		if isWhitespace(ch) {
			t.advance()
			return nil
		}
		if ch == '/' {
			t.advance()
			t.state = stateSelfClosingStartTag
			return nil
		}
		if ch == '=' {
			t.advance()
			t.state = stateBeforeAttrValue
			return nil
		}
		if ch == '>' {
			t.advance()
			t.state = stateData
			t.setRawTagState()
			tok := t.current
			return &tok
		}
		t.curAttr = ""
		t.curVal = ""
		t.state = stateAttrName
		return nil

	// ----- BEFORE ATTRIBUTE VALUE -----
	case stateBeforeAttrValue:
		if isWhitespace(ch) {
			t.advance()
			return nil
		}
		if ch == '"' {
			t.advance()
			t.state = stateAttrValueDouble
			return nil
		}
		if ch == '\'' {
			t.advance()
			t.state = stateAttrValueSingle
			return nil
		}
		if ch == '>' {
			t.advance()
			t.saveAttr()
			t.state = stateData
			t.setRawTagState()
			tok := t.current
			return &tok
		}
		t.state = stateAttrValueUnquoted
		return nil

	// ----- ATTRIBUTE VALUE (double quoted) -----
	case stateAttrValueDouble:
		if ch == '"' {
			t.advance()
			t.saveAttr()
			t.state = stateAfterAttrValueQuoted
			return nil
		}
		if ch == '&' {
			t.advance()
			t.curVal += t.consumeCharRef()
			return nil
		}
		t.advance()
		t.curVal += string(ch)
		return nil

	// ----- ATTRIBUTE VALUE (single quoted) -----
	case stateAttrValueSingle:
		if ch == '\'' {
			t.advance()
			t.saveAttr()
			t.state = stateAfterAttrValueQuoted
			return nil
		}
		if ch == '&' {
			t.advance()
			t.curVal += t.consumeCharRef()
			return nil
		}
		t.advance()
		t.curVal += string(ch)
		return nil

	// ----- ATTRIBUTE VALUE (unquoted) -----
	case stateAttrValueUnquoted:
		if isWhitespace(ch) {
			t.advance()
			t.saveAttr()
			t.state = stateBeforeAttrName
			return nil
		}
		if ch == '>' {
			t.advance()
			t.saveAttr()
			t.state = stateData
			t.setRawTagState()
			tok := t.current
			return &tok
		}
		if ch == '&' {
			t.advance()
			t.curVal += t.consumeCharRef()
			return nil
		}
		t.advance()
		t.curVal += string(ch)
		return nil

	// ----- AFTER ATTRIBUTE VALUE (quoted) -----
	case stateAfterAttrValueQuoted:
		if isWhitespace(ch) {
			t.advance()
			t.state = stateBeforeAttrName
			return nil
		}
		if ch == '/' {
			t.advance()
			t.state = stateSelfClosingStartTag
			return nil
		}
		if ch == '>' {
			t.advance()
			t.state = stateData
			t.setRawTagState()
			tok := t.current
			return &tok
		}
		t.state = stateBeforeAttrName
		return nil

	// ----- SELF CLOSING START TAG -----
	case stateSelfClosingStartTag:
		if ch == '>' {
			t.advance()
			t.current.SelfClosing = true
			if t.current.Type == TokenStartTag {
				t.current.Type = TokenSelfClosingTag
			}
			t.state = stateData
			tok := t.current
			return &tok
		}
		t.state = stateBeforeAttrName
		return nil

	// ----- MARKUP DECLARATION OPEN -----
	case stateMarkupDeclarationOpen:
		rest := string(t.input[t.pos:])
		if strings.HasPrefix(rest, "--") {
			t.pos += 2
			t.current = Token{Type: TokenComment}
			t.state = stateCommentStart
			return nil
		}
		if strings.HasPrefix(strings.ToUpper(rest), "DOCTYPE") {
			t.pos += 7
			t.current = Token{Type: TokenDoctype}
			t.state = stateDoctype
			return nil
		}
		// bogus comment
		t.state = stateData
		return nil

	// ----- COMMENT START -----
	case stateCommentStart:
		if ch == '-' {
			t.advance()
			t.state = stateCommentStartDash
			return nil
		}
		t.state = stateComment
		return nil

	case stateCommentStartDash:
		if ch == '-' {
			t.advance()
			t.state = stateCommentEnd
			return nil
		}
		t.current.Data += "-"
		t.state = stateComment
		return nil

	case stateComment:
		if ch == '-' {
			t.advance()
			t.state = stateCommentEndDash
			return nil
		}
		t.advance()
		t.current.Data += string(ch)
		return nil

	case stateCommentEndDash:
		if ch == '-' {
			t.advance()
			t.state = stateCommentEnd
			return nil
		}
		t.current.Data += "-"
		t.state = stateComment
		return nil

	case stateCommentEnd:
		if ch == '>' {
			t.advance()
			t.state = stateData
			tok := t.current
			return &tok
		}
		if ch == '-' {
			t.advance()
			t.current.Data += "-"
			return nil
		}
		t.current.Data += "--"
		t.state = stateComment
		return nil

	// ----- DOCTYPE -----
	case stateDoctype:
		if isWhitespace(ch) {
			t.advance()
			t.state = stateBeforeDoctypeName
			return nil
		}
		t.state = stateBeforeDoctypeName
		return nil

	case stateBeforeDoctypeName:
		if isWhitespace(ch) {
			t.advance()
			return nil
		}
		if ch == '>' {
			t.advance()
			t.state = stateData
			tok := t.current
			return &tok
		}
		t.state = stateDoctypeName
		return nil

	case stateDoctypeName:
		if isWhitespace(ch) {
			t.advance()
			// skip until >
			for t.pos < len(t.input) && t.input[t.pos] != '>' {
				t.pos++
			}
			if t.pos < len(t.input) {
				t.pos++ // consume >
			}
			t.state = stateData
			tok := t.current
			return &tok
		}
		if ch == '>' {
			t.advance()
			t.state = stateData
			tok := t.current
			return &tok
		}
		t.advance()
		t.current.Data += string(unicode.ToLower(ch))
		return nil
	}

	t.advance()
	return nil
}

func (t *Tokenizer) saveAttr() {
	if t.curAttr != "" {
		if _, exists := t.current.Attrs[t.curAttr]; !exists {
			t.current.AttrOrder = append(t.current.AttrOrder, t.curAttr)
		}
		t.current.Attrs[t.curAttr] = t.curVal
	}
	t.curAttr = ""
	t.curVal = ""
}

func (t *Tokenizer) setRawTagState() {
	switch t.current.TagName {
	case "script":
		t.state = stateScriptData
		t.rawTag = "script"
	case "style":
		t.state = stateRawText
		t.rawTag = "style"
	case "textarea":
		t.state = stateRCDATA
		t.rawTag = "textarea"
	case "title":
		t.state = stateRCDATA
		t.rawTag = "title"
	}
}

// consumeCharRef reads a character reference starting after '&'.
func (t *Tokenizer) consumeCharRef() string {
	if t.pos >= len(t.input) {
		return "&"
	}
	start := t.pos
	if t.input[t.pos] == '#' {
		t.pos++
		if t.pos >= len(t.input) {
			t.pos = start
			return "&"
		}
		// Numeric reference
		hex := false
		if t.input[t.pos] == 'x' || t.input[t.pos] == 'X' {
			hex = true
			t.pos++
		}
		numStart := t.pos
		for t.pos < len(t.input) && isHexDigit(t.input[t.pos], hex) {
			t.pos++
		}
		if t.pos > numStart {
			numStr := string(t.input[numStart:t.pos])
			var code int64
			if hex {
				_, err := parseIntHex(numStr, &code)
				if err != nil {
					t.pos = start
					return "&"
				}
			} else {
				for _, r := range numStr {
					code = code*10 + int64(r-'0')
				}
			}
			if t.pos < len(t.input) && t.input[t.pos] == ';' {
				t.pos++
			}
			if code > 0 && code <= 0x10FFFF {
				return string(rune(code))
			}
		}
		t.pos = start
		return "&"
	}

	// Named reference
	end := t.pos
	for end < len(t.input) && (unicode.IsLetter(t.input[end]) || unicode.IsDigit(t.input[end])) {
		end++
	}
	name := string(t.input[t.pos:end])
	if end < len(t.input) && t.input[end] == ';' {
		end++
	}
	if r, ok := namedEntities[name]; ok {
		t.pos = end
		return r
	}
	t.pos = start
	return "&"
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

func isHexDigit(r rune, hex bool) bool {
	if r >= '0' && r <= '9' {
		return true
	}
	if hex {
		return (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
	}
	return false
}

func parseIntHex(s string, out *int64) (int, error) {
	var v int64
	for _, ch := range s {
		v <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			v += int64(ch - '0')
		case ch >= 'a' && ch <= 'f':
			v += int64(ch-'a') + 10
		case ch >= 'A' && ch <= 'F':
			v += int64(ch-'A') + 10
		default:
			return 0, &parseError{}
		}
	}
	*out = v
	return len(s), nil
}

type parseError struct{}

func (e *parseError) Error() string { return "parse error" }

// decodeEntities decodes HTML entities in a string.
func decodeEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	t := NewTokenizer("<span>" + s + "</span>")
	var out strings.Builder
	for {
		tok := t.Next()
		if tok.Type == TokenEOF {
			break
		}
		if tok.Type == TokenChar {
			out.WriteString(tok.Data)
		}
	}
	return out.String()
}

// namedEntities maps HTML entity names to their Unicode representations.
var namedEntities = map[string]string{
	"amp":   "&",
	"lt":    "<",
	"gt":    ">",
	"quot":  "\"",
	"apos":  "'",
	"nbsp":  "\u00a0",
	"copy":  "©",
	"reg":   "®",
	"trade": "™",
	"mdash": "—",
	"ndash": "–",
	"laquo": "«",
	"raquo": "»",
	"ldquo": "\u201C",
	"rdquo": "\u201D",
	"lsquo": "\u2018",
	"rsquo": "\u2019",
	"hellip": "…",
	"bull":  "•",
	"middot": "·",
	"euro":  "€",
	"pound": "£",
	"yen":   "¥",
	"cent":  "¢",
	"deg":   "°",
	"plusmn": "±",
	"times":  "×",
	"divide": "÷",
	"frac12": "½",
	"frac14": "¼",
	"frac34": "¾",
	"sup2":   "²",
	"sup3":   "³",
	"alpha":  "α",
	"beta":   "β",
	"gamma":  "γ",
	"delta":  "δ",
	"pi":     "π",
	"sigma":  "σ",
	"omega":  "ω",
	"infin":  "∞",
	"sum":    "∑",
	"prod":   "∏",
	"radic":  "√",
	"minus":  "−",
	"lowast": "∗",
	"prop":   "∝",
	"empty":  "∅",
	"isin":   "∈",
	"notin":  "∉",
	"and":    "∧",
	"or":     "∨",
	"cap":    "∩",
	"cup":    "∪",
	"int":    "∫",
	"there4": "∴",
	"sim":    "∼",
	"cong":   "≅",
	"asymp":  "≈",
	"ne":     "≠",
	"equiv":  "≡",
	"le":     "≤",
	"ge":     "≥",
	"sub":    "⊂",
	"sup":    "⊃",
	"sube":   "⊆",
	"supe":   "⊇",
	"oplus":  "⊕",
	"otimes": "⊗",
	"perp":   "⊥",
	"sdot":   "⋅",
	"larr":   "←",
	"uarr":   "↑",
	"rarr":   "→",
	"darr":   "↓",
	"harr":   "↔",
	"crarr":  "↵",
	"lArr":   "⇐",
	"uArr":   "⇑",
	"rArr":   "⇒",
	"dArr":   "⇓",
	"hArr":   "⇔",
	"forall": "∀",
	"part":   "∂",
	"exist":  "∃",
	"nabla":  "∇",
	"iota":   "ι",
	"kappa":  "κ",
	"lambda": "λ",
	"mu":     "μ",
	"nu":     "ν",
	"xi":     "ξ",
	"omicron": "ο",
	"rho":    "ρ",
	"tau":    "τ",
	"upsilon": "υ",
	"phi":    "φ",
	"chi":    "χ",
	"psi":    "ψ",
	"zeta":   "ζ",
	"eta":    "η",
	"theta":  "θ",
	"epsilon": "ε",
	"szlig":  "ß",
	"agrave": "à",
	"aacute": "á",
	"acirc":  "â",
	"atilde": "ã",
	"auml":   "ä",
	"aring":  "å",
	"aelig":  "æ",
	"ccedil": "ç",
	"egrave": "è",
	"eacute": "é",
	"ecirc":  "ê",
	"euml":   "ë",
	"igrave": "ì",
	"iacute": "í",
	"icirc":  "î",
	"iuml":   "ï",
	"eth":    "ð",
	"ntilde": "ñ",
	"ograve": "ò",
	"oacute": "ó",
	"ocirc":  "ô",
	"otilde": "õ",
	"ouml":   "ö",
	"ugrave": "ù",
	"uacute": "ú",
	"ucirc":  "û",
	"uuml":   "ü",
	"yacute": "ý",
	"thorn":  "þ",
	"yuml":   "ÿ",
	"Agrave": "À",
	"Aacute": "Á",
	"Acirc":  "Â",
	"Atilde": "Ã",
	"Auml":   "Ä",
	"Aring":  "Å",
	"AElig":  "Æ",
	"Ccedil": "Ç",
	"Egrave": "È",
	"Eacute": "É",
	"Ecirc":  "Ê",
	"Euml":   "Ë",
	"Igrave": "Ì",
	"Iacute": "Í",
	"Icirc":  "Î",
	"Iuml":   "Ï",
	"ETH":    "Ð",
	"Ntilde": "Ñ",
	"Ograve": "Ò",
	"Oacute": "Ó",
	"Ocirc":  "Ô",
	"Otilde": "Õ",
	"Ouml":   "Ö",
	"Ugrave": "Ù",
	"Uacute": "Ú",
	"Ucirc":  "Û",
	"Uuml":   "Ü",
	"Yacute": "Ý",
	"THORN":  "Þ",
}
