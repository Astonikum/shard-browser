// Package css implements CSS tokenization, parsing, and the style cascade.
package css

import (
	"strings"
	"unicode"
)

// TokType identifies the type of a CSS token.
type TokType int

const (
	TokIdent      TokType = iota // identifier: foo, background-color
	TokFunction                  // foo(
	TokAtKeyword                 // @media
	TokHash                      // #foo
	TokString                    // "foo" or 'foo'
	TokURL                       // url(...)
	TokDelim                     // single character delimiter
	TokNumber                    // 42
	TokPercentage                // 42%
	TokDimension                 // 42px
	TokWhitespace                // space/tab/newline
	TokCDO                       // <!--
	TokCDC                       // -->
	TokColon                     // :
	TokSemicolon                 // ;
	TokComma                     // ,
	TokLBracket                  // [
	TokRBracket                  // ]
	TokLParen                    // (
	TokRParen                    // )
	TokLBrace                    // {
	TokRBrace                    // }
	TokEOF
	TokBadString
	TokBadURL
)

// Tok is a CSS token.
type Tok struct {
	Type    TokType
	Value   string  // string representation
	Number  float64 // numeric value (for Number, Percentage, Dimension)
	Unit    string  // unit for Dimension (px, em, etc.)
	IsID    bool    // for Hash: whether it's a valid id
}

// Tokenizer tokenizes a CSS string.
type Tokenizer struct {
	input []rune
	pos   int
}

// NewTokenizer creates a new CSS tokenizer.
func NewTokenizer(input string) *Tokenizer {
	return &Tokenizer{input: []rune(input)}
}

// All returns all tokens.
func (t *Tokenizer) All() []Tok {
	var toks []Tok
	for {
		tok := t.Next()
		toks = append(toks, tok)
		if tok.Type == TokEOF {
			break
		}
	}
	return toks
}

// Next returns the next token.
func (t *Tokenizer) Next() Tok {
	if t.pos >= len(t.input) {
		return Tok{Type: TokEOF}
	}

	ch := t.input[t.pos]

	// Whitespace
	if isWSRune(ch) {
		return t.consumeWhitespace()
	}

	// String
	if ch == '"' || ch == '\'' {
		return t.consumeString(ch)
	}

	// Hash
	if ch == '#' {
		t.pos++
		name := t.consumeName()
		if name == "" {
			return Tok{Type: TokDelim, Value: "#"}
		}
		isID := isIdentStart(t.input[t.pos-len([]rune(name))-1:])
		_ = isID
		return Tok{Type: TokHash, Value: name, IsID: isIdentStart2([]rune(name))}
	}

	// Number, percentage, dimension
	if isDigit(ch) || (ch == '.' && t.pos+1 < len(t.input) && isDigit(t.input[t.pos+1])) ||
		((ch == '+' || ch == '-') && t.pos+1 < len(t.input) &&
			(isDigit(t.input[t.pos+1]) || (t.input[t.pos+1] == '.' && t.pos+2 < len(t.input) && isDigit(t.input[t.pos+2])))) {
		return t.consumeNumericToken()
	}

	// Ident-like
	if isIdentStartRune(ch) {
		return t.consumeIdentLike()
	}

	// @ keyword
	if ch == '@' {
		t.pos++
		name := t.consumeName()
		if name != "" {
			return Tok{Type: TokAtKeyword, Value: name}
		}
		return Tok{Type: TokDelim, Value: "@"}
	}

	// URL function handled in ident-like

	// Single char tokens
	t.pos++
	switch ch {
	case '(':
		return Tok{Type: TokLParen, Value: "("}
	case ')':
		return Tok{Type: TokRParen, Value: ")"}
	case '[':
		return Tok{Type: TokLBracket, Value: "["}
	case ']':
		return Tok{Type: TokRBracket, Value: "]"}
	case '{':
		return Tok{Type: TokLBrace, Value: "{"}
	case '}':
		return Tok{Type: TokRBrace, Value: "}"}
	case ':':
		return Tok{Type: TokColon, Value: ":"}
	case ';':
		return Tok{Type: TokSemicolon, Value: ";"}
	case ',':
		return Tok{Type: TokComma, Value: ","}
	case '/':
		// Check for comment
		if t.pos < len(t.input) && t.input[t.pos] == '*' {
			t.pos++
			t.consumeComment()
			return t.Next()
		}
		return Tok{Type: TokDelim, Value: "/"}
	}

	return Tok{Type: TokDelim, Value: string(ch)}
}

func (t *Tokenizer) consumeWhitespace() Tok {
	start := t.pos
	for t.pos < len(t.input) && isWSRune(t.input[t.pos]) {
		t.pos++
	}
	return Tok{Type: TokWhitespace, Value: string(t.input[start:t.pos])}
}

func (t *Tokenizer) consumeString(quote rune) Tok {
	t.pos++ // consume opening quote
	var sb strings.Builder
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		if ch == quote {
			t.pos++
			return Tok{Type: TokString, Value: sb.String()}
		}
		if ch == '\n' {
			return Tok{Type: TokBadString}
		}
		if ch == '\\' {
			t.pos++
			if t.pos >= len(t.input) {
				break
			}
			if t.input[t.pos] == '\n' {
				t.pos++
				continue
			}
			sb.WriteRune(t.consumeEscape())
			continue
		}
		t.pos++
		sb.WriteRune(ch)
	}
	return Tok{Type: TokString, Value: sb.String()}
}

func (t *Tokenizer) consumeEscape() rune {
	// Assume t.pos is after backslash
	if t.pos >= len(t.input) {
		return 0xFFFD
	}
	ch := t.input[t.pos]
	if isHexRune(ch) {
		var val rune
		count := 0
		for t.pos < len(t.input) && isHexRune(t.input[t.pos]) && count < 6 {
			val = val*16 + hexVal(t.input[t.pos])
			t.pos++
			count++
		}
		if t.pos < len(t.input) && isWSRune(t.input[t.pos]) {
			t.pos++
		}
		if val == 0 || val > 0x10FFFF {
			return 0xFFFD
		}
		return val
	}
	t.pos++
	return ch
}

func (t *Tokenizer) consumeName() string {
	var sb strings.Builder
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		if isNameRune(ch) {
			t.pos++
			sb.WriteRune(ch)
		} else if ch == '\\' && t.pos+1 < len(t.input) {
			t.pos++
			sb.WriteRune(t.consumeEscape())
		} else {
			break
		}
	}
	return sb.String()
}

func (t *Tokenizer) consumeNumericToken() Tok {
	repr, num := t.consumeNumber()
	if t.pos < len(t.input) && t.input[t.pos] == '%' {
		t.pos++
		return Tok{Type: TokPercentage, Value: repr, Number: num}
	}
	if t.pos < len(t.input) && isIdentStartRune(t.input[t.pos]) {
		unit := t.consumeName()
		return Tok{Type: TokDimension, Value: repr, Number: num, Unit: strings.ToLower(unit)}
	}
	return Tok{Type: TokNumber, Value: repr, Number: num}
}

func (t *Tokenizer) consumeNumber() (string, float64) {
	var sb strings.Builder
	if t.pos < len(t.input) && (t.input[t.pos] == '+' || t.input[t.pos] == '-') {
		sb.WriteRune(t.input[t.pos])
		t.pos++
	}
	for t.pos < len(t.input) && isDigit(t.input[t.pos]) {
		sb.WriteRune(t.input[t.pos])
		t.pos++
	}
	if t.pos+1 < len(t.input) && t.input[t.pos] == '.' && isDigit(t.input[t.pos+1]) {
		sb.WriteRune('.')
		t.pos++
		for t.pos < len(t.input) && isDigit(t.input[t.pos]) {
			sb.WriteRune(t.input[t.pos])
			t.pos++
		}
	}
	if t.pos+1 < len(t.input) && (t.input[t.pos] == 'e' || t.input[t.pos] == 'E') {
		next := t.input[t.pos+1]
		if isDigit(next) || ((next == '+' || next == '-') && t.pos+2 < len(t.input) && isDigit(t.input[t.pos+2])) {
			sb.WriteRune(t.input[t.pos])
			t.pos++
			if t.input[t.pos] == '+' || t.input[t.pos] == '-' {
				sb.WriteRune(t.input[t.pos])
				t.pos++
			}
			for t.pos < len(t.input) && isDigit(t.input[t.pos]) {
				sb.WriteRune(t.input[t.pos])
				t.pos++
			}
		}
	}
	s := sb.String()
	var num float64
	parseFloat(s, &num)
	return s, num
}

func (t *Tokenizer) consumeIdentLike() Tok {
	name := t.consumeName()
	if t.pos < len(t.input) && t.input[t.pos] == '(' {
		t.pos++
		lower := strings.ToLower(name)
		if lower == "url" {
			return t.consumeURL()
		}
		return Tok{Type: TokFunction, Value: lower}
	}
	return Tok{Type: TokIdent, Value: strings.ToLower(name)}
}

func (t *Tokenizer) consumeURL() Tok {
	// Skip whitespace
	for t.pos < len(t.input) && isWSRune(t.input[t.pos]) {
		t.pos++
	}
	if t.pos >= len(t.input) {
		return Tok{Type: TokURL, Value: ""}
	}
	if t.input[t.pos] == '"' || t.input[t.pos] == '\'' {
		str := t.consumeString(t.input[t.pos])
		// skip whitespace and closing paren
		for t.pos < len(t.input) && isWSRune(t.input[t.pos]) {
			t.pos++
		}
		if t.pos < len(t.input) && t.input[t.pos] == ')' {
			t.pos++
		}
		return Tok{Type: TokURL, Value: str.Value}
	}
	var sb strings.Builder
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		if ch == ')' {
			t.pos++
			break
		}
		if isWSRune(ch) {
			for t.pos < len(t.input) && isWSRune(t.input[t.pos]) {
				t.pos++
			}
			if t.pos < len(t.input) && t.input[t.pos] == ')' {
				t.pos++
			}
			break
		}
		t.pos++
		sb.WriteRune(ch)
	}
	return Tok{Type: TokURL, Value: sb.String()}
}

func (t *Tokenizer) consumeComment() {
	for t.pos < len(t.input)-1 {
		if t.input[t.pos] == '*' && t.input[t.pos+1] == '/' {
			t.pos += 2
			return
		}
		t.pos++
	}
}

// Helper functions

func isWSRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isHexRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func hexVal(r rune) rune {
	switch {
	case r >= '0' && r <= '9':
		return r - '0'
	case r >= 'a' && r <= 'f':
		return r - 'a' + 10
	case r >= 'A' && r <= 'F':
		return r - 'A' + 10
	}
	return 0
}

func isNameRune(r rune) bool {
	return isIdentStartRune(r) || isDigit(r) || r == '-'
}

func isIdentStartRune(r rune) bool {
	return unicode.IsLetter(r) || r == '_' || r == '-' || r > 0x80
}

func isIdentStart(runes []rune) bool {
	if len(runes) == 0 {
		return false
	}
	return isIdentStartRune(runes[0])
}

func isIdentStart2(runes []rune) bool {
	if len(runes) == 0 {
		return false
	}
	r := runes[0]
	return unicode.IsLetter(r) || r == '_' || r > 0x80
}

func parseFloat(s string, out *float64) {
	if s == "" {
		return
	}
	neg := false
	i := 0
	if i < len(s) && s[i] == '+' {
		i++
	} else if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	var intPart float64
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		intPart = intPart*10 + float64(s[i]-'0')
		i++
	}
	var fracPart float64
	var fracDiv float64 = 1
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			fracPart = fracPart*10 + float64(s[i]-'0')
			fracDiv *= 10
			i++
		}
	}
	result := intPart + fracPart/fracDiv
	if neg {
		result = -result
	}
	*out = result
}
