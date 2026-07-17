# P1-C CodeCard 生产链路验收记录

状态：**HEADLESS AUTOMATION PASS / LIVE QUALITY PASS / WINDOWS DEVICE NOT RUN**
记录日期：2026-07-17（Asia/Shanghai）
验证基线：`3f3123ecf1e0c141c94a37ec4af4b01d4f43705d`

## 结论

P1-C 的 Linux/headless 自动化范围已经闭环：固定评测合同、受控 builder、CodeCard 源码合同、本地 JavaScript SDK、真实 Chromium 离线/RPC、签名发布以及 PostgreSQL/MinIO 故障恢复均通过。

本记录不是完整 P1-C 产品验收。真实 DeepSeek CodeCard 20 例付费质量门已达到阈值，但 Windows WebView2 参考设备矩阵尚未执行，因此不得标记为完整 `P1-C PASS` 或发布就绪。

## 实施基线

| 范围 | 提交 | 结果 |
|---|---|---|
| 固定 20 例 CodeCard 评测合同 | `ae5f339` | PASS（合同） |
| 有界源码 API 与本地 JS SDK | `1a8129d` | PASS |
| 固定 builder 供应链与 CI | `35dda9f` | PASS（本地、扫描、SBOM、GHCR digest） |
| 有界真实模型质量门 | `3f3123e` | PASS（真实运行 19/20） |
| Linux Chromium 离线运行时 | `aa7cf80` | PASS |
| PostgreSQL/MinIO 发布恢复 | `005c826` | PASS |
| Windows 设备执行包 | `5a6a7fd` | PASS（静态），设备 NOT RUN |

## 质量合同

- 固定 20 个用例，自由画板、小游戏、可视化、复杂交互、纯离线工具五类各 4 个。
- 总通过门槛为 16/20，同时每类至少 3/4；不能用优势类别掩盖单类失败。
- 每例最多 3 次模型调用，全套最多 60 次、输入 300k tokens、输出 160k tokens、90 分钟。
- 只有配置可验证的模型单价且总费用不超过 5 美元时，才允许声明费用门通过。
- CI 为显式 opt-in，并要求受保护环境确认；不保存 prompt、生成源码或上游响应正文。

质量门是有界且可复核的。2026-07-16 在提交 `9a3bd64` 上通过 GitHub run `29466241445` 首次执行 `deepseek-v4-pro`：20 个案例全部耗尽三次调用且均失败，汇总为 0/20、60 次模型调用、40,259 输入 tokens、101,763 输出 tokens、1,408,390 ms。该次结果为 **LIVE QUALITY FAIL**，不是环境未执行；后续诊断和修复过程如下，所有运行均未保存 prompt、生成源码、上游正文或凭据。

第一次运行只产生过粗的 `generation_failed` 类别，无法区分源码信封解析与沙箱构建失败。提交 `cffdb60` 和 `bd89e5b` 增加了不包含正文的稳定管线与沙箱阶段分类。2026-07-17 在 `bd89e5b` 上执行 GitHub run `29546939570`：2/20 成功，56 次模型调用、37,419 输入 tokens、88,659 输出 tokens、1,283,935 ms；其余 18 个案例最终全部归类为 `sandbox-typecheck`。这证明系统性失败位于 TypeScript 严格检查层，不是模型 API、源码信封、依赖策略、Vitest、Vite 或 bundle 校验层。

提交 `a674e5c` 进一步只提取 `TS` 加四位数字的稳定诊断编号，不记录文件名、源码行或错误正文。run `29548204248` 在 `4a3bcfd` 上为 0/20，全部 20 例稳定归类为 `sandbox-typecheck-ts2307`，确认失败集中于缺失模块。模板自身的 Preact、Vitest 和本地 SDK 依赖在固定 builder 基线中可解析；根因是模型被允许生成 `src/card.test.tsx`，却常规引入模板未声明的测试库。

提交 `3f3123e` 通过 TDD 将模型输出收紧为 `src/card.tsx` 和可选 `src/card.css`，测试继续由平台固定门禁负责，没有新增依赖或放宽 TypeScript。2026-07-17 的验证 run `29549396016` 通过：19/20 成功（门槛 16/20）、25 次模型调用、11,745 输入 tokens、36,916 输出 tokens、654,385 ms；唯一失败为 `sandbox-typecheck-ts2345`。日志仍未保存 prompt、生成源码、上游正文或凭据。

## 自动化证据

| 门禁 | 结果 | 证据摘要 |
|---|---|---|
| Go 格式、vet、race | PASS | 当前提交全部 package 通过 |
| Flutter analyze/test | PASS | 无 analyzer 问题；222 tests passed，1 个预期 skip |
| CodeCard 模板 | PASS | 类型检查、5 个测试、构建和 bundle 校验通过 |
| 依赖审计 | PASS | 模板 164 packages、浏览器门 4 packages，high/critical 均为 0 |
| Linux Chromium | PASS | 3/3：离线 storage/RPC、重启恢复与 token/origin 轮换、非法 Host/凭据/关闭 session 拒绝 |
| 聚合安全门 | PASS | Flutter、Go race、跨语言合同、模板及 Chromium 门均通过 |
| 持久化发布恢复 | PASS | 真实 Docker builder、PostgreSQL、MinIO、SHA-256、manifest、Ed25519、lease 恢复和独立下载复核通过 |

本地 builder 镜像身份为 `sha256:bda75454bf5477e79d100b74bf498f2c72613c32d02b173a6f154918836101a2`。2026-07-15 的持久化复跑生成备份 `backup-20260715T142632Z`，manifest SHA-256 为 `1761d16f78c5c0a37104169f49b0d2be4a03416793ef75cd7658d52f8a499e1a`。

public GitHub 仓库的 builder run `29456945755` 在提交 `35dda9f` 上实际通过 verify 与 publish：受控候选经过 locked-down sandbox 集成门、Trivy high/critical 拒绝门（0 个漏洞）、CycloneDX SBOM 上传（artifact ID `8359724748`，ZIP SHA-256 `2a846194964b6c796b56f3b22722ef4ce539e0d58c52791f8c3d9d7dbfc7608e`），随后发布为 `ghcr.io/zhangzqs-vibecoding/agent-card-codecard-builder@sha256:1d264ede578c085dd3b3057479c52a9cbd1595060c80c47a0e52a20db38485b1`。

## 已证明的安全边界

- CodeCard bundle 通过随机 `*.localhost` origin 服务，session 重建后旧 origin/token 失效。
- 本地 SDK 只暴露版本化 context、storage 和 capability invoke，不提供任意网络包装。
- CSP 阻断外部请求；无凭据、外来 Host、已关闭 session 和越权 RPC 均 fail closed。
- builder 固定基础镜像 digest、pnpm 版本和 lockfile，安装阶段禁用 lifecycle scripts，以非 root 用户构建。
- 制品发布前验证文件目录、入口、依赖策略、SHA-256 和 Ed25519；lease 恢复不重复调用模型或创建第二版本。
- 真实模型凭据仅允许由临时环境变量注入；本轮验证显式移除了相关环境变量。

## 剩余产品闸门

1. 在 Windows 11 x64 参考设备运行 `packaging/windows/run-codecard-device-gate.ps1`，完成 WebView2、本地 JavaScript、三个 Surface、DPI/IME、离线重启、storage 和单卡崩溃隔离矩阵。

Windows 设备门禁完成前，P1-C 保持 **HEADLESS AUTOMATION PASS / LIVE QUALITY PASS / WINDOWS DEVICE NOT RUN**。
