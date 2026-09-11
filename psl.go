package argwild

import (
	"strings"

	ps "github.com/phroun/pawscript"
)

// parsePSLBlock parses the inner text of a "(...)" argument block with the
// pawscript library.
//
// A PSL list holds an ordered sequence and named members side by side, and an
// argument block may use either or both: (1, 2, 3), (name: "x", count: 5), or
// (1, name: "x"). The node it comes back as holds all of it, so a block that
// mixes the two is not half-read.
func parsePSLBlock(inner string) (interface{}, error) {
	n, err := ps.ParsePSL("(" + inner + ")")
	if err != nil {
		return nil, err
	}
	return n, nil
}

// pslToString serializes a parsed PSL value back to its source form.
func pslToString(v interface{}) string {
	switch t := v.(type) {
	case *ps.PSLNode:
		return ps.SerializePSLNode(t)
	case ps.PSLList:
		return ps.SerializePSLList(t)
	case ps.PSLMap:
		return ps.SerializePSL(t)
	case []interface{}:
		return ps.SerializePSLList(ps.PSLList(t))
	case map[string]interface{}:
		return ps.SerializePSL(ps.PSLMap(t))
	default:
		return "()"
	}
}

// ToPSL converts the whole parse result into a pawscript PSLList so it can be
// serialized with pawscript's PSL serializer and reparsed losslessly.
//
// The shape is an ordered list of stanza maps:
//
//	operand stanza: (kind: "operand", value: <value>)
//	arg-set stanza: (kind: "args", switches: (<switch>, <switch>, ...))
//
// Each switch is:
//
//	(lead: "-"|"--"|"+", name: <string>, state: "bare"|"on"|"off"|"valued",
//	 values: (<value>, ...))
//
// Values become plain PSL scalars (string / int64 / float64) or, for PSL-block
// arguments, the nested PSL structure itself.
func (r *Result) ToPSL() ps.PSLList {
	out := make(ps.PSLList, 0, len(r.Stanzas))
	for _, st := range r.Stanzas {
		switch s := st.(type) {
		case *Operand:
			out = append(out, ps.PSLMap{
				"kind":  "operand",
				"value": s.Value.Interface(),
			})
		case *ArgSet:
			switches := make(ps.PSLList, 0, len(s.Switches))
			for _, sw := range s.Switches {
				vals := make(ps.PSLList, 0, len(sw.Values))
				for _, v := range sw.Values {
					vals = append(vals, v.Interface())
				}
				switches = append(switches, ps.PSLMap{
					"lead":   sw.Lead.String(),
					"name":   sw.Name,
					"state":  sw.State.String(),
					"values": vals,
				})
			}
			out = append(out, ps.PSLMap{
				"kind":     "args",
				"switches": switches,
			})
		}
	}
	return out
}

// ToPSLString serializes the parse result to a PSL string. When pretty is true
// the output is indented across multiple lines.
func (r *Result) ToPSLString(pretty bool) string {
	list := r.ToPSL()
	if pretty {
		// SerializePSLPretty operates on a PSLMap; wrap the list under a single
		// "stanzas" key so the whole tree can be pretty-printed.
		return strings.TrimRight(ps.SerializePSLPretty(ps.PSLMap{"stanzas": list}), "\n")
	}
	return ps.SerializePSLList(list)
}
