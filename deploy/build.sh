#!/usr/bin/env bash
# 构建发布包。版本号取自参数、标签或提交哈希。
#
#   VERSION=v1.0.0 ./deploy/build.sh    # 指定版本
#   ./deploy/build.sh                    # 取 git describe
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
BUILT="$(date -u +%FT%TZ)"
OUT="dist"

echo "== 构建 $VERSION ($COMMIT) =="

echo "-- 前端 --"
(cd web && npm ci --silent && npm run build)

echo "-- 后端 --"
mkdir -p "$OUT"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILT}" \
  -o "$OUT/harbor" ./cmd/harbor

echo "-- 附文件 --"
cp -f deploy/harbor.service "$OUT/"
cp -f README.md "$OUT/" 2>/dev/null || true

echo "-- 产物 --"
ls -lh "$OUT"
echo
echo "校验：$OUT/harbor -version"
echo "版本：$VERSION"
