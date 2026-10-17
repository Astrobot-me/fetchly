#!/bin/bash
# Fetchly launcher for macOS. Double-click to start.
# Closing this window quits the app.
# Written for the bash 3.2 that ships with macOS.

# ---- Release settings --------------------------------------------------
# GitHub repository that hosts the release files (owner/name).
REPO="astrobot-me/fetchly"
# -----------------------------------------------------------------------

# Apple Silicon reports hw.optional.arm64=1, even under Rosetta.
if [ "$(sysctl -n hw.optional.arm64 2>/dev/null)" = "1" ]; then
  ARCH=arm64
else
  ARCH=amd64
fi
ASSET="fetchly-darwin-$ARCH"

# YTGUI_DOWNLOAD_BASE and YTGUI_HOME are for testing only.
BASE="${YTGUI_DOWNLOAD_BASE:-https://github.com/$REPO/releases/latest/download}"
DATA="${YTGUI_HOME:-$HOME/Library/Application Support/ytgui}"
APPDIR="$DATA/app"
BIN="$APPDIR/fetchly"

fail() {
  echo
  echo "  $1"
  echo
  read -r -n 1 -s -p "  Press any key to close this window."
  echo
  exit 1
}

# The app is downloaded only when it is missing. An existing copy is
# never replaced here; updates happen inside the app.
if [ ! -x "$BIN" ]; then
  echo
  echo "  Fetchly is starting for the first time."
  echo "  Downloading the app (about 8 MB)..."
  echo

  if [ "$REPO" = "OWNER/REPO" ] && [ -z "${YTGUI_DOWNLOAD_BASE:-}" ]; then
    fail "This launcher has not been set up with a download address yet."
  fi
  mkdir -p "$APPDIR" || fail "Could not create the folder $APPDIR"

  TMP="$APPDIR/fetchly.download"
  SUMS="$APPDIR/checksums.txt"
  rm -f "$TMP" "$SUMS"

  if ! curl -fsSL --retry 3 -o "$SUMS" "$BASE/checksums.txt" ||
     ! curl -fL --retry 3 --progress-bar -o "$TMP" "$BASE/$ASSET"; then
    rm -f "$TMP" "$SUMS"
    fail "Could not download the app. Check your internet connection and try again."
  fi

  # Check the download against the published SHA-256 before running it.
  WANT=$(awk -v a="$ASSET" '{ n = $2; sub(/^\*/, "", n); if (n == a) print tolower($1) }' "$SUMS")
  GOT=$(shasum -a 256 "$TMP" | awk '{ print tolower($1) }')
  if [ -z "$WANT" ] || [ "$WANT" != "$GOT" ]; then
    rm -f "$TMP" "$SUMS"
    fail "The download could not be verified, so it was not installed. Please try again later."
  fi

  chmod +x "$TMP"
  # curl does not quarantine files, but clear the flag in case.
  xattr -d com.apple.quarantine "$TMP" 2>/dev/null
  mv -f "$TMP" "$BIN" || fail "Could not install the app into $APPDIR"
  rm -f "$SUMS"
fi

"$BIN" "$@"
CODE=$?
if [ "$CODE" -ne 0 ]; then
  fail "Fetchly stopped because of a problem. See the message above."
fi
exit 0
