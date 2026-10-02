package packs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

const (
	ManifestVersion   = 1
	ManifestFilename  = "index-manifest.json"
	DefaultSourceRepo = "openhat-security/hfpacks"
)

// Manifest lists category packs (runhug-compatible).
type Manifest struct {
	Version     int        `json:"version"`
	GeneratedAt string     `json:"generated_at"`
	SourceRepo  string     `json:"source_repo,omitempty"`
	Packs       []PackInfo `json:"packs"`
}

// PackInfo describes one release asset DB.
type PackInfo struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Pipeline       string `json:"pipeline,omitempty"`
	Filter         string `json:"filter,omitempty"`
	Rows           int    `json:"rows"`
	HubTotal       int    `json:"hub_total,omitempty"`       // Hub models matching category (approx at build)
	QualitySkipped int    `json:"quality_skipped,omitempty"` // below min likes/downloads during build
	SizeBytes      int64  `json:"size_bytes"`
	SHA256         string `json:"sha256"`
	DBFilename     string `json:"db_filename"`
	Watermark      string `json:"watermark"`
}

// DBFilenameFor returns the release asset name for a category id.
func DBFilenameFor(id string) string {
	return "index-" + id + ".db"
}

// WriteManifest writes the manifest as indented JSON.
func WriteManifest(path string, m *Manifest) error {
	if m.Version == 0 {
		m.Version = ManifestVersion
	}
	if m.GeneratedAt == "" {
		m.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// FileSHA256 returns the hex-encoded SHA-256 of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// MetadataKeyWatermark is the metadata key for a category watermark.
func MetadataKeyWatermark(categoryID string) string {
	return "pack:" + categoryID + ":watermark"
}

func parseLM(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

func formatWatermark(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
