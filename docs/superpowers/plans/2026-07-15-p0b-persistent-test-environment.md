# P0-B Persistent Test Environment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Use superpowers:test-driven-development for every behavior change and superpowers:verification-before-completion before marking P0-B complete. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不拆分现有 Go 模块化单体的前提下，建立可重复验证的 PostgreSQL + MinIO 持久化测试环境，证明服务重启、依赖故障、远程制品下载和元数据/制品成对备份恢复均满足 P0-B 验收条件。

**Architecture:** 保留当前 `all` 模式和既有 PostgreSQL、S3 adapter。HTTP 层新增一个无鉴权 `/readyz`，通过 bootstrap 注入的窄接口并行探测 PostgreSQL 与对象存储；`/healthz` 继续只表示进程存活。持久化闭环由 opt-in Go 集成测试验证，Docker 脚本只负责创建隔离的临时 PostgreSQL/MinIO、注入随机测试凭据、运行测试和清理。备份恢复使用 PostgreSQL 原生 dump/restore 与 MinIO 对象镜像，二者共享同一个备份代次，避免只恢复一半。

**Tech Stack:** Go 1.26、PostgreSQL 17、MinIO/S3、Docker、Ed25519、SHA-256、POSIX shell、`net/http`、`database/sql`、`minio-go/v7`。

**Repository rule:** 不硬编码或提交任何密钥、密码、token、私钥、运行数据或备份文件；真实模型不参与本工作包。允许按用户要求进行经验证的小步提交，但不自动 push。

---

## 范围与完成定义

本计划只关闭 P0-B，不实现 Windows 客户端行为、worker heartbeat/事务幂等、生产级密钥托管、Kubernetes、云厂商 IaC 或高可用集群。测试环境可以使用 HTTP/受控局域网 URL；公网 TLS 和域名部署属于具体环境运维，不阻塞 Linux/headless 的代码闸门，但进入 P0-C 前必须记录 Windows 参考设备实际可达的服务地址。

P0-B 只有同时满足以下条件才能标记 `PASS`：

- 关闭并重新创建 Go runtime 后，原 generation session、job、card 和 version 仍可查询。
- 重新创建 runtime 后获得的制品 URL 不是 `memory://`；独立验证器通过该 URL 下载并校验 SHA-256、manifest 和 Ed25519 签名。
- PostgreSQL 或 MinIO 任一不可用时 `/readyz` 返回 `503`，而 `/healthz` 仍返回 `200`。
- 使用同一个备份代次恢复 PostgreSQL 元数据和 MinIO 制品后，上述查询、下载和验签再次通过。
- 持久化环境配置缺失或不完整时 fail closed，不会悄悄退回内存 repository。
- 仓库密钥扫描、Go 格式化、静态检查、race test、合同门禁和相关集成测试全部通过。

## Task 1：定义 readiness HTTP 边界

**Files:**
- Modify: `services/cloud/internal/httpapi/server.go`
- Modify: `services/cloud/internal/httpapi/server_test.go`
- Modify: `services/cloud/internal/httpapi/accesslog_test.go`

- [x] **Step 1：先写失败测试**

新增表驱动测试，覆盖 readiness checker 成功、返回错误和超时三种情况。断言：

- `GET /readyz` 成功返回 `200` 与 `{"status":"ok","service":"agent-card-cloud"}`。
- checker 失败或请求 context 超时时返回 `503` 与稳定的 `{"status":"unavailable","service":"agent-card-cloud"}`。
- 响应和 access log 不包含数据库 DSN、S3 endpoint、底层错误字符串或凭据。
- `POST /readyz` 返回 `405`；既有 `/healthz` 行为不变。

- [x] **Step 2：运行 RED**

```bash
cd services/cloud
go test ./internal/httpapi -run 'Ready|Health|AccessLog' -count=1
```

预期：因 `ReadinessChecker`、配置字段和 `/readyz` 路由尚不存在而失败。

- [x] **Step 3：实现最小 readiness handler**

在 `httpapi` 定义窄接口：

```go
type ReadinessChecker interface {
    Ready(context.Context) error
}
```

将其加入 `CloudHandlerConfig`。`/readyz` 使用请求 context 调用 checker，只向客户端返回稳定状态，不暴露原始错误；底层诊断只允许以结构化、安全分类记录。不要把 readiness 合并进 `/healthz`。

- [x] **Step 4：运行 GREEN**

```bash
cd services/cloud
go test ./internal/httpapi -run 'Ready|Health|AccessLog' -count=1
go test ./internal/httpapi -count=1
```

- [x] **Step 5：提交检查点**

```bash
git diff --check
git diff --stat
git add services/cloud/internal/httpapi
git commit -m "feat: add readiness endpoint contract"
```

## Task 2：实现 PostgreSQL 与 S3 组合 readiness

**Files:**
- Modify: `services/cloud/internal/publish/s3_object_store.go`
- Modify: `services/cloud/internal/publish/s3_object_store_integration_test.go`
- Modify: `services/cloud/internal/bootstrap/runtime.go`
- Modify: `services/cloud/internal/bootstrap/runtime_test.go`

- [x] **Step 1：先写失败测试**

在 bootstrap 测试中用可控 probe 证明：两个依赖均成功才 ready；任一失败都失败；探测遵循 context。S3 集成测试停止 MinIO 后必须观察到 probe 失败，恢复后再次成功。测试错误只比较类别，不把 endpoint 或凭据写入失败消息。

- [x] **Step 2：运行 RED**

```bash
cd services/cloud
go test ./internal/bootstrap -run Ready -count=1
go test ./internal/publish -run S3ObjectStoreReady -count=1
```

预期：因 S3 probe 和组合 checker 尚不存在而失败。

- [x] **Step 3：实现最小依赖探测**

- PostgreSQL 使用 `DB.PingContext`。
- `S3ObjectStore.Ready` 使用 `BucketExists` 验证服务可达且目标 bucket 存在；readiness 不自动创建 bucket。
- bootstrap 组合 checker 在一个有界 context 内并行探测数据库和对象存储，等待全部结果并返回稳定分类错误。
- `Runtime` 保存 readiness checker，并注入 `NewCloudHandler`。
- 内存开发模式使用显式的 always-ready checker；它只服务本地开发，不作为 P0-B 通过证据。

不要扩大 `publish.ObjectStore` 接口；readiness 是部署能力，不是发布领域能力。

- [x] **Step 4：运行 GREEN 与 race test**

```bash
cd services/cloud
go test ./internal/publish ./internal/bootstrap -count=1
go test ./internal/bootstrap -run Ready -race -count=1
```

- [x] **Step 5：提交检查点**

```bash
git diff --check
git add services/cloud/internal/publish/s3_object_store.go \
  services/cloud/internal/publish/s3_object_store_integration_test.go \
  services/cloud/internal/bootstrap/runtime.go \
  services/cloud/internal/bootstrap/runtime_test.go
git commit -m "feat: probe persistent runtime readiness"
```

## Task 3：禁止持久化部署静默回退到内存

**Files:**
- Modify: `services/cloud/internal/bootstrap/runtime.go`
- Modify: `services/cloud/internal/bootstrap/runtime_test.go`
- Modify: `services/cloud/.env.example`

- [x] **Step 1：先写失败测试**

为 `AGENTCARD_PERSISTENCE_REQUIRED=true` 增加测试矩阵：

- PostgreSQL 和 S3 均缺失时启动失败。
- 只配置其中一个时启动失败。
- 两者完整配置时使用生产 adapter。
- 未设置该开关且两者都为空时仍允许当前单进程内存开发模式。
- 非法布尔值启动失败，避免拼写错误降低安全等级。

- [x] **Step 2：运行 RED**

```bash
cd services/cloud
go test ./internal/bootstrap -run PersistenceRequired -count=1
```

- [x] **Step 3：实现显式 fail-closed 配置**

只新增一个布尔配置，不引入新的 mode 层次。远程测试部署必须设置 `AGENTCARD_PERSISTENCE_REQUIRED=true`。错误信息只指出缺失的配置名，不回显配置值。更新 `.env.example` 说明该开关在远程环境必须启用。

- [x] **Step 4：运行 GREEN**

```bash
cd services/cloud
go test ./internal/bootstrap -count=1
```

- [x] **Step 5：提交检查点**

```bash
git diff --check
git add services/cloud/internal/bootstrap/runtime.go \
  services/cloud/internal/bootstrap/runtime_test.go \
  services/cloud/.env.example
git commit -m "feat: require persistence for remote runtime"
```

## Task 4：证明 runtime 重建与远程签名制品闭环

**Files:**
- Modify: `services/cloud/internal/bootstrap/runtime_test.go`
- Create: `services/cloud/internal/bootstrap/artifact_verifier_test.go`

- [x] **Step 1：扩展现有 opt-in 集成测试并先确认 RED**

将 `TestProductionRuntimesSharePostgresJobsAndS3Artifacts` 拆成可复用 fixture，并覆盖完整顺序：

1. 启动 API runtime 和 worker runtime。
2. 创建、补充并确认 generation；运行 worker 到 ready。
3. 记录 session ID、job ID、card ID、version ID、artifact SHA-256 和公钥。
4. 关闭两个 runtime，使用同一环境重新创建全新 runtime。
5. 通过公开 HTTP API 重新查询 session、card 和 version，断言标识和状态未改变。
6. 获取新的签名 URL，断言 scheme 为 `http` 或 `https` 且不是 `memory://`。
7. 使用独立 `http.Client` 下载 ZIP，不复用 publisher 内存对象。
8. 独立解析制品，复算 SHA-256，并用记录的 Ed25519 公钥验证 manifest/signature。

先让测试要求“重建后查询 version 与下载并验签”；在补齐 helper 前确认测试失败或不能编译。

- [x] **Step 2：运行 RED**

```bash
cd services/cloud
go test ./internal/bootstrap -run ProductionRuntimes -count=1
```

若未提供测试环境变量，测试应明确 `SKIP`；Task 6 的门禁脚本负责提供环境并把它变为必跑。

- [x] **Step 3：实现独立验证器 helper**

验证器只接收 URL、预期 digest、公钥和 key ID。它必须：

- 禁止重定向到不同 origin。
- 设置下载超时与最大响应体大小。
- 拒绝非 `200`、非 ZIP、路径穿越、重复文件、缺失 manifest/signature。
- 对原始制品 bytes 复算 SHA-256。
- 使用合同规定的 canonical manifest bytes 验证 Ed25519。

优先复用 `artifact` 包的只读解析/规范化函数；若现有函数与 builder 耦合，只抽取最小纯函数，不复制第二套协议。

- [x] **Step 4：运行 GREEN**

```bash
cd services/cloud
go test ./internal/bootstrap -run ProductionRuntimes -count=1
```

- [x] **Step 5：提交检查点**

```bash
git diff --check
git add services/cloud/internal/bootstrap/runtime_test.go \
  services/cloud/internal/bootstrap/artifact_verifier_test.go
git commit -m "test: prove persistent signed artifact recovery"
```

## Task 5：加入依赖故障与恢复矩阵

**Files:**
- Modify: `services/cloud/internal/bootstrap/runtime_test.go`
- Modify: `docs/verification/m3-production-integration.md`

- [x] **Step 1：先写失败的 opt-in 场景**

通过测试环境提供的控制命令或 Docker 容器名依次暂停 PostgreSQL、恢复 PostgreSQL、暂停 MinIO、恢复 MinIO。每一步只从 HTTP 观察：

| 场景 | `/healthz` | `/readyz` |
|---|---:|---:|
| 两个依赖正常 | 200 | 200 |
| PostgreSQL 不可用 | 200 | 503 |
| PostgreSQL 恢复 | 200 | 200 |
| MinIO 不可用 | 200 | 503 |
| MinIO 恢复 | 200 | 200 |

测试必须有总超时和有限轮询，不使用固定长 sleep。

- [x] **Step 2：运行场景并修复实现缺口**

```bash
cd services/cloud
go test ./internal/bootstrap -run 'ProductionReadinessFailureMatrix' -count=1
```

只修复从该矩阵暴露出的 readiness 问题；不要在 P0-B 顺带实现 worker 重试或自动故障转移。

- [x] **Step 3：更新原 M3 证据边界**

在 `m3-production-integration.md` 保留历史 PASS，同时链接新的 P0-B 证据，并明确 M3 的一次运行共享测试不能替代重启/故障/恢复证明。

- [x] **Step 4：提交检查点**

```bash
git diff --check
git add services/cloud/internal/bootstrap/runtime_test.go \
  docs/verification/m3-production-integration.md
git commit -m "test: cover persistent dependency readiness"
```

## Task 6：提供可重复的 Docker 持久化门禁与成对备份恢复

**Files:**
- Create: `tooling/persistence/run-p0b-gate.sh`
- Create: `tooling/persistence/README.md`
- Modify: `.gitignore`

- [x] **Step 1：先写脚本静态测试**

在 shell 脚本开头提供 `--check` 模式，验证 Docker、`go`、`curl`、`sha256sum` 和空闲端口能力；任何依赖缺失必须非零退出。使用 `tooling/security/run-security-gate.sh` 的 shell 规范作为参考，但不复用与本任务无关的逻辑。

先运行：

```bash
sh tooling/persistence/run-p0b-gate.sh --check
```

预期：文件不存在而失败。

- [x] **Step 2：实现隔离环境生命周期**

脚本必须：

- 使用 `mktemp -d` 创建仓库外临时目录并注册 `trap` 清理。
- 在进程内生成随机数据库密码、S3 access key/secret 和 Ed25519 测试私钥；只通过环境变量传递，不打印值。
- 创建唯一 Docker network、PostgreSQL 17 容器、MinIO 容器和命名 volumes；容器只将随机端口绑定到 loopback。
- 等待真正的依赖 health check，而非固定 sleep。
- 创建 bucket 后运行 Go Postgres/S3/bootstrap 集成测试。
- 故障时保留脱敏诊断，但始终删除容器、network、volume 和临时文件。

镜像版本必须固定到仓库已验证的主版本；若固定 digest，README 记录更新方式。脚本不得使用 `latest`。

- [x] **Step 3：实现同代次备份和恢复演练**

脚本在第一次垂直测试成功后：

1. 停止应用请求写入。
2. 生成一个 backup ID。
3. 使用 `pg_dump --format=custom` 导出数据库。
4. 使用 MinIO client 将 bucket 完整镜像到同一 backup ID 目录。
5. 生成只含文件名、大小和 SHA-256 的备份 manifest，不包含凭据。
6. 删除并重建数据库 schema 和 bucket 内容。
7. 使用 `pg_restore` 和 MinIO mirror 恢复同一 backup ID。
8. 再次运行只读恢复验证：查询原 ID、下载原制品、复算 digest、验证 manifest 和 Ed25519。

数据库 dump 与对象镜像任一失败都必须判定整个备份失败，不允许继续形成“可用备份”标记。

- [x] **Step 4：运行完整门禁**

```bash
sh tooling/persistence/run-p0b-gate.sh
```

预期：输出逐阶段 PASS 摘要；不输出任何生成凭据；退出后不存在本次命名的容器、network、volume 或仓库内运行数据。

- [x] **Step 5：更新忽略规则与使用说明**

`.gitignore` 防止误提交本地 backup/evidence 临时目录。README 记录先决条件、运行命令、失败排查和“只用于测试环境”的边界，不提供固定密码示例。

- [x] **Step 6：提交检查点**

```bash
git diff --check
git add tooling/persistence .gitignore
git commit -m "test: automate P0-B persistence gate"
```

## Task 7：收口证据、路线图状态与完整验证

**Files:**
- Create: `docs/verification/p0b-persistent-test-environment.md`
- Modify: `docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md`
- Modify: `docs/superpowers/plans/2026-07-15-p0b-persistent-test-environment.md`
- Modify: `docs/verification/m4-acceptance.md`

- [x] **Step 1：执行完整自动化验证**

```bash
test -z "$(gofmt -l services/cloud)"
cd services/cloud
go vet ./...
CGO_ENABLED=1 go test ./... -race -count=1
cd ../..
sh tooling/security/run-contract-gate.sh
sh tooling/security/run-security-gate.sh
sh tooling/persistence/run-p0b-gate.sh
git diff --check
```

所有命令必须使用不含真实模型凭据的环境。若当前 shell 有真实 provider key，先为命令显式 `env -u`，但不要打印环境。

- [x] **Step 2：执行敏感信息与运行产物审计**

检查 tracked 和 untracked 文件，确认没有：

- DeepSeek/API key、数据库密码、S3 secret、Ed25519 私钥或 bearer token。
- PostgreSQL dump、MinIO 对象镜像、ZIP 制品、测试日志或 `.env`。
- `memory://` 出现在持久化集成测试的成功结果中。

只记录审计结论，不把可疑值输出到终端或文档。

- [x] **Step 3：形成可复核证据**

`docs/verification/p0b-persistent-test-environment.md` 至少记录：

- 精确 commit、日期、Go/PostgreSQL/MinIO/Docker 版本。
- 测试环境类型和网络边界，不记录 hostname 凭据或私有地址。
- runtime 重建前后的 session/job/card/version ID 一致性结论。
- 下载 URL scheme、artifact SHA-256、签名 key ID 和验签结果；不记录私钥。
- PostgreSQL/MinIO 故障矩阵结果。
- backup ID、备份 manifest digest、恢复后查询/下载/验签结果。
- Windows 远程访问继续标记为 `P0-C DEVICE NOT RUN`。

- [x] **Step 4：更新状态但不夸大结论**

仅当全部 P0-B 条件通过后：

- 将路线图 P0-B 更新为 `HEADLESS PASS` 或 `PASS`，具体取决于是否已在固定远程测试服务器验证网络可达 URL。
- 在 M4 验收文档链接 P0-B 证据，但不改变 Windows/macOS 未执行状态。
- 勾选本计划全部完成步骤。

若 Docker 本地门禁通过但固定远程服务器尚未执行，只能标记 `LOCAL HEADLESS PASS / REMOTE NOT RUN`。

- [x] **Step 5：最终自审与提交**

```bash
git status --short
git diff --check
git diff --stat HEAD
git log -n 8 --oneline
```

确认每个提交边界单一、没有无关用户改动、没有自动 push。然后提交文档证据：

```bash
git add docs/verification/p0b-persistent-test-environment.md \
  docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md \
  docs/superpowers/plans/2026-07-15-p0b-persistent-test-environment.md \
  docs/verification/m4-acceptance.md
git commit -m "docs: record P0-B persistence evidence"
```

## 建议实施顺序

严格按 Task 1 → 7 顺序执行。Task 1–3 先建立清晰的存活/就绪与配置安全边界；Task 4–5 在既有 adapter 上补足重建和故障证据；Task 6 将临时手工步骤固化为一次可重复门禁；Task 7 最后统一验证并更新里程碑。任何任务出现失败都先定位根因，不跳过测试或放宽断言。
