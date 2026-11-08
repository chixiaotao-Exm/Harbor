#!/usr/bin/env bash
# 安装到本机。只新增 /opt/harbor 与 harbor.service，不动现有站点。
set -euo pipefail

HOST="${HARBOR_HOST:-root@YOUR_SERVER_IP}"
DIR=/opt/harbor
BIN=harbor
PORT=1321

echo "== 1. 预检 =="
if ss -lntp | grep -q ":${PORT} "; then
  echo "端口 ${PORT} 已被占用：" >&2
  ss -lntp | grep ":${PORT} " >&2
  exit 1
fi
echo "端口 ${PORT} 空闲，继续"

echo "== 2. 交叉编译 =="
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o "${BIN}" ./cmd/harbor

echo "== 3. 上传 =="
ssh "${HOST}" "mkdir -p ${DIR}/data && chmod 700 ${DIR}/data"
scp -q "${BIN}" "${HOST}:${DIR}/${BIN}"
scp -q deploy/harbor.service "${HOST}:/etc/systemd/system/harbor.service"
ssh "${HOST}" "chmod 755 ${DIR}/${BIN}"

echo "== 4. 自检 =="
ssh "${HOST}" "${DIR}/${BIN} -data ${DIR}/data -check"

echo "== 5. 起服务 =="
ssh "${HOST}" "systemctl daemon-reload && systemctl enable --now harbor"
sleep 2
ssh "${HOST}" "systemctl --no-pager --lines=5 status harbor || true"
ssh "${HOST}" "curl -fsS http://127.0.0.1:${PORT}/api/health && echo"

echo "完成：http://127.0.0.1:${PORT}（本机回环，需要外网再单独加 nginx）"
