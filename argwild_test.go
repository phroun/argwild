package argwild

import (
	"reflect"
	"testing"

	ps "github.com/phroun/pawscript"
)

// mustParse parses s and fails the test on error.
func mustParse(t *testing.T, s string) *Result {
	t.Helper()
	r, err := ParseString(s)
	if err != nil {
		t.Fatalf("ParseString(%q): %v", s, err)
	}
	return r
}

// onlyArgs asserts the result is a single ArgSet stanza and returns its
// switches.
func onlyArgs(t *testing.T, s string) []Switch {
	t.Helper()
	r := mustParse(t, s)
	if len(r.Stanzas) != 1 {
		t.Fatalf("%q: got %d stanzas, want 1: %s", s, len(r.Stanzas), r)
	}
	as, ok := r.Stanzas[0].(*ArgSet)
	if !ok {
		t.Fatalf("%q: stanza is %T, want *ArgSet", s, r.Stanzas[0])
	}
	return as.Switches
}

// oneSwitch asserts the result is a single ArgSet with a single switch.
func oneSwitch(t *testing.T, s string) Switch {
	t.Helper()
	sw := onlyArgs(t, s)
	if len(sw) != 1 {
		t.Fatalf("%q: got %d switches, want 1", s, len(sw))
	}
	return sw[0]
}

func TestLeadsAndNames(t *testing.T) {
	sw := oneSwitch(t, "-o")
	if sw.Lead != LeadShort || sw.Name != "o" || !sw.IsBare() {
		t.Errorf("-o => %+v", sw)
	}

	sw = oneSwitch(t, "--option")
	if sw.Lead != LeadLong || sw.Name != "option" || !sw.IsBare() {
		t.Errorf("--option => %+v", sw)
	}
}

func TestBareOnOff(t *testing.T) {
	if sw := oneSwitch(t, "-o"); !sw.IsBare() {
		t.Errorf("-o should be bare: %v", sw.State)
	}
	if sw := oneSwitch(t, "-o+"); !sw.IsOn() {
		t.Errorf("-o+ should be on: %v", sw.State)
	}
	if sw := oneSwitch(t, "-o-"); !sw.IsOff() {
		t.Errorf("-o- should be off: %v", sw.State)
	}
	// The distinction must survive on long options too.
	if sw := oneSwitch(t, "--flag+"); !sw.IsOn() {
		t.Errorf("--flag+ should be on: %v", sw.State)
	}
	if sw := oneSwitch(t, "--flag-"); !sw.IsOff() {
		t.Errorf("--flag- should be off: %v", sw.State)
	}
}

func TestAttachedValues(t *testing.T) {
	sw := oneSwitch(t, "-oArg")
	if !sw.HasValues() || sw.Values[0].Kind != KindBare || sw.Values[0].Text != "Arg" {
		t.Errorf("-oArg => %v", sw)
	}

	// -oArg and -o=Arg are equivalent.
	if a, b := oneSwitch(t, "-oArg"), oneSwitch(t, "-o=Arg"); !reflect.DeepEqual(a, b) {
		t.Errorf("-oArg (%v) != -o=Arg (%v)", a, b)
	}

	// -o"Quoted Arg" and -o="Quoted Arg" are equivalent.
	a := oneSwitch(t, `-o"Quoted Arg"`)
	b := oneSwitch(t, `-o="Quoted Arg"`)
	if !reflect.DeepEqual(a, b) {
		t.Errorf(`-o"Quoted Arg" (%v) != -o="Quoted Arg" (%v)`, a, b)
	}
	if a.Values[0].Kind != KindQuoted || a.Values[0].Text != "Quoted Arg" {
		t.Errorf("quoted value wrong: %v", a.Values[0])
	}

	// --option=Arg
	sw = oneSwitch(t, "--option=Arg")
	if sw.Name != "option" || sw.Values[0].Text != "Arg" {
		t.Errorf("--option=Arg => %v", sw)
	}
}

func TestNumericAttached(t *testing.T) {
	sw := oneSwitch(t, "-o12")
	v := sw.Values[0]
	if v.Kind != KindNumber || v.Int != 12 || v.IsFloat {
		t.Errorf("-o12 => %v", v)
	}
	// A token that only starts numeric is bare, not a number.
	sw = oneSwitch(t, "-o12abc")
	if sw.Values[0].Kind != KindBare || sw.Values[0].Text != "12abc" {
		t.Errorf("-o12abc => %v", sw.Values[0])
	}
}

func TestCommaListsAttached(t *testing.T) {
	sw := oneSwitch(t, "-oA1,A2")
	if len(sw.Values) != 2 || sw.Values[0].Text != "A1" || sw.Values[1].Text != "A2" {
		t.Errorf("-oA1,A2 => %v", sw.Values)
	}
	sw = oneSwitch(t, `-o"A1","A2"`)
	if len(sw.Values) != 2 || sw.Values[0].Text != "A1" || sw.Values[1].Text != "A2" {
		t.Errorf(`-o"A1","A2" => %v`, sw.Values)
	}
}

func TestSpaceBinding(t *testing.T) {
	// Space + numeric binds.
	sw := oneSwitch(t, "-o 12")
	if !sw.HasValues() || sw.Values[0].Int != 12 {
		t.Errorf("-o 12 => %v", sw)
	}

	// Space + quoted, with a comma chain mixing quoted and numeric.
	sw = oneSwitch(t, `-o "Quoted Arg", "Comma chain", 3`)
	if len(sw.Values) != 3 {
		t.Fatalf(`-o "Quoted Arg", "Comma chain", 3 => %d values: %v`, len(sw.Values), sw.Values)
	}
	if sw.Values[0].Text != "Quoted Arg" || sw.Values[1].Text != "Comma chain" || sw.Values[2].Int != 3 {
		t.Errorf("values => %v", sw.Values)
	}

	// Space + bare word does NOT bind: -o is bare, NotThis is an operand.
	r := mustParse(t, "-o NotThis")
	if len(r.Stanzas) != 2 {
		t.Fatalf("-o NotThis => %d stanzas: %s", len(r.Stanzas), r)
	}
	if as, ok := r.Stanzas[0].(*ArgSet); !ok || !as.Switches[0].IsBare() {
		t.Errorf("-o NotThis: first stanza should be a bare switch: %s", r)
	}
	if op, ok := r.Stanzas[1].(*Operand); !ok || op.Value.Text != "NotThis" {
		t.Errorf("-o NotThis: second stanza should be operand NotThis: %s", r)
	}
}

func TestTrailingCommaBinds(t *testing.T) {
	// -o 2, 5 => o = [2, 5]
	sw := oneSwitch(t, "-o 2, 5")
	if len(sw.Values) != 2 || sw.Values[0].Int != 2 || sw.Values[1].Int != 5 {
		t.Errorf("-o 2, 5 => %v", sw.Values)
	}

	// The comma binds whatever follows, even a bare word.
	sw = oneSwitch(t, "-oA1,NotThis")
	if len(sw.Values) != 2 || sw.Values[1].Kind != KindBare || sw.Values[1].Text != "NotThis" {
		t.Errorf("-oA1,NotThis => %v", sw.Values)
	}
}

func TestPlusForms(t *testing.T) {
	// +32: empty-name value-only line number.
	sw := oneSwitch(t, "+32")
	if sw.Lead != LeadPlus || sw.Name != "" || len(sw.Values) != 1 || sw.Values[0].Int != 32 {
		t.Errorf("+32 => %+v", sw)
	}

	// +x=4: named plus switch.
	sw = oneSwitch(t, "+x=4")
	if sw.Lead != LeadPlus || sw.Name != "x" || sw.Values[0].Int != 4 {
		t.Errorf("+x=4 => %+v", sw)
	}

	// +32,40: comma list on the value-only form.
	sw = oneSwitch(t, "+32,40")
	if len(sw.Values) != 2 || sw.Values[0].Int != 32 || sw.Values[1].Int != 40 {
		t.Errorf("+32,40 => %v", sw.Values)
	}

	// A lone "+" with a space is an operand, not a switch.
	r := mustParse(t, "+ x")
	if len(r.Operands()) == 0 {
		t.Errorf("`+ x` should yield operands: %s", r)
	}
}

func TestLoneDashOperand(t *testing.T) {
	// A lone "-" is an operand (the stdin convention), not a switch.
	r := mustParse(t, "cat -")
	ops := r.Operands()
	if len(ops) != 2 || ops[0].Value.Text != "cat" || ops[1].Value.Text != "-" {
		t.Errorf("`cat -` operands => %v", ops)
	}
}

func TestEmptyNamePSL(t *testing.T) {
	// "+(psl)": plus lead, empty name, PSL value (paren is not a name char).
	sw := oneSwitch(t, "+(1, 2)")
	if sw.Lead != LeadPlus || sw.Name != "" || sw.Values[0].Kind != KindPSL {
		t.Errorf("+(1, 2) => %+v", sw)
	}
}

func TestPSLValues(t *testing.T) {
	sw := oneSwitch(t, "-o(1, 2, 3)")
	if sw.Values[0].Kind != KindPSL {
		t.Fatalf("-o(1, 2, 3) => %v", sw.Values[0])
	}
	block, ok := sw.Values[0].PSL.(*ps.PSLNode)
	if !ok || block.Len() != 3 {
		t.Fatalf("PSL not a 3-item block: %#v", sw.Values[0].PSL)
	}

	// PSL blocks in any value position, including a comma chain of blocks.
	sw = oneSwitch(t, "-o(1, 2),(3, 4)")
	if len(sw.Values) != 2 || sw.Values[0].Kind != KindPSL || sw.Values[1].Kind != KindPSL {
		t.Fatalf("-o(1,2),(3,4) => %v", sw.Values)
	}
	second := sw.Values[1].PSL.(*ps.PSLNode)
	if v, _ := second.Item(0); second.Len() != 2 || v != int64(3) {
		t.Errorf("second PSL block => %#v", second)
	}

	// A named PSL block keeps its names.
	sw = oneSwitch(t, `-o(name: "test", count: 5)`)
	named, ok := sw.Values[0].PSL.(*ps.PSLNode)
	if !ok {
		t.Fatalf("named PSL block => %#v", sw.Values[0].PSL)
	}
	if m := named.Map(); m.GetInt("count", -1) != 5 || m.GetString("name", "") != "test" {
		t.Errorf("named PSL block => %#v", m)
	}

	// A block that mixes the two keeps both halves.
	sw = oneSwitch(t, `-o(1, 2, name: "test")`)
	mixed, ok := sw.Values[0].PSL.(*ps.PSLNode)
	if !ok {
		t.Fatalf("mixed PSL block => %#v", sw.Values[0].PSL)
	}
	if mixed.Len() != 2 {
		t.Errorf("mixed PSL block kept %d ordered items, want 2: %#v", mixed.Len(), mixed.Items)
	}
	if v, _ := mixed.Get("name"); v != "test" {
		t.Errorf("mixed PSL block's name came back as %#v", v)
	}
	if out := sw.Values[0].AsString(); out != `(name: "test", 1, 2)` {
		t.Errorf("mixed PSL block serializes as %s", out)
	}
}

func TestEndOfOptions(t *testing.T) {
	r := mustParse(t, "-a -- -b input")
	// -a is a switch; -b and input become operands.
	ops := r.Operands()
	if len(ops) != 2 || ops[0].Value.Text != "-b" || ops[1].Value.Text != "input" {
		t.Errorf("`-a -- -b input` operands => %v", ops)
	}
	if len(r.ArgSets()) != 1 || r.ArgSets()[0].Switches[0].Name != "a" {
		t.Errorf("`-a -- -b input` argsets => %v", r.ArgSets())
	}
}

func TestStanzaOrdering(t *testing.T) {
	// ffmpeg-style: globals, operand, phase args, operand. Note that a value
	// must attach ("-c=x264"): "-c x264" would leave -c bare and make x264 its
	// own operand, per the space-before-a-bare-word rule.
	r := mustParse(t, "--verbose -o2 input.mp4 -c=x264 output.mp4")
	kinds := make([]string, len(r.Stanzas))
	for i, st := range r.Stanzas {
		switch st.(type) {
		case *ArgSet:
			kinds[i] = "args"
		case *Operand:
			kinds[i] = "op"
		}
	}
	want := []string{"args", "op", "args", "op"}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("stanza kinds = %v, want %v (%s)", kinds, want, r)
	}
	// First arg set holds the globals.
	g := r.Stanzas[0].(*ArgSet).Switches
	if len(g) != 2 || g[0].Name != "verbose" || g[1].Name != "o" {
		t.Errorf("globals => %v", g)
	}
	// -c=x264: attached value binds.
	phase := r.Stanzas[2].(*ArgSet).Switches
	if len(phase) != 1 || phase[0].Name != "c" || phase[0].Values[0].Text != "x264" {
		t.Errorf("phase args => %v", phase)
	}
}

func TestConsecutiveOperands(t *testing.T) {
	r := mustParse(t, "a b -x")
	if len(r.Stanzas) != 3 {
		t.Fatalf("`a b -x` => %d stanzas: %s", len(r.Stanzas), r)
	}
	if _, ok := r.Stanzas[0].(*Operand); !ok {
		t.Errorf("stanza 0 should be operand: %s", r)
	}
	if _, ok := r.Stanzas[1].(*Operand); !ok {
		t.Errorf("stanza 1 should be operand: %s", r)
	}
}

func TestParseArgs(t *testing.T) {
	// Elements with spaces are re-quoted and bind per the space rules.
	r, err := ParseArgs([]string{"-o", "12"})
	if err != nil {
		t.Fatal(err)
	}
	if sw := r.ArgSets()[0].Switches[0]; !sw.HasValues() || sw.Values[0].Int != 12 {
		t.Errorf("ParseArgs -o 12 => %v", sw)
	}

	r, _ = ParseArgs([]string{"-o", "NotThis"})
	if len(r.Operands()) != 1 || r.Operands()[0].Value.Text != "NotThis" {
		t.Errorf("ParseArgs -o NotThis => %s", r)
	}

	// A single element carrying a space becomes one quoted operand.
	r, _ = ParseArgs([]string{"input file.mp4"})
	if len(r.Operands()) != 1 || r.Operands()[0].Value.Text != "input file.mp4" {
		t.Errorf("ParseArgs quoted operand => %s", r)
	}
}

func TestRoundTripPSL(t *testing.T) {
	src := `--verbose -o 2, "hi there" input.mp4 -c(codec: "x264") output.mp4 +32`
	r := mustParse(t, src)

	out := r.ToPSLString(false)
	doc, err := ps.ParsePSL(out)
	if err != nil {
		t.Fatalf("reparse ToPSLString: %v\n%s", err, out)
	}
	if doc.Len() != len(r.Stanzas) {
		t.Fatalf("round-trip stanza count %d != %d\n%s", doc.Len(), len(r.Stanzas), out)
	}

	// Spot-check: the first stanza is an args stanza with two switches.
	stanza, ok := doc.Child(0)
	if !ok {
		t.Fatalf("first stanza => %#v", doc.Items[0])
	}
	first := stanza.Map()
	if first.GetString("kind", "") != "args" {
		t.Fatalf("first stanza => %#v", first)
	}
	if items := first.GetItems("switches"); len(items) != 2 {
		t.Errorf("first arg set switches => %#v", first["switches"])
	}

	// Pretty form must be non-empty and parseable-ish (contains our markers).
	pretty := r.ToPSLString(true)
	if pretty == "" {
		t.Error("pretty output empty")
	}
}

// A PSL block argument survives the round trip as the list it is, rather than
// as a rendering of the Go value it was carried in.
func TestRoundTripKeepsAPSLBlockAsAList(t *testing.T) {
	r := mustParse(t, `-c(codec: "x264", 2)`)
	out := r.ToPSLString(false)

	doc, err := ps.ParsePSL(out)
	if err != nil {
		t.Fatalf("reparse ToPSLString: %v\n%s", err, out)
	}
	stanza, ok := doc.Child(0)
	if !ok {
		t.Fatalf("first stanza is not a list: %s", out)
	}
	switches := stanza.Map().GetItems("switches")
	if len(switches) != 1 {
		t.Fatalf("the arg set has %d switches: %s", len(switches), out)
	}
	sw, ok := switches[0].(*ps.PSLNode)
	if !ok {
		t.Fatalf("the switch came back as %T: %s", switches[0], out)
	}
	values := sw.Map().GetItems("values")
	if len(values) != 1 {
		t.Fatalf("the switch has %d values: %s", len(values), out)
	}
	block, ok := values[0].(*ps.PSLNode)
	if !ok {
		t.Fatalf("the PSL block came back as %T (%#v): %s", values[0], values[0], out)
	}
	if v, _ := block.Get("codec"); v != "x264" {
		t.Errorf("the block's codec came back as %#v: %s", v, out)
	}
	if v, _ := block.Item(0); v != int64(2) {
		t.Errorf("the block's ordered item came back as %#v: %s", v, out)
	}
}
