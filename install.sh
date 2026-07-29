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
# Requires: curl, tar, sha256sum. Linux only (amd64, arm64).

set -euo pipefail

OWNER_REPO="thedataflows/dotdrift"
BINDIR="${DOTDRIFT_BINDIR:-$HOME/.local/bin}"
VERSION="${1:-latest}"

err() { printf 'install: error: %s\n' "$*" >&2; }
die() { err "$*"; exit 1; }

for dep in curl tar sha256sum; do
	command -v "$dep" >/dev/null 2>&1 || die "required command not found: $dep"
done

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

# "latest" is resolved by following the releases/latest redirect to its tag.
if [[ "$VERSION" == "latest" ]]; then
	redirect="$(curl -fsSL -o /dev/null -w '%{url_effective}' \
		"https://github.com/${OWNER_REPO}/releases/latest")"
	VERSION="${redirect##*/}"
	[[ -n "$VERSION" ]] || die "could not resolve the latest release tag"
fi

# --- download --------------------------------------------------------------

base="https://github.com/${OWNER_REPO}/releases/download/${VERSION}"
asset="dotdrift_${VERSION}_${os}_${arch}.tar.gz"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

printf '==> dotdrift %s (%s/%s)\n' "$VERSION" "$os" "$arch"
curl -fsSL -o "${tmp}/${asset}" "${base}/${asset}"
curl -fsSL -o "${tmp}/sha256sums.txt" "${base}/sha256sums.txt" \
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
