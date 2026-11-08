# 发布流程

仓库作为版本管理，主干保持随时可发布。

## 分支

| 分支 | 用途 |
| --- | --- |
| `main` | 稳定，只接 PR 与发布提交，始终可构建可部署 |
| `feat/*` | 新功能 |
| `fix/*` | 修缺陷 |
| `chore/*` | 杂务、文档、依赖 |

改动走分支，提 PR 合进 `main`，不直接往 `main` 推。

## 提交信息

```
<类型>: <一句话>

<可选的正文，说明为什么>
```

类型取 `feat` / `fix` / `chore` / `docs` / `refactor` / `perf` / `test`。
一句话写完“做了什么”，正文写“为什么”。

## 版本号

`主.次.修`，改动性质决定进位：

- `feat` 且不兼容 → 进位主版本
- `feat` 兼容 → 进位次版本
- `fix` / `chore` → 进位修订号

## 发布步骤

1. 从 `main` 切 `release/vX.Y.Z`
2. 改 `CHANGELOG.md`：把 `Unreleased` 内容挪到新版本标题下，写日期
3. 提交 `chore: 发布 vX.Y.Z`
4. 打标签并推送：

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin release/vX.Y.Z --tags
```

5. 提 PR 合回 `main`
6. 用构建脚本出包（版本号由标签注入）：

```bash
VERSION=vX.Y.Z ./deploy/build.sh
```

7. 上传产物，按 `deploy/upgrade.sh` 升级目标机

## 构建产物

`.gitignore` 排除了二进制与前端产物，发布包从 CI 或本地构建取，不进仓库。

## 回滚

升级脚本验活失败会自动回滚。手工回滚：把 `/opt/harbor/backup/harbor-<时间戳>` 拷回 `/opt/harbor/harbor` 后重启。

## 检查清单

发布前逐项确认：

- [ ] `go vet ./...`、`gofmt -l .` 无输出
- [ ] `go test ./...` 全绿
- [ ] `cd web && npm run build` 通过
- [ ] `CGO_ENABLED=0 GOOS=linux go build ./cmd/harbor` 通过
- [ ] `CHANGELOG.md` 已更新、版本号已进位
- [ ] 仓库无密钥、无本地路径残留
