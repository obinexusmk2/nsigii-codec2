// ltcodec — NSIGII Linkable-Then-Executable codec CLI
//
// Usage:
//
//	ltcodec coder   -input <file>    [-output <file.lt>]  [-v]
//	ltcodec decoder -input <file.lt> [-output <file>]     [-v]
//	ltcodec flash   save|undo|redo|status  [-target <file.lt>] [-flash-root <dir>] [-v]
//	ltcodec filter  -input <file.lt> [-sort name|size|type] [-query <pattern>] [-v]
//	ltcodec rollback --downgrade [-target <file.lt>] [-flash-root <dir>] [-v]
//	ltcodec wheel   --update|--upgrade [-target <file.lt>] [-flash-root <dir>] [-v]
//
// The .lt file format is a stateless zip-based container (Linkable Then
// Executable).  Stateless means artefacts never expire via versioning
// schemes — they remain integrable into any ecosystem indefinitely.
package main

import (
	"flag"
	"fmt"
	"os"

	"nsigii_ltcodec/pkg/codec"
	"nsigii_ltcodec/pkg/state"
	"nsigii_ltcodec/pkg/transform"
)

const banner = `
  ╔══════════════════════════════════════════════════════╗
  ║  NSIGII ltcodec  —  Linkable Then Executable         ║
  ║  Stateless · Isomorphic · Trident-Verified           ║
  ╚══════════════════════════════════════════════════════╝
`

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	fmt.Print(banner)

	subcommand := os.Args[1]
	args := os.Args[2:]

	switch subcommand {
	case "coder":
		runCoder(args)
	case "decoder":
		runDecoder(args)
	case "flash":
		runFlash(args)
	case "filter":
		runFilter(args)
	case "rollback":
		runRollback(args)
	case "wheel":
		runWheel(args)
	case "version", "-version", "--version":
		runVersion()
	case "help", "-help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "ltcodec: unknown subcommand %q\n", subcommand)
		printHelp()
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Subcommand: coder
// ─────────────────────────────────────────────────────────────────────────────

func runCoder(args []string) {
	fs := flag.NewFlagSet("coder", flag.ExitOnError)
	input := fs.String("input", "", "input file (any type: .xdt, .html, .eml, …)")
	output := fs.String("output", "", "output .lt file (default: <input>.lt)")
	verbose := fs.Bool("v", false, "verbose output")
	_ = fs.Parse(args)

	if *input == "" {
		fmt.Fprintln(os.Stderr, "coder: -input is required")
		fs.Usage()
		os.Exit(1)
	}

	if err := codec.Encode(codec.CoderConfig{
		InputPath:  *input,
		OutputPath: *output,
		Verbose:    *verbose,
	}); err != nil {
		fatalf("coder: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Subcommand: decoder
// ─────────────────────────────────────────────────────────────────────────────

func runDecoder(args []string) {
	fs := flag.NewFlagSet("decoder", flag.ExitOnError)
	input := fs.String("input", "", "input .lt archive")
	output := fs.String("output", "", "output file (default: decoded_<original>)")
	verbose := fs.Bool("v", false, "verbose output")
	_ = fs.Parse(args)

	if *input == "" {
		fmt.Fprintln(os.Stderr, "decoder: -input is required")
		fs.Usage()
		os.Exit(1)
	}

	if err := codec.Decode(codec.DecoderConfig{
		InputPath:  *input,
		OutputPath: *output,
		Verbose:    *verbose,
	}); err != nil {
		fatalf("decoder: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Subcommand: flash
// ─────────────────────────────────────────────────────────────────────────────

func runFlash(args []string) {
	// Action is the first positional arg (save|undo|redo|status)
	action := "status"
	rest := args
	if len(args) > 0 && isFlashAction(args[0]) {
		action = args[0]
		rest = args[1:]
	}

	fs := flag.NewFlagSet("flash", flag.ExitOnError)
	target := fs.String("target", "", ".lt file to snapshot (required for save)")
	flashRoot := fs.String("flash-root", "", "override .ltflash directory")
	verbose := fs.Bool("v", false, "verbose output")
	_ = fs.Parse(rest)

	if err := codec.Flash(codec.FlashConfig{
		Action:     action,
		TargetPath: *target,
		FlashRoot:  *flashRoot,
		Verbose:    *verbose,
	}); err != nil {
		fatalf("flash: %v", err)
	}
}

func isFlashAction(s string) bool {
	switch s {
	case "save", "undo", "redo", "status":
		return true
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Subcommand: filter
// ─────────────────────────────────────────────────────────────────────────────

func runFilter(args []string) {
	fs := flag.NewFlagSet("filter", flag.ExitOnError)
	input := fs.String("input", "", "input .lt archive")
	sortBy := fs.String("sort", "name", "sort field: name | size | type")
	query := fs.String("query", "", "pattern to match against section names / content-type")
	verbose := fs.Bool("v", false, "verbose output")
	_ = fs.Parse(args)

	if *input == "" {
		fmt.Fprintln(os.Stderr, "filter: -input is required")
		fs.Usage()
		os.Exit(1)
	}

	if _, err := codec.Filter(codec.FilterConfig{
		InputPath: *input,
		SortBy:    *sortBy,
		Query:     *query,
		Verbose:   *verbose,
	}); err != nil {
		fatalf("filter: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Subcommand: rollback
// ─────────────────────────────────────────────────────────────────────────────

func runRollback(args []string) {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	downgrade := fs.Bool("downgrade", false, "roll back to the previous flash state")
	target := fs.String("target", "", ".lt file to restore after rollback")
	flashRoot := fs.String("flash-root", "", "override .ltflash directory")
	verbose := fs.Bool("v", false, "verbose output")
	_ = fs.Parse(args)

	if err := state.Rollback(state.RollbackConfig{
		Downgrade:  *downgrade,
		FlashRoot:  *flashRoot,
		TargetPath: *target,
		Verbose:    *verbose,
	}); err != nil {
		fatalf("rollback: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Subcommand: wheel
// ─────────────────────────────────────────────────────────────────────────────

func runWheel(args []string) {
	fs := flag.NewFlagSet("wheel", flag.ExitOnError)
	update := fs.Bool("update", false, "advance pointer to the latest flash state")
	upgrade := fs.Bool("upgrade", false, "archive current states, start new root epoch")
	target := fs.String("target", "", ".lt file for upgrade source")
	flashRoot := fs.String("flash-root", "", "override .ltflash directory")
	verbose := fs.Bool("v", false, "verbose output")
	_ = fs.Parse(args)

	if err := state.Wheel(state.WheelConfig{
		Update:     *update,
		Upgrade:    *upgrade,
		FlashRoot:  *flashRoot,
		TargetPath: *target,
		Verbose:    *verbose,
	}); err != nil {
		fatalf("wheel: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Subcommand: version
// ─────────────────────────────────────────────────────────────────────────────

func runVersion() {
	proto := state.NewStatelessProtocol()
	fmt.Printf("  ltcodec v1.0.0\n")
	fmt.Printf("  Format:    .lt (Linkable Then Executable)\n")
	fmt.Printf("  Transform: Isomorphic XOR + Trident 3-channel\n")
	fmt.Printf("  Stateless: %v (no expiry)\n", true)
	fmt.Printf("  Protocol:  NSIGII / Trident Verification\n\n")
	proto.PrintMatrix()

	// Show a sample discriminant state
	sample := []byte("NSIGII-STATELESS-LT-CODEC")
	result := transform.RunTrident(sample)
	fmt.Printf("\n  Sample trident pass on %q:\n", sample)
	fmt.Printf("    state=%s  Δ=%.4f  polarity=%c  RWX=0x%02X\n",
		result.State, result.Discriminant, result.Polarity, result.RWXFlags)
}

// ─────────────────────────────────────────────────────────────────────────────
// Help
// ─────────────────────────────────────────────────────────────────────────────

func printHelp() {
	fmt.Print(banner)
	fmt.Print(`
Subcommands:
  coder    Encode any file into a .lt archive
  decoder  Decode a .lt archive back to the original file
  flash    Snapshot versioning: save | undo | redo | status
  filter   Inspect and sort sections of a .lt archive
  rollback Rollback to a prior flash state  (--downgrade)
  wheel    Advance (--update) or epoch-bump (--upgrade) the state
  version  Print version and protocol info

Examples:
  ltcodec coder   -input report.html  -output report.lt
  ltcodec decoder -input report.lt    -output report_out.html
  ltcodec flash   save   -target report.lt
  ltcodec flash   undo
  ltcodec flash   redo
  ltcodec flash   status
  ltcodec filter  -input report.lt  -sort size  -query payload
  ltcodec rollback --downgrade -target report.lt
  ltcodec wheel   --update
  ltcodec wheel   --upgrade -target report.lt

`)
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "ltcodec error: "+format+"\n", a...)
	os.Exit(1)
}
