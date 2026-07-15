# 同卡版本生命周期设计

状态：**Linux/headless 已实施并通过；Windows device 未执行**
记录日期：2026-07-15（Asia/Shanghai）  
上位需求：`docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md` P1-A

## 1. 目标

用户可以从一张自己拥有的已发布卡片发起修改，让 Agent 在上一版事实基础上生成同一 `cardId` 的新 `versionId`；客户端安装前展示能力差异和状态 schema 兼容性，并能以可恢复方式升级或回滚现有实例。

MVP 不执行 Agent 生成的状态迁移。`stateSchemaVersion` 相同才复用状态；不同时先备份旧状态，用户只能保留旧版，或安装新版并重置状态。

## 2. 现状

- 云端已有不可变 `card_versions`、按卡查询历史、稳定发布 reservation 和签名制品。
- generation 创建请求没有 `baseCardId/baseVersionId`，worker 每次都生成新 `cardId`，display version 固定为 `1.0.0`。
- Agent 只读取确认需求，没有读取上一版 manifest 或源码。
- 客户端能安装任意历史版本，但总是创建新实例；SQLite 虽有 `switchInstanceVersion`，没有事务化状态/授权处理和 UI。
- `CardDefinition.stateSchemaVersion` 已是跨 Go/Dart/JSON Schema 的合同事实。

## 3. 方案比较

### 3.1 服务端绑定基线、客户端事务化切换——采用

generation session 记录可选且成对出现的 `baseCardId/baseVersionId`。服务端在创建时验证版本归属，确认快照冻结基线；worker 从受信发布仓库读取并验证上一版签名制品，复用 `cardId`，生成新 `versionId` 和递增 patch display version。客户端把已验证的新安装与实例切换、状态备份和权限替换放入一个 SQLite 事务。

优点是身份、安全边界和失败恢复明确，模型不能伪造基线。缺点是需要为发布仓库补只读制品能力。

### 3.2 客户端把旧源码拼进 prompt——不采用

实现快，但客户端内容可能被篡改、请求膨胀，云端无法证明 base version 归属，也会形成第二套合同事实。

### 3.3 每次仍创建新卡，由客户端做别名——不采用

无法形成真实版本历史，回滚、能力差异和幂等发布都不可信，与路线图的稳定 `cardId` 要求冲突。

## 4. 云端协议与归属

`POST /v1/generations` 增加可选 `baseCardId` 和 `baseVersionId`：

- 两者必须同时为空或同时为非空。
- 非空时，发布仓库必须能按当前 `userId/cardId/versionId` 找到该版本，否则统一返回 `NOT_FOUND`，不泄露其他用户卡片存在性。
- session 和 `RequirementSnapshot` 同时保存两字段；确认后不可修改。
- PostgreSQL migration 增加 nullable 两列，并以 CHECK 约束保证成对出现；内存与 PostgreSQL repository 使用同一验证规则。
- Dart `GenerationPort`/`CloudApiClient` 使用同一字段名；普通新卡请求保持兼容。

## 5. 基线制品与 Agent 输入

发布层增加只读接口，按已授权版本读取不可变 artifact bytes。worker 使用现有 artifact 验证器重新检查 SHA-256、manifest 和 Ed25519，再提取：

- 完整 `CardDefinition`；
- NativeCard 的 `payload/native.json`，或 CodeCard 允许文件目录中的 UTF-8 源文件；
- 上一版 runtime、capabilities、network policy 和 `stateSchemaVersion`。

提取结果设置严格总字节和文件数上限，不把签名、验证报告或任意 ZIP 路径交给模型。Agent prompt 明确“修改现有卡片”，包含上一版只读源码和本次增量需求；模型仍只能输出既有受控文件合同，不能改变 `cardId/versionId` 或扩大确认快照的能力候选集。

## 6. 版本身份与编号

- 新卡继续由 reservation 分配新 `cardId/versionId`，display version 为 `1.0.0`。
- 迭代任务的 reservation 必须使用快照中的 `baseCardId`，只生成新 `versionId`。
- worker 查询同卡全部版本，解析严格 `major.minor.patch`，取最高值并增加 patch；非法历史编号视为服务端数据错误，不猜测。
- 并发迭代可能竞争同一 display version，因此 PostgreSQL 对 `(user_id, card_id, display_version)` 建唯一约束；冲突作为可重试发布竞争，重新计算一次，仍冲突则稳定失败。
- 发布的新 manifest `stateSchemaVersion` 默认继承上一版。模型无权自行改变；未来需要 schema 变化时由确定性生成策略显式设置并测试。

## 7. 客户端升级与回滚

新增 `CardVersionLifecycle`，输入为现有 `instanceId` 和已下载验签的目标安装：

1. 验证目标 `cardId` 与实例相同，目标 version 已安装且签名有效。
2. 计算 capability/domain 差异；新增能力必须先经现有 permission UX 明确授权，拒绝则不改实例。
3. 相同 `stateSchemaVersion`：事务内切换 `versionId`，保留 `stateNamespace` 和状态；只保留仍被目标声明覆盖的 grants。
4. 不同 schema：先把旧状态写入版本备份表。用户选择“继续旧版”则不改变任何数据；选择“安装新版并重置”则事务内切换版本、创建新 state namespace、清理旧 grants，旧状态备份保留用于回滚。
5. 回滚走同一入口。若目标 schema 与当前相同则复用当前状态；不同则备份当前状态，并优先恢复该实例/目标版本最近的可信备份，否则重置。
6. 任一步失败时 SQLite 事务回滚，内存 `WorkspaceController` 仍保留旧卡；事务成功后才用 `InstalledWorkspaceCardFactory` 替换内存 card，使 runtime 重建一次。

## 8. 状态备份

SQLite schema v5 增加 `card_state_backups`：

- `backup_id`、`instance_id`、`card_id`、`version_id`、`state_schema_version`；
- 原 `state_namespace`、完整有界 JSON snapshot、`created_at`；
- 不对已删除 instance 建外键，以便后续卸载“保留数据”复用；
- 单 snapshot 最大 1 MiB，每个 card 最多保留最近 10 个，事务提交前删除更旧记录。

备份只包含本地状态，不包含 capability grants、session token、访问令牌或制品源码。

## 9. UI

- 卡片菜单新增“基于此版本修改”，打开 Agent Studio 并把 base IDs 绑定到 controller；确认区明确显示基线版本。
- 版本历史对已存在的同卡实例提供“升级到此版本”或“回滚到此版本”，不再把这两个动作等同于创建新实例。
- 确认对话框展示新增/移除 capability、network domain 差异和 state schema 结论。
- schema 不兼容时只提供“继续使用当前版本”和“安装并重置”；不得出现自动迁移措辞。
- 成功后显示当前 display version；失败只显示稳定错误，旧版继续可用。

## 10. 测试与验收

- Go：成对字段验证、跨用户隐藏、确认快照、PostgreSQL round-trip、基线制品验证、Agent prompt、同 card 发布、display version、并发冲突和 lease 恢复。
- 跨语言合同：OpenAPI/Go/Dart 对新增字段一致。
- Flutter：SQLite v5 migration、备份上限、兼容切换、不兼容取消/重置、grant 收缩、失败回滚和 runtime 单次重建。
- Widget：从实例发起修改、差异确认、升级、回滚和稳定错误。
- Docker：PostgreSQL/MinIO 中真实发布 v1→v2，两个版本同 `cardId`、不同 `versionId`，重启后可下载并独立验签。

Linux/headless 全部通过后只能标记 `P1-A VERSION LIFECYCLE HEADLESS PASS`。Windows 三 Surface 状态保持和真实交互继续为 `DEVICE NOT RUN`。

## 11. 非目标

- Agent 生成或执行任意状态迁移代码。
- 分支、合并、多人协作、语义化版本由模型决定。
- 跨 card 导入状态或 capability 自动批准。
- 删除历史云版本或重写不可变制品。
