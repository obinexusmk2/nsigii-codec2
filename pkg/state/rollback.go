package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nsigii_ltcodec/pkg/codec"
)

// RollbackConfig holds options for the rollback subcommand.
type RollbackConfig struct {
	Downgrade  bool   // --downgrade: revert to previous flash checkpoint
	FlashRoot  string // .ltflash directory (default: inferred from target)
	TargetPath string // .lt file to restore after rollback (optional)
	Verbose    bool
}

// WheelConfig holds options for the wheel subcommand.
type WheelConfig struct {
	Update    bool   // --update:  advance pointer to latest state
	Upgrade   bool   // --upgrade: promote current state to a new root epoch
	FlashRoot string
	TargetPath string
	Verbose   bool
}

// Rollback executes ltcodec rollback --downgrade.
// Moves the flash pointer back by one; if TargetPath is set, also restores
// the named .lt file to match the downgraded state.
//
// CLI: ltcodec rollback --downgrade [-target file.lt]
func Rollback(cfg RollbackConfig) error {
	if !cfg.Downgrade {
		return fmt.Errorf("rollback: use --downgrade to rollback the codec state")
	}

	root := resolveFlashRoot(cfg.FlashRoot, cfg.TargetPath)

	// Delegate to flash undo
	if err := codec.Flash(codec.FlashConfig{
		Action:    "undo",
		FlashRoot: root,
		Verbose:   cfg.Verbose,
	}); err != nil {
		return fmt.Errorf("rollback: %w", err)
	}

	// Optionally restore .lt to the now-active state
	if cfg.TargetPath != "" {
		activePath := codec.ActiveStatePath(root)
		if activePath == "" {
			return fmt.Errorf("rollback: no active state found in %s", root)
		}
		data, err := os.ReadFile(activePath)
		if err != nil {
			return fmt.Errorf("rollback: read state %s: %w", activePath, err)
		}
		if err := os.WriteFile(cfg.TargetPath, data, 0644); err != nil {
			return fmt.Errorf("rollback: restore to %s: %w", cfg.TargetPath, err)
		}
		fmt.Printf("[ROLLBACK] restored %s ← %s\n", cfg.TargetPath, activePath)
	}

	return nil
}

// Wheel executes ltcodec wheel --update | --upgrade.
//
//	--update:  advance the flash pointer to the most recent saved state
//	--upgrade: archive all current states and start a new root epoch from
//	           the current .lt (version-bump without expiry)
//
// CLI: ltcodec wheel --update|--upgrade [-target file.lt]
func Wheel(cfg WheelConfig) error {
	root := resolveFlashRoot(cfg.FlashRoot, cfg.TargetPath)

	switch {
	case cfg.Update:
		return wheelUpdate(root, cfg.Verbose)
	case cfg.Upgrade:
		return wheelUpgrade(root, cfg.TargetPath, cfg.Verbose)
	default:
		return fmt.Errorf("wheel: specify --update or --upgrade")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// wheelUpdate advances the flash pointer to the latest saved state.
func wheelUpdate(root string, verbose bool) error {
	idx, err := readFlashIndex(root)
	if err != nil {
		return fmt.Errorf("wheel --update: no flash history in %s", root)
	}
	latest := len(idx.States) - 1
	if idx.Current == latest {
		fmt.Printf("[WHEEL] already at latest state (state %d)\n", latest)
		return nil
	}
	for idx.Current < latest {
		if err := codec.Flash(codec.FlashConfig{
			Action:    "redo",
			FlashRoot: root,
			Verbose:   verbose,
		}); err != nil {
			return fmt.Errorf("wheel --update: redo: %w", err)
		}
		idx, _ = readFlashIndex(root)
	}
	fmt.Printf("[WHEEL] updated → state %d (latest)\n", idx.Current)
	return nil
}

// wheelUpgrade archives all current flash states and starts a fresh epoch.
// The current active state becomes the new state_0.  This is a version bump
// that preserves the stateless property — the new epoch has no expiry.
func wheelUpgrade(root, targetPath string, verbose bool) error {
	// Determine the source bytes for the new root state
	var sourceData []byte
	var err error

	activePath := codec.ActiveStatePath(root)
	if activePath != "" {
		sourceData, err = os.ReadFile(activePath)
		if err != nil {
			return fmt.Errorf("wheel --upgrade: read active state: %w", err)
		}
	} else if targetPath != "" {
		sourceData, err = os.ReadFile(targetPath)
		if err != nil {
			return fmt.Errorf("wheel --upgrade: read target %s: %w", targetPath, err)
		}
	} else {
		return fmt.Errorf("wheel --upgrade: no active flash state or -target specified")
	}

	// Read the current index (may not exist)
	idx, _ := readFlashIndex(root)

	// Archive old states
	if idx != nil && len(idx.States) > 0 {
		archDir := filepath.Join(root, "archive_"+time.Now().Format("20060102T150405"))
		if err := os.MkdirAll(archDir, 0755); err != nil {
			return fmt.Errorf("wheel --upgrade: mkdir archive: %w", err)
		}
		for _, s := range idx.States {
			src := filepath.Join(root, s)
			dst := filepath.Join(archDir, s)
			if d, err := os.ReadFile(src); err == nil {
				_ = os.WriteFile(dst, d, 0644)
			}
			_ = os.Remove(src)
		}
		if verbose {
			fmt.Printf("[WHEEL] archived %d prior states → %s\n", len(idx.States), archDir)
		}
	}

	// Write new root epoch
	if err := os.MkdirAll(root, 0755); err != nil {
		return fmt.Errorf("wheel --upgrade: mkdir flash root: %w", err)
	}
	newState := "state_0.lt"
	if err := os.WriteFile(filepath.Join(root, newState), sourceData, 0644); err != nil {
		return fmt.Errorf("wheel --upgrade: write new root: %w", err)
	}

	newIdx := codec.FlashIndexFile{
		Current:   0,
		States:    []string{newState},
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(newIdx, "", "  ")
	if err := os.WriteFile(filepath.Join(root, codec.FlashIndex), data, 0644); err != nil {
		return fmt.Errorf("wheel --upgrade: write index: %w", err)
	}

	priorCount := 0
	if idx != nil {
		priorCount = len(idx.States)
	}
	fmt.Printf("[WHEEL] upgraded → new root epoch  (archived %d prior states)\n", priorCount)
	return nil
}

// resolveFlashRoot picks the .ltflash directory from explicit config or target path.
func resolveFlashRoot(explicit, targetPath string) string {
	if explicit != "" {
		return explicit
	}
	if targetPath != "" {
		return filepath.Join(filepath.Dir(targetPath), codec.FlashDir)
	}
	return filepath.Join(".", codec.FlashDir)
}

// readFlashIndex reads the flash.json index from a flash root directory.
func readFlashIndex(root string) (*codec.FlashIndexFile, error) {
	data, err := os.ReadFile(filepath.Join(root, codec.FlashIndex))
	if err != nil {
		return nil, err
	}
	var f codec.FlashIndexFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
