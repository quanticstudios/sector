#!/bin/sh
# Installs a sector release on Linux; see "Install" in README.md.
set -eu

repo=quanticstudios/sector

# platform names the release archive's os_arch for `uname -s` and `uname -m`.
platform() {
    case "$1" in
        Linux) ;;
        *) printf 'sector: no release for %s; sector runs on Linux only\n' "$1" >&2; return 1 ;;
    esac
    case "$2" in
        x86_64 | amd64) printf 'linux_amd64\n' ;;
        aarch64 | arm64) printf 'linux_arm64\n' ;;
        *) printf 'sector: no release for %s on %s\n' "$2" "$1" >&2; return 1 ;;
    esac
}

# pick prints "sum name" for the sector_*_$1.tar.gz line of checksums.txt $2.
pick() {
    awk -v s="_$1.tar.gz" '{ n = $2; sub(/^\*/, "", n) }
        n ~ /^sector_/ && substr(n, length(n) - length(s) + 1) == s { print $1, n; exit }' "$2"
}

# packages reads ldd output and prints the apt and pacman packages for each library it did not find.
packages() {
    awk '/=> not found/ { print $1 }' | while IFS= read -r lib; do
        case "$lib" in
            libEGL.so.*) echo 'libegl1 libglvnd' ;;
            libGLESv2.so.*) echo 'libgles2 libglvnd' ;;
            libGLdispatch.so.*) echo 'libglvnd0 libglvnd' ;;
            libwayland-client.so.*) echo 'libwayland-client0 wayland' ;;
            libwayland-cursor.so.*) echo 'libwayland-cursor0 wayland' ;;
            libwayland-egl.so.*) echo 'libwayland-egl1 wayland' ;;
            libxkbcommon-x11.so.*) echo 'libxkbcommon-x11-0 libxkbcommon-x11' ;;
            libxkbcommon.so.*) echo 'libxkbcommon0 libxkbcommon' ;;
            libX11-xcb.so.*) echo 'libx11-xcb1 libx11' ;;
            libX11.so.*) echo 'libx11-6 libx11' ;;
            libXcursor.so.*) echo 'libxcursor1 libxcursor' ;;
            libXfixes.so.*) echo 'libxfixes3 libxfixes' ;;
            libXrender.so.*) echo 'libxrender1 libxrender' ;;
            libXau.so.*) echo 'libxau6 libxau' ;;
            libXdmcp.so.*) echo 'libxdmcp6 libxdmcp' ;;
            libxcb-xkb.so.*) echo 'libxcb-xkb1 libxcb' ;;
            libxcb.so.*) echo 'libxcb1 libxcb' ;;
            libffi.so.*) echo 'libffi8 libffi' ;;
            *) echo "$lib $lib" ;;
        esac
    done
}

# libraries prints ldd's lines for $1, plus a not-found line for GLESv2, which Gio opens at run time.
libraries() {
    ldd "$1" 2>/dev/null || true
    if command -v ldconfig >/dev/null 2>&1 && ! ldconfig -p 2>/dev/null | grep -q 'libGLESv2\.so\.2'; then
        echo 'libGLESv2.so.2 => not found'
    fi
}

desktop_entry() {
    share=$HOME/.local/share
    mkdir -p "$share/sector" "$share/applications"
    install -m 644 "$tmp/icon.png" "$share/sector/icon.png"
    # why: Exec is quoted, so ", `, $ and \ need a backslash, and % doubles.
    exec_bin=$(printf '%s' "$dir/sector" | sed -e 's/[\\"`$]/\\&/g' -e 's/\\/\\\\/g' -e 's/%/%%/g')
    cat >"$share/applications/sector.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=sector
Comment=System monitor grouped by agent, repo and app
Exec="$exec_bin"
Icon=$share/sector/icon.png
Terminal=false
Categories=System;Monitor;
Keywords=btop;top;monitor;cpu;memory;
StartupWMClass=sector
EOF
    chmod 644 "$share/applications/sector.desktop"
    printf 'Added sector to the app launcher.\n'
}

main() {
    plat=$(platform "$(uname -s)" "$(uname -m)")
    version=${SECTOR_VERSION:-}
    case "$version" in
        [0-9]*) version=v$version ;;
    esac
    if [ -n "$version" ]; then
        base="https://github.com/$repo/releases/download/$version"
    else
        base="https://github.com/$repo/releases/latest/download"
    fi
    dir=${SECTOR_INSTALL_DIR:-"$HOME/.local/bin"}

    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    # why: the archive name carries the version, so the latest one is found in checksums.txt.
    curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
    line=$(pick "$plat" "$tmp/checksums.txt")
    if [ -z "$line" ]; then
        printf 'sector: the %s release has no %s archive\n' "${version:-latest}" "$plat" >&2
        exit 1
    fi
    want=${line%% *}
    name=${line#* }
    printf 'Downloading %s\n' "$name"
    curl -fsSL -o "$tmp/$name" "$base/$name"
    got=$(sha256sum "$tmp/$name" | cut -d ' ' -f 1)
    if [ "$want" != "$got" ]; then
        printf 'sector: %s does not match checksums.txt; nothing installed\n' "$name" >&2
        exit 1
    fi
    tar -xzf "$tmp/$name" -C "$tmp"
    mkdir -p "$dir"
    # why: a rename leaves a running sector on its old binary.
    cp "$tmp/sector" "$dir/.sector.new"
    chmod 755 "$dir/.sector.new"
    mv -f "$dir/.sector.new" "$dir/sector"

    if [ -n "${WAYLAND_DISPLAY:-}${DISPLAY:-}${XDG_CURRENT_DESKTOP:-}" ]; then
        desktop_entry
    fi

    missing=$(libraries "$dir/sector" | packages | sort -u)
    if [ -z "$missing" ]; then
        printf 'Installed %s in %s.\n' "$("$dir/sector" --version)" "$dir"
    else
        v=${name#sector_}
        printf 'Installed sector v%s in %s.\n' "${v%_"$plat".tar.gz}" "$dir"
        printf 'sector needs libraries this machine lacks. Install them with:\n'
        printf '  Debian, Ubuntu: sudo apt install %s\n' "$(printf '%s\n' "$missing" | cut -d ' ' -f 1 | sort -u | paste -sd ' ' -)"
        printf '  Arch: sudo pacman -S %s\n' "$(printf '%s\n' "$missing" | cut -d ' ' -f 2 | sort -u | paste -sd ' ' -)"
    fi
    case ":$PATH:" in
        *":$dir:"*) ;;
        *) printf 'Add %s to PATH in your shell profile.\n' "$dir" ;;
    esac
}

# why: main runs last, so a download cut short runs nothing.
if [ "${SECTOR_GET_TEST:-}" != 1 ]; then
    main
fi
