// Package argwild parses command lines into a reusable, structured form.
//
// argwild is built around three ideas:
//
//   - Switches. A switch is an option token led by "-" (short, single-character
//     name), "--" (long name), or "+" (a value-only or short-style token, used
//     for things like the "+32" line-number convention). Every switch records
//     whether it was bare, explicitly turned on ("-o+"), explicitly turned off
//     ("-o-"), or given one or more values.
//
//   - Operands. Any positional token that is not a switch. Operands act as
//     phase dividers, ffmpeg-style: global switches, an operand, switches that
//     apply to the next phase, another operand, and so on.
//
//   - Stanzas. The parse result is an ordered sequence of stanzas. Each stanza
//     is either an ArgSet (a run of consecutive switches) or an Operand. Order
//     is preserved exactly; argwild stays neutral about whether an ArgSet binds
//     to the operand before or after it — that is the caller's decision.
//
// A switch value may be a bare word, a quoted string, a number, or a PSL block
// written in parentheses. PSL (PawScript Serialized List) blocks are parsed
// with the pawscript library, and the entire parse tree can be handed back to
// pawscript for serialization via Result.ToPSL and Result.ToPSLString.
package argwild

import (
	"fmt"
	"strconv"
	"strings"
)

// Lead identifies which introducer began a switch.
type Lead int

const (
	// LeadShort is a single dash: "-o". The name is exactly one character and
	// everything after it attaches as a value.
	LeadShort Lead = iota
	// LeadLong is a double dash: "--option". The name may be multiple
	// characters; values attach via "=", a space (for quoted/numeric values),
	// or an immediately following quote or PSL block.
	LeadLong
	// LeadPlus is a plus sign: "+". When followed by a digit the name is empty
	// and the digits are a value (the "+32" line-number form). When followed by
	// a letter it behaves like a short switch: "+x=4".
	LeadPlus
)

// String renders the lead as it appears in source ("-", "--", or "+").
func (l Lead) String() string {
	switch l {
	case LeadShort:
		return "-"
	case LeadLong:
		return "--"
	case LeadPlus:
		return "+"
	default:
		return "?"
	}
}

// State describes how a switch was written with respect to on/off/value.
type State int

const (
	// StateBare is a switch with no polarity marker and no value: "-o".
	StateBare State = iota
	// StateOn is an explicitly enabled switch: "-o+".
	StateOn
	// StateOff is an explicitly disabled switch: "-o-".
	StateOff
	// StateValued is a switch carrying one or more values. Values is non-empty.
	StateValued
)

// String renders the state as a short label.
func (s State) String() string {
	switch s {
	case StateBare:
		return "bare"
	case StateOn:
		return "on"
	case StateOff:
		return "off"
	case StateValued:
		return "valued"
	default:
		return "?"
	}
}

// ValueKind identifies the lexical form of a Value.
type ValueKind int

const (
	// KindBare is an unquoted, non-numeric word: the "Arg" in "-oArg".
	KindBare ValueKind = iota
	// KindQuoted is a quoted string: the "Quoted Arg" in `-o"Quoted Arg"`.
	KindQuoted
	// KindNumber is an integer or floating-point literal: the 12 in "-o12".
	KindNumber
	// KindPSL is a parenthesized PSL block, parsed into PSL. The parsed result
	// is stored in Value.PSL.
	KindPSL
)

// String renders the kind as a short label.
func (k ValueKind) String() string {
	switch k {
	case KindBare:
		return "bare"
	case KindQuoted:
		return "quoted"
	case KindNumber:
		return "number"
	case KindPSL:
		return "psl"
	default:
		return "?"
	}
}

// Value is a single argument value attached to a switch or standing alone as an
// operand.
type Value struct {
	Kind ValueKind

	// Text holds the literal text for KindBare and KindQuoted values (without
	// the surrounding quotes), and the original source spelling for KindNumber.
	Text string

	// Int and Float hold the parsed number when Kind == KindNumber. IsFloat
	// reports which one is authoritative.
	Int     int64
	Float   float64
	IsFloat bool

	// PSL holds the parsed PSL structure when Kind == KindPSL. It is a
	// pawscript PSLList or PSLMap (both are Go maps/slices of interface{}), so
	// it embeds directly into a serialized parse tree.
	PSL interface{}
}

// Interface returns the value as a plain Go value suitable for embedding in a
// PSL structure: string for bare and quoted values, int64 or float64 for
// numbers, and the parsed PSL value for PSL blocks.
func (v Value) Interface() interface{} {
	switch v.Kind {
	case KindNumber:
		if v.IsFloat {
			return v.Float
		}
		return v.Int
	case KindPSL:
		return v.PSL
	default:
		return v.Text
	}
}

// AsString returns a human-readable rendering of the value. For PSL blocks it
// returns the block's serialized form.
func (v Value) AsString() string {
	switch v.Kind {
	case KindNumber:
		if v.IsFloat {
			return strconv.FormatFloat(v.Float, 'g', -1, 64)
		}
		return strconv.FormatInt(v.Int, 10)
	case KindPSL:
		return pslToString(v.PSL)
	default:
		return v.Text
	}
}

// String implements fmt.Stringer for debugging.
func (v Value) String() string {
	return fmt.Sprintf("%s(%s)", v.Kind, v.AsString())
}

// Switch is a single parsed option.
type Switch struct {
	Lead   Lead
	Name   string // "o", "option", or "" for the "+32" value-only form.
	State  State
	Values []Value
}

// IsBare reports whether the switch was written with no value and no polarity.
func (s Switch) IsBare() bool { return s.State == StateBare }

// IsOn reports whether the switch was explicitly enabled ("-o+").
func (s Switch) IsOn() bool { return s.State == StateOn }

// IsOff reports whether the switch was explicitly disabled ("-o-").
func (s Switch) IsOff() bool { return s.State == StateOff }

// HasValues reports whether the switch carries one or more values.
func (s Switch) HasValues() bool { return len(s.Values) > 0 }

// First returns the switch's first value, if any.
func (s Switch) First() (Value, bool) {
	if len(s.Values) == 0 {
		return Value{}, false
	}
	return s.Values[0], true
}

// String implements fmt.Stringer for debugging.
func (s Switch) String() string {
	var b strings.Builder
	b.WriteString(s.Lead.String())
	b.WriteString(s.Name)
	switch s.State {
	case StateOn:
		b.WriteString("+")
	case StateOff:
		b.WriteString("-")
	case StateValued:
		// Render "+32" (empty name) without a separator; otherwise "-o=val".
		if s.Name != "" {
			b.WriteString("=")
		}
		for i, v := range s.Values {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(v.AsString())
		}
	}
	return b.String()
}

// Stanza is one element of a parse result: either an *ArgSet or an *Operand.
// Use a type switch to distinguish them.
type Stanza interface {
	isStanza()
	// String renders the stanza for debugging.
	String() string
}

// ArgSet is a run of consecutive switches with no operand between them.
type ArgSet struct {
	Switches []Switch
}

func (*ArgSet) isStanza() {}

// String implements fmt.Stringer for debugging.
func (a *ArgSet) String() string {
	parts := make([]string, len(a.Switches))
	for i, s := range a.Switches {
		parts[i] = s.String()
	}
	return "args[" + strings.Join(parts, " ") + "]"
}

// Operand is a single positional value.
type Operand struct {
	Value Value
}

func (*Operand) isStanza() {}

// String implements fmt.Stringer for debugging.
func (o *Operand) String() string {
	return "operand(" + o.Value.AsString() + ")"
}

// Result is the top-level parse output: an ordered sequence of stanzas.
type Result struct {
	Stanzas []Stanza
}

// Operands returns every operand in order, ignoring argument sets. This is a
// convenience for callers that only care about the phase dividers.
func (r *Result) Operands() []*Operand {
	var out []*Operand
	for _, st := range r.Stanzas {
		if op, ok := st.(*Operand); ok {
			out = append(out, op)
		}
	}
	return out
}

// ArgSets returns every argument set in order, ignoring operands.
func (r *Result) ArgSets() []*ArgSet {
	var out []*ArgSet
	for _, st := range r.Stanzas {
		if as, ok := st.(*ArgSet); ok {
			out = append(out, as)
		}
	}
	return out
}

// String implements fmt.Stringer for debugging.
func (r *Result) String() string {
	parts := make([]string, len(r.Stanzas))
	for i, st := range r.Stanzas {
		parts[i] = st.String()
	}
	return strings.Join(parts, " ")
}
