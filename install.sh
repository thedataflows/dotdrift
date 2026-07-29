#!/usr/bin/env bash
#
# install.sh — install dotdrift from GitHub Releases.
#
# Usage:
#   ./install.sh [version]
#
#   version   release tag to install (default: latest)
#
# Environment:
#   DOTDRIFT_BINDIR   where to install the binary (default: ~/.local/bin)
#
# Examples:
#   ./install.sh                 # latest release
#   ./install.sh v0.4.0          # a specific release
#   curl -fsSL <raw>/install.sh | bash                 # latest, piped
#   curl -fsSL <raw>/install.sh | bash -s v0.4.0       # pinned, piped
#
# Requires: curl or wget, tar, sha256sum. Linux only (amd64, arm64).

set -euo pipefail

OWNER_REPO="thedataflows/dotdrift"
BINDIR="${DOTDRIFT_BINDIR:-$HOME/.local/bin}"
VERSION="${1:-latest}"

err()  { printf 'install: error: %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

have curl || have wget || die "required command not found: curl (or wget)"
for dep in tar sha256sum; do
	have "$dep" || die "required command not found: $dep"
done

# Fetch a URL to a local path with whichever HTTP client is available.
http_get() {  # http_get <url> <dest>
	if have curl; then curl -fsSL -o "$2" "$1"
	else              wget -qO "$2" "$1"
	fi
}

# Resolve the newest release tag. curl follows the releases/latest redirect
# and reads the final URL; wget falls back to the GitHub releases API.
resolve_latest() {
	if have curl; then
		local url
		url="$(curl -fsSL -o /dev/null -w '%{url_effective}' \
			"https://github.com/${OWNER_REPO}/releases/latest")"
		printf '%s\n' "${url##*/}"
	else
		wget -qO- "https://api.github.com/repos/${OWNER_REPO}/releases/latest" \
			| sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1
	fi
}

# --- platform --------------------------------------------------------------

case "$(uname -s)" in
	Linux*) os=linux ;;
	*) die "no prebuilt release for $(uname -s); dotdrift ships Linux binaries only" ;;
esac

case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "no prebuilt release for $(uname -m); supported: amd64, arm64" ;;
esac

# --- resolve version -------------------------------------------------------

# "latest" resolves to the newest release tag (see resolve_latest).
if [[ "$VERSION" == "latest" ]]; then
	VERSION="$(resolve_latest || true)"
	[[ -n "$VERSION" ]] || die "could not resolve the latest release tag"
fi

# --- download --------------------------------------------------------------

base="https://github.com/${OWNER_REPO}/releases/download/${VERSION}"
asset="dotdrift_${VERSION}_${os}_${arch}.tar.gz"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

printf '==> dotdrift %s (%s/%s)\n' "$VERSION" "$os" "$arch"
http_get "${base}/${asset}" "${tmp}/${asset}"
http_get "${base}/sha256sums.txt" "${tmp}/sha256sums.txt" \
	|| die "no checksum published for ${VERSION}; refusing to install unverified"

# --- verify + install ------------------------------------------------------

# ponytail: sha256sums.txt lists every arch; --ignore-missing checks only ours.
(cd "$tmp" && sha256sum -c --ignore-missing sha256sums.txt >/dev/null) \
	|| die "checksum verification failed for ${asset}"

tar -xzf "${tmp}/${asset}" -C "$tmp" dotdrift
mkdir -p "$BINDIR"
install -m 0755 "${tmp}/dotdrift" "${BINDIR}/dotdrift"

printf '==> installed %s/dotdrift\n' "$BINDIR"
case ":${PATH}:" in
	*":${BINDIR}:"*) ;;
	*) printf '    note: %s is not on your PATH\n' "$BINDIR" ;;
esac
