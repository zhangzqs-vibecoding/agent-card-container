# P1-B Worker Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans task-by-task, superpowers:test-driven-development for every behavior change, systematic-debugging for failures, and verification-before-completion before changing milestone status.

**Goal:** 让 PostgreSQL worker 在长模型调用、并发 worker、进程重启、临时依赖故障和发布阶段部分失败下保持单任务所有权、稳定发布标识、有限重试与可恢复终态，不产生重复版本或永久卡死会话。

**Architecture:** 保留 Go 模块化单体、PostgreSQL job store 和现有同步 worker，不引入消息中间件或工作流框架。确认会话与首次 job 入队在 PostgreSQL repository 的同一事务完成；job 持久化稳定 card/version ID、下一次可运行时间和租约。worker 通过独立 heartbeat goroutine 延长租约，用同一个可取消 context 贯穿 Agent、沙箱与发布。发布恢复先查询稳定 version：若已存在则只补齐 session/job 终态；若不存在才重新生成和发布。发布、网络和 PostgreSQL 临时错误进入有上限的确定性退避，永久错误立即失败；模型 provider 临时错误继续由 Agent 在单任务三次总调用上限内处理，不叠加 job 级模型调用。

**Tech Stack:** Go 1.26、PostgreSQL 17、`database/sql`、`context`、`log/slog`、现有 generation/jobs/worker/publish 模块、Docker P0-B 持久化门禁。

**Non-goals:** 不实现多租户调度、优先级队列、Kafka、Temporal、跨区域容灾、Agent 框架迁移或 Windows UI。P1-B 不改变模型单次 generation 内最多三次调用的 P0-A 规则。

---

## 完成定义

- 确认会话和首次 job 入队要么同时提交，要么同时回滚。
- 执行时间超过初始租约时，健康 worker 持续持有 job；失去租约后立即取消下游工作且不能写终态。
- 同一 job 的所有 attempt 使用同一 card ID、version ID 和发布幂等键。
- 模型 408/429/上游 5xx 在 Agent 的三次总调用上限内处理；网络超时和临时 PostgreSQL/S3 发布错误按预定 job 退避重试，永久校验/权限/合同错误立即失败；job attempt 不超过上限。
- 进程在“上传后、版本入库后、会话 ready 后”任一点退出，重新 claim 后都能收敛到一个 version 和 completed job。
- 取消能传播到模型、沙箱、上传和 heartbeat，且 cancelled 不被迟到成功/失败覆盖。
- 并发与故障注入测试不出现重复版本、错误 worker 完成 job、永久 running/queued session 或 goroutine 泄漏。

## Task 1：扩展 job 租约与稳定发布身份

**Files:**
- Create: `services/cloud/migrations/007_job_reliability_fields.sql`
- Modify: `services/cloud/internal/jobs/store.go`
- Modify: `services/cloud/internal/jobs/store_test.go`
- Modify: `services/cloud/internal/jobs/postgres_store.go`
- Modify: `services/cloud/internal/jobs/postgres_store_integration_test.go`

- [x] 先写失败测试：`ExtendLease` 仅允许当前 owner 延长未过期租约；`ReservePublication` 第一次保存候选 card/version，后续 attempt 返回原值；`Retry` 清除 owner、设置 `available_at`，到期前不可 claim。
- [x] 运行 memory 与 PostgreSQL RED 测试，确认接口/列缺失。
- [x] migration 为 `generation_jobs` 增加 nullable `card_id`、`version_id`、`available_at`；回填 `available_at=created_at` 后设为非空，并增加 card/version 成对约束。
- [x] 扩展 `jobs.Store`：

```go
ExtendLease(ctx context.Context, jobID, worker string, now time.Time, lease time.Duration) error
ReservePublication(ctx context.Context, jobID, worker, cardID, versionID string, at time.Time) (Publication, error)
Retry(ctx context.Context, jobID, worker string, at, availableAt time.Time) error
```

- [x] PostgreSQL 更新都带 `status='running' AND lease_owner=$worker AND lease_until>$now` fencing 条件；旧 owner 返回 `ErrConflict`。
- [x] Claim 只选择 `available_at <= now` 的 queued job，仍使用 `FOR UPDATE SKIP LOCKED`。
- [x] 运行 package/race/Docker PostgreSQL 测试并提交：`feat: persist job lease and publication identity`。

## Task 2：实现租约 heartbeat 与丢失租约取消

**Files:**
- Modify: `services/cloud/internal/worker/worker.go`
- Modify: `services/cloud/internal/worker/worker_test.go`
- Create: `services/cloud/internal/worker/heartbeat.go`
- Create: `services/cloud/internal/worker/heartbeat_test.go`
- Modify: `services/cloud/internal/bootstrap/runtime.go`

- [x] 先写 fake-clock/fake-store 失败测试：长任务至少续租两次；正常结束停止 ticker；parent cancel 停止；续租冲突取消工作 context；不会在 `RunOnce` 返回后继续调用 store。
- [x] 给 worker Config 注入 `LeaseDuration`、`HeartbeatInterval` 和 ticker/clock seam；生产默认租约 90 秒、heartbeat 30 秒，校验 heartbeat 严格小于租约一半。
- [x] Claim 后立即创建 child context 并启动一个 heartbeat；Agent、sandbox、Publisher、generation 更新全部使用 child context。
- [x] heartbeat 丢失 ownership 时 cancel child context；主流程等待 heartbeat 完整退出，再决定是否允许终态写入。
- [x] context cancellation 不调用永久 `MarkFailed`；用户取消保持 cancelled，lease loss 留给新 owner 恢复。
- [x] 运行 worker race 与 goroutine 泄漏测试并提交：`feat: heartbeat generation job leases`。

## Task 3：原子确认与 job 入队

**Files:**
- Modify: `services/cloud/internal/generation/repository.go`
- Modify: `services/cloud/internal/generation/postgres_repository.go`
- Modify: `services/cloud/internal/generation/postgres_repository_integration_test.go`
- Modify: `services/cloud/internal/generation/service.go`
- Modify: `services/cloud/internal/generation/service_test.go`
- Modify: `services/cloud/internal/bootstrap/runtime.go`

- [x] 先写 PostgreSQL 故障注入测试：job insert 失败时 session 仍为 `awaiting_confirmation` 且无 frozen snapshot/queued event；成功时 queued session 与唯一 job 同时可见。
- [x] 在 generation 包定义可选的窄 `AtomicConfirmationRepository`，接收 session change、opaque job ID 与时间；不要让 generation 导入 jobs 包。
- [x] `PostgresRepository.ConfirmAndEnqueue` 复用现有 locked load/update helper，在同一 SQL transaction 内更新 session/messages/events 并插入 job。
- [x] Service 在 repository 支持该能力时只走原子路径；memory/自定义 repository 保留当前 queue adapter fallback，便于单元测试和本地开发。
- [x] 重复确认保持 `ErrConflict`；数据库重试不能产生第二个 job。
- [x] 运行 generation、bootstrap、P0-B Docker gate并提交：`feat: atomically confirm and enqueue generation`。

## Task 4：稳定发布与部分失败恢复

**Files:**
- Modify: `services/cloud/internal/publish/publisher.go`
- Modify: `services/cloud/internal/publish/publisher_test.go`
- Modify: `services/cloud/internal/worker/worker.go`
- Modify: `services/cloud/internal/worker/worker_test.go`
- Modify: `services/cloud/internal/bootstrap/runtime_test.go`

- [x] 先写失败矩阵：对象上传后失败、version insert 后失败、`MarkReady` 后 `Complete` 失败、进程退出并由另一 worker reclaim。
- [x] worker claim 后调用 `ReservePublication`，不再每 attempt 直接生成新 ID。
- [x] Publisher 增加只读 `FindVersion(user, card, version)`；若稳定 version 已存在且元数据一致，跳过 Agent/构建/上传，直接补 `MarkReady` 与 job complete。
- [x] session 为 `generating`/`validating` 时允许相同 job 的恢复执行；session 已 ready 且 version ID 一致时只补 complete；不允许回退或覆盖 cancelled/failed。
- [x] 对象仍以 archive SHA 内容寻址，version create 仍不可变；相同 stable version 但不同 artifact 必须 `ErrVersionConflict`，不能静默覆盖。
- [x] 在真实 PostgreSQL/MinIO 中逐阶段中断并确认最终只有一个 card version，提交：`feat: recover idempotent generation publication`。

## Task 5：有限重试与错误分类

**Files:**
- Create: `services/cloud/internal/worker/errors.go`
- Create: `services/cloud/internal/worker/errors_test.go`
- Modify: `services/cloud/internal/worker/worker.go`
- Modify: `services/cloud/internal/worker/worker_test.go`
- Modify: `services/cloud/internal/modelprovider/http_provider.go`
- Modify: `services/cloud/internal/publish/s3_object_store.go`

- [x] 先写表驱动分类测试：S3 429/5xx、transport timeout、PostgreSQL connection/serialization/deadlock 为 job-transient；合同/NativeCard validation、version conflict、缺少确认快照以及 Agent 已耗尽的 provider 错误为 job-permanent；context cancelled 单独处理。模型 408/429/5xx 仍由 Agent 内三次上限处理。
- [x] 定义内部 typed/category errors，只暴露稳定类别；不得解析包含凭据的错误字符串来决策。
- [x] transient failure 调用 `Retry`，退避为确定性的 1s、2s 上限，不把 session 标记 failed；耗尽 `MaxAttempts` 后才进入 failed。
- [x] permanent failure 立即 `MarkFailed + Fail`；cancelled 不重试；lease loss 不写终态。
- [x] 日志只记录 job/session/attempt/stage/errorKind/nextAttemptAt，不记录 prompt、模型输出、URL 查询、DSN 或上游正文。
- [x] 运行分类、日志泄漏和 race 测试并提交：`feat: retry transient generation failures`。

## Task 6：取消与并发故障矩阵

**Files:**
- Modify: `services/cloud/internal/worker/worker_test.go`
- Modify: `services/cloud/internal/jobs/postgres_store_integration_test.go`
- Modify: `services/cloud/internal/bootstrap/runtime_test.go`
- Modify: `tooling/persistence/run-p0b-gate.sh`

- [x] 增加 blocked provider、blocked builder、blocked object upload 三个取消测试，确认 context 到达 adapter 且无版本发布。
- [x] 增加两 worker 并发 claim、旧租约 owner 迟到 complete、heartbeat 中断后新 owner 接管测试。
- [x] 增加进程强制中断矩阵：generating、validating、object uploaded、version inserted、session ready 五个阶段。
- [x] 扩展 Docker gate 运行这些 opt-in 测试；每个场景使用有限轮询和总超时，不用固定长 sleep。
- [x] 断言数据库最终没有永久 running job、没有 ready session 对应非 completed job、没有一个 session 多个 version。
- [x] 提交：`test: prove worker crash and cancellation recovery`。

## Task 7：证据与路线图收口

**Files:**
- Create: `docs/verification/p1b-worker-reliability.md`
- Modify: `docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md`
- Modify: `docs/verification/m4-acceptance.md`
- Modify: `docs/superpowers/plans/2026-07-15-p1b-worker-reliability.md`

- [x] 运行 `gofmt`、`go vet`、全 Go race、合同门、安全门和扩展后的 P0-B Docker gate。
- [x] 执行敏感信息、goroutine、残留容器/卷/备份和 `git diff --check` 审计。
- [x] 证据记录精确 commit、attempt/退避、heartbeat、并发 owner fencing、五阶段中断和最终数据库不变量，不记录真实 prompt 或凭据。
- [x] 只有完整矩阵通过才将 P1-B 标记 `HEADLESS PASS`；Windows 消费继续 `DEVICE NOT RUN`。
- [x] 小步提交文档证据，不 push。

## 建议提交序列

1. `feat: persist job lease and publication identity`
2. `feat: heartbeat generation job leases`
3. `feat: atomically confirm and enqueue generation`
4. `feat: recover idempotent generation publication`
5. `feat: retry transient generation failures`
6. `test: prove worker crash and cancellation recovery`
7. `docs: record P1-B worker reliability evidence`
