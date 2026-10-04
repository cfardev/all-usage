#!/bin/sh
# Build release archives into dist/.
# Names match install.sh and internal/cli/update.go:
#   all-usage_<os>_<arch>.tar.gz  (linux, darwin)
#   all-usage_<os>_<arch>.zip     (windows)
# Each archive contains a single binary at the root.
set -eu

version=${1:?usage: scripts/dist.sh <version>}

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
out="$root/dist"
rm -rf "$out"
mkdir -p "$out"
cd "$root"

for spec in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
	os=${spec%%/*}
	arch=${spec#*/}
	bin=all-usage
	ext=tar.gz
	if [ "$os" = windows ]; then
		bin=all-usage.exe
		ext=zip
	fi
	echo "building ${os}/${arch}"
	stage="$out/.stage"
	rm -rf "$stage"
	mkdir -p "$stage"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
		go build -trimpath \
		-ldflags "-s -w -X github.com/cfardev/all-usage/internal/buildinfo.Version=${version}" \
		-o "$stage/$bin" .
	archive="all-usage_${os}_${arch}.${ext}"
	if [ "$ext" = tar.gz ]; then
		tar -C "$stage" -czf "$out/$archive" "$bin"
	else
		python3 - "$out/$archive" "$stage/$bin" "$bin" <<'PY'
import sys, zipfile
archive, src, name = sys.argv[1:]
with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as z:
    z.write(src, name)
PY
	fi
	rm -rf "$stage"
done

(
	cd "$out"
	sha256sum all-usage_*.tar.gz all-usage_*.zip > checksums.txt
	sha256sum -c checksums.txt
)
echo "wrote $out"
