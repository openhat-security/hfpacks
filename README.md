# hfpacks

Standalone CLI that crawls the Hugging Face Hub and writes **category SQLite index packs** compatible with [runhug](https://github.com/adamsiwiec1/runhug) (`index-*.db` + `index-manifest.json`).

Proxy handling is automated like a `ProxyPool`: fetch a list → shuffle → keep a batch → `Next()` on a schedule / on errors → refetch when exhausted. End users still only download packs via `runhug init` / `update --packs`.

## Install

```bash
go build -o bin/hfpacks ./cmd/hfpacks
```

## Quick start

```bash
# optional but recommended
export HF_TOKEN=hf_…

# proxies are fetched automatically (ProxyScrape US list by default)
./bin/hfpacks build -out dist/index
```

No `GEONODE_PROXY_URL` required for the default path.

## Usage

```
hfpacks build [flags]
hfpacks categories
hfpacks help
```

### Proxy automation

| Mode | How |
|------|-----|
| **Default** | GET `PROXY_LIST_URL` (default: ProxyScrape free US `ip:port` list), shuffle, keep `-proxy-batch` (10), rotate every `-switch-every` Hub calls (60). On 429/403/transport error, advance to next proxy (up to 3 attempts per page). When the batch is empty, fetch again. |
| **Geonode auth on list IPs** | Set `GEONODE_USER` / `GEONODE_PASS` — applied to each scraped `ip:port`. |
| **Geonode gateway** | Set `GEONODE_PROXY_URL=http://user:pass@host:port` — expanded into `-proxy-batch` session URLs (`user-session-N`) so the pool still rotates. |
| **Direct** | `-no-proxy` (CI / low volume with `HF_TOKEN`). |

```bash
# custom list feed
export PROXY_LIST_URL='https://…'

# Geonode username/password on scraped IPs
export GEONODE_USER=…
export GEONODE_PASS=…

# or sticky Geonode gateway with session rotation
export GEONODE_PROXY_URL='http://USER:PASS@proxy.geonode.io:9000'

./bin/hfpacks build -out dist/index -switch-every 60 -proxy-batch 10 -proxy-tries 50
# keep trying forever on flaky free proxies:
./bin/hfpacks build -out dist/index -proxy-tries 0
```

### Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `-out` | `dist/index` | Output directory |
| `-categories` | all | Comma-separated ids |
| `-limit` | `0` | Max rows/category (`HFPACKS_INDEX_LIMIT` / `RUNHUG_INDEX_LIMIT`) |
| `-min-likes` / `-min-downloads` | `3` / `100` | Quality floors |
| `-sleep-ms` | `250` | Pause between Hub pages |
| `-no-proxy` | off | Skip proxy pool |
| `-switch-every` | `60` | Next proxy every N successful Hub calls |
| `-proxy-batch` | `10` | Proxies kept per list fetch |
| `-proxy-tries` | `50` | Max proxies to try per Hub page (`0` = keep trying / refetch lists) |
| `-token` | `$HF_TOKEN` | Hub auth |

## Output (runhug-compatible)

```
dist/index/
  index-manifest.json
  index-text-generation.db
  …
```

## Why Go

Pack builds are network-bound. Schema stays aligned with runhug’s SQLite pack format.
