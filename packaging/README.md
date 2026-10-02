# Packaging

Release channels for **hfpacks** (mirrors [runhug](https://github.com/openhat-security/runhug) layout).

| Channel | Install |
|--------|---------|
| GitHub Release | `curl -fsSL …/scripts/install.sh \| bash` |
| Homebrew | `brew install --cask openhat-security/tap/hfpacks` |
| apt | `curl -fsSL …/install-hfpacks-apt.sh \| sudo bash` (Pages: `openhat-security/packages`) |
| dnf | `curl -fsSL …/install-hfpacks-dnf.sh \| sudo bash` |
| AUR | `yay -S hfpacks-bin` (publish `packaging/aur/PKGBUILD` to AUR) |
| Scoop | `scoop bucket add openhat …` then `scoop install hfpacks` |
| winget | `winget install OpenHatSecurity.Hfpacks` (after manifest PR) |
| npm | `npm i -g hfpacks` |
| Go | `go install github.com/openhat-security/hfpacks/cmd/hfpacks@latest` |

Tagged releases (`v*`) run GoReleaser (`.github/workflows/release.yml`).

## Secrets (on `openhat-security/hfpacks`)

Same as runhug: `NPM_TOKEN`, `PACKAGING_TOKEN` (or `HOMEBREW_TAP_TOKEN` / `SCOOP_TOKEN`), optional `WINGET_PAT`, `HF_TOKEN` for index-pack CI.

Copy org secrets from the **runhug** repo into **hfpacks** repo settings (same names). Without `PACKAGING_TOKEN`, GoReleaser publishes GitHub assets only and **skips** Homebrew/Scoop upload (`brew install` will fail until the cask is pushed).

Required for full parity:

| Secret | Purpose |
|--------|---------|
| `PACKAGING_TOKEN` | Push `Casks/hfpacks.rb` + Scoop manifest + apt/dnf Pages |
| `NPM_TOKEN` | `npm i -g hfpacks` |
| `HF_TOKEN` | Index pack CI on release |
| `WINGET_PAT` | optional winget PRs |

## Cut a release

```bash
./scripts/release.sh patch   # 0.2.0 → 0.2.1
make release BUMP=minor
```

Pushing `v*` publishes CLI binaries. **Index SQLite packs** upload when the release is **published** (`release-index-packs.yml`).
