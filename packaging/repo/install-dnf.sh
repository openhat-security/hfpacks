#!/usr/bin/env bash
# Add the hfpacks dnf/yum repo and install hfpacks.
set -euo pipefail
REPO_URL="${REPO_URL:-@REPO_URL@}"
if [ "$(id -u)" -ne 0 ]; then
  echo "re-run with sudo" >&2
  exit 1
fi
curl -fsSL "${REPO_URL}/hfpacks.repo" -o /etc/yum.repos.d/hfpacks.repo
if command -v dnf >/dev/null 2>&1; then
  dnf install -y hfpacks
elif command -v yum >/dev/null 2>&1; then
  yum install -y hfpacks
else
  echo "need dnf or yum" >&2
  exit 1
fi
echo "OK: $(hfpacks version)"
