#!/usr/bin/env bash
# Add the hfpacks apt repo and install hfpacks.
set -euo pipefail
REPO_URL="${REPO_URL:-@REPO_URL@}"
if [ "$(id -u)" -ne 0 ]; then
  echo "re-run with sudo" >&2
  exit 1
fi
curl -fsSL "${REPO_URL}/hfpacks.list" -o /etc/apt/sources.list.d/hfpacks.list
# Optional GPG key when present
if curl -fsSL "${REPO_URL}/hfpacks.asc" -o /usr/share/keyrings/hfpacks.asc 2>/dev/null; then
  sed -i 's/\[trusted=yes\]/[signed-by=\/usr\/share\/keyrings\/hfpacks.asc]/' /etc/apt/sources.list.d/hfpacks.list || true
fi
apt-get update -qq
apt-get install -y hfpacks
echo "OK: $(hfpacks version)"
