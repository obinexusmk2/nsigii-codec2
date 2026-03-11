// Package format defines the NSIGII Linkable-Then-Executable (.lt) file format.
//
// An .lt file is a zip archive — "linkable" means any resource type can be
// assembled into it (HTML, binary, fonts, email, etc.); "then executable"
// means the host system can unpack and run/serve it.  The format is
// STATELESS: version numbers identify artefacts but never expire them.
//
// Archive layout:
//
//	.lt.meta    – JSON metadata  (UUID, content-type, stateless flag)
//	.lt.payload – transformed payload bytes
//	.lt.parity  – JSON parity record  (checksum + even/odd counts)
//	.lt.index   – JSON section index
package format

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Section names — the four canonical .lt sections.
const (
	SectionMeta    = ".lt.meta"
	SectionPayload = ".lt.payload"
	SectionParity  = ".lt.parity"
	SectionIndex   = ".lt.index"

	Magic          = "LTFORMAT"
	VersionMajor   = 1
)

// Meta is the .lt.meta section — identity, provenance, stateless flag.
type Meta struct {
	Magic        string     `json:"magic"`
	Version      [4]uint8   `json:"version"`       // [major,minor,patch,build] — ID only, never expiry
	UUID         string     `json:"uuid"`
	CreatedAt    time.Time  `json:"created_at"`
	ContentType  string     `json:"content_type"`  // "binary", "text", "html", "email", …
	OriginalName string     `json:"original_name"`
	// Stateless: this artefact can always be integrated into any ecosystem;
	// it does not expire via the standard major.minor.patch versioning scheme.
	Stateless    bool       `json:"stateless"`
	// HereNow encodes the space-time state at encode time.
	SpaceThen    string     `json:"space_then"` // "present_in_space_then_time"
	TimeThen     string     `json:"time_then"`  // "present_in_time_then_space"
}

// ParityRecord is the .lt.parity section — integrity verification.
type ParityRecord struct {
	SectionName string `json:"section"`
	ByteCount   int    `json:"byte_count"`
	EvenCount   int    `json:"even_count"`
	OddCount    int    `json:"odd_count"`
	Checksum    uint64 `json:"checksum"` // rotating XOR checksum
}

// IndexEntry is one row of the .lt.index section.
type IndexEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`   // META | PAYL | PARY | INDX
	Size int64  `json:"size"`
}

// NewMeta creates a Meta with a fresh random UUID.
func NewMeta(contentType, originalName string) *Meta {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	uuid := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])

	return &Meta{
		Magic:        Magic,
		Version:      [4]uint8{VersionMajor, 0, 0, 0},
		UUID:         uuid,
		CreatedAt:    time.Now(),
		ContentType:  contentType,
		OriginalName: originalName,
		Stateless:    true,
		SpaceThen:    "present_in_space_then_time",
		TimeThen:     "present_in_time_then_space",
	}
}

// Build assembles a .lt archive from a Meta and transformed payload bytes.
// Returns the raw zip bytes ready to write to disk.
func Build(meta *Meta, payload []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// -- .lt.meta --------------------------------------------------------
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("lt.Build: marshal meta: %w", err)
	}
	if err := writeSection(zw, SectionMeta, metaJSON); err != nil {
		return nil, err
	}

	// -- .lt.payload -----------------------------------------------------
	if err := writeSection(zw, SectionPayload, payload); err != nil {
		return nil, err
	}

	// -- .lt.parity ------------------------------------------------------
	pr := computeParity(SectionPayload, payload)
	parityJSON, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("lt.Build: marshal parity: %w", err)
	}
	if err := writeSection(zw, SectionParity, parityJSON); err != nil {
		return nil, err
	}

	// -- .lt.index -------------------------------------------------------
	idx := []IndexEntry{
		{Name: SectionMeta, Type: "META", Size: int64(len(metaJSON))},
		{Name: SectionPayload, Type: "PAYL", Size: int64(len(payload))},
		{Name: SectionParity, Type: "PARY", Size: int64(len(parityJSON))},
	}
	idxJSON, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("lt.Build: marshal index: %w", err)
	}
	// Add index entry for itself
	idx = append(idx, IndexEntry{Name: SectionIndex, Type: "INDX", Size: int64(len(idxJSON))})
	idxJSON, _ = json.MarshalIndent(idx, "", "  ")
	if err := writeSection(zw, SectionIndex, idxJSON); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("lt.Build: close archive: %w", err)
	}

	return buf.Bytes(), nil
}

// Open reads a .lt archive and extracts Meta, payload, and index.
func Open(data []byte) (*Meta, []byte, []IndexEntry, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("lt.Open: invalid archive: %w", err)
	}

	var meta *Meta
	var payload []byte
	var idx []IndexEntry
	var storedParity *ParityRecord

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("lt.Open: open section %s: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("lt.Open: read section %s: %w", f.Name, err)
		}

		switch f.Name {
		case SectionMeta:
			meta = &Meta{}
			if err := json.Unmarshal(content, meta); err != nil {
				return nil, nil, nil, fmt.Errorf("lt.Open: parse meta: %w", err)
			}
		case SectionPayload:
			payload = content
		case SectionParity:
			storedParity = &ParityRecord{}
			if err := json.Unmarshal(content, storedParity); err != nil {
				return nil, nil, nil, fmt.Errorf("lt.Open: parse parity: %w", err)
			}
		case SectionIndex:
			if err := json.Unmarshal(content, &idx); err != nil {
				return nil, nil, nil, fmt.Errorf("lt.Open: parse index: %w", err)
			}
		}
	}

	if meta == nil {
		return nil, nil, nil, fmt.Errorf("lt.Open: missing %s", SectionMeta)
	}
	if payload == nil {
		return nil, nil, nil, fmt.Errorf("lt.Open: missing %s", SectionPayload)
	}

	// Parity integrity check
	if storedParity != nil {
		if !verifyParity(payload, storedParity) {
			return nil, nil, nil, fmt.Errorf("lt.Open: parity mismatch — payload may be corrupted")
		}
	}

	return meta, payload, idx, nil
}

// DetectContentType infers a content-type string from a filename extension.
func DetectContentType(name string) string {
	if len(name) == 0 {
		return "binary"
	}
	// Find last dot
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			ext := name[i+1:]
			switch ext {
			case "html", "htm":
				return "text/html"
			case "css":
				return "text/css"
			case "js", "mjs":
				return "application/javascript"
			case "json":
				return "application/json"
			case "txt", "md":
				return "text/plain"
			case "eml", "msg":
				return "message/email"
			case "png", "jpg", "jpeg", "gif", "svg":
				return "image/" + ext
			case "mp4", "mkv", "mov", "avi":
				return "video/" + ext
			case "xdt":
				return "application/xdt"
			case "lt":
				return "application/lt"
			default:
				return "application/" + ext
			}
		}
	}
	return "binary"
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

func writeSection(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("lt: create section %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("lt: write section %s: %w", name, err)
	}
	return nil
}

func computeParity(name string, data []byte) *ParityRecord {
	pr := &ParityRecord{SectionName: name, ByteCount: len(data)}
	var cs uint64
	for _, b := range data {
		if b%2 == 0 {
			pr.EvenCount++
		} else {
			pr.OddCount++
		}
		cs ^= uint64(b)
		cs = (cs << 1) | (cs >> 63) // rotating left 1
	}
	pr.Checksum = cs
	return pr
}

func verifyParity(data []byte, stored *ParityRecord) bool {
	computed := computeParity(stored.SectionName, data)
	return computed.Checksum == stored.Checksum &&
		computed.EvenCount == stored.EvenCount &&
		computed.OddCount == stored.OddCount
}
