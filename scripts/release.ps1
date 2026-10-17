<#
.SYNOPSIS
  Builds a Fetchly release: UI, three backend binaries, checksums.txt and
  Fetchly.zip. With -Release, also prepares the release notes. With
  -Publish, uploads everything as a DRAFT GitHub release.

.EXAMPLE
  ./scripts/release.ps1 -Version 0.1.0              # steps 1-8: build only
  ./scripts/release.ps1 -Version 0.1.0 -Release     # + step 9: release notes
  ./scripts/release.ps1 -Version 0.1.0 -Publish     # + step 10: upload (implies -Release)

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

  # Prepare the release notes from release-notes/<version>.md (step 9).
  [switch]$Release,

  # Upload as a draft GitHub release (step 10; needs `gh auth login`).
  # Implies -Release, so an upload always has its notes.
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
if ($Publish) { $Release = $true }

$Root    = Split-Path -Parent $PSScriptRoot
$Backend = Join-Path $Root 'ytgui-backend'
$Web     = Join-Path $Root 'ytgui-web'
$OutDir  = Join-Path $Root "out/release/$Version"
$NotesFile = Join-Path $Root "release-notes/$Version.md"

# Keep build caches inside the repo (.cache/ is git-ignored) unless the
# caller set their own, so a release doesn't fill the system drive.
$CacheDir = Join-Path $Root '.cache'
if (-not $env:GOCACHE)          { $env:GOCACHE = Join-Path $CacheDir 'go-build' }
if (-not $env:GOTMPDIR)         { $env:GOTMPDIR = Join-Path $CacheDir 'go-tmp' }
if (-not $env:npm_config_cache) { $env:npm_config_cache = Join-Path $CacheDir 'npm' }
New-Item -ItemType Directory -Force $env:GOCACHE, $env:GOTMPDIR, $env:npm_config_cache | Out-Null
$Tag     = "v$Version"
# $IsWindows doesn't exist in Windows PowerShell 5.1 (and strict mode
# rejects undefined variables), so ask .NET instead.
$OnWindows = [Environment]::OSVersion.Platform -eq 'Win32NT'

$Targets = @(
  @{ GOOS = 'windows'; GOARCH = 'amd64'; Name = 'fetchly-windows-amd64.exe'; Label = 'Windows (Intel/AMD 64-bit)' },
  @{ GOOS = 'darwin';  GOARCH = 'arm64'; Name = 'fetchly-darwin-arm64';      Label = 'macOS (Apple Silicon)' },
  @{ GOOS = 'darwin';  GOARCH = 'amd64'; Name = 'fetchly-darwin-amd64';      Label = 'macOS (Intel)' }
)
$Launchers = @('start.bat', 'start.command', 'README.txt') |
  ForEach-Object { Join-Path $Backend "launchers/$_" }

# ---- Output helpers -------------------------------------------------------
# Keep this file pure ASCII: Windows PowerShell 5.1 reads a script without
# a BOM as ANSI, so literal symbols would break parsing. Build them here.
$Tick = [char]0x2713; $Cross = [char]0x2717; $Arrow = [char]0x2192
$Up = [char]0x2191; $Dots = [char]0x2026
try { [Console]::OutputEncoding = [Text.Encoding]::UTF8 } catch { }

$TotalSteps = 8 + [int][bool]$Release + [int][bool]$Publish
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
    Write-Host ("      $Tick done in {0:N1}s" -f $script:StepClock.Elapsed.TotalSeconds) -ForegroundColor Green
    $script:StepClock = $null
  }
}
function Info($msg) { Write-Host "      $msg" -ForegroundColor Gray }
function Ok($msg)   { Write-Host "      $Tick $msg" -ForegroundColor Green }
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
  Write-Host "$Cross Release failed during step $($script:StepNo)/${TotalSteps}: $_" -ForegroundColor Red
  Write-Host ("  after {0:N1}s" -f $Clock.Elapsed.TotalSeconds) -ForegroundColor Red
  exit 1
}

Write-Host ""
Write-Host "Fetchly release $Version" -ForegroundColor White
Info "Repository : $Repo"
Info "Notes      : $(if ($Release) { "yes, writes release-notes/$Version.md (kept if it already exists)" } else { 'no (add -Release)' })"
Info "Publish    : $(if ($Publish) { 'yes, as a draft release' } else { 'no (add -Publish)' })"
Info "Feed URL   : $(if ($FeedUrl) { $FeedUrl } else { '(none; the built-in tool list is used)' })"
Info "Output     : $OutDir"
Info "Go cache   : $env:GOCACHE"
Info "npm cache  : $env:npm_config_cache"

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
  Info "Installing exact dependency versions from package-lock.json (npm ci)$Dots"
  Exec { npm ci --no-audit --no-fund }
  Info "Type-checking and bundling (npm run build)$Dots"
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
Ok "Copied ytgui-web/dist $Arrow ytgui-backend/web/dist"

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
    Info "($i/$($Targets.Count)) $($t.Label)  [$($t.GOOS)/$($t.GOARCH)]$Dots"
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
# mkzip uses only the standard library, so it runs as a single file
# without being part of the backend's Go module.
Exec { go run (Join-Path $PSScriptRoot 'mkzip/main.go') -o (Join-Path $OutDir 'Fetchly.zip') -dir Fetchly @Launchers }
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

# ---- 9. Release notes (-Release) ------------------------------------------
$PreparedNotes = Join-Path $OutDir 'release-notes.md'
if ($Release) {
  Step "Writing release notes"
  if (Test-Path $NotesFile) {
    # Never overwrite notes that may have been edited by hand.
    Ok "release-notes/$Version.md already exists; kept as written (delete it to regenerate)"
  } else {
    $template = Join-Path $Root 'release-notes/TEMPLATE.md'
    if (-not (Test-Path $template)) { throw "release-notes/TEMPLATE.md is missing; it is needed to write new release notes." }
    # Changes since the previous v* tag, or since the first commit if none.
    $prevTag = @(git -C $Root tag --list 'v*' --sort=-v:refname) | Where-Object { $_ -ne $Tag } | Select-Object -First 1
    $range = if ($prevTag) { "$prevTag..HEAD" } else { 'HEAD' }
    $changes = @(git -C $Root log --no-merges --format='- %s' $range)
    if ($changes.Count -eq 0) { $changes = @('- No changes recorded.') }
    Info "Changes since $(if ($prevTag) { $prevTag } else { 'the first commit' }): $($changes.Count) commit(s)"
    $text = (Get-Content -Raw -Encoding UTF8 $template).
      Replace('{{VERSION}}', $Version).
      Replace('{{DATE}}', (Get-Date -Format 'yyyy-MM-dd')).
      Replace('{{CHANGES}}', ($changes -join "`n"))
    [IO.File]::WriteAllText($NotesFile, $text, (New-Object Text.UTF8Encoding $false))
    Ok "Wrote release-notes/$Version.md from TEMPLATE.md; review and edit it, then commit it"
  }
  $body = (Get-Content -Raw -Encoding UTF8 $NotesFile).TrimEnd()
  # Append the checksums so people can verify downloads from the notes alone.
  $sums = (Get-Content -Raw (Join-Path $OutDir 'checksums.txt')).TrimEnd()
  $body += "`n`n### Checksums (SHA-256)`n`n" + '```' + "`n$sums`n" + '```' + "`n"
  [IO.File]::WriteAllText($PreparedNotes, $body, (New-Object Text.UTF8Encoding $false))
  $heading = (Get-Content -Encoding UTF8 $NotesFile | Where-Object { $_ -match '^#' } | Select-Object -First 1)
  $words = ($body -split '\s+' | Where-Object { $_ }).Count
  Info "Title line : $heading"
  Info "Sections   : $((@(Get-Content -Encoding UTF8 $NotesFile | Where-Object { $_ -match '^### ' }) | ForEach-Object { $_ -replace '^### ', '' }) -join ', ')"
  Ok "release-notes.md written ($words words, checksums appended)"
}

# ---- 10. Upload (-Publish) ------------------------------------------------
$files = @($assets + 'checksums.txt') | ForEach-Object { Join-Path $OutDir $_ }
if ($Publish) {
  Step "Uploading draft release $Tag to $Repo"
  Info "Release notes: out/release/$Version/release-notes.md"
  foreach ($f in $files) { Info ("$Up {0}  ({1})" -f (Split-Path -Leaf $f), (Size $f)) }
  # Exec { gh release create $Tag --repo $Repo --draft --title "Fetchly $Version" --notes-file $PreparedNotes @files }
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
} elseif ($Release) {
  Write-Host "Next: check out/release/$Version/release-notes.md, then upload as a draft:" -ForegroundColor Yellow
  Write-Host "  ./scripts/release.ps1 -Version $Version -Publish"
} else {
  Write-Host "Next: prepare the release notes, or upload as a draft:" -ForegroundColor Yellow
  Write-Host "  ./scripts/release.ps1 -Version $Version -Release"
  Write-Host "  ./scripts/release.ps1 -Version $Version -Publish"
}
