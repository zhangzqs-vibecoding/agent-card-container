# 云端同卡迭代实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让云端生成会话可以绑定用户拥有的基线版本，并生成同一 `cardId` 的不可变新版本。

**Architecture:** `baseCardId/baseVersionId` 从 OpenAPI 进入 generation session 和不可变确认快照；Service 通过只读基线目录验证归属；worker 从发布层读取并验证基线制品后交给现有有界 CodingAgent，并复用 card identity。发布仓库负责严格 display version 递增与并发唯一性。

**Tech Stack:** Go 1.24、PostgreSQL、MinIO/S3、现有签名 ZIP artifact、Dart Cloud client、Docker persistence gate。

---

## Task 1：冻结跨语言迭代请求合同

**Files:**

- Modify: `contracts/cloud/openapi.yaml`
- Modify: `services/cloud/internal/generation/session.go`
- Modify: `services/cloud/internal/generation/session_test.go`
- Modify: `services/cloud/internal/generation/service.go`
- Modify: `services/cloud/internal/generation/service_test.go`
- Modify: `apps/desktop/lib/src/cloud/cloud_api_client.dart`
- Modify: `apps/desktop/test/cloud/cloud_api_client_test.dart`

- [x] RED：Go 测试证明 base IDs 只能成对出现，确认快照会冻结两字段，新卡保持空值。
- [x] GREEN：为 `CreateInput/CreateRequest/Session/RequirementSnapshot` 增加字段和统一 validator。
- [x] RED：Dart HTTP 测试证明普通请求不发送 base 字段，迭代请求精确发送两个字段且拒绝单边参数。
- [x] GREEN：扩展 `CloudApiClient.createGeneration`，不改变现有调用默认行为。
- [x] 更新 OpenAPI request/session/snapshot schema，跨语言安全门必须接受同一字段名。
- [x] 运行 Go generation 与 Dart cloud client 测试。
- [x] 提交：`feat: define card iteration request contract`。

## Task 2：验证基线版本归属

**Files:**

- Modify: `services/cloud/internal/generation/service.go`
- Modify: `services/cloud/internal/generation/service_test.go`
- Modify: `services/cloud/internal/bootstrap/runtime.go`
- Modify: `services/cloud/internal/bootstrap/runtime_test.go`

- [x] RED：创建迭代会话时，缺失版本、card/version 不匹配和其他用户版本统一返回 `generation.ErrNotFound`；归属正确才创建。
- [x] GREEN：引入窄 `BaseVersionCatalog.OwnsVersion` 端口并由现有 Publisher adapter 实现；普通新卡不查询目录。
- [x] RED：HTTP 测试证明未授权基线只返回 404 `NOT_FOUND`，不泄露存在性。
- [x] GREEN：bootstrap 将 publisher 作为目录注入 generation service。
- [x] 运行 generation/httpapi/bootstrap 测试。
- [x] 提交：`feat: authorize card iteration baselines`。

## Task 3：PostgreSQL 持久化基线

**Files:**

- Create: `services/cloud/migrations/008_generation_base_version.sql`
- Modify: `services/cloud/internal/generation/postgres_repository.go`
- Modify: `services/cloud/internal/generation/postgres_repository_test.go`
- Modify: `services/cloud/internal/generation/postgres_repository_integration_test.go`

- [x] RED：repository round-trip 测试证明 draft 与 confirmed snapshot 重启后仍保留 base IDs。
- [x] GREEN：migration 增加 nullable 两列及成对 CHECK；insert/select/scan 全部接线。
- [x] RED：数据库直接写入单边 base 字段必须被约束拒绝。
- [x] 运行 migration、repository 单元与 PostgreSQL 集成测试。
- [x] 提交：`feat: persist generation base versions`。

## Task 4：读取并验证上一版制品

**Files:**

- Modify: `services/cloud/internal/publish/publisher.go`
- Modify: `services/cloud/internal/publish/s3_object_store.go`
- Modify: `services/cloud/internal/publish/s3_object_store_integration_test.go`
- Modify: `services/cloud/internal/publish/publisher_test.go`
- Create: `services/cloud/internal/agent/base_artifact.go`
- Create: `services/cloud/internal/agent/base_artifact_test.go`

- [x] RED：对象存储读取缺失、hash 不符、签名不符、ZIP 非法、文件目录越界或总量超限时失败。
- [x] GREEN：为对象存储增加有界 `Get`；Memory/S3 实现严格大小上限和错误分类。
- [x] GREEN：基线解析器复用 artifact canonical/signature 规则，只输出 CardDefinition 和允许的 UTF-8 payload/source 文件。
- [x] RED：不同 card/version manifest 与请求身份不一致时拒绝。
- [x] 运行 publish/agent 测试和 MinIO 集成测试。
- [x] 提交：`feat: load verified card iteration baselines`。

## Task 5：把基线交给有界 Agent

**Files:**

- Modify: `services/cloud/internal/agent/agent.go`
- Modify: `services/cloud/internal/agent/agent_test.go`
- Modify: `services/cloud/internal/agent/native_prompt.go`
- Modify: `services/cloud/internal/agent/native_prompt_test.go`

- [ ] RED：迭代 prompt 必须包含稳定的基线 definition/source 和增量需求，并明确禁止改变身份、依赖和能力边界；新卡 prompt 不包含基线段。
- [ ] GREEN：`agent.Request` 增加可选只读 `BaseArtifact`，Native/CodeCard prompt 复用同一有界序列化器。
- [ ] RED：基线超限、非法 UTF-8 或未知文件在调用 provider 前失败。
- [ ] 运行 agent 全套和固定 eval 合同测试。
- [ ] 提交：`feat: ground coding agent on prior card version`。

## Task 6：同 card 发布和 display version

**Files:**

- Modify: `services/cloud/internal/worker/worker.go`
- Modify: `services/cloud/internal/worker/worker_test.go`
- Modify: `services/cloud/internal/jobs/store.go`
- Modify: `services/cloud/internal/jobs/postgres_store.go`
- Modify: `services/cloud/migrations/008_generation_base_version.sql`
- Modify: `services/cloud/internal/publish/postgres_repository.go`

- [ ] RED：基于 v1 的迭代 reservation 使用原 `cardId`、新 `versionId`；普通新卡仍分配新 card。
- [ ] GREEN：worker 在 reservation 前解析快照身份；job reservation 幂等保持第一次选择。
- [ ] RED：历史 `1.0.0/1.0.1` 生成 `1.0.2`，非法 display version fail closed；并发重复编号由唯一约束拒绝并稳定分类。
- [ ] GREEN：增加严格 semver patch 计算和 `(user_id, card_id, display_version)` 唯一约束。
- [ ] RED：基线 runtime/schema/capabilities 被确定性继承，模型不能扩大能力或改变 schema。
- [ ] 运行 worker race、jobs 和 publish PostgreSQL 测试。
- [ ] 提交：`feat: publish immutable card iterations`。

## Task 7：真实 PostgreSQL/MinIO 恢复闭环

**Files:**

- Modify: `services/cloud/internal/worker/postgres_recovery_integration_test.go`
- Modify: `tooling/persistence/run-p0b-gate.sh`

- [ ] RED：真实 v1→v2 中任一 publish/complete 故障不能产生新 card、重复 version 或重复模型调用。
- [ ] GREEN：在现有持久化门中发布 v1，再以它为基线发布 v2；独立下载并验签两份制品。
- [ ] 断言同 `cardId`、不同 `versionId`、display version 递增，lease 恢复只完成预留版本。
- [ ] 运行完整 P0-B Docker gate 并确认无容器/volume 残留。
- [ ] 提交：`test: prove persistent card iteration recovery`。

## Task 8：验收与文档

**Files:**

- Create: `docs/verification/p1a-cloud-card-iteration.md`
- Modify: `docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md`
- Modify: `docs/verification/m4-acceptance.md`
- Modify: `docs/superpowers/plans/2026-07-15-cloud-card-iteration.md`

- [ ] 运行 Go format/vet/race、Flutter analyze/test、共享安全门和 P0-B 持久化门。
- [ ] 执行敏感信息、运行数据、构建、容器和 volume 残留审计。
- [ ] 记录 `P1-A CLOUD ITERATION HEADLESS PASS`；客户端升级/回滚和 Windows 设备仍明确开放。
- [ ] 提交：`docs: record cloud card iteration evidence`，不 push。
