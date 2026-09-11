# argwild

`argwild` is a Go module that parses command lines into a reusable, structured
form. It can read the process's own arguments or parse an arbitrary string, and
either way it returns a value you can inspect as many times as you like.

It is designed for rich, ffmpeg-style command lines where switches and operands
interleave, and it integrates with [PawScript](https://github.com/phroun/pawscript)'s
**PSL** (PawScript Serialized List) format: parenthesized argument values are
parsed as PSL, and the entire parse tree can be serialized back to PSL.

```go
import "github.com/phroun/argwild"

r, err := argwild.ParseString(`--verbose -o=2 input.mp4 --codec=x264 output.mp4`)
// r.Stanzas:
//   args[--verbose -o=2]
//   operand(input.mp4)
//   args[--codec=x264]
//   operand(output.mp4)
```

## Installation

```sh
go get github.com/phroun/argwild
```

Requires Go 1.24+. Builds on Linux, macOS, and Windows (amd64 and arm64). The
only transitive dependency is `github.com/phroun/pawscript` (plus
`golang.org/x/term`/`golang.org/x/sys`) — no GUI toolkits are pulled in.

## Concepts

The result of a parse is an ordered sequence of **stanzas**. Each stanza is one
of two things:

- an **ArgSet** — a run of consecutive switches, or
- an **Operand** — a single positional value.

Operands act as phase dividers. This lets you express "global switches, then an
operand, then switches that apply to the next phase, then another operand,"
exactly like `ffmpeg`. `argwild` preserves order faithfully and stays neutral
about whether an ArgSet binds to the operand before or after it — that is your
call.

### Switches

A switch is led by one of three introducers:

| Lead  | Example        | Notes                                              |
|-------|----------------|----------------------------------------------------|
| `-`   | `-o`           | Short: the name is exactly **one** character.      |
| `--`  | `--option`     | Long: multi-character name.                         |
| `+`   | `+32`, `+x=4`  | `+` then a digit is a value-only switch with an empty name (the line-number form); `+` then a letter behaves like a short switch. |

There is **no** getopt-style clustering: `-abc` is the switch `a` with the value
`bc`, not `-a -b -c`.

Every switch records one of four states:

| State    | Written  | Meaning              |
|----------|----------|----------------------|
| bare     | `-o`     | present              |
| on       | `-o+`    | explicitly enabled   |
| off      | `-o-`    | explicitly disabled  |
| valued   | `-o=x`   | carries one or more values |

The bare / on / off distinction is preserved so you can tell `-o` from `-o+`
from `-o-`.

### Values

A switch value can be:

- **bare** — `-oArg`
- **quoted** — `-o"Quoted Arg"` (`"` or `'`; `\"` and `\\` escapes)
- **number** — `-o12` (integer or float)
- **PSL block** — `-o(1, 2, 3)` — parsed with pawscript into a PSL list or map

Values comma-chain into lists:

```
-oA1,A2            -> [A1, A2]
-o"A1","A2"        -> [A1, A2]
-o(1,2),(3,4)      -> [ (1,2), (3,4) ]   (a list of PSL blocks)
```

`-oArg` and `-o=Arg` are equivalent; `-o"x"` and `-o="x"` are equivalent. The
full grammar applies to short, long, and `+` switches alike.

### Spacing rules

Attached values always bind. After a **space**, a value binds **only if it is
quoted, numeric, or a PSL block** — a bare word does not:

```
-o 12                          -> -o = 12
-o "Quoted Arg", "b", 3        -> -o = ["Quoted Arg", "b", 3]
-o NotThis                     -> -o is bare;  NotThis is an operand
```

A **trailing comma binds whatever follows**, across spaces and including bare
words:

```
-o 2, 5                        -> -o = [2, 5]
-oA1, NotThis                  -> -o = [A1, "NotThis"]
```

> Because `-o NotThis` leaves `-o` bare, binding a bare value needs the attached
> or `=` form: `-c=x264`, not `-c x264`. (`-c x264` yields a bare `-c` and an
> operand `x264`.)

### End of options

`--` on its own ends option parsing; everything after it is treated as operands.
A lone `-` is an operand (the conventional stdin marker).

## Entry points

```go
// Parse a raw string with full fidelity (quoting, spacing, PSL, commas).
r, err := argwild.ParseString(line)

// Parse an already-split argument vector. Elements containing whitespace are
// re-quoted so they are treated as single quoted tokens.
r, err := argwild.ParseArgs(os.Args[1:])

// Convenience for os.Args[1:].
r, err := argwild.Parse()
```

`ParseString` is the full-fidelity path and the right choice for command lines a
user typed into a prompt. In `ParseArgs`/`Parse`, the shell has already removed
quotes, so `argwild` reconstructs an equivalent line by re-quoting any element
that contains whitespace; the spacing rules then apply across elements (e.g.
`{"-o","12"}` binds, `{"-o","NotThis"}` does not).

## Inspecting the result

```go
for _, st := range r.Stanzas {
    switch s := st.(type) {
    case *argwild.ArgSet:
        for _, sw := range s.Switches {
            switch {
            case sw.IsOn():      // -o+
            case sw.IsOff():     // -o-
            case sw.HasValues(): // -o=...
                v, _ := sw.First()
                _ = v.Interface() // string | int64 | float64 | PSL value
            default:             // -o (bare)
            }
        }
    case *argwild.Operand:
        _ = s.Value.AsString()
    }
}

// Convenience filters:
r.Operands() // []*Operand in order
r.ArgSets()  // []*ArgSet in order
```

`Value.Interface()` returns a plain Go value (`string`, `int64`, `float64`, or a
`*pawscript.PSLNode` for a PSL block); `Value.AsString()` returns a display
string.

## PSL round-trip

The whole parse tree maps onto PSL and can be handed back to pawscript:

```go
psl := r.ToPSL()            // pawscript.PSLList of stanza maps
s   := r.ToPSLString(false) // compact PSL string
s   := r.ToPSLString(true)  // pretty, indented
```

The shape is an ordered list of stanza maps:

```
operand:  (kind: "operand", value: <value>)
arg set:  (kind: "args", switches: (<switch>, ...))
switch:   (lead: "-"|"--"|"+", name: <string>,
           state: "bare"|"on"|"off"|"valued", values: (<value>, ...))
```

This serializes with pawscript's PSL serializer and reparses losslessly with
`pawscript.ParsePSL`, whose result holds the stanzas in `Items`.

## License

See [LICENSE](LICENSE).
