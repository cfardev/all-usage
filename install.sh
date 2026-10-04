#!/bin/sh
# Install the latest all-usage release binary.
#   curl -fsSL https://raw.githubusercontent.com/cfardev/all-usage/main/install.sh | bash
set -eu

repo="cfardev/all-usage"

if ! command -v curl >/dev/null 2>&1; then
	echo "all-usage: curl is required" >&2
	exit 1
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
linux | darwin) ;;
*)
	echo "all-usage: unsupported OS: $os" >&2
	echo "Download a Windows build from https://github.com/${repo}/releases/latest" >&2
	exit 1
	;;
esac

arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*)
	echo "all-usage: unsupported architecture: $arch" >&2
	exit 1
	;;
esac

# Asset names match scripts/dist.sh and internal/cli/update.go.
asset="all-usage_${os}_${arch}.tar.gz"

need_sudo=0
if [ -n "${ALL_USAGE_INSTALL_DIR:-}" ]; then
	dest=$ALL_USAGE_INSTALL_DIR
elif [ -n "${HOME:-}" ] && mkdir -p "${HOME}/.local/bin" 2>/dev/null && [ -w "${HOME}/.local/bin" ]; then
	dest="${HOME}/.local/bin"
else
	dest=/usr/local/bin
fi
# mkdir succeeding is not enough: /usr/local/bin exists and still needs sudo.
if [ ! -d "$dest" ] || [ ! -w "$dest" ]; then
	if mkdir -p "$dest" 2>/dev/null && [ -w "$dest" ]; then
		need_sudo=0
	else
		need_sudo=1
	fi
fi

url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${repo}/releases/latest") || {
	echo "all-usage: could not find the latest release" >&2
	exit 1
}
tag=${url##*/}
case "$tag" in
v[0-9]*) ;;
*)
	echo "all-usage: could not find the latest release (${url})" >&2
	exit 1
	;;
esac

echo "Installing all-usage ${tag} (${os}/${arch}) to ${dest}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

base="https://github.com/${repo}/releases/download/${tag}"
curl -fsSL "${base}/${asset}" -o "${tmp}/${asset}"
curl -fsSL "${base}/checksums.txt" -o "${tmp}/checksums.txt"

line=$(awk -v file="$asset" '$2 == file { print; exit }' "$tmp/checksums.txt") || true
if [ -z "$line" ]; then
	echo "all-usage: no checksum for ${asset}" >&2
	exit 1
fi
printf '%s\n' "$line" > "$tmp/check"
(
	cd "$tmp"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -c check
	else
		shasum -a 256 -c check
	fi
)

tar -xzf "${tmp}/${asset}" -C "$tmp" all-usage

if [ "$need_sudo" -eq 1 ]; then
	sudo mkdir -p "$dest"
	sudo install -m 755 "$tmp/all-usage" "$dest/all-usage"
else
	install -m 755 "$tmp/all-usage" "$dest/all-usage"
fi

echo "Installed: ${dest}/all-usage"
"$dest/all-usage" --version

case ":${PATH:-}:" in
*":${dest}:"*) ;;
*)
	printf '\nAdd it to your PATH:\n  export PATH="%s:$PATH"\n' "$dest"
	;;
esac
