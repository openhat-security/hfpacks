# Install the latest hfpacks Windows amd64 release from openhat-security/hfpacks.
# Prefers hfpacks_<ver>_windows_amd64.zip (GoReleaser); falls back to bare .exe.
# Usage: irm https://raw.githubusercontent.com/openhat-security/hfpacks/main/scripts/install.ps1 | iex
$ErrorActionPreference = "Stop"
$Repo = if ($env:HFPACKS_REPO) { $env:HFPACKS_REPO } else { "openhat-security/hfpacks" }
$AssetPrefixes = @("hfpacks_", "hfpacks-cli_")
$BinName = "hfpacks.exe"

$headers = @{ Accept = "application/vnd.github+json"; "User-Agent" = "hfpacks-install" }
if ($env:TAG) {
  $tagHint = $env:TAG
  $api = "https://api.github.com/repos/$Repo/releases/tags/$tagHint"
} elseif ($env:VERSION) {
  $verHint = $env:VERSION.TrimStart("v")
  $api = "https://api.github.com/repos/$Repo/releases/tags/v$verHint"
} else {
  $api = "https://api.github.com/repos/$Repo/releases/latest"
}
$release = Invoke-RestMethod -Uri $api -Headers $headers
$tag = $release.tag_name
$ver = $tag.TrimStart("v")

$asset = $null
$isZip = $false
foreach ($prefix in $AssetPrefixes) {
  foreach ($suffix in @("windows_amd64.zip", "windows_amd64.exe")) {
    $assetName = "${prefix}${ver}_${suffix}"
    $asset = $release.assets | Where-Object { $_.name -eq $assetName } | Select-Object -First 1
    if ($asset) {
      $isZip = $suffix.EndsWith(".zip")
      break
    }
  }
  if ($asset) { break }
}
if (-not $asset) {
  throw "Asset not found for prefixes $($AssetPrefixes -join ', ') (tag $tag)"
}

$destDir = Join-Path $env:LOCALAPPDATA "hfpacks\bin"
New-Item -ItemType Directory -Force -Path $destDir | Out-Null
$dest = Join-Path $destDir $BinName

Write-Host "Downloading $($asset.browser_download_url)"
if ($isZip) {
  $tmpZip = Join-Path $env:TEMP "hfpacks-$ver.zip"
  $tmpDir = Join-Path $env:TEMP "hfpacks-$ver-extract"
  Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $tmpZip
  if (Test-Path $tmpDir) { Remove-Item -Recurse -Force $tmpDir }
  Expand-Archive -Path $tmpZip -DestinationPath $tmpDir -Force
  $found = Get-ChildItem -Path $tmpDir -Recurse -Filter "hfpacks.exe" | Select-Object -First 1
  if (-not $found) { throw "hfpacks.exe not found inside $($asset.name)" }
  Copy-Item -Force $found.FullName $dest
  Remove-Item -Force $tmpZip -ErrorAction SilentlyContinue
  Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
} else {
  Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $dest
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not ($userPath -split ";" | Where-Object { $_ -eq $destDir })) {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$destDir", "User")
  $env:Path = "$env:Path;$destDir"
  Write-Host "Added $destDir to your user PATH (new shells pick this up)."
}

Write-Host "Installed $BinName → $dest ($tag)"
try { & $dest --version } catch { try { & $dest version } catch {} }
