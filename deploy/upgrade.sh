#!/usr/bin/env bash
# 升级：备份 → 换二进制 → 重启 → 验活。失败自动回滚。
set -euo pipefail

HOST="${HARBOR_HOST:-root@YOUR_SERVER_IP}"
DIR=/opt/harbor
PORT=1321
STAMP=$(date +%Y%m%d-%H%M%S)

echo "== 1. 备份当前版本 =="
ssh "${HOST}" "set -e
  mkdir -p ${DIR}/backup
  cp -f ${DIR}/harbor ${DIR}/backup/harbor-${STAMP}
  sqlite3 ${DIR}/data/harbor.db \".backup '${DIR}/backup/harbor-${STAMP}.db'\" || true
  echo 备份完成 ${STAMP}"

echo "== 2. 编译并上传 =="
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o harbor.new ./cmd/harbor
scp -q harbor.new "${HOST}:${DIR}/harbor.new"

echo "== 3. 切换并重启 =="
ssh "${HOST}" "set -e
  mv -f ${DIR}/harbor.new ${DIR}/harbor
  chmod 755 ${DIR}/harbor
  systemctl restart harbor"

echo "== 4. 验活 =="
sleep 2
if ssh "${HOST}" "curl -fsS http://127.0.0.1:${PORT}/api/health"; then
  echo
  echo "升级成功"
else
  echo "验活失败，回滚到 ${STAMP}" >&2
  ssh "${HOST}" "set -e
    cp -f ${DIR}/backup/harbor-${STAMP} ${DIR}/harbor
    systemctl restart harbor"
  exit 1
fi
