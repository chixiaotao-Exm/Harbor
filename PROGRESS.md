# Harbor v2 实施进度

依据 [harbor-v2-PLAN.md](../harbor-v2-PLAN.md)。逐段推进，每段自测通过才进下一段。

代码在 `harbor-go/`（原 Python 版 `harbor/` 保留作对照）。

## 已完成

### 段 1 · 骨架与存储 ✅
- `internal/config`：配置读写（600 权限）、脱敏
- `internal/store`：SQLite WAL、迁移幂等、账号/设置/事件表
- `internal/api`：健康检查、设置读写、静态托管回退
- `cmd/harbor`：启动、`-check` 自检、`-schema` 导表

证据：`go test ./internal/store/` 通过（含 20 协程 × 50 次并发写 1000 行无 BUSY）；
`/api/health` 返回 200；设置 PUT 后脱敏返回 `cpug…kNHf` 且落库落盘。

### 段 2 · 出口与取号 ✅
- `internal/netx`：串内空格/换行/全角清洗、sid 与 session 轮换、探活取 IP、任务级出口固定与 Renew
- `internal/mailbox`：全局取号锁、域名降级、验证码轮询

证据：`go test ./internal/netx/` 通过（含 10 并发取样 maxPar==1、任务内 IP 恒定、Renew 必变）；
`go test ./internal/mailbox/` 通过（含并发严格串行、域名降级、全空报 ErrNoMail）。

### 段 3 · 指纹与放行凭据 ✅
- `internal/fingerprint`：随机档案（UA/时区/语言/分辨率/特征位/设备号）、载荷加密、难度求解、凭据与 SO 头组装
- `internal/events`：事件总线、SSE 输出、慢订阅丢弃

证据：`go test ./internal/fingerprint/` 通过（载荷往返、256 种种子、200 组随机难度全部命中、字段域校验、20 次载荷不重复）；
`go test ./internal/events/` 通过（订阅/取消/慢订阅不阻塞/SSE 真流）。

全量：`go test ./...` 全绿，`go vet` 与 `gofmt -l` 无输出。

### 段 4 · 上游协议 ✅
- `internal/flow`：csrf → 登录 → 验证页 → 发码 → 校验 → 建档 → 回调取票据 → 写密码 → 绑双因子
- 每步带放行凭据；403/429 归为可重试的 `ErrBlocked`；错误统一带阶段名
- 客户端补 cookie 罐（`fhttp/cookiejar`），修掉 409 "session no longer valid"

证据：`go test ./internal/flow/` 全绿（含 happy path、403 分类、缺 URL、密码页判定、密码失败仍保号、名字/密码不重复、TOTP）；
`go test -tags=live` 真跑一单成功：
`earthy.asunder5j@icloud.com----Vwh33nh4M49mI6E8----YLLT427BJ4A6GZX5EOLXCR4XSTZMDSSK`（71 秒，含出口→取号→注册→密码→2FA）。

## 进行中

段 8 · 前端与联调。

## 已完成（续）

### 段 5 · 主任务队列 ✅
- `internal/runner`：并发槽位、间隔、停止只挡新单、统计与成功率、工作明细、取号失败自动暂停
- 接进 API：`/run/start`、`/run/stop`、`/run/stat`、`/run/items`

证据：`go test ./internal/runner/` 全绿（并发上限不超 threads、停止只挡新位、在跑的仍收尾、取号失败计失败、连续 5 次失败暂停、重复启动被拒、明细就地更新、统计有事件）。

### 段 6 · 账号接口 ✅
- `GET /accounts`（两页签独立分页 + 搜索）、`/accounts/stats`、`POST /accounts/select`、`POST /accounts/export`
- 导出严格分段：mfa 三段、plain 两段，文件名带类型

证据：`go test ./internal/api/` 全绿（空页、超页、size 上下限、页签不串、搜索只在页签内、两种导出格式、按 id 导出、文件名头、跨页全选只覆盖本页签、注入字符不破行）。

### 段 7 · 异步任务 Q1–Q5 ✅
- `internal/mfa`：状态机、看板、失败页、每步 3 次重试与 3/6/12 退避、重试换出口
- Q1 失败走 Q2 密码登录；无密码直入失败页并标 `need_password_login`
- `internal/flow` 补 `PasswordLogin`、`EnrollTOTP`、`ActivateTOTP`、`FactorDecision`
- 接进 API：`/mfa/start`、`/mfa/board`、`/mfa/failed`、`/mfa/retry`

证据：`go test ./internal/mfa/` 全绿（Q1 通跳过 Q2、Q1 失败转密码登录、Q2 失败进失败页、无密码直入失败页、重试次数与退避序列、Q5 成功清空所有列、失败页可重投、已绑跳过）。

全量：`go test ./...` 全绿，`go vet` 与 `gofmt -l` 无输出。

## 待办

（无，八段全部完成）

### 段 8 · 前端与联调 ✅
- Vue 3 + Vite + TS，四个页面：工作台 / 设置 / 账号 / 异步任务
- 白粉配色、280px 侧栏、网格底纹、小圆角、等宽数字，骨架对齐参考站
- 前端产物 `//go:embed` 进二进制，深链回退 index.html
- 设置页粘贴即存本机、代理脏串自动清洗

证据：`npm run build` 无 TS 报错；`gofmt`/`go vet` 全绿；交叉编译出 18MB Linux 单文件。

**端到端实跑（服务器）**：
- 主任务 2 单并发 2 → 2 成功 0 失败，100% 成功率
- 账号页 `plain` 2 条 → 异步投递 → `mfa` 2 条，失败页空
- 导出：plain `账号----密码` 两段，mfa `账号----密码----密钥` 三段，文件名带类型

**部署验证**：预检端口空闲、目录与单元无冲突 → 上传 → `-check` 自检通过 →
`systemctl enable --now harbor` 成功 → 首页 200、深链 200、健康 200 →
现有 4 个站点全部仍 200、无失败单元。


## 后续修正

- **刷新丢任务**：日志只存浏览器内存。events.Hub 加 400 条历史环，新订阅先回放；新增 GET /api/events/history 供刷新后补回；SSE 重放按内容去重。
- **日志顺序**：工作台日志与明细改为最新在上，插入队首、超出从尾部裁。
- 历史环上限测试与回放测试已补，全量 go test ./... 绿。
