#!/usr/bin/env bash
set -euo pipefail

PKG="./tools/singbox-sub"
OUT="tools/singbox-sub/dist"

mkdir -p "$OUT"

targets=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  name="singbox-sub-${os}-${arch}"
  [[ "$os" == "windows" ]] && name="${name}.exe"

  echo "Building ${name}..."
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "${OUT}/${name}" "$PKG"
done

echo ""
echo "Done:"
ls -lh "$OUT"
