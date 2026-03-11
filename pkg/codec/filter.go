package codec

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"nsigii_ltcodec/pkg/format"
	"nsigii_ltcodec/pkg/transform"
)

// FilterConfig holds options for the filter subcommand.
type FilterConfig struct {
	InputPath string
	SortBy    string // "name" | "size" | "type" (default: "name")
	Query     string // pattern to match against section name or content-type
	Verbose   bool
}

// FilterResult holds the output of a filter pass.
type FilterResult struct {
	Entries      []format.IndexEntry
	TridentState transform.TridentState
	Polarity     byte
	ContentType  string
	UUID         string
}

// Filter opens a .lt archive, applies sorting and optional querying,
// and prints a summary of matching sections.
//
// CLI: ltcodec filter -input file.lt [-sort name|size|type] [-query pattern]
func Filter(cfg FilterConfig) (*FilterResult, error) {
	// ── Read archive ──────────────────────────────────────────────────────
	ltBytes, err := os.ReadFile(cfg.InputPath)
	if err != nil {
		return nil, fmt.Errorf("filter: read %q: %w", cfg.InputPath, err)
	}

	meta, payload, idx, err := format.Open(ltBytes)
	if err != nil {
		return nil, fmt.Errorf("filter: open archive: %w", err)
	}

	// ── Trident state analysis ────────────────────────────────────────────
	state := transform.DiscriminantState(payload)
	polarity := transform.PolaritySign(payload)

	// ── Query filter ──────────────────────────────────────────────────────
	filtered := idx
	if cfg.Query != "" {
		query := strings.ToLower(cfg.Query)
		filtered = filtered[:0]
		for _, e := range idx {
			if strings.Contains(strings.ToLower(e.Name), query) ||
				strings.Contains(strings.ToLower(e.Type), query) ||
				strings.Contains(strings.ToLower(meta.ContentType), query) {
				filtered = append(filtered, e)
			}
		}
	}

	// ── Sort ──────────────────────────────────────────────────────────────
	switch strings.ToLower(cfg.SortBy) {
	case "size":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Size < filtered[j].Size
		})
	case "type":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Type < filtered[j].Type
		})
	default: // "name"
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Name < filtered[j].Name
		})
	}

	// ── Print ─────────────────────────────────────────────────────────────
	fmt.Printf("[FILTER] archive:    %s\n", cfg.InputPath)
	fmt.Printf("[FILTER] uuid:       %s\n", meta.UUID)
	fmt.Printf("[FILTER] content:    %s\n", meta.ContentType)
	fmt.Printf("[FILTER] original:   %s\n", meta.OriginalName)
	fmt.Printf("[FILTER] stateless:  %v\n", meta.Stateless)
	fmt.Printf("[FILTER] trident:    %s | polarity: %c\n", state, polarity)
	fmt.Printf("[FILTER] sections:   %d total, %d matched\n\n", len(idx), len(filtered))

	fmt.Printf("  %-20s  %-6s  %s\n", "NAME", "TYPE", "SIZE (bytes)")
	fmt.Printf("  %s\n", strings.Repeat("─", 48))
	for _, e := range filtered {
		fmt.Printf("  %-20s  %-6s  %d\n", e.Name, e.Type, e.Size)
	}
	fmt.Println()

	if cfg.Verbose {
		even, odd, parityByte := transform.ParityAxis(payload)
		fmt.Printf("[FILTER] parity:     even=%d odd=%d axis=0x%02X\n", even, odd, parityByte)
	}

	return &FilterResult{
		Entries:      filtered,
		TridentState: state,
		Polarity:     polarity,
		ContentType:  meta.ContentType,
		UUID:         meta.UUID,
	}, nil
}
