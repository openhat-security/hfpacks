package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/openhat-security/hfpacks/internal/hf"
	"github.com/openhat-security/hfpacks/internal/packs"
	"github.com/openhat-security/hfpacks/internal/proxy"
)

const usage = `hfpacks - Hugging Face Hub → local index packs (producer for runhug)

usage:
  hfpacks build [flags]                    crawl Hub, write packs + index-manifest.json
  hfpacks export [flags]                   convert existing index-*.db → csv / parquet
  hfpacks upsert <org/model>... [flags]    fetch named models and upsert into a SQLite index
  hfpacks categories                       list category ids
  hfpacks help                             show this message

build flags:
  -out string           output dir (default "dist/index")
  -categories string    comma-separated ids (default: all)
  -limit int            max rows per category (0 = unlimited)
  -min-likes int        quality floor (default 3)
  -min-downloads int    quality floor (default 100)
  -sleep-ms int         pause between pages (default 250)
  -full                 request Hub expand fields (default true)
  -format string        sqlite,csv,parquet (comma-separated; default sqlite)
  -no-proxy             connect directly (skip auto proxy pool)
  -token string         HF token (else HF_TOKEN env)
  -switch-every int     next proxy every N Hub calls (default 60)
  -proxy-batch int      proxies to keep per fetch (default 10)
  -proxy-tries int      max proxies to try per Hub page (default 50; 0 = keep trying)
  -source-repo string   manifest source_repo (default openhat-security/hfpacks)

export flags:
  -out string           directory with index-*.db (default "dist/index")
  -format string        csv,parquet (default csv,parquet)

upsert flags:
  -db string            SQLite path (e.g. ~/.config/runhug/models.db)
  -out string           pack dir (use with -category)
  -category string      pack id under -out (text-generation, gguf, …)
  -no-proxy / -token / -switch-every / -proxy-batch / -proxy-tries
                        same as build (default db: ~/.config/runhug/models.db if present)

proxy (automated, no manual per-run URL required):
  fetches a free proxy list (ProxyScrape US by default), shuffles, batches,
  and rotates — same idea as a ProxyPool.Next() loop. On failure, keep
  trying the next proxy (and refetch the list when the batch is empty).
  override list: PROXY_LIST_URL
  optional auth on list IPs: GEONODE_USER / GEONODE_PASS
  sticky Geonode gateway: GEONODE_PROXY_URL (expanded into session proxies)

examples:
  hfpacks build -out dist/index
  hfpacks build -format sqlite,csv,parquet -no-proxy -token $HF_TOKEN
  hfpacks export -out dist/index -format csv,parquet
  hfpacks upsert Qwen/Qwen3-8B -db ~/.config/runhug/models.db
  hfpacks categories
`

// Run dispatches subcommands.
func Run(args []string) error {
	cmd := "help"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd = args[0]
		args = args[1:]
	} else if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		cmd = "help"
		args = args[1:]
	}

	switch cmd {
	case "build":
		return runBuild(args)
	case "export":
		return runExport(args)
	case "upsert":
		return runUpsert(args)
	case "categories":
		return runCategories(args)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func runCategories(args []string) error {
	fs := newFlagSet("categories")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	for _, c := range packs.DefaultCategories() {
		fmt.Printf("%s\t%s\tpipeline=%s\tfilter=%s\n", c.ID, c.Title, c.Pipeline, c.Filter)
	}
	return nil
}

func runBuild(args []string) error {
	fs := newFlagSet("build")
	out := fs.String("out", "dist/index", "output directory for dbs + manifest")
	cats := fs.String("categories", "", "comma-separated category ids (default: all)")
	limit := fs.Int("limit", 0, "max rows per category (0 = unlimited)")
	minLikes := fs.Int("min-likes", packs.DefaultMinLikes, "skip models with fewer likes (0 disables with -min-downloads 0)")
	minDownloads := fs.Int("min-downloads", packs.DefaultMinDownloads, "skip models with fewer downloads")
	sleepMs := fs.Int("sleep-ms", 250, "sleep between Hub pages")
	full := fs.Bool("full", true, "request Hub expand fields")
	noProxy := fs.Bool("no-proxy", false, "connect directly (skip auto proxy pool)")
	token := fs.String("token", "", "HF token (else HF_TOKEN env)")
	switchEvery := fs.Int("switch-every", 60, "next proxy every N Hub calls")
	proxyBatch := fs.Int("proxy-batch", 10, "proxies to keep per list fetch")
	proxyTries := fs.Int("proxy-tries", 50, "max proxies to try per Hub page (0 = keep trying)")
	sourceRepo := fs.String("source-repo", packs.DefaultSourceRepo, "manifest source_repo")
	format := fs.String("format", packs.FormatSQLite, "output formats: sqlite,csv,parquet (comma-separated)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	formats := packs.ParseFormats(*format)
	if !packs.NeedsSQLite(formats) {
		return fmt.Errorf("build always writes sqlite (runhug contract); include sqlite in -format or omit -format")
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

	var catIDs []string
	if s := strings.TrimSpace(*cats); s != "" {
		for _, p := range strings.Split(s, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				catIDs = append(catIDs, p)
			}
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 12*time.Hour)
	defer cancel()

	effLimit := packs.ResolveLimit(*limit)
	fmt.Fprintf(os.Stderr, "[+] building packs → %s (limit=%s min_likes=%d min_downloads=%d proxy=%v)\n",
		*out, limitLabel(effLimit), *minLikes, *minDownloads, !*noProxy)
	if tok == "" {
		fmt.Fprintf(os.Stderr, "[!] no HF_TOKEN — Hub rate limits will be lower\n")
	}

	opts := packs.BuildOpts{
		OutDir:       *out,
		Limit:        *limit,
		Categories:   catIDs,
		Sleep:        time.Duration(*sleepMs) * time.Millisecond,
		Full:         *full,
		SourceRepo:   *sourceRepo,
		MinLikes:     *minLikes,
		MinDownloads: *minDownloads,
	}
	if *minLikes == 0 && *minDownloads == 0 {
		opts.MinLikes = -1 // disable quality floors
	}

	man, err := packs.Build(ctx, client, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[+] done — %d packs in %s\n", len(man.Packs), *out)
	for _, p := range man.Packs {
		fmt.Fprintf(os.Stderr, "    %s  rows=%d  %s\n", p.ID, p.Rows, p.DBFilename)
	}
	if err := packs.ExportSidecars(*out, formats); err != nil {
		return fmt.Errorf("export sidecars: %w", err)
	}
	return nil
}

func runExport(args []string) error {
	fs := newFlagSet("export")
	out := fs.String("out", "dist/index", "directory containing index-*.db")
	format := fs.String("format", "csv,parquet", "csv,parquet (comma-separated)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	formats := packs.ParseFormats(*format)
	var side []string
	for _, f := range formats {
		if f == packs.FormatCSV || f == packs.FormatParquet {
			side = append(side, f)
		}
	}
	if len(side) == 0 {
		return fmt.Errorf("export needs -format csv and/or parquet")
	}
	fmt.Fprintf(os.Stderr, "[+] exporting from %s → %s\n", *out, strings.Join(side, ","))
	return packs.ExportSidecars(*out, side)
}

func limitLabel(n int) string {
	if n <= 0 {
		return "unlimited"
	}
	return strconv.Itoa(n)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	return fs
}

// parseFlags parses flags that may be interspersed with positional args.
func parseFlags(fs *flag.FlagSet, args []string) error {
	bools := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		type boolFlag interface{ IsBoolFlag() bool }
		if v, ok := f.Value.(boolFlag); ok && v.IsBoolFlag() {
			bools[f.Name] = true
		}
	})
	var flagArgs, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' || a == "-" {
			positional = append(positional, a)
			continue
		}
		flagArgs = append(flagArgs, a)
		if strings.Contains(a, "=") {
			continue
		}
		name := strings.TrimLeft(a, "-")
		if bools[name] {
			continue
		}
		if fs.Lookup(name) == nil {
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return fs.Parse(append(flagArgs, positional...))
}
