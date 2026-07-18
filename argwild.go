package argwild

import (
	"os"
	"strings"
)

// Parse parses the current process's command-line arguments (os.Args without
// the program name) into a Result. It is equivalent to ParseArgs(os.Args[1:]).
func Parse() (*Result, error) {
	return ParseArgs(os.Args[1:])
}

// ParseArgs parses an already-split argument vector, such as os.Args[1:].
//
// Each element is an atomic token: elements containing whitespace are re-quoted
// so the parser treats them as single quoted values, reconstructing an
// equivalent command line before applying the full grammar. This means the
// spacing rules still apply across elements — for example ParseArgs([]string{
// "-o", "12"}) binds 12 to -o (space before a numeric value binds), while
// ParseArgs([]string{"-o", "NotThis"}) leaves -o bare and NotThis an operand
// (space before a bare word does not bind).
//
// For complete control over quoting and spacing, parse a raw string with
// ParseString instead.
func ParseArgs(args []string) (*Result, error) {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = requoteArg(a)
	}
	return ParseString(strings.Join(parts, " "))
}

// requoteArg wraps an argv element in double quotes when it contains whitespace
// so the parser sees it as one quoted token. Elements without whitespace are
// passed through unchanged so attached forms like "-o=A,B" and "+32" keep their
// meaning.
func requoteArg(a string) string {
	if a == "" {
		return `""`
	}
	if !strings.ContainsAny(a, " \t\r\n") {
		return a
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range a {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}
