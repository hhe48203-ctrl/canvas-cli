#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/canvas-install-test.XXXXXX")
trap 'rm -rf "$work"' 0 1 2 15
mkdir -p "$work/bin"

cat >"$work/bin/go" <<'EOF'
#!/bin/sh
set -eu
[ "$1" = install ]
printf '%s\n' "$2" >"$FAKE_GO_ARGS"
[ "${FAKE_GO_FAIL:-0}" = 0 ] || exit 1
cat >"$GOBIN/canvas-cli" <<SCRIPT
#!/bin/sh
echo "$FAKE_CANVAS_VERSION"
SCRIPT
chmod 0755 "$GOBIN/canvas-cli"
EOF
chmod 0755 "$work/bin/go"

export PATH="$work/bin:/usr/bin:/bin"
export HOME="$work/home"
export FAKE_GO_ARGS="$work/go-args"
export CANVAS_CLI_VERSION=deadbeef

FAKE_CANVAS_VERSION=one "$root/scripts/install.sh"
[ "$("$HOME/.local/bin/canvas")" = one ]
[ "$(cat "$FAKE_GO_ARGS")" = github.com/hhe48203-ctrl/canvas-cli@deadbeef ]

FAKE_CANVAS_VERSION=two "$root/scripts/install.sh"
[ "$("$HOME/.local/bin/canvas")" = two ]

(cd "$work" && CANVAS_CLI_INSTALL_DIR=relative FAKE_CANVAS_VERSION=custom "$root/scripts/install.sh")
[ "$("$work/relative/canvas")" = custom ]

if FAKE_CANVAS_VERSION=broken FAKE_GO_FAIL=1 "$root/scripts/install.sh"; then
	echo "failed build unexpectedly installed" >&2
	exit 1
fi
[ "$("$HOME/.local/bin/canvas")" = two ]

echo "installer test passed"
