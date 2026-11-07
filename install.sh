#!/bin/sh
set -eu

REPO="grangedevgroup-code/AutMouseClicker"
BIN="${HOME}/.local/bin/autmouseclicker"

fail() {
	printf 'AutMouseClicker installer: %s\n' "$*" >&2
	exit 1
}

if [ "${1:-}" = "--uninstall" ]; then
	[ -x "$BIN" ] || fail "AutMouseClicker is not installed in $BIN"
	exec "$BIN" uninstall
fi

[ "$(uname -s)" = Linux ] || fail "only Linux is supported for now"
case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*) fail "there is no build for $(uname -m) yet" ;;
esac

VERSION="${AMC_VERSION:-latest}"
if [ "$VERSION" = latest ]; then
	BASE="https://github.com/$REPO/releases/latest/download"
else
	BASE="https://github.com/$REPO/releases/download/$VERSION"
fi
ASSET="autmouseclicker-linux-$ARCH.tar.gz"

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q --https-only -O "$2" "$1"
	else
		fail "install curl or wget first"
	fi
}

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
trap 'exit 1' HUP INT TERM

printf 'Downloading %s (%s)\n' "$ASSET" "$VERSION"
fetch "$BASE/$ASSET" "$TMP/$ASSET" || fail "could not download $BASE/$ASSET"
fetch "$BASE/SHA256SUMS" "$TMP/SHA256SUMS" || fail "could not download the checksums"
grep " $ASSET\$" "$TMP/SHA256SUMS" > "$TMP/expected" || fail "no checksum listed for $ASSET"
(cd "$TMP" && sha256sum -c expected >/dev/null 2>&1) || fail "the download does not match its checksum"
tar -xzf "$TMP/$ASSET" -C "$TMP"
"$TMP/autmouseclicker/autmouseclicker" install
