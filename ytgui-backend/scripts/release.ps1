<#
.SYNOPSIS
  Builds a Fetchly release: UI, three backend binaries, checksums.txt and
  Fetchly.zip. With -Publish, uploads them as a DRAFT GitHub release.

.EXAMPLE
  ./scripts/release.ps1 -Version 0.1.0
  ./scripts/release.ps1 -Version 0.1.0 -Publish

.NOTES
  Run from anywhere; paths are resolved from this script's location.
  A draft release is not "latest", so launchers ignore it until you
  publish it on GitHub (or: gh release edit v<version> --draft=false).
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory)]
  [ValidatePattern('^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$')]
  [string]$Version,

  # Upload as a draft GitHub release (needs `gh auth login`).
  [switch]$Publish,

  # Build even if the git working tree has uncommitted changes.
  [switch]$AllowDirty,

  # Hosted tools feed URL stamped into the binaries. Empty = use the
  # copy embedded in the binary (feed/tools.json at build time).
  [string]$FeedUrl = '',

  [string]$Repo = 'astrobot-me/fetchly'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$Backend = Split-Path -Parent $PSScriptRoot
$Root    = Split-Path -Parent $Backend
$Web     = Join-Path $Root 'ytgui-web'
$OutDir  = Join-Path $Backend "out/release/$Version"
$Tag     = "v$Version"

$Targets = @(
  @{ GOOS = 'windows'; GOARCH = 'amd64'; Name = 'fetchly-windows-amd64.exe' },
  @{ GOOS = 'darwin';  GOARCH = 'arm64'; Name = 'fetchly-darwin-arm64' },
  @{ GOOS = 'darwin';  GOARCH = 'amd64'; Name = 'fetchly-darwin-amd64' }
)
$Launchers = @('start.bat', 'start.command', 'README.txt') |
  ForEach-Object { Join-Path $Backend "launchers/$_" }

function Step($msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }

# Run a native command and stop on a non-zero exit code.
function Exec([scriptblock]$cmd) {
  & $cmd
  if ($LASTEXITCODE -ne 0) { throw "Command failed (exit $LASTEXITCODE): $cmd" }
}

# ---- Checks ---------------------------------------------------------------
Step "Checking tools and repository"
foreach ($tool in @('go', 'npm', 'git')) {
  if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "$tool is not installed or not on PATH." }
}
if ($Publish) {
  if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { throw "gh (GitHub CLI) is needed for -Publish." }
  gh auth status *> $null
  if ($LASTEXITCODE -ne 0) { throw "gh is not logged in. Run: gh auth login" }
  gh release view $Tag --repo $Repo *> $null
  if ($LASTEXITCODE -eq 0) { throw "Release $Tag already exists on $Repo." }
}
$dirty = git -C $Root status --porcelain
if ($dirty -and -not $AllowDirty) {
  throw "The working tree has uncommitted changes. Commit them, or pass -AllowDirty.`n$($dirty -join "`n")"
}
# `npm ci` deletes node_modules first. A running Vite dev server keeps
# native modules locked on Windows, so the delete fails halfway and
# leaves node_modules broken. Refuse before touching anything.
if ($IsWindows -or $env:OS -eq 'Windows_NT') {
  $webFull = (Resolve-Path $Web).Path
  $devServers = Get-CimInstance Win32_Process -Filter "Name = 'node.exe'" |
    Where-Object { $_.CommandLine -and $_.CommandLine -like "*$webFull*" }
  if ($devServers) {
    throw "A dev server is running from $webFull (PID $($devServers.ProcessId -join ', ')). Stop it first (Ctrl+C in its window)."
  }
}
$commit = (git -C $Root rev-parse --short HEAD).Trim()
Write-Host "Version $Version from commit $commit"

# ---- UI -------------------------------------------------------------------
Step "Building the UI"
Push-Location $Web
try {
  Exec { npm ci --no-audit --no-fund }
  Exec { npm run build }
} finally { Pop-Location }

Step "Embedding the UI into the backend"
$embedDist = Join-Path $Backend 'web/dist'
# Delete the old build first so stale hashed files are not embedded.
Get-ChildItem $embedDist -Force | Where-Object Name -ne '.gitkeep' | Remove-Item -Recurse -Force
Copy-Item (Join-Path $Web 'dist/*') $embedDist -Recurse -Force

# ---- Binaries -------------------------------------------------------------
if (Test-Path $OutDir) { Remove-Item $OutDir -Recurse -Force }
New-Item -ItemType Directory -Force $OutDir | Out-Null

$ldflags = "-s -w -X main.version=$Version"
if ($FeedUrl) { $ldflags += " -X main.feedURL=$FeedUrl" }

$saved = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }
Push-Location $Backend
try {
  foreach ($t in $Targets) {
    Step "Building $($t.Name)"
    $env:GOOS = $t.GOOS; $env:GOARCH = $t.GOARCH; $env:CGO_ENABLED = '0'
    Exec { go build -trimpath -ldflags $ldflags -o (Join-Path $OutDir $t.Name) . }
  }
} finally {
  foreach ($k in $saved.Keys) { Set-Item "env:$k" $saved[$k] -ErrorAction SilentlyContinue }
  if (-not $saved.GOOS) { Remove-Item env:GOOS -ErrorAction SilentlyContinue }
  if (-not $saved.GOARCH) { Remove-Item env:GOARCH -ErrorAction SilentlyContinue }
  if (-not $saved.CGO_ENABLED) { Remove-Item env:CGO_ENABLED -ErrorAction SilentlyContinue }
  Pop-Location
}

# Make sure the version was stamped (the Windows build can run here).
if ($IsWindows -or $env:OS -eq 'Windows_NT') {
  $stamped = (& (Join-Path $OutDir 'fetchly-windows-amd64.exe') version).Trim()
  if ($stamped -ne $Version) { throw "Built binary reports version '$stamped', expected '$Version'." }
}

# ---- Zip for users --------------------------------------------------------
Step "Building Fetchly.zip"
Push-Location $Backend
try {
  Exec { go run ./scripts/mkzip -o (Join-Path $OutDir 'Fetchly.zip') -dir Fetchly @Launchers }
} finally { Pop-Location }

# ---- Checksums ------------------------------------------------------------
Step "Writing checksums.txt"
# Format "<sha256>  <name>" with LF endings, as start.bat/start.command expect.
$assets = @($Targets | ForEach-Object Name) + 'Fetchly.zip'
$lines = foreach ($a in $assets) {
  $h = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $OutDir $a)).Hash.ToLower()
  "$h  $a"
}
[IO.File]::WriteAllText((Join-Path $OutDir 'checksums.txt'), (($lines -join "`n") + "`n"))

Step "Release files in $OutDir"
Get-ChildItem $OutDir | Format-Table Name, @{ n = 'Size (MB)'; e = { [math]::Round($_.Length / 1MB, 1) } } -AutoSize | Out-Host
Get-Content (Join-Path $OutDir 'checksums.txt') | Out-Host

# ---- Publish --------------------------------------------------------------
if (-not $Publish) {
  Write-Host "`nNot published. To upload as a draft: ./scripts/release.ps1 -Version $Version -Publish" -ForegroundColor Yellow
  return
}

Step "Creating draft release $Tag on $Repo"
$notes = @"
Fetchly $Version (commit $commit)

Download **Fetchly.zip**, unzip it, and double-click start.bat (Windows) or start.command (Mac).
See README.txt inside the zip for first-run steps.
"@
$files = @($assets + 'checksums.txt') | ForEach-Object { Join-Path $OutDir $_ }
Exec { gh release create $Tag --repo $Repo --draft --title "Fetchly $Version" --notes $notes @files }

Write-Host "`nDraft created. Test it, then publish it on GitHub or run:" -ForegroundColor Green
Write-Host "  gh release edit $Tag --repo $Repo --draft=false"
