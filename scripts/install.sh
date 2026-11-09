#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
bin=$HOME/.local/bin/sector
share=$HOME/.local/share/sector
apps=$HOME/.local/share/applications

cd "$root"
mkdir -p "$HOME/.local/bin" "$share" "$apps"
version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
GOFLAGS=-tags=novulkan mise exec -- go build -trimpath -ldflags "-X main.version=$version" -o "$bin" ./cmd/sector

icon=$share/icon.png
if command -v rsvg-convert >/dev/null 2>&1; then
    rsvg-convert -w 256 -h 256 -o "$icon" assets/logo.svg
else
    # why: most launchers draw an svg icon too, so a missing rsvg-convert still leaves an icon.
    icon=$share/icon.svg
    install -m 644 assets/logo.svg "$icon"
fi

# why: Exec is quoted, so ", `, $ and \ need a backslash, and % doubles.
exec_bin=$(printf '%s' "$bin" | sed -e 's/[\\"`$]/\\&/g' -e 's/\\/\\\\/g' -e 's/%/%%/g')
cat >"$apps/sector.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=sector
Comment=System monitor grouped by agent, repo and app
Exec="$exec_bin"
Icon=$icon
Terminal=false
Categories=System;Monitor;
Keywords=btop;top;monitor;cpu;memory;
StartupWMClass=sector
EOF
chmod 644 "$apps/sector.desktop"

printf 'Installed sector %s in %s.\n' "$version" "$bin"
printf 'Add %s/.local/bin to PATH if needed.\n' "$HOME"
