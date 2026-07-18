package argwild

import (
	"strconv"
	"strings"
)

// parser is a single-use recursive-descent scanner over a command-line string.
type parser struct {
	src        []rune
	pos        int
	optionsEnd bool // set once "--" has been seen; everything after is operands.
}

// ParseString parses a raw command-line string into a Result.
//
// This is the full-fidelity entry point: it understands quoting, the spacing
// rules, comma-chained values, and parenthesized PSL blocks directly from the
// text, so it is the right choice for command lines a user typed into a prompt.
func ParseString(s string) (*Result, error) {
	p := &parser{src: []rune(s)}
	return p.parse()
}

func (p *parser) parse() (*Result, error) {
	res := &Result{}
	var cur []Switch // the argument set currently being accumulated.

	flush := func() {
		if len(cur) > 0 {
			res.Stanzas = append(res.Stanzas, &ArgSet{Switches: cur})
			cur = nil
		}
	}

	for {
		p.skipSpaces()
		if p.eof() {
			break
		}

		// After "--", every remaining token is an operand regardless of lead.
		if !p.optionsEnd && p.isSwitchStart() {
			sw, isMarker, err := p.parseSwitch()
			if err != nil {
				return nil, err
			}
			if isMarker {
				// "--" end-of-options marker: change mode, emit nothing.
				continue
			}
			cur = append(cur, sw)
			continue
		}

		// Operand: a single standalone value closes the current arg set.
		val, err := p.parseValue(true)
		if err != nil {
			return nil, err
		}
		flush()
		res.Stanzas = append(res.Stanzas, &Operand{Value: val})
	}

	flush()
	return res, nil
}

// isSwitchStart reports whether the cursor is at the beginning of a switch. A
// leading '-' or '+' introduces a switch, except that a bare '+' not followed
// by a name character or digit is treated as an operand.
func (p *parser) isSwitchStart() bool {
	c := p.peek()
	if c != '-' && c != '+' {
		return false
	}
	// A lone "-" or "+" (followed by whitespace or end of input) is an operand,
	// not a switch — for example the conventional "-" stdin marker.
	n := p.at(p.pos + 1)
	return n != 0 && !isSpace(n)
}

// parseSwitch parses one switch beginning at the cursor. It returns isMarker
// true when the token was the bare "--" end-of-options marker.
func (p *parser) parseSwitch() (sw Switch, isMarker bool, err error) {
	// Read the lead.
	switch {
	case p.peek() == '-' && p.at(p.pos+1) == '-':
		p.advance()
		p.advance()
		// Bare "--" (followed by space or eof) is the end-of-options marker.
		if p.eof() || isSpace(p.peek()) {
			p.optionsEnd = true
			return Switch{}, true, nil
		}
		sw.Lead = LeadLong
		sw.Name = p.readLongName()
	case p.peek() == '-':
		p.advance()
		sw.Lead = LeadShort
		sw.Name = p.readShortName()
	case p.peek() == '+':
		p.advance()
		sw.Lead = LeadPlus
		if isDigit(p.peek()) {
			// "+32": empty name, value-only. Fall through to value parsing.
			sw.Name = ""
		} else {
			sw.Name = p.readShortName()
		}
	}

	if err := p.readStateAndValues(&sw); err != nil {
		return Switch{}, false, err
	}
	return sw, false, nil
}

// readShortName reads a single-character option name. It returns "" if the
// cursor is already at a value boundary.
func (p *parser) readShortName() string {
	c := p.peek()
	// These characters begin a value rather than name the switch, so the name
	// is empty and value parsing takes over (e.g. "+(psl)", `-"x"`).
	if c == 0 || isSpace(c) || c == '=' || c == '"' || c == '\'' || c == '(' || c == ',' {
		return ""
	}
	p.advance()
	return string(c)
}

// readLongName reads a multi-character long-option name: letters, digits, and
// dashes up to the first '=', quote, '(' or whitespace.
func (p *parser) readLongName() string {
	start := p.pos
	for !p.eof() {
		c := p.peek()
		if isSpace(c) || c == '=' || c == '"' || c == '\'' || c == '(' {
			break
		}
		// A trailing "+" or "-" that terminates the token is a polarity marker,
		// not part of the name.
		if (c == '+' || c == '-') && p.isTokenEndAt(p.pos+1) {
			break
		}
		p.advance()
	}
	return string(p.src[start:p.pos])
}

// isTokenEndAt reports whether position i is at the end of the current token
// (whitespace, end of input, or a comma).
func (p *parser) isTokenEndAt(i int) bool {
	c := p.at(i)
	return c == 0 || isSpace(c) || c == ','
}

// readStateAndValues fills in the switch's polarity and/or values starting at
// the cursor, which sits immediately after the name.
func (p *parser) readStateAndValues(sw *Switch) error {
	c := p.peek()

	// Explicit polarity: "-o+" / "-o-", where the sign terminates the token.
	if (c == '+' || c == '-') && p.isTokenEndAt(p.pos+1) {
		p.advance()
		if c == '+' {
			sw.State = StateOn
		} else {
			sw.State = StateOff
		}
		return nil
	}

	// Attached value via "=".
	if c == '=' {
		p.advance()
		return p.readValueList(sw, false)
	}

	// Attached value with no separator: "-oArg", "-o2", `-o"x"`, "-o(psl)",
	// and the "+32" digit form.
	if c != 0 && !isSpace(c) {
		return p.readValueList(sw, false)
	}

	// A space follows. A value binds only if the next token is quoted, numeric,
	// or a PSL block; a bare word is left for the next stanza.
	if isSpace(c) {
		save := p.pos
		p.skipSpaces()
		if p.startsSpaceBindableValue() {
			return p.readValueList(sw, true)
		}
		// Not a bindable value: rewind so the token is parsed independently.
		p.pos = save
	}

	// Nothing attached: a bare switch.
	sw.State = StateBare
	return nil
}

// startsSpaceBindableValue reports whether the cursor (already past spaces) is
// at a value that a space is permitted to bind to: a quote, a PSL block, or a
// fully numeric token.
func (p *parser) startsSpaceBindableValue() bool {
	c := p.peek()
	if c == '"' || c == '\'' || c == '(' {
		return true
	}
	if isDigit(c) {
		// Only a wholly numeric token qualifies; "12abc" is a bare word.
		tok := p.previewBareToken()
		return isNumber(tok)
	}
	return false
}

// readValueList parses one value at the cursor and any comma-chained
// continuations. If firstIsSpaceBound is false, the first value is attached
// (bare words are allowed); continuation values after a comma always allow bare
// words, because a comma binds whatever follows.
func (p *parser) readValueList(sw *Switch, firstIsSpaceBound bool) error {
	v, ok, err := p.parseValueAllowingBare(true)
	if err != nil {
		return err
	}
	if !ok {
		// e.g. "-o=" with nothing after: treat as bare.
		sw.State = StateBare
		return nil
	}
	sw.State = StateValued
	sw.Values = append(sw.Values, v)

	for {
		save := p.pos
		p.skipSpaces()
		if p.peek() != ',' {
			p.pos = save
			break
		}
		p.advance() // consume the comma.
		p.skipSpaces()
		nv, ok, err := p.parseValueAllowingBare(true)
		if err != nil {
			return err
		}
		if !ok {
			// Dangling trailing comma: ignore.
			break
		}
		sw.Values = append(sw.Values, nv)
	}
	return nil
}

// parseValueAllowingBare parses a single value at the cursor. When allowBare is
// true, a bare word is accepted; otherwise only quoted, numeric, or PSL values
// are. It returns ok=false if there is no value to parse.
func (p *parser) parseValueAllowingBare(allowBare bool) (Value, bool, error) {
	c := p.peek()
	if c == 0 || isSpace(c) || c == ',' {
		return Value{}, false, nil
	}
	if !allowBare && c != '"' && c != '\'' && c != '(' && !isDigit(c) {
		return Value{}, false, nil
	}
	v, err := p.parseValue(allowBare)
	if err != nil {
		return Value{}, false, err
	}
	return v, true, nil
}

// parseValue parses a single value at the cursor: a quoted string, a PSL block,
// a number, or (when allowBare) a bare word.
func (p *parser) parseValue(allowBare bool) (Value, error) {
	c := p.peek()
	switch {
	case c == '"' || c == '\'':
		s, err := p.readQuoted()
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: KindQuoted, Text: s}, nil
	case c == '(':
		return p.readPSL()
	}

	tok := p.readBareToken()
	if tok == "" {
		return Value{}, &ParseError{Pos: p.pos, Msg: "expected a value"}
	}
	if isNumber(tok) {
		return numberValue(tok), nil
	}
	if !allowBare {
		// Should not happen given caller guards, but stay safe.
		return Value{Kind: KindBare, Text: tok}, nil
	}
	return Value{Kind: KindBare, Text: tok}, nil
}

// numberValue builds a KindNumber Value from a numeric token.
func numberValue(tok string) Value {
	if i, err := strconv.ParseInt(tok, 10, 64); err == nil {
		return Value{Kind: KindNumber, Text: tok, Int: i}
	}
	f, _ := strconv.ParseFloat(tok, 64)
	return Value{Kind: KindNumber, Text: tok, Float: f, IsFloat: true}
}

// readQuoted reads a "..." or '...' string, honoring \\ and \" (or \') escapes.
// The cursor must be on the opening quote.
func (p *parser) readQuoted() (string, error) {
	quote := p.advance()
	var b strings.Builder
	for !p.eof() {
		c := p.advance()
		if c == '\\' {
			n := p.peek()
			if n == quote || n == '\\' {
				b.WriteRune(p.advance())
				continue
			}
			b.WriteRune('\\')
			continue
		}
		if c == quote {
			return b.String(), nil
		}
		b.WriteRune(c)
	}
	return "", &ParseError{Pos: p.pos, Msg: "unterminated quoted string"}
}

// readPSL reads a balanced "(...)" block, parses it with pawscript, and returns
// a KindPSL Value. Parentheses and quotes inside the block are respected.
func (p *parser) readPSL() (Value, error) {
	start := p.pos
	inner, err := p.readBalancedParens()
	if err != nil {
		return Value{}, err
	}
	parsed, err := parsePSLBlock(inner)
	if err != nil {
		return Value{}, &ParseError{Pos: start, Msg: "invalid PSL block: " + err.Error()}
	}
	return Value{Kind: KindPSL, Text: inner, PSL: parsed}, nil
}

// readBalancedParens consumes a parenthesized block starting at the cursor and
// returns its inner text (without the outer parentheses). Nested parentheses
// and quoted strings inside are tracked so that quotes may contain parens.
func (p *parser) readBalancedParens() (string, error) {
	p.advance() // consume '('.
	start := p.pos
	depth := 1
	for !p.eof() {
		c := p.peek()
		switch c {
		case '"', '\'':
			if _, err := p.readQuoted(); err != nil {
				return "", err
			}
			continue
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				inner := string(p.src[start:p.pos])
				p.advance() // consume ')'.
				return inner, nil
			}
		}
		p.advance()
	}
	return "", &ParseError{Pos: start, Msg: "unterminated PSL block"}
}

// readBareToken consumes and returns a run of characters up to the next
// whitespace or comma. Quotes and parens are not treated specially here; they
// only introduce quoted/PSL values when they begin a value.
func (p *parser) readBareToken() string {
	start := p.pos
	for !p.eof() {
		c := p.peek()
		if isSpace(c) || c == ',' {
			break
		}
		p.advance()
	}
	return string(p.src[start:p.pos])
}

// previewBareToken returns the bare token at the cursor without consuming it.
func (p *parser) previewBareToken() string {
	start := p.pos
	i := start
	for i < len(p.src) {
		c := p.src[i]
		if isSpace(c) || c == ',' {
			break
		}
		i++
	}
	return string(p.src[start:i])
}

// --- low-level cursor helpers ---

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) peek() rune { return p.at(p.pos) }

func (p *parser) at(i int) rune {
	if i < 0 || i >= len(p.src) {
		return 0
	}
	return p.src[i]
}

func (p *parser) advance() rune {
	c := p.at(p.pos)
	p.pos++
	return c
}

func (p *parser) skipSpaces() bool {
	skipped := false
	for !p.eof() && isSpace(p.peek()) {
		p.advance()
		skipped = true
	}
	return skipped
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// isNumber reports whether s is a decimal integer or float, optionally signed.
func isNumber(s string) bool {
	if s == "" {
		return false
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		// Reject forms ParseFloat accepts but that are not plain decimals
		// (e.g. "0x1p-2", "Inf", "NaN").
		for _, r := range s {
			if !isDigit(r) && r != '.' && r != '-' && r != '+' && r != 'e' && r != 'E' {
				return false
			}
		}
		return true
	}
	return false
}

// ParseError describes a failure to parse a command line, with the rune offset
// at which the problem was detected.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string {
	return "argwild: " + e.Msg + " at position " + strconv.Itoa(e.Pos)
}
