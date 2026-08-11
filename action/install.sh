#!/usr/bin/env bash
#
# Download a released envseal binary and verify it before use.
#
# A release is only trusted if its archive matches the SHA-256 published alongside it.
#
#   install.sh <version|latest> <destination-directory>

set -euo pipefail

readonly REPO="PeacexF/envseal"

version="${1:-latest}"
dest="${2:-}"

if [ -z "$dest" ]; then
	echo "usage: install.sh <version|latest> <destination-directory>" >&2
	exit 2
fi

# resolve_latest asks the API for the newest tag. A token is used when present
# purely to avoid the unauthenticated rate limit.
resolve_latest() {
	local url="https://api.github.com/repos/$REPO/releases/latest" response
	if [ -n "${GITHUB_TOKEN:-}" ]; then
		response=$(curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" "$url")
	else
		response=$(curl -fsSL "$url")
	fi

	printf '%s' "$response" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | cut -d'"' -f4
}

detect_os() {
	case "$(uname -s)" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	MINGW* | MSYS* | CYGWIN*) echo windows ;;
	*)
		echo "unsupported operating system: $(uname -s)" >&2
		exit 1
		;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*)
		echo "unsupported architecture: $(uname -m)" >&2
		exit 1
		;;
	esac
}

# verify compares the archive against the checksum published with the release.
verify() {
	local archive="$1" checksums="$2" expected actual

	expected=$(grep " $(basename "$archive")\$" "$checksums" | cut -d' ' -f1)
	if [ -z "$expected" ]; then
		echo "no checksum published for $(basename "$archive")" >&2
		exit 1
	fi

	if command -v sha256sum >/dev/null 2>&1; then
		actual=$(sha256sum "$archive" | cut -d' ' -f1)
	else
		actual=$(shasum -a 256 "$archive" | cut -d' ' -f1)
	fi

	if [ "$expected" != "$actual" ]; then
		echo "checksum mismatch for $(basename "$archive")" >&2
		echo "  published: $expected" >&2
		echo "  download:  $actual" >&2
		exit 1
	fi
	echo "checksum verified: $(basename "$archive")"
}

if [ "$version" = "latest" ]; then
	version=$(resolve_latest)
	if [ -z "$version" ]; then
		echo "unable to determine the latest release" >&2
		exit 1
	fi
fi
version="${version#v}"

os=$(detect_os)
arch=$(detect_arch)

extension=tar.gz
if [ "$os" = "windows" ]; then
	extension=zip
fi

archive="envseal_${version}_${os}_${arch}.${extension}"
base="https://github.com/$REPO/releases/download/v${version}"

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

echo "downloading $archive"
curl -fsSL -o "$workdir/$archive" "$base/$archive"
curl -fsSL -o "$workdir/checksums.txt" "$base/checksums.txt"

verify "$workdir/$archive" "$workdir/checksums.txt"

mkdir -p "$dest"
if [ "$extension" = "zip" ]; then
	unzip -q -o "$workdir/$archive" envseal.exe -d "$dest"
else
	tar -xzf "$workdir/$archive" -C "$dest" envseal
	chmod +x "$dest/envseal"
fi

echo "installed envseal v$version to $dest"
