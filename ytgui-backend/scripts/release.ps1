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
$OnWindows = $IsWindows -or $env:OS -eq 'Windows_NT'

$Targets = @(
  @{ GOOS = 'windows'; GOARCH = 'amd64'; Name = 'fetchly-windows-amd64.exe'; Label = 'Windows (Intel/AMD 64-bit)' },
  @{ GOOS = 'darwin';  GOARCH = 'arm64'; Name = 'fetchly-darwin-arm64';      Label = 'macOS (Apple Silicon)' },
  @{ GOOS = 'darwin';  GOARCH = 'amd64'; Name = 'fetchly-darwin-amd64';      Label = 'macOS (Intel)' }
)
$Launchers = @('start.bat', 'start.command', 'README.txt') |
  ForEach-Object { Join-Path $Backend "launchers/$_" }

# ---- Output helpers -------------------------------------------------------
$TotalSteps = if ($Publish) { 9 } else { 8 }
$script:StepNo = 0
$script:StepClock = $null
$Clock = [Diagnostics.Stopwatch]::StartNew()

function Step($msg) {
  Done
  $script:StepNo++
  $script:StepClock = [Diagnostics.Stopwatch]::StartNew()
  Write-Host ""
  Write-Host ("[{0}/{1}] {2}" -f $script:StepNo, $TotalSteps, $msg) -ForegroundColor Cyan
}
# Closes the current step with its elapsed time.
function Done {
  if ($script:StepClock) {
    Write-Host ("      ✓ done in {0:N1}s" -f $script:StepClock.Elapsed.TotalSeconds) -ForegroundColor Green
    $script:StepClock = $null
  }
}
function Info($msg) { Write-Host "      $msg" -ForegroundColor Gray }
function Ok($msg)   { Write-Host "      ✓ $msg" -ForegroundColor Green }
function Warn($msg) { Write-Host "      ! $msg" -ForegroundColor Yellow }
function Size($path) { "{0:N1} MB" -f ((Get-Item -LiteralPath $path).Length / 1MB) }

# Runs a native command, printing its output in order, and stops on a
# non-zero exit code.
function Exec([scriptblock]$cmd) {
  & $cmd | Out-Host
  if ($LASTEXITCODE -ne 0) { throw "Command failed (exit $LASTEXITCODE): $cmd" }
}

trap {
  Write-Host ""
  Write-Host "✗ Release failed during step $($script:StepNo)/${TotalSteps}: $_" -ForegroundColor Red
  Write-Host ("  after {0:N1}s" -f $Clock.Elapsed.TotalSeconds) -ForegroundColor Red
  exit 1
}

Write-Host ""
Write-Host "Fetchly release $Version" -ForegroundColor White
Info "Repository : $Repo"
Info "Publish    : $(if ($Publish) { 'yes, as a draft release' } else { 'no (build only)' })"
Info "Feed URL   : $(if ($FeedUrl) { $FeedUrl } else { '(none; the built-in tool list is used)' })"
Info "Output     : $OutDir"

# ---- 1. Checks ------------------------------------------------------------
Step "Checking tools"
foreach ($tool in @('go', 'node', 'npm', 'git')) {
  if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "$tool is not installed or not on PATH." }
}
Ok ("Go      " + ((go version) -replace '^go version ', ''))
Ok ("Node    " + (node --version))
Ok ("npm     " + (npm --version))
Ok ("git     " + ((git --version) -replace '^git version ', ''))
if ($Publish) {
  if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { throw "gh (GitHub CLI) is needed for -Publish." }
  Ok ("gh      " + ((gh --version | Select-Object -First 1) -replace '^gh version ', ''))
  gh auth status *> $null
  if ($LASTEXITCODE -ne 0) { throw "gh is not logged in. Run: gh auth login" }
  Ok "gh is logged in"
  gh release view $Tag --repo $Repo *> $null
  if ($LASTEXITCODE -eq 0) { throw "Release $Tag already exists on $Repo." }
  Ok "No release $Tag on $Repo yet"
}

# ---- 2. Repository --------------------------------------------------------
Step "Checking the repository"
$commit = (git -C $Root rev-parse --short HEAD).Trim()
$branch = (git -C $Root rev-parse --abbrev-ref HEAD).Trim()
$subject = (git -C $Root log -1 --format=%s).Trim()
Info "Branch : $branch"
Info "Commit : $commit  $subject"
$dirty = @(git -C $Root status --porcelain)
if ($dirty.Count -gt 0) {
  if (-not $AllowDirty) {
    throw "The working tree has $($dirty.Count) uncommitted change(s). Commit them, or pass -AllowDirty.`n$($dirty -join "`n")"
  }
  Warn "$($dirty.Count) uncommitted change(s) will be included (-AllowDirty):"
  $dirty | Select-Object -First 10 | ForEach-Object { Warn "  $_" }
} else {
  Ok "Working tree is clean"
}
# `npm ci` deletes node_modules first. A running Vite dev server keeps
# native modules locked on Windows, so the delete fails halfway and
# leaves node_modules broken. Refuse before touching anything.
if ($OnWindows) {
  $webFull = (Resolve-Path $Web).Path
  $devServers = @(Get-CimInstance Win32_Process -Filter "Name = 'node.exe'" |
    Where-Object { $_.CommandLine -and $_.CommandLine -like "*$webFull*" })
  if ($devServers.Count -gt 0) {
    throw "A dev server is running from $webFull (PID $($devServers.ProcessId -join ', ')). Stop it first (Ctrl+C in its window)."
  }
  Ok "No dev server is using ytgui-web"
}

# ---- 3. UI ----------------------------------------------------------------
Step "Building the UI"
Push-Location $Web
try {
  Info "Installing exact dependency versions from package-lock.json (npm ci)…"
  Exec { npm ci --no-audit --no-fund }
  Info "Type-checking and bundling (npm run build)…"
  Exec { npm run build }
} finally { Pop-Location }
$uiFiles = @(Get-ChildItem (Join-Path $Web 'dist') -Recurse -File)
Ok ("UI built: {0} files, {1:N0} KB" -f $uiFiles.Count, (($uiFiles | Measure-Object Length -Sum).Sum / 1KB))

# ---- 4. Embed -------------------------------------------------------------
Step "Embedding the UI into the backend"
$embedDist = Join-Path $Backend 'web/dist'
# Delete the old build first so stale hashed files are not embedded.
$old = @(Get-ChildItem $embedDist -Force | Where-Object Name -ne '.gitkeep')
$old | Remove-Item -Recurse -Force
Info "Removed $($old.Count) old item(s) from web/dist"
Copy-Item (Join-Path $Web 'dist/*') $embedDist -Recurse -Force
Ok "Copied ytgui-web/dist → ytgui-backend/web/dist"

# ---- 5. Binaries ----------------------------------------------------------
Step "Building the backend for $($Targets.Count) platforms"
if (Test-Path $OutDir) {
  Remove-Item $OutDir -Recurse -Force
  Info "Cleared the old output folder"
}
New-Item -ItemType Directory -Force $OutDir | Out-Null

$ldflags = "-s -w -X main.version=$Version"
if ($FeedUrl) { $ldflags += " -X main.feedURL=$FeedUrl" }
Info "Flags: CGO_ENABLED=0 -trimpath -ldflags `"$ldflags`""

$saved = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }
Push-Location $Backend
try {
  $i = 0
  foreach ($t in $Targets) {
    $i++
    Info "($i/$($Targets.Count)) $($t.Label)  [$($t.GOOS)/$($t.GOARCH)]…"
    $env:GOOS = $t.GOOS; $env:GOARCH = $t.GOARCH; $env:CGO_ENABLED = '0'
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Exec { go build -trimpath -ldflags $ldflags -o (Join-Path $OutDir $t.Name) . }
    Ok ("{0}  {1}  ({2:N1}s)" -f $t.Name, (Size (Join-Path $OutDir $t.Name)), $sw.Elapsed.TotalSeconds)
  }
} finally {
  foreach ($k in $saved.Keys) {
    if ($saved[$k]) { Set-Item "env:$k" $saved[$k] } else { Remove-Item "env:$k" -ErrorAction SilentlyContinue }
  }
  Pop-Location
}

# ---- 6. Verify ------------------------------------------------------------
Step "Checking the built binaries"
if ($OnWindows) {
  $stamped = (& (Join-Path $OutDir 'fetchly-windows-amd64.exe') version).Trim()
  if ($stamped -ne $Version) { throw "Built binary reports version '$stamped', expected '$Version'." }
  Ok "fetchly-windows-amd64.exe runs and reports version $stamped"
} else {
  Warn "Not on Windows: skipped running the Windows binary"
}
Info "The macOS binaries can't run here; test them on a Mac before publishing."

# ---- 7. Zip ---------------------------------------------------------------
Step "Building Fetchly.zip for users"
foreach ($l in $Launchers) { Info "+ Fetchly/$(Split-Path -Leaf $l)" }
Push-Location $Backend
try {
  Exec { go run ./scripts/mkzip -o (Join-Path $OutDir 'Fetchly.zip') -dir Fetchly @Launchers }
} finally { Pop-Location }
Ok ("Fetchly.zip  {0:N1} KB  (start.command marked executable)" -f ((Get-Item (Join-Path $OutDir 'Fetchly.zip')).Length / 1KB))

# ---- 8. Checksums ---------------------------------------------------------
Step "Writing checksums.txt"
# Format "<sha256>  <name>" with LF endings, as start.bat/start.command expect.
$assets = @($Targets | ForEach-Object Name) + 'Fetchly.zip'
$lines = foreach ($a in $assets) {
  $h = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $OutDir $a)).Hash.ToLower()
  Info "$h  $a"
  "$h  $a"
}
[IO.File]::WriteAllText((Join-Path $OutDir 'checksums.txt'), (($lines -join "`n") + "`n"))
Ok "checksums.txt covers $($assets.Count) files"

# ---- 9. Publish -----------------------------------------------------------
$files = @($assets + 'checksums.txt') | ForEach-Object { Join-Path $OutDir $_ }
if ($Publish) {
  Step "Uploading draft release $Tag to $Repo"
  $notes = @"
Fetchly $Version (commit $commit)

Download **Fetchly.zip**, unzip it, and double-click start.bat (Windows) or start.command (Mac).
See README.txt inside the zip for first-run steps.
"@
  foreach ($f in $files) { Info ("↑ {0}  ({1})" -f (Split-Path -Leaf $f), (Size $f)) }
  Exec { gh release create $Tag --repo $Repo --draft --title "Fetchly $Version" --notes $notes @files }
  Ok "Draft release $Tag created"
}
Done

# ---- Summary --------------------------------------------------------------
Write-Host ""
Write-Host ("Release $Version ready in {0:N1}s" -f $Clock.Elapsed.TotalSeconds) -ForegroundColor Green
Get-ChildItem $OutDir |
  Format-Table Name, @{ n = 'Size'; e = { if ($_.Length -ge 1MB) { "{0:N1} MB" -f ($_.Length / 1MB) } else { "{0:N1} KB" -f ($_.Length / 1KB) } }; a = 'right' } -AutoSize |
  Out-Host
Write-Host "Folder: $OutDir"
Write-Host ""
if ($Publish) {
  Write-Host "Next: test the draft on clean machines, then publish it:" -ForegroundColor Yellow
  Write-Host "  gh release edit $Tag --repo $Repo --draft=false"
} else {
  Write-Host "Next: to upload these files as a draft GitHub release:" -ForegroundColor Yellow
  Write-Host "  ./scripts/release.ps1 -Version $Version -Publish"
}
