#!/bin/sh
# Runs get.sh against a fake release with stub curl, uname, ldd and ldconfig: sh scripts/get_test.sh
set -eu

get=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)/get.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fail=0
bad() {
    printf 'FAIL: %s\n' "$*" >&2
    fail=1
}

SECTOR_GET_TEST=1 . "$get"
unset SECTOR_GET_TEST

check() {
    got=$(platform "$1" "$2" 2>/dev/null) || got=error
    [ "$got" = "$3" ] || bad "platform $1 $2 = $got, want $3"
}
check Linux x86_64 linux_amd64
check Linux aarch64 linux_arm64
check Linux armv7l error
check Darwin arm64 error

got=$(printf '%s\n' 'libc.so.6 => /usr/lib/libc.so.6' 'libxkbcommon.so.0 => not found' \
    'libX11-xcb.so.1 => not found' 'libX11.so.6 => not found' | packages | sort -u | paste -sd '|' -)
[ "$got" = 'libx11-6 libx11|libx11-xcb1 libx11|libxkbcommon0 libxkbcommon' ] || bad "packages = $got"

# A fake v0.1.0 release: an x86_64 archive and checksums for it and an arm64 one.
rel=$tmp/release
mkdir -p "$rel" "$tmp/stage" "$tmp/bin"
printf '#!/bin/sh\necho sector v0.1.0\n' >"$tmp/stage/sector"
chmod 755 "$tmp/stage/sector"
printf 'png\n' >"$tmp/stage/icon.png"
tar -czf "$rel/sector_0.1.0_linux_amd64.tar.gz" -C "$tmp/stage" sector icon.png
(cd "$rel" && sha256sum sector_0.1.0_linux_amd64.tar.gz >checksums.txt)
printf '%s  sector_0.1.0_linux_arm64.tar.gz\n' 0000 >>"$rel/checksums.txt"

cat >"$tmp/bin/curl" <<EOF
#!/bin/sh
for a; do url=\$a; done
printf '%s\n' "\$url" >>"$tmp/curl.log"
cp "$rel/\${url##*/}" "\$3"
EOF
printf '#!/bin/sh\n[ "$1" = -s ] && echo Linux || echo x86_64\n' >"$tmp/bin/uname"
printf '#!/bin/sh\ncat "%s/ldd.out" 2>/dev/null\n' "$tmp" >"$tmp/bin/ldd"
printf '#!/bin/sh\n[ -e "%s/no-gles" ] || echo "libGLESv2.so.2 (libc6,x86-64) => /usr/lib/libGLESv2.so.2"\n' "$tmp" >"$tmp/bin/ldconfig"
chmod 755 "$tmp/bin/"*

# run runs get.sh with a fresh HOME and no graphical session; extra VAR=value arguments are added.
run() {
    rm -rf "$tmp/home" "$tmp/curl.log"
    mkdir "$tmp/home"
    env -u SECTOR_GET_TEST -u WAYLAND_DISPLAY -u DISPLAY -u XDG_CURRENT_DESKTOP -u SECTOR_VERSION -u SECTOR_INSTALL_DIR \
        HOME="$tmp/home" PATH="$tmp/bin:$PATH" "$@" sh "$get" 2>&1
}

out=$(run)
[ -x "$tmp/home/.local/bin/sector" ] || bad 'no binary in ~/.local/bin'
printf '%s' "$out" | grep -qF 'Installed sector v0.1.0 in' || bad "no version printed: $out"
grep -qF '/releases/latest/download/checksums.txt' "$tmp/curl.log" || bad 'did not fetch the latest release'
[ ! -e "$tmp/home/.local/share/applications/sector.desktop" ] || bad 'desktop entry without a graphical session'
printf '%s' "$out" | grep -qF 'needs libraries' && bad "reported missing libraries: $out"

out=$(run DISPLAY=:1 SECTOR_VERSION=0.1.0 SECTOR_INSTALL_DIR="$tmp/home/bin")
[ -x "$tmp/home/bin/sector" ] || bad 'SECTOR_INSTALL_DIR ignored'
grep -qF '/releases/download/v0.1.0/sector_0.1.0_linux_amd64.tar.gz' "$tmp/curl.log" || bad 'SECTOR_VERSION ignored'
entry=$tmp/home/.local/share/applications/sector.desktop
grep -qxF "Exec=\"$tmp/home/bin/sector\"" "$entry" 2>/dev/null || bad 'desktop entry missing or Exec wrong'
grep -qxF "Icon=$tmp/home/.local/share/sector/icon.png" "$entry" || bad 'desktop entry Icon wrong'
[ -f "$tmp/home/.local/share/sector/icon.png" ] || bad 'no icon'

printf '%s\n' 'libxkbcommon.so.0 => not found' >"$tmp/ldd.out"
out=$(run)
printf '%s' "$out" | grep -qF 'sudo apt install libxkbcommon0' || bad "no apt hint: $out"
printf '%s' "$out" | grep -qF 'sudo pacman -S libxkbcommon' || bad "no pacman hint: $out"
printf '%s' "$out" | grep -qF 'Installed sector v0.1.0 in' || bad "no version from the archive name: $out"
rm "$tmp/ldd.out"

: >"$tmp/no-gles"
out=$(run)
printf '%s' "$out" | grep -qF 'sudo apt install libgles2' || bad "no hint for GLESv2: $out"
rm "$tmp/no-gles"

printf 'tampered\n' >>"$rel/sector_0.1.0_linux_amd64.tar.gz"
if out=$(run); then
    bad "installed a tampered archive: $out"
fi
printf '%s' "$out" | grep -qF 'does not match checksums.txt' || bad "wrong refusal: $out"
[ ! -e "$tmp/home/.local/bin/sector" ] || bad 'tampered archive left a binary'

exit "$fail"
