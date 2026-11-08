# Harbor

批量开户工作台。Go 后端 + Vue 前端，单二进制部署，白粉界面。


## 结构

```
cmd/harbor        入口
internal/config   配置读写
internal/store    SQLite（WAL）
internal/netx     代理清洗、sid 轮换、出口探活
internal/mailbox  接码与收码
internal/fingerprint 指纹与放行凭据
internal/flow     上游协议
internal/runner   主任务队列
internal/mfa      异步任务状态机
internal/api      路由与处理
web               Vue 3 + Vite
deploy            systemd、安装、升级、备份
```

## 本地开发

```bash
go run ./cmd/harbor -data ./data            # 后端，127.0.0.1:1321
(cd web && npm install && npm run dev)      # 前端，5273，已代理 /api
```

## 构建

```bash
(cd web && npm install && npm run build)    # 产物进 web/dist，会被内嵌
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o harbor ./cmd/harbor
```

## 部署（Ubuntu 24.04）

```bash
bash deploy/install.sh          # 预检端口 → 编译 → 上传 → 自检 → 起服务
bash deploy/upgrade.sh          # 备份 → 换版 → 重启 → 验活，失败自动回滚
bash deploy/backup.sh           # 备份库与配置，保留 14 份
```

脚本里的 `HARBOR_HOST` 默认是 `root@YOUR_SERVER_IP`，先改掉或用环境变量覆盖：

```bash
export HARBOR_HOST=root@1.2.3.4
```

直接监听 `0.0.0.0:1321`，可用 `http://<服务器IP>:1321` 访问；要走域名再单独加 nginx server 块。

## 配置

密钥与代理不写进仓库，两种填法：页面「设置」里填，或直接改 `data/config.toml.json`（自动生成，600 权限）。

| 字段 | 说明 |
| --- | --- |
| `api_key` | 接码平台密钥 |
| `proxy` | 代理，含 `-region-XX-` 段，切换地区会改写它 |
| `domain` | 默认邮箱域名 |

环境变量模板见 `.env.example`。

## 测试

```bash
go test ./...                   # 单元测试
go vet ./... && gofmt -l .
(cd web && npm run build)       # 前端类型检查与构建
```

真实链路（会花余额）：

```bash
HARBOR_KEY=xxx HARBOR_PROXY=socks5://... \
  go test -tags=live -run TestLiveRegister -v ./internal/flow/
```

## 页面

- **工作台**：数量 / 并发 / 间隔 / 域名，成功率，日志，工作明细。
- **设置**：API Key、代理、默认行为。密钥与代理本机留痕，接口打码返回。
- **账号**：已绑 / 未绑两页签，各自分页、全选、勾选，导出 txt。
- **异步任务**：Q1–Q5 列实时显示处理中的账号，失败页可重投。

导出格式分两种：未绑 `账号----密码`，已绑 `账号----密码----双因子密钥`。

## 说明

- 代理里的空格、换行、全角冒号自动清洗，不写协议补 `socks5h://`。
- 取码 240 秒未到即退号，不占余额；域名无货自动降级。
- 出口被拦（403/429）换 sid 重试；连续 5 次取号失败自动暂停整批。
- 登录态约 10 天有效；双因子密钥为 Base32，可导入任意验证器。
