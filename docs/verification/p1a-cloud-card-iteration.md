# P1-A 云端同卡迭代验收记录

状态：**P1-A CLOUD ITERATION HEADLESS PASS / CLIENT LIFECYCLE OPEN / WINDOWS DEVICE NOT RUN**

记录日期：2026-07-15（Asia/Shanghai）

验证代码基线：`d8e6da369cfeb2b8ca45b6dbb088320c3a8b77fc`

## 结论

云端同卡迭代已经在 Linux/headless 范围闭环：生成会话可以绑定用户拥有的 `baseCardId/baseVersionId`；worker 从 PostgreSQL/MinIO 读取不可变旧版本，重新校验 SHA-256、Ed25519、manifest、ZIP 和身份后，把受控源码作为只读上下文交给 CodingAgent；新版本复用原 `cardId`、生成新 `versionId` 并严格递增 patch display version。

真实 PostgreSQL/MinIO 故障恢复测试证明，v2 已发布但 job complete 失败后，租约恢复只完成原 reservation，不再次调用模型，不创建新 card 或重复 version。客户端能力差异确认、状态备份、升级、回滚和实例生命周期尚未实施；Windows WebView2、多窗口、悬浮、DPI、IME 与多屏继续为 `DEVICE NOT RUN`。

## 实施提交

| 范围 | 提交 | 结果 |
|---|---|---|
| 跨语言 base version 合同 | `75a5a79` | PASS |
| 基线版本归属与跨用户隐藏 | `cb9c05b` | PASS |
| PostgreSQL 基线持久化 | `05aa34f` | PASS |
| 有界读取与签名制品解析 | `3b4dead` | PASS |
| Agent 只读基线上下文 | `f8a4510` | PASS |
| 严格 display version 与唯一冲突 | `9ace119` | PASS |
| 同 card 不可变发布 | `c15c6a6` | PASS |
| PostgreSQL/MinIO 故障恢复 | `d8e6da3` | PASS |

## 已证明行为

- `baseCardId/baseVersionId` 只能成对出现，并同时进入 Go、OpenAPI、Dart 请求、session 和不可变确认快照。
- 创建迭代会话时按当前 `userId/cardId/versionId` 校验归属；不存在、错配和其他用户版本统一返回 `NOT_FOUND`。
- PostgreSQL 两列和 CHECK 约束保证重启后仍保留成对基线，直接写入单边字段会失败。
- 对象读取有显式压缩大小上限；Memory 与 S3 对缺失、超限和临时错误提供稳定分类。
- worker 在把基线交给模型前重新校验元数据 SHA-256、manifest Ed25519、key ID、card/version 身份、ZIP 路径、文件 hash/size、展开总量和 UTF-8 源码白名单。
- NativeCard 只向模型暴露 `payload/native.json`；CodeCard 只暴露 `src/card.tsx`、`src/card.css`、`src/card.test.tsx`，验证报告和构建输出不会成为源码上下文。
- 新生成的 CodeCard 会把受控源码写入签名制品的 `source/web/` 目录，避免把压缩 bundle 冒充可迭代源码；缺少源码的旧 Web 制品 fail closed。
- 迭代 prompt 使用确定性只读序列化，明确禁止改变 runtime、状态 schema、依赖和 capability 边界；缺失、超限、非法 UTF-8 或未知源码在 provider 调用前失败。
- 迭代 reservation 使用基线 `cardId` 和新 `versionId`；普通新卡仍分配新 card。错误 reservation 身份会 fail closed。
- display version 严格解析 `major.minor.patch` 并只增加 patch；非法历史拒绝，内存并发测试和 PostgreSQL 唯一约束保证同一显示版本只能一个写入成功。
- 新 manifest 确定性继承基线 runtime、`stateSchemaVersion`、capabilities、network policy、host/catalog 和尺寸边界，模型不能扩大。
- 真实 v1→v2 为同一 `cardId`、不同 `versionId`、`1.0.0→1.0.1`；两份制品均经 S3 HTTP 下载、SHA-256 与 Ed25519 复核。
- v2 发布后 complete 故障的 lease 恢复不重复模型调用，最终数据库中只有一个 card 和两个 immutable version。

## 自动化证据

| 门禁 | 结果 | 摘要 |
|---|---|---|
| Go format | PASS | `gofmt -l .` 无输出 |
| Go vet | PASS | `go vet ./...` exit 0 |
| Go race | PASS | Go 1.26.3，`CGO_ENABLED=1 go test -race ./...` 全模块通过 |
| Flutter format | PASS | 124 files，0 changed |
| Flutter analyze | PASS | No issues found |
| Flutter test | PASS | 241 tests passed，1 个预期 skip |
| 共享安全门 | PASS | Go/Dart/TypeScript 合同、Flutter 安全测试、Node 类型/测试/构建、依赖审计和 3/3 Linux Chromium 离线/RPC 场景通过 |
| CodeCard 依赖审计 | PASS | template 164 packages、browser 4 packages，high/critical 均为 0 |
| P0-B Docker gate | PASS | PostgreSQL/MinIO、migration 008、v1→v2、lease 恢复、服务重启、成对备份恢复均通过；backup `backup-20260715T161535Z`，manifest SHA-256 `f45e2ae1508808932d46384ae45b2af02d5b022b36cbe5c519f1acc8abb78ce2` |
| 仓库与运行残留审计 | PASS | 无疑似 key/私钥文件命中；无 `.db/.sqlite/.log/.pem/.key` 运行文件；无 P0-B container/network/volume 残留 |

## 未覆盖范围

- 未运行真实模型 CodeCard 20 例质量门；该门需要临时模型凭据和价格配置，属于 P1-C live quality。
- 未实现 Flutter 客户端的“基于此版本修改”入口、能力差异确认、状态备份、事务化升级和回滚。
- 未实现实例复制、删除、卸载以及“保留数据/删除数据”选择。
- 未在 Windows 参考设备验证 WebView2、三类 Surface、DPI、IME、多显示器、断网和机器重启。
- 未证明固定远程测试服务器或 Windows 到该服务器的网络可达性。

因此，本记录不能用于宣称完整 P1-A、MVP 或发行验收通过。
