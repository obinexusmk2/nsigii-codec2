package codec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Flash implements the NSIGII flash versioning system.
//
// Flash model:   1/2 + 1/2 = 1  (two half-states combine into a complete unit)
//                1/2 × 1/2 = 1/4 (requires 4 flashes for a full unit)
//
// Storage: a .ltflash/ directory holding:
//
//	flash.json        – index: { current, states: ["state_0.lt", …] }
//	state_0.lt, …     – snapshots
//
// CLI:
//
//	ltcodec flash save  [-target file.lt]   – snapshot current .lt
//	ltcodec flash undo                      – move pointer back
//	ltcodec flash redo                      – move pointer forward

const (
	FlashDir   = ".ltflash"
	FlashIndex = "flash.json"
)

// FlashIndexFile is the on-disk structure persisted to flash.json.
type FlashIndexFile struct {
	Current    int      `json:"current"`     // index of active state
	States     []string `json:"states"`      // filenames in FlashDir
	UpdatedAt  string   `json:"updated_at"`
}

// FlashConfig holds options for flash subcommands.
type FlashConfig struct {
	Action     string // "save" | "undo" | "redo" | "status"
	TargetPath string // .lt file to snapshot (required for save)
	FlashRoot  string // override .ltflash location (default: next to target)
	Verbose    bool
}

// Flash executes a flash subcommand.
func Flash(cfg FlashConfig) error {
	root := cfg.FlashRoot
	if root == "" && cfg.TargetPath != "" {
		root = filepath.Join(filepath.Dir(cfg.TargetPath), FlashDir)
	}
	if root == "" {
		root = filepath.Join(".", FlashDir)
	}

	switch cfg.Action {
	case "save":
		return flashSave(root, cfg.TargetPath, cfg.Verbose)
	case "undo":
		return flashUndo(root, cfg.Verbose)
	case "redo":
		return flashRedo(root, cfg.Verbose)
	case "status", "":
		return flashStatus(root)
	default:
		return fmt.Errorf("flash: unknown action %q (save | undo | redo | status)", cfg.Action)
	}
}

// ─────────────────────────────────────────────────────────────────────────────

// flashSave snapshots the .lt file as the next flash state.
// Implements:  state_{n} → state_{n+1}  (half-flash accumulation)
func flashSave(root, target string, verbose bool) error {
	if target == "" {
		return fmt.Errorf("flash save: -target <file.lt> is required")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("flash save: read %q: %w", target, err)
	}

	if err := os.MkdirAll(root, 0755); err != nil {
		return fmt.Errorf("flash save: mkdir %q: %w", root, err)
	}

	idx, _ := loadFlashIndex(root) // ok if missing — start fresh

	// Truncate any redo states beyond current pointer
	if idx.Current < len(idx.States)-1 {
		// Remove future states (new save forks the timeline)
		for _, s := range idx.States[idx.Current+1:] {
			_ = os.Remove(filepath.Join(root, s))
		}
		idx.States = idx.States[:idx.Current+1]
	}

	// Write snapshot
	stateName := fmt.Sprintf("state_%d.lt", len(idx.States))
	snapPath := filepath.Join(root, stateName)
	if err := os.WriteFile(snapPath, data, 0644); err != nil {
		return fmt.Errorf("flash save: write snapshot: %w", err)
	}

	idx.States = append(idx.States, stateName)
	idx.Current = len(idx.States) - 1
	idx.UpdatedAt = time.Now().Format(time.RFC3339)

	if err := saveFlashIndex(root, idx); err != nil {
		return fmt.Errorf("flash save: write index: %w", err)
	}

	fmt.Printf("[FLASH] saved → %s (state %d/%d)\n",
		stateName, idx.Current, len(idx.States)-1)
	if verbose {
		fmt.Printf("[FLASH] 1/2 flash unit accumulated.  next save completes 1 unit.\n")
	}
	return nil
}

// flashUndo steps back one flash state (pointer -= 1).
func flashUndo(root string, verbose bool) error {
	idx, err := loadFlashIndex(root)
	if err != nil {
		return fmt.Errorf("flash undo: no flash history — run 'flash save' first")
	}
	if idx.Current <= 0 {
		return fmt.Errorf("flash undo: already at earliest state (state 0)")
	}

	prev := idx.Current - 1
	statePath := filepath.Join(root, idx.States[prev])

	if verbose {
		fmt.Printf("[FLASH] undo: %s → %s\n", idx.States[idx.Current], idx.States[prev])
	}

	idx.Current = prev
	idx.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := saveFlashIndex(root, idx); err != nil {
		return err
	}

	fmt.Printf("[FLASH] undo → state %d/%d  (%s)\n",
		idx.Current, len(idx.States)-1, statePath)
	return nil
}

// flashRedo steps forward one flash state (pointer += 1).
func flashRedo(root string, verbose bool) error {
	idx, err := loadFlashIndex(root)
	if err != nil {
		return fmt.Errorf("flash redo: no flash history")
	}
	if idx.Current >= len(idx.States)-1 {
		return fmt.Errorf("flash redo: already at latest state (state %d)", idx.Current)
	}

	next := idx.Current + 1
	statePath := filepath.Join(root, idx.States[next])

	if verbose {
		fmt.Printf("[FLASH] redo: %s → %s\n", idx.States[idx.Current], idx.States[next])
	}

	idx.Current = next
	idx.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := saveFlashIndex(root, idx); err != nil {
		return err
	}

	fmt.Printf("[FLASH] redo → state %d/%d  (%s)\n",
		idx.Current, len(idx.States)-1, statePath)
	return nil
}

// flashStatus prints the current flash index.
func flashStatus(root string) error {
	idx, err := loadFlashIndex(root)
	if err != nil {
		fmt.Printf("[FLASH] no flash history at %s\n", root)
		return nil
	}

	fmt.Printf("[FLASH] directory:  %s\n", root)
	fmt.Printf("[FLASH] states:     %d total\n", len(idx.States))
	fmt.Printf("[FLASH] current:    state %d\n", idx.Current)
	fmt.Printf("[FLASH] updated:    %s\n", idx.UpdatedAt)
	fmt.Println()
	for i, s := range idx.States {
		marker := "  "
		if i == idx.Current {
			marker = "→ "
		}
		fmt.Printf("  %s[%d] %s\n", marker, i, s)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────

func loadFlashIndex(root string) (*FlashIndexFile, error) {
	data, err := os.ReadFile(filepath.Join(root, FlashIndex))
	if err != nil {
		return &FlashIndexFile{Current: -1}, err
	}
	var idx FlashIndexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		return &FlashIndexFile{Current: -1}, err
	}
	return &idx, nil
}

func saveFlashIndex(root string, idx *FlashIndexFile) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, FlashIndex), data, 0644)
}

// ActiveStatePath returns the file path of the currently active flash state.
// Returns an empty string if no flash history exists.
func ActiveStatePath(root string) string {
	idx, err := loadFlashIndex(root)
	if err != nil || idx.Current < 0 || idx.Current >= len(idx.States) {
		return ""
	}
	return filepath.Join(root, idx.States[idx.Current])
}
