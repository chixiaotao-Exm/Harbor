#!/usr/bin/env bash
# 备份库与配置，保留最近 14 份。
set -euo pipefail

DIR="${HARBOR_DIR:-/opt/harbor}"
KEEP=${KEEP:-14}
STAMP=$(date +%Y%m%d-%H%M%S)

mkdir -p "${DIR}/backup"
chmod 700 "${DIR}/backup"

sqlite3 "${DIR}/data/harbor.db" ".backup '${DIR}/backup/harbor-${STAMP}.db'"
chmod 600 "${DIR}/backup/harbor-${STAMP}.db"
cp -f "${DIR}/data/config.toml.json" "${DIR}/backup/config-${STAMP}.json" 2>/dev/null || true

# 清理旧份
ls -1t "${DIR}"/backup/harbor-*.db 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -f
ls -1t "${DIR}"/backup/config-*.json 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -f

echo "已备份到 ${DIR}/backup/harbor-${STAMP}.db（保留 ${KEEP} 份）"
