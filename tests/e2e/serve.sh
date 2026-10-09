#!/bin/sh
# SPDX-License-Identifier: MIT
# Starts the built lanpaper binary for the browser tests. The server reads its
# static files and writes data/ relative to the working directory, so it runs
# in a throwaway directory and the repository stays clean.
set -eu
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
BIN=${LANPAPER_BIN:-$ROOT/lanpaper}
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
cp -r "$ROOT/admin.html" "$ROOT/login.html" "$ROOT/static" "$WORK/"
cd "$WORK"
export PORT=${PORT:-18090} ADMIN_USER=e2e-admin ADMIN_PASS=e2e-test-password-123
"$BIN"
