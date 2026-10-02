# hfpacks

**Find the best model. Deploy it in minutes. Run it for pennies.**

npm wrapper for the [hfpacks](https://github.com/openhat-security/hfpacks) CLI.
`postinstall` downloads the matching GitHub Release binary (macOS / Linux / Windows × amd64 / arm64). Requires Node ≥ 18.

Full docs, demo GIF, and packaging notes: [github.com/openhat-security/hfpacks](https://github.com/openhat-security/hfpacks).

## Install

```bash
# npm (this package)
npm install -g hfpacks
# or: npx hfpacks wizard

# macOS / Linux — Homebrew
brew install --cask openhat-security/tap/hfpacks

# Debian / Ubuntu — apt
curl -fsSL https://openhat-security.github.io/packages/install-apt.sh | sudo bash

# Fedora / RHEL — dnf
curl -fsSL https://openhat-security.github.io/packages/install-dnf.sh | sudo bash

# Arch — AUR
yay -S hfpacks-bin

# Windows — Scoop
scoop bucket add openhat https://github.com/openhat-security/scoop-bucket
scoop install hfpacks

# Windows — winget
winget install OpenHatSecurity.Runhug

# macOS / Linux — direct binary
curl -fsSL https://raw.githubusercontent.com/openhat-security/hfpacks/main/scripts/install.sh | bash

# Windows — direct binary (PowerShell)
irm https://raw.githubusercontent.com/openhat-security/hfpacks/main/scripts/install.ps1 | iex
```

## Quick start

```bash
hfpacks wizard
hfpacks connect && hfpacks connect hf
hfpacks search -q "small instruct llm"
hfpacks deploy <model> --dry-run --estimate
hfpacks deploy <model>
hfpacks start claude          # or: hfpacks start opencode
```

Plans stay short by default — use `--verbose` / `-v` for cost assumptions and extra detail. `hfpacks help` / `hfpacks gcp help` are colorized command lists.
