#!/bin/sh
set -eu

module=github.com/hhe48203-ctrl/canvas-cli
version=${CANVAS_CLI_VERSION:-main}

if [ -n "${CANVAS_CLI_INSTALL_DIR:-}" ]; then
	install_dir=$CANVAS_CLI_INSTALL_DIR
elif [ -n "${HOME:-}" ]; then
	install_dir=$HOME/.local/bin
else
	echo "canvas: set HOME or CANVAS_CLI_INSTALL_DIR" >&2
	exit 1
fi
case $install_dir in
	/*) ;;
	*) install_dir=$(pwd)/$install_dir ;;
esac
if [ -d "$install_dir/canvas" ]; then
	echo "canvas: install target is a directory: $install_dir/canvas" >&2
	exit 1
fi

command -v go >/dev/null 2>&1 || {
	echo "canvas: Go is required: https://go.dev/dl/" >&2
	exit 1
}

work=$(mktemp -d "${TMPDIR:-/tmp}/canvas-install.XXXXXX")
target_tmp=
cleanup() {
	rm -rf "$work"
	[ -z "$target_tmp" ] || rm -f "$target_tmp"
}
trap cleanup 0 1 2 15

echo "Installing canvas@$version..." >&2
GOBIN=$work go install "$module@$version"
built=$work/canvas-cli
CANVAS_USAGE_LOG=0 "$built" --help >/dev/null

mkdir -p "$install_dir"
target_tmp=$(mktemp "$install_dir/.canvas.XXXXXX")
cat "$built" >"$target_tmp"
chmod 0755 "$target_tmp"
mv -f "$target_tmp" "$install_dir/canvas"
target_tmp=

echo "Installed $install_dir/canvas" >&2
case ":${PATH:-}:" in
	*:"$install_dir":*) ;;
	*) echo "Add $install_dir to PATH to run canvas." >&2 ;;
esac
