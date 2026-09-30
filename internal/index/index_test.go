package index

import (
	"path/filepath"
	"testing"

	"github.com/openhat-security/hfpacks/internal/hf"
)

func TestInsertModelUpsert(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.db")
	idx, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	ok, err := idx.HasModel("org/a")
	if err != nil || ok {
		t.Fatalf("HasModel empty: ok=%v err=%v", ok, err)
	}

	m := hf.Model{ID: "org/a", Author: "org", Likes: 1, Downloads: 10, PipelineTag: "text-generation"}
	if err := idx.InsertModel(m); err != nil {
		t.Fatal(err)
	}
	ok, err = idx.HasModel("org/a")
	if err != nil || !ok {
		t.Fatalf("HasModel after insert: ok=%v err=%v", ok, err)
	}

	m.Likes = 99
	m.Downloads = 1000
	if err := idx.InsertModel(m); err != nil {
		t.Fatal(err)
	}

	var likes int
	var downloads int64
	err = idx.db.QueryRow(`SELECT likes, downloads FROM models WHERE id = ?`, "org/a").Scan(&likes, &downloads)
	if err != nil {
		t.Fatal(err)
	}
	if likes != 99 || downloads != 1000 {
		t.Fatalf("likes=%d downloads=%d", likes, downloads)
	}
}
