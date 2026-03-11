// Package codec implements the four NSIGII ltcodec operations:
// coder, decoder, flash, and filter.
package codec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"nsigii_ltcodec/pkg/format"
	"nsigii_ltcodec/pkg/transform"
)

// CoderConfig holds options for the coder subcommand.
type CoderConfig struct {
	InputPath  string // any file: .xdt, .html, .eml, binary, …
	OutputPath string // destination .lt file (default: <input>.lt)
	Verbose    bool
}

// Encode reads any input file, applies the isomorphic XOR transform,
// passes the payload through the trident 3-channel verifier, and writes
// a .lt archive to disk.
//
// CLI: ltcodec coder -input <file> [-output <file.lt>]
func Encode(cfg CoderConfig) error {
	// ── Resolve output path ───────────────────────────────────────────────
	if cfg.OutputPath == "" {
		cfg.OutputPath = deriveOutputPath(cfg.InputPath)
	}

	// ── Read input ────────────────────────────────────────────────────────
	var rawData []byte
	var err error

	if cfg.InputPath == "-" || cfg.InputPath == "" {
		rawData, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("coder: read stdin: %w", err)
		}
	} else {
		rawData, err = os.ReadFile(cfg.InputPath)
		if err != nil {
			return fmt.Errorf("coder: read input %q: %w", cfg.InputPath, err)
		}
	}

	if cfg.Verbose {
		fmt.Printf("[CODER] input:   %s (%d bytes)\n", cfg.InputPath, len(rawData))
	}

	// ── Build metadata ────────────────────────────────────────────────────
	originalName := filepath.Base(cfg.InputPath)
	contentType := format.DetectContentType(originalName)
	meta := format.NewMeta(contentType, originalName)

	// ── Isomorphic XOR transform ──────────────────────────────────────────
	key := transform.DeriveKey(meta.UUID)
	encoded := transform.Encode(rawData, key)

	if cfg.Verbose {
		_, _, parityByte := transform.ParityAxis(encoded)
		polarity := transform.PolaritySign(encoded)
		fmt.Printf("[CODER] uuid:    %s\n", meta.UUID)
		fmt.Printf("[CODER] parity:  axis=0x%02X polarity=%c\n", parityByte, polarity)
	}

	// ── Trident 3-channel verification ────────────────────────────────────
	result := transform.RunTrident(encoded)

	if cfg.Verbose {
		fmt.Printf("[CODER] trident: state=%s Δ=%.4f wheel=%d° RWX=0x%02X\n",
			result.State, result.Discriminant, result.WheelDeg, result.RWXFlags)
	}

	// Use the trident-processed payload (may be enzyme-repaired on CHAOS)
	payload := result.Data

	// ── Build .lt archive ─────────────────────────────────────────────────
	ltBytes, err := format.Build(meta, payload)
	if err != nil {
		return fmt.Errorf("coder: build lt archive: %w", err)
	}

	// ── Write output ──────────────────────────────────────────────────────
	if err := os.WriteFile(cfg.OutputPath, ltBytes, 0644); err != nil {
		return fmt.Errorf("coder: write output %q: %w", cfg.OutputPath, err)
	}

	if cfg.Verbose || !cfg.Verbose { // always print summary
		fmt.Printf("[CODER] output:  %s (%d bytes)\n", cfg.OutputPath, len(ltBytes))
		fmt.Printf("[CODER] state:   %s | polarity: %c | verified: %v\n",
			result.State, result.Polarity, result.Verified)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────

func deriveOutputPath(inputPath string) string {
	if inputPath == "" || inputPath == "-" {
		return "output.lt"
	}
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return filepath.Join(filepath.Dir(inputPath), name+".lt")
}
