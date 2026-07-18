package argwild_test

import (
	"fmt"

	"github.com/phroun/argwild"
)

// ExampleParseString shows the ffmpeg-style stanza decomposition: global
// switches, an operand, phase switches, another operand.
func ExampleParseString() {
	r, err := argwild.ParseString(`--verbose -o=2 input.mp4 --codec=x264 output.mp4`)
	if err != nil {
		panic(err)
	}
	for _, st := range r.Stanzas {
		fmt.Println(st)
	}
	// Output:
	// args[--verbose -o=2]
	// operand(input.mp4)
	// args[--codec=x264]
	// operand(output.mp4)
}

// ExampleResult_ToPSLString shows the whole parse serialized to PSL, ready to
// hand back to the pawscript library.
func ExampleResult_ToPSLString() {
	r, _ := argwild.ParseString(`+32 file.txt -o "a, b", 3`)
	fmt.Println(r.ToPSLString(false))
	// Output:
	// ((kind: "args", switches: ((lead: "+", name: "", state: "valued", values: (32)))), (kind: "operand", value: "file.txt"), (kind: "args", switches: ((lead: "-", name: "o", state: "valued", values: ("a, b", 3)))))
}

// ExampleSwitch shows inspecting on/off/bare state and typed values.
func ExampleSwitch() {
	r, _ := argwild.ParseString(`-a -b+ -c- -d=5`)
	for _, sw := range r.ArgSets()[0].Switches {
		switch {
		case sw.IsOn():
			fmt.Printf("%s: on\n", sw.Name)
		case sw.IsOff():
			fmt.Printf("%s: off\n", sw.Name)
		case sw.HasValues():
			v, _ := sw.First()
			fmt.Printf("%s: value %s\n", sw.Name, v.AsString())
		default:
			fmt.Printf("%s: bare\n", sw.Name)
		}
	}
	// Output:
	// a: bare
	// b: on
	// c: off
	// d: value 5
}
