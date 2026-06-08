// Package usage provides a declarative command-line argument parser.
//
// # Motivation
//
// Traditional CLI argument parsing couples grammar declaration with parsing
// logic and help-text generation. This package decouples them: you declare
// the grammar as a tree of Atom values, and Parse/Render are derived
// automatically from the tree structure.
//
// # Quick Start
//
// Declare a usage (a sequence of atoms):
//
//	var addUsage = usage.Usage{
//	    usage.Verbatim("add"),
//	    usage.Placeholder("name"),
//	    usage.Optional(usage.Flag("force")),
//	}
//
// Parse command-line arguments:
//
//	parsed, err := addUsage.Parse(os.Args[1:])
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Use the results:
//
//	name := parsed[1].String        // the <name> value
//	force := parsed[2].String       // "--force" or "" if skipped
//	_ = name, force
//
// # Atom Types
//
// Each atom type implements the Atom interface. Constructors are the primary
// way to create atoms:
//
//	Verbatim("text")          — literal match, renders as "text"
//	Placeholder("name")       — matches any one argument, renders as <name>
//	Flag("verbose")           — matches --verbose (any position), renders as --verbose
//	Either(a, b, c)           — first-match choice, renders as (a|b|c)
//	EitherVerbatim("a", "b")  — shortcut for Either(Verbatim("a"), Verbatim("b"))
//	Sequence(a, b, c)         — space-separated sequence, renders as "a b c"
//	MakePath(a, b)            — "/"-separated compound, renders as "a/b"
//	Colon(a)                  — appends ":", renders as "a:"
//
// ## Combinators
//
// Every atom can be wrapped:
//
//	atom.Optional()           — matches zero or one time, renders as [atom]
//	atom.List(min, sep...)    — matches min+ times, renders as "atom ..."
//
// # Predefined Atoms
//
//	File            <file>
//	ID              <id>
//	Key             <key>
//	Value           <value>
//	Name            <name>
//	KV              <key>=<value>  (compound with "=")
//
// # Parsing Results
//
// Parsed contains the result for one atom:
//
//	String      string       — the parsed text value
//	List        []*Parsed    — sub-results (for compound, list, etc.)
//	StringList  []string     — flat string slice of all sub-results
//	Skipped     bool         — true if the atom was optional and unmatched
//	BranchID    int          — for Either, which branch matched (0-indexed)
//
// Get returns the string value or a default if Skipped:
//
//	val := parsed.Get("default")
//
// # Error Handling
//
// Parse returns errors that distinguish parse failures from internal
// errors. Use errors.As or a type switch to inspect specific error types.
//
// # Explain Mode
//
// Pass Config{ExplainOnly: true} to Usage.Parse to enable diagnostic output.
// When set, a successful parse prints a tree rendering of how each atom
// matched and returns ErrExplainOnly:
//
//	parsed, err := myUsage.Parse(args, usage.Config{ExplainOnly: true})
//	if err == usage.ErrExplainOnly {
//	    // explain output already written to stdout; exit cleanly
//	}
//
// # Usage with --help
//
// Atom.Render() produces colorized help text fragments. Compose them into
// your help output:
//
//	var sb strings.Builder
//	for _, atom := range myUsage {
//	    sb.WriteString(atom.Render())
//	    sb.WriteByte(' ')
//	}
//	// sb.String() → "add <name> [--verbose]"
//
// # Design Notes
//
// This package was extracted from incus CLI, preserving the Atom composition
// model while removing incus-specific dependencies (remote server, color
// coupling, etc.). The key insight: every grammar element is an Atom that
// knows how to parse itself AND render itself, and atoms compose via List
// and Optional to express repetition and optionality.
package usage
