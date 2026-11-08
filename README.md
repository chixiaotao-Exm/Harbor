# Harbor

批量开户工作台。Go 后端 + Vue 前端，单二进制部署，白粉界面。

## 能力

- **一条任务一个出口**：开工前先探活拿 IP，全程固定；只有重试才换。
- **取号串行不预取**：任一时刻在手邮箱不超过并发数，跑完一单才补下一单。
- **过放行校验**：按上游机制现场生成凭据并解工作量证明，逐请求附带。
- **一套一档指纹**：UA、时区、语言、分辨率、特征位、设备号现生成，账号之间不同源。
- **主任务**：取号 → 收码 → 校验 → 建档 → 登录态 → 加密码。
- **异步任务**：Q1 校验登录态 →（失败）Q2 密码登录 → Q3 登记 → Q4 激活 → Q5 固化；
  每步自动重试 3 次并换出口，超限落失败页，可单条或全部重投。

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

直接监听 `0.0.0.0:1321`，可用 http://45.67.219.166:1321 访问；要走域名再单独加 nginx server 块。

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
