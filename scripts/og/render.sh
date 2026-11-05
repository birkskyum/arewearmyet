#!/bin/bash
# Renders og.html to public/og.png with headless Chrome. Needs Geist-VariableFont_wght.ttf (SIL OFL,
# github.com/vercel/geist-font) next to og.html; it isn't committed.
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
chrome=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
[[ -f $here/Geist-VariableFont_wght.ttf ]] || { echo "Put Geist-VariableFont_wght.ttf in $here first" >&2; exit 1; }
"$chrome" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 --window-size=1200,630 \
  --screenshot="$here/../../public/og.png" "file://$here/og.html" 2>/dev/null
echo "Wrote public/og.png"
