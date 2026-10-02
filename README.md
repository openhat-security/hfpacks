# hfpacks

Standalone CLI that crawls the Hugging Face Hub and writes **local index packs**.
[runhug](https://github.com/openhat-security/runhug) installs the SQLite packs from
**this repo’s GitHub Releases** (`runhug init` / `runhug packs install` /
`runhug update --packs`).

Proxy handling is automated like a `ProxyPool`: fetch a list → shuffle → keep a batch →
`Next()` on a schedule / on errors → refetch when exhausted.

## Install

**End users of search indexes** should use [runhug](https://github.com/openhat-security/runhug) (`runhug packs install`) — not this CLI.

**Build / maintain packs:**

```bash
# Homebrew (after a tagged release)
brew install --cask openhat-security/tap/hfpacks

# curl installer (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/openhat-security/hfpacks/master/scripts/install.sh | bash

# npm
npm i -g hfpacks

# Go
go install github.com/openhat-security/hfpacks/cmd/hfpacks@latest

# from source
go build -o bin/hfpacks ./cmd/hfpacks
```

See [packaging/README.md](packaging/README.md) for apt, dnf, Scoop, winget, and AUR.

## Quick start

```bash
# optional but recommended
export HF_TOKEN=hf_…

# proxies are fetched automatically (ProxyScrape US list by default)
./bin/hfpacks build -out dist/index

# also emit csv + parquet alongside sqlite
./bin/hfpacks build -out dist/index -format sqlite,csv,parquet

# or export from existing .db files
./bin/hfpacks export -out dist/index -format csv,parquet
```

## Formats

| Format | File | Who uses it |
|--------|------|-------------|
| **sqlite** (default) | `index-<cat>.db` + `index-manifest.json` | **runhug** (required contract) |
| **csv** | `index-<cat>.csv` | spreadsheets / other tools |
| **parquet** | `index-<cat>.parquet` | analytics / data pipelines |

runhug only downloads SQLite assets from Releases. CSV/parquet are optional exports.

## Usage

```
hfpacks build [flags]
hfpacks export [flags]
hfpacks upsert <org/model>... [flags]
hfpacks categories
hfpacks help
```

### Upsert named models

```bash
hfpacks upsert Qwen/Qwen3-8B -db ~/.config/runhug/models.db
hfpacks upsert microsoft/Phi-4 -out dist/index -category text-generation
```

### Proxy automation

| Mode | How |
|------|-----|
| **Default** | GET `PROXY_LIST_URL` (default: ProxyScrape free US `ip:port` list), shuffle, keep `-proxy-batch` (10), rotate every `-switch-every` Hub calls (60). On 429/403/transport error, advance to next proxy. When the batch is empty, fetch again. |
| **Geonode auth on list IPs** | Set `GEONODE_USER` / `GEONODE_PASS` — applied to each scraped `ip:port`. |
| **Geonode gateway** | Set `GEONODE_PROXY_URL=http://user:pass@host:port` — expanded into `-proxy-batch` session URLs. |
| **Direct** | `-no-proxy` (CI / low volume with `HF_TOKEN`). |

### Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `-out` | `dist/index` | Output directory |
| `-categories` | all | Comma-separated ids |
| `-limit` | `0` | Max rows/category (`HFPACKS_INDEX_LIMIT` / `RUNHUG_INDEX_LIMIT`) |
| `-min-likes` / `-min-downloads` | `3` / `100` | Quality floors |
| `-format` | `sqlite` | `sqlite`, `csv`, `parquet` (comma-separated) |
| `-sleep-ms` | `250` | Pause between Hub pages |
| `-no-proxy` | off | Skip proxy pool |
| `-token` | `$HF_TOKEN` | Hub auth |

## Output (runhug-compatible)

```
dist/index/
  index-manifest.json
  index-text-generation.db
  index-text-generation.csv      # if -format includes csv
  index-text-generation.parquet  # if -format includes parquet
  …
```

Manifest pack entries include `rows`, optional `hub_total` (Hub category size at build),
and `quality_skipped`. runhug `packs list` uses these for coverage % / remaining.

`source_repo` defaults to `openhat-security/hfpacks`. CI
(`.github/workflows/release-index-packs.yml`) builds packs and uploads them to
**this** repo’s Releases only — never to runhug. runhug points here by default
(`RUNHUG_PACKS_REPO` override).

## Why Go

Pack builds are network-bound. Schema stays aligned with runhug’s SQLite pack format.
