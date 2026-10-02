package packs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openhat-security/hfpacks/internal/hf"
	"github.com/openhat-security/hfpacks/internal/index"
)

func TestParseFormats(t *testing.T) {
	got := ParseFormats("")
	if len(got) != 1 || got[0] != FormatSQLite {
		t.Fatalf("empty: %v", got)
	}
	got = ParseFormats("csv,parquet,sqlite,csv")
	if len(got) != 3 {
		t.Fatalf("dedupe: %v", got)
	}
	got = ParseFormats("db")
	if len(got) != 1 || got[0] != FormatSQLite {
		t.Fatalf("db alias: %v", got)
	}
}

func TestExportSidecarsCSVParquet(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index-text-generation.db")
	idx, err := index.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.InsertModel(hf.Model{
		ID: "org/m", Author: "org", Downloads: 10, Likes: 2,
		PipelineTag: "text-generation", Tags: []string{"license:apache-2.0"},
	}); err != nil {
		t.Fatal(err)
	}
	idx.Close()

	if err := ExportSidecars(dir, []string{FormatCSV, FormatParquet}); err != nil {
		t.Fatal(err)
	}
	csvPath := filepath.Join(dir, "index-text-generation.csv")
	pqPath := filepath.Join(dir, "index-text-generation.parquet")
	if _, err := os.Stat(csvPath); err != nil {
		t.Fatalf("csv: %v", err)
	}
	if _, err := os.Stat(pqPath); err != nil {
		t.Fatalf("parquet: %v", err)
	}
}
