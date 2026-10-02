package packs

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openhat-security/hfpacks/internal/index"
	"github.com/parquet-go/parquet-go"
)

// Export formats for pack data (sqlite is produced by Build; csv/parquet are optional).
const (
	FormatSQLite  = "sqlite"
	FormatCSV     = "csv"
	FormatParquet = "parquet"
)

// ParseFormats splits a comma-separated format list. Empty → sqlite only.
func ParseFormats(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{FormatSQLite}
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		switch p {
		case FormatSQLite, FormatCSV, FormatParquet, "db":
			if p == "db" {
				p = FormatSQLite
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{FormatSQLite}
	}
	return out
}

// NeedsSQLite reports whether sqlite output is requested.
func NeedsSQLite(formats []string) bool {
	for _, f := range formats {
		if f == FormatSQLite {
			return true
		}
	}
	return false
}

// ExportSidecars writes csv and/or parquet next to each index-*.db in dir.
func ExportSidecars(dir string, formats []string) error {
	wantCSV, wantParquet := false, false
	for _, f := range formats {
		switch f {
		case FormatCSV:
			wantCSV = true
		case FormatParquet:
			wantParquet = true
		}
	}
	if !wantCSV && !wantParquet {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "index-") || !strings.HasSuffix(name, ".db") {
			continue
		}
		dbPath := filepath.Join(dir, name)
		base := strings.TrimSuffix(name, ".db")
		idx, err := index.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", name, err)
		}
		rows, err := idx.AllRows()
		idx.Close()
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if wantCSV {
			out := filepath.Join(dir, base+".csv")
			if err := WriteCSV(out, rows); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "  wrote %s (%d rows)\n", out, len(rows))
		}
		if wantParquet {
			out := filepath.Join(dir, base+".parquet")
			if err := WriteParquet(out, rows); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "  wrote %s (%d rows)\n", out, len(rows))
		}
	}
	return nil
}

// WriteCSV writes model rows as CSV.
func WriteCSV(path string, rows []index.ModelRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{
		"id", "author", "description", "tags", "likes", "downloads",
		"library_name", "license", "pipeline_tag", "last_modified",
	}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{
			r.ID, r.Author, r.Description, r.Tags,
			strconv.Itoa(r.Likes), strconv.FormatInt(r.Downloads, 10),
			r.LibraryName, r.License, r.PipelineTag,
			strconv.FormatInt(r.LastModified, 10),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// parquetRow mirrors ModelRow for parquet-go schema inference.
type parquetRow struct {
	ID           string `parquet:"id"`
	Author       string `parquet:"author"`
	Description  string `parquet:"description"`
	Tags         string `parquet:"tags"`
	Likes        int32  `parquet:"likes"`
	Downloads    int64  `parquet:"downloads"`
	LibraryName  string `parquet:"library_name"`
	License      string `parquet:"license"`
	PipelineTag  string `parquet:"pipeline_tag"`
	LastModified int64  `parquet:"last_modified"`
}

// WriteParquet writes model rows as a parquet file.
func WriteParquet(path string, rows []index.ModelRow) error {
	out := make([]parquetRow, len(rows))
	for i, r := range rows {
		out[i] = parquetRow{
			ID: r.ID, Author: r.Author, Description: r.Description, Tags: r.Tags,
			Likes: int32(r.Likes), Downloads: r.Downloads,
			LibraryName: r.LibraryName, License: r.License,
			PipelineTag: r.PipelineTag, LastModified: r.LastModified,
		}
	}
	return parquet.WriteFile(path, out)
}
