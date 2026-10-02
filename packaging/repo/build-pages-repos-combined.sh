#!/usr/bin/env bash
# Build apt + dnf Pages tree from mixed .deb/.rpm (hfpacks + runhug).
set -euo pipefail

PKG_DIR="$(cd "${1:?pkg dir}" && pwd)"
OUT_RAW="${2:?out dir}"
REPO_URL="${REPO_URL:-https://openhat-security.github.io/packages}"

rm -rf "$OUT_RAW"
mkdir -p "$OUT_RAW/deb/pool/main" "$OUT_RAW/deb/dists/stable/main/binary-amd64" "$OUT_RAW/deb/dists/stable/main/binary-arm64" "$OUT_RAW/rpm"
OUT="$(cd "$OUT_RAW" && pwd)"

shopt -s nullglob
debs=("$PKG_DIR"/*.deb)
if ((${#debs[@]})); then
  cp -a "${debs[@]}" "$OUT/deb/pool/main/"
  build_apt_meta() {
    pushd "$OUT/deb" >/dev/null
    for arch in amd64 arm64; do
      mkdir -p "dists/stable/main/binary-${arch}"
      apt-ftparchive --arch "$arch" packages pool/main > "dists/stable/main/binary-${arch}/Packages" || true
      gzip -9fk "dists/stable/main/binary-${arch}/Packages" || true
    done
    apt-ftparchive release dists/stable > dists/stable/Release
    popd >/dev/null
  }
  if command -v apt-ftparchive >/dev/null 2>&1; then
    build_apt_meta
  elif command -v docker >/dev/null 2>&1; then
    docker run --rm -v "$OUT/deb:/deb" -w /deb debian:bookworm-slim bash -lc '
      apt-get update -qq && apt-get install -y -qq apt-utils gzip >/dev/null
      for arch in amd64 arm64; do
        mkdir -p "dists/stable/main/binary-${arch}"
        apt-ftparchive --arch "$arch" packages pool/main > "dists/stable/main/binary-${arch}/Packages" || true
        gzip -9fk "dists/stable/main/binary-${arch}/Packages" || true
      done
      apt-ftparchive release dists/stable > dists/stable/Release
    '
  fi
  if [ -n "${GPG_PRIVATE_KEY:-}" ] && command -v gpg >/dev/null 2>&1; then
    gnupg_home="$(mktemp -d)"
    export GNUPGHOME="$gnupg_home"
    printf '%s\n' "$GPG_PRIVATE_KEY" | gpg --batch --import
    gpg --batch --yes -abs -o "$OUT/deb/dists/stable/Release.gpg" "$OUT/deb/dists/stable/Release"
    gpg --batch --export --armor > "$OUT/openhat.asc"
  fi
fi

rpms=("$PKG_DIR"/*.rpm)
if ((${#rpms[@]})); then
  cp -a "${rpms[@]}" "$OUT/rpm/"
  if command -v createrepo_c >/dev/null 2>&1; then
    createrepo_c "$OUT/rpm"
  elif command -v docker >/dev/null 2>&1; then
    docker run --rm -v "$OUT/rpm:/repo" fedora:latest bash -lc 'dnf install -y -q createrepo_c >/dev/null && createrepo_c /repo'
  fi
fi

ROOT="$(cd "$(dirname "$0")" && pwd)"
RUNHUG_ROOT="$ROOT/runhug"

emit() {
  local tpl="$1" dst="$2"
  sed "s#@REPO_URL@#${REPO_URL}#g" "$tpl" > "$dst"
}

emit "$ROOT/index.html.in" "$OUT/index.html"
emit "$ROOT/apt/sources.list.in" "$OUT/hfpacks.list"
emit "$ROOT/rpm/hfpacks.repo.in" "$OUT/hfpacks.repo"
emit "$ROOT/install-apt.sh" "$OUT/install-hfpacks-apt.sh"
emit "$ROOT/install-dnf.sh" "$OUT/install-hfpacks-dnf.sh"
chmod +x "$OUT/install-hfpacks-apt.sh" "$OUT/install-hfpacks-dnf.sh"

if [ -f "$RUNHUG_ROOT/apt/sources.list.in" ]; then
  emit "$RUNHUG_ROOT/apt/sources.list.in" "$OUT/runhug.list"
  emit "$RUNHUG_ROOT/rpm/runhug.repo.in" "$OUT/runhug.repo"
  emit "$RUNHUG_ROOT/install-apt.sh" "$OUT/install-apt.sh"
  emit "$RUNHUG_ROOT/install-dnf.sh" "$OUT/install-dnf.sh"
  chmod +x "$OUT/install-apt.sh" "$OUT/install-dnf.sh"
fi

touch "$OUT/.nojekyll"
echo "OK combined pages tree → $OUT"
