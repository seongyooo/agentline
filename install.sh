#!/bin/sh
# Install AgentLine.
#
#   curl -fsSL https://raw.githubusercontent.com/seongyooo/agentline/main/install.sh | sh
#
# Downloads the release binary for this machine and puts it somewhere on PATH.
# No Go toolchain, no build step. Set VERSION to pin one, and BINDIR to choose
# where it lands.

set -eu

REPO=seongyooo/agentline
BINDIR=${BINDIR:-}
VERSION=${VERSION:-latest}

fail() {
	echo "install: $*" >&2
	exit 1
}

# platform prints the goos_goarch pair the release archives are named with.
platform() {
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	case "$os" in
	linux | darwin) ;;
	mingw* | msys* | cygwin*)
		fail "on Windows, use Scoop or download the zip from https://github.com/$REPO/releases"
		;;
	*) fail "unsupported system: $os" ;;
	esac

	arch=$(uname -m)
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "unsupported architecture: $arch" ;;
	esac

	echo "${os}_${arch}"
}

# resolve turns "latest" into the tag it points at, since the download URL
# needs the real tag.
resolve() {
	if [ "$VERSION" != latest ]; then
		echo "$VERSION"
		return
	fi

	tag=$(download "https://api.github.com/repos/$REPO/releases/latest" |
		sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
	[ -n "$tag" ] || fail "could not find the latest release"
	echo "$tag"
}

# download fetches a URL to stdout with whichever tool is present.
download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO- "$1"
	else
		fail "needs curl or wget"
	fi
}

# destination picks somewhere on PATH that can be written to, preferring a
# directory the user owns so the script never needs a password.
destination() {
	if [ -n "$BINDIR" ]; then
		echo "$BINDIR"
		return
	fi
	for dir in "$HOME/.local/bin" "$HOME/bin" /usr/local/bin; do
		if [ -d "$dir" ] && [ -w "$dir" ]; then
			echo "$dir"
			return
		fi
	done
	# Nothing suitable exists yet; make the one most systems already expect.
	mkdir -p "$HOME/.local/bin"
	echo "$HOME/.local/bin"
}

main() {
	target=$(platform)
	tag=$(resolve)
	version=${tag#v}
	bindir=$(destination)

	url="https://github.com/$REPO/releases/download/$tag/agentline_${version}_${target}.tar.gz"

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	echo "installing agentline $tag ($target)"
	download "$url" >"$tmp/agentline.tar.gz" || fail "could not download $url"
	tar -xzf "$tmp/agentline.tar.gz" -C "$tmp" agentline

	install -m 0755 "$tmp/agentline" "$bindir/agentline" 2>/dev/null ||
		{ cp "$tmp/agentline" "$bindir/agentline" && chmod 0755 "$bindir/agentline"; }

	echo "installed $bindir/agentline"

	# Saying so now beats the command appearing not to exist afterwards.
	case ":$PATH:" in
	*":$bindir:"*) ;;
	*) echo "note: $bindir is not on your PATH — add it to use 'agentline' by name" ;;
	esac
}

main
