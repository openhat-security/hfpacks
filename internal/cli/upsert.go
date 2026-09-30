package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openhat-security/hfpacks/internal/hf"
	"github.com/openhat-security/hfpacks/internal/index"
	"github.com/openhat-security/hfpacks/internal/packs"
	"github.com/openhat-security/hfpacks/internal/proxy"
)

func runUpsert(args []string) error {
	fs := newFlagSet("upsert")
	dbPath := fs.String("db", "", "SQLite index path (e.g. ~/.config/runhug/models.db or index-text-generation.db)")
	out := fs.String("out", "", "pack directory (use with -category)")
	category := fs.String("category", "", "pack category id under -out (e.g. text-generation, gguf)")
	noProxy := fs.Bool("no-proxy", false, "connect directly (skip auto proxy pool)")
	token := fs.String("token", "", "HF token (else HF_TOKEN env)")
	switchEvery := fs.Int("switch-every", 60, "next proxy every N Hub calls")
	proxyBatch := fs.Int("proxy-batch", 10, "proxies to keep per list fetch")
	proxyTries := fs.Int("proxy-tries", 50, "max proxies to try per Hub GET (0 = keep trying)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ids := fs.Args()
	if len(ids) == 0 {
		printUpsertHelp(os.Stderr)
		return fmt.Errorf("at least one org/model id required")
	}

	path, err := resolveUpsertDB(*dbPath, *out, *category)
	if err != nil {
		printUpsertHelp(os.Stderr)
		return err
	}

	tok := strings.TrimSpace(*token)
	if tok == "" {
		tok = strings.TrimSpace(os.Getenv("HF_TOKEN"))
	}

	pool, err := proxy.NewPool(proxy.PoolOpts{
		NoProxy:     *noProxy,
		SwitchEvery: *switchEvery,
		BatchSize:   *proxyBatch,
		Timeout:     45 * time.Second,
	})
	if err != nil {
		return err
	}
	client := hf.New(tok, pool)
	client.MaxProxyTries = *proxyTries

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	idx, err := index.Open(path)
	if err != nil {
		return err
	}
	defer idx.Close()

	fmt.Fprintf(os.Stderr, "[+] upsert → %s (%d model(s))\n", path, len(ids))
	inserted, updated := 0, 0
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		m, err := client.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		repo := m.RepoID()
		existed, err := idx.HasModel(repo)
		if err != nil {
			return err
		}
		if err := idx.InsertModel(*m); err != nil {
			return fmt.Errorf("upsert %s: %w", repo, err)
		}
		if existed {
			updated++
			fmt.Fprintf(os.Stderr, "  ~ %s  (updated)  likes=%d downloads=%d\n", repo, m.Likes, m.Downloads)
		} else {
			inserted++
			fmt.Fprintf(os.Stderr, "  + %s  (inserted) likes=%d downloads=%d\n", repo, m.Likes, m.Downloads)
		}
	}
	_ = idx.SetMetadata("last_update", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "[+] done — inserted %d, updated %d → %s\n", inserted, updated, path)
	return nil
}

func resolveUpsertDB(dbPath, out, category string) (string, error) {
	dbPath = strings.TrimSpace(dbPath)
	out = strings.TrimSpace(out)
	category = strings.TrimSpace(category)

	switch {
	case dbPath != "":
		return dbPath, nil
	case out != "" && category != "":
		if _, ok := packs.LookupCategory(category); !ok {
			return "", fmt.Errorf("unknown category %q (try: hfpacks categories)", category)
		}
		return filepath.Join(out, packs.DBFilenameFor(category)), nil
	case out != "" && category == "":
		return "", fmt.Errorf("-out requires -category (e.g. -category text-generation)")
	default:
		// Common local runhug index
		if home, err := os.UserHomeDir(); err == nil {
			cand := filepath.Join(home, ".config", "runhug", "models.db")
			if _, err := os.Stat(cand); err == nil {
				return cand, nil
			}
		}
		return "", fmt.Errorf("set -db <path> or -out <dir> -category <id> (no ~/.config/runhug/models.db found)")
	}
}

func printUpsertHelp(w *os.File) {
	fmt.Fprintln(w, `usage:
  hfpacks upsert <org/model> [org/model...] [flags]

flags:
  -db string         SQLite path to upsert into
  -out string        pack dir (with -category)
  -category string   pack id under -out (text-generation, gguf, …)
  -no-proxy          direct Hub
  -token string      HF token (else HF_TOKEN)

examples:
  hfpacks upsert Qwen/Qwen3-8B -db ~/.config/runhug/models.db
  hfpacks upsert Qwen/Qwen3-8B -out dist/index -category text-generation
  hfpacks upsert meta-llama/Llama-3.2-3B-Instruct microsoft/Phi-4 -no-proxy`)
}
