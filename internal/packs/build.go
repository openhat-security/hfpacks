package packs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adamsiwiec1/hfpacks/internal/hf"
	"github.com/adamsiwiec1/hfpacks/internal/index"
)

const (
	DefaultMinLikes     = 3
	DefaultMinDownloads = 100
	EnvIndexLimit       = "HFPACKS_INDEX_LIMIT"
	EnvIndexLimitLegacy = "RUNHUG_INDEX_LIMIT"
)

// BuildOpts configures pack generation.
type BuildOpts struct {
	OutDir       string
	Limit        int
	Categories   []string
	Sleep        time.Duration
	Full         bool
	SourceRepo   string
	MinLikes     int
	MinDownloads int
}

// DefaultLimit reads HFPACKS_INDEX_LIMIT or RUNHUG_INDEX_LIMIT.
func DefaultLimit() int {
	for _, key := range []string{EnvIndexLimit, EnvIndexLimitLegacy} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			n, err := strconv.Atoi(v)
			if err == nil && n >= 0 {
				return n
			}
		}
	}
	return 0
}

// ResolveLimit returns an explicit positive Limit, else env, else 0.
func ResolveLimit(limit int) int {
	if limit > 0 {
		return limit
	}
	return DefaultLimit()
}

// Build writes index-<cat>.db files and index-manifest.json into opts.OutDir.
func Build(ctx context.Context, client *hf.Client, opts BuildOpts) (*Manifest, error) {
	if opts.OutDir == "" {
		return nil, fmt.Errorf("out dir required")
	}
	opts.Limit = ResolveLimit(opts.Limit)
	if opts.MinLikes < 0 {
		opts.MinLikes = 0
		opts.MinDownloads = 0
	} else if opts.MinLikes == 0 && opts.MinDownloads == 0 {
		opts.MinLikes = DefaultMinLikes
		opts.MinDownloads = DefaultMinDownloads
	}
	if opts.Sleep <= 0 {
		opts.Sleep = 250 * time.Millisecond
	}
	if opts.SourceRepo == "" {
		opts.SourceRepo = DefaultSourceRepo
	}
	if err := os.MkdirAll(opts.OutDir, 0755); err != nil {
		return nil, err
	}

	cats := selectCategories(opts.Categories)
	if len(cats) == 0 {
		return nil, fmt.Errorf("no categories selected")
	}
	manifest := &Manifest{
		Version:     ManifestVersion,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		SourceRepo:  opts.SourceRepo,
		Packs:       make([]PackInfo, 0, len(cats)),
	}

	for _, cat := range cats {
		if err := ctx.Err(); err != nil {
			return manifest, err
		}
		info, err := buildOne(ctx, client, opts, cat)
		if err != nil {
			return manifest, fmt.Errorf("build %s: %w", cat.ID, err)
		}
		manifest.Packs = append(manifest.Packs, info)
	}

	manPath := filepath.Join(opts.OutDir, ManifestFilename)
	if err := WriteManifest(manPath, manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func selectCategories(ids []string) []Category {
	if len(ids) == 0 {
		return DefaultCategories()
	}
	var out []Category
	for _, id := range ids {
		c, ok := LookupCategory(id)
		if !ok {
			fmt.Fprintf(os.Stderr, "[!] unknown category %q — skipping\n", id)
			continue
		}
		out = append(out, c)
	}
	return out
}

func buildOne(ctx context.Context, client *hf.Client, opts BuildOpts, cat Category) (PackInfo, error) {
	dbName := DBFilenameFor(cat.ID)
	dbPath := filepath.Join(opts.OutDir, dbName)
	_ = os.Remove(dbPath)

	idx, err := index.Open(dbPath)
	if err != nil {
		return PackInfo{}, err
	}
	defer idx.Close()

	pipelines := []string{}
	if cat.Pipeline != "" {
		pipelines = append(pipelines, cat.Pipeline)
	}
	pipelines = append(pipelines, cat.ExtraPipelines...)
	if len(pipelines) == 0 && cat.Filter == "" {
		pipelines = []string{"any"}
	}

	seen := map[string]bool{}
	var maxLM time.Time
	total := 0
	skippedDup := 0
	filterSkipped := 0
	unlimited := opts.Limit <= 0

	perPipeLimit := opts.Limit
	if !unlimited && len(pipelines) > 1 {
		perPipeLimit = (opts.Limit + len(pipelines) - 1) / len(pipelines)
	}

	maxPages := 0
	if !unlimited {
		maxPages = (opts.Limit / 50) + 200
		if maxPages < 200 {
			maxPages = 200
		}
		if maxPages > 50000 {
			maxPages = 50000
		}
	}

	fetch := func(task string) error {
		if !unlimited && total >= opts.Limit {
			return nil
		}
		lim := 0
		if !unlimited {
			remaining := opts.Limit - total
			if remaining <= 0 {
				return nil
			}
			lim = perPipeLimit
			if lim > remaining {
				lim = remaining
			}
		}
		models, err := client.ListModels(ctx, hf.ListOpts{
			Task:          task,
			Filter:        cat.Filter,
			Sort:          "downloads",
			Direction:     "-1",
			Limit:         lim,
			PageSize:      100,
			MaxPages:      maxPages,
			Sleep:         opts.Sleep,
			Full:          opts.Full,
			MinLikes:      opts.MinLikes,
			MinDownloads:  opts.MinDownloads,
			FilterSkipped: &filterSkipped,
		})
		if err != nil {
			return err
		}
		keptBefore := total
		for _, m := range models {
			id := m.RepoID()
			if id == "" || seen[id] {
				if id != "" && seen[id] {
					skippedDup++
				}
				continue
			}
			seen[id] = true
			if err := idx.InsertModel(m); err != nil {
				return err
			}
			total++
			if lm := parseLM(m.LastModified); lm.After(maxLM) {
				maxLM = lm
			}
			if !unlimited && total >= opts.Limit {
				break
			}
		}
		fmt.Fprintf(os.Stderr, "  [%s] task=%q kept+=%d total=%d (filter_skip=%d dup_skip=%d)\n",
			cat.ID, task, total-keptBefore, total, filterSkipped, skippedDup)
		return nil
	}

	fmt.Fprintf(os.Stderr, "[+] building pack %s (limit=%s min_likes=%d min_downloads=%d)\n",
		cat.ID, limitLabel(opts.Limit), opts.MinLikes, opts.MinDownloads)

	if len(pipelines) == 0 {
		if err := fetch(""); err != nil {
			return PackInfo{}, err
		}
	} else {
		for _, p := range pipelines {
			if !unlimited && total >= opts.Limit {
				break
			}
			task := p
			if task == "any" {
				task = ""
			}
			if err := fetch(task); err != nil {
				return PackInfo{}, err
			}
		}
	}

	fmt.Fprintf(os.Stderr, "  [%s] done rows=%d filter_skip=%d dup_skip=%d\n",
		cat.ID, total, filterSkipped, skippedDup)

	wm := formatWatermark(maxLM)
	_ = idx.SetMetadata("created_at", time.Now().UTC().Format(time.RFC3339))
	_ = idx.SetMetadata("last_update", time.Now().UTC().Format(time.RFC3339))
	_ = idx.SetMetadata("category", cat.ID)
	if wm != "" {
		_ = idx.SetMetadata(MetadataKeyWatermark(cat.ID), wm)
		_ = idx.SetMetadata("watermark", wm)
	}
	if err := idx.Close(); err != nil {
		return PackInfo{}, err
	}

	sum, err := FileSHA256(dbPath)
	if err != nil {
		return PackInfo{}, err
	}
	fi, err := os.Stat(dbPath)
	if err != nil {
		return PackInfo{}, err
	}

	pipeline := cat.Pipeline
	if len(cat.ExtraPipelines) > 0 {
		pipeline = strings.Join(append([]string{cat.Pipeline}, cat.ExtraPipelines...), ",")
	}

	return PackInfo{
		ID:         cat.ID,
		Title:      cat.Title,
		Pipeline:   strings.Trim(pipeline, ","),
		Filter:     cat.Filter,
		Rows:       total,
		SizeBytes:  fi.Size(),
		SHA256:     sum,
		DBFilename: dbName,
		Watermark:  wm,
	}, nil
}

func limitLabel(n int) string {
	if n <= 0 {
		return "unlimited"
	}
	return strconv.Itoa(n)
}
