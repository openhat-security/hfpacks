package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/adamsiwiec1/hfpacks/internal/hf"
)

// Index is a runhug-compatible models SQLite database.
type Index struct {
	db   *sql.DB
	path string
}

// Open opens or creates the search index at path.
func Open(path string) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create index directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	idx := &Index{db: db, path: path}
	if err := idx.createTables(); err != nil {
		db.Close()
		return nil, err
	}
	return idx, nil
}

// Close closes the database.
func (idx *Index) Close() error {
	return idx.db.Close()
}

func (idx *Index) createTables() error {
	schema := `
CREATE TABLE IF NOT EXISTS models (
	id TEXT PRIMARY KEY,
	author TEXT,
	description TEXT,
	tags TEXT,
	likes INTEGER DEFAULT 0,
	downloads INTEGER DEFAULT 0,
	library_name TEXT,
	license TEXT,
	pipeline_tag TEXT,
	last_modified INTEGER,
	indexed_at INTEGER DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_models_author ON models(author);
CREATE INDEX IF NOT EXISTS idx_models_likes ON models(likes DESC);
CREATE INDEX IF NOT EXISTS idx_models_downloads ON models(downloads DESC);
CREATE INDEX IF NOT EXISTS idx_models_library ON models(library_name);
CREATE INDEX IF NOT EXISTS idx_models_license ON models(license);
CREATE INDEX IF NOT EXISTS idx_models_pipeline ON models(pipeline_tag);
CREATE INDEX IF NOT EXISTS idx_models_id_lower ON models(LOWER(id));

CREATE TABLE IF NOT EXISTS metadata (
	key TEXT PRIMARY KEY,
	value TEXT
);
`
	_, err := idx.db.Exec(schema)
	return err
}

// InsertModel upserts a Hub model into the index.
func (idx *Index) InsertModel(m hf.Model) error {
	id := m.RepoID()
	if id == "" {
		return nil
	}
	tagsJSON, _ := json.Marshal(m.Tags)
	lastMod := time.Time{}
	if m.LastModified != "" {
		lastMod, _ = time.Parse(time.RFC3339, m.LastModified)
	}
	_, err := idx.db.Exec(`
		INSERT INTO models (id, author, description, tags, likes, downloads, library_name, license, pipeline_tag, last_modified)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			author = excluded.author,
			description = excluded.description,
			tags = excluded.tags,
			likes = excluded.likes,
			downloads = excluded.downloads,
			library_name = excluded.library_name,
			license = excluded.license,
			pipeline_tag = excluded.pipeline_tag,
			last_modified = excluded.last_modified,
			indexed_at = strftime('%s', 'now')
	`, id, m.Author, m.CardDescription(), string(tagsJSON), m.Likes, m.Downloads,
		m.LibraryName, m.License(), m.PipelineTag, lastMod.Unix())
	return err
}

// SetMetadata stores a metadata key-value pair.
func (idx *Index) SetMetadata(key, value string) error {
	_, err := idx.db.Exec(`
		INSERT INTO metadata (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}
