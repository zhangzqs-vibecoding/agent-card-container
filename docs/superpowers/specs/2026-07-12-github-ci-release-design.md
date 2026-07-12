# GitHub CI and Portable Release Design

日期：2026-07-12  
状态：待实施  
目标仓库：`zhangzqs-vibecoding/agent-card-container`  
可见性：Public  
默认分支：`develop`

## 1. 目标

在不泄露任何密钥、token、私钥或本地运行数据的前提下，将当前
Git 仓库推送到 GitHub public 组织仓库，建立 Linux、Windows 和
macOS 的可重复 CI，并产出用户可下载、解压和启动的 Windows x64
便携 ZIP。

## 2. 发布通道

### 2.1 Develop snapshot

`develop` push 或手动运行成功后，Windows job 上传名为
`AgentCardContainer-windows-x64-<short-sha>.zip` 的 Actions Artifact。ZIP
根目录直接包含可执行文件、Flutter runtime、插件 DLL、`sqlite3.dll`、
`libsodium.dll` 和一份 `README.txt`，不要求用户二次复制依赖。

Snapshot 可未签名，但必须通过结构、DLL allowlist、敏感文件、版本和
SHA-256 检查。文档必须说明未签名产物可能触发 Windows SmartScreen。

### 2.2 Tagged release

`vMAJOR.MINOR.PATCH` tag 触发同一构建链，创建 GitHub Release，附件包含
Windows 便携 ZIP 和 `SHA256SUMS.txt`。Release 只使用该 tag 对应的完整
commit，不从其他 workflow run 复用未绑定来源的二进制。

未配置签名 secrets 时，Release 必须标记为 prerelease 和 unsigned。配置后，
workflow 对 EXE 和最终 ZIP 内二进制执行 Authenticode 检查；签名失败不得
降级为“已签名”发布。

## 3. CI 架构

### 3.1 Shared quality gate

Ubuntu job 执行：

- Go format、`go vet ./...` 和 `CGO_ENABLED=1 go test ./... -race`。
- Flutter analyze 和完整 unit/widget suite。
- Go、Dart、TypeScript 共享 contract fixture 校验。
- CodeCard 依赖 allowlist、typecheck、test、build、bundle policy 和 audit。
- 凭据、私钥、`.env`、数据库、运行日志和卡片状态扫描。

### 3.2 Platform jobs

- Linux：analyze、test 和 ordinary-window debug build；Wayland overlay 保持
  out of scope。
- Windows：analyze、test、release build，使用 pinned vcpkg commit 构建 x64
  SQLite/libsodium，然后组装并检查便携 ZIP。
- macOS：analyze、test 和 release build；没有 Apple 证书时只作编译门禁，
  不声称 notarized。

所有第三方 Actions 必须锁定完整 commit SHA。workflow 设置最小
permissions、timeout、concurrency cancellation 和有限 artifact retention。

## 4. Windows 便携包约束

便携 ZIP 需满足：

1. 只包含 `flutter build windows --release` 输出、已批准的两个原生 DLL 和
   启动说明。
2. 不包含 `.env`、SQLite 运行库、日志、诊断包、卡片制品、API key、
   signing key 或 GitHub token。
3. 解压后双击 `agent_card_desktop.exe` 即可启动。已安装 Evergreen
   WebView2 时 CodeCard 可用；缺失时应显示明确的官方安装引导，不静默
   下载未验证程序。
4. CI 对解压后 bundle 再次执行 verifier，防止“构建目录正确、ZIP
   内容错误”。

## 5. Secrets 与信任边界

- Public repository 不存储任何真实 secret，包括历史 commit、artifact 和 log。
- DeepSeek live test 仅在手动 workflow 且明确启用时运行，从 GitHub
  Actions Secret 读取 API key；PR、fork PR 和默认 push 不获取该 secret。
- 签名证书、密码和 timestamp 配置只来自 environment-protected secrets。
- Artifact 上传前再执行敏感文件和凭据扫描；命中即失败。

## 6. GitHub 仓库初始化

1. 在 `zhangzqs-vibecoding` 组织下创建 public
   `agent-card-container`，不自动创建 README、license 或 `.gitignore`，避免历史分叉。
2. 配置 `origin`，推送本地 `develop`，并将远程默认分支设为
   `develop`。
3. 启用 Actions，检查 workflow 是否被 GitHub 正确解析。
4. 只在 CI 和 Windows ZIP 都实际成功后才声称交付完成。

## 7. 验收标准

- GitHub public 仓库可访问，`develop` 与本地提交一致。
- 三个平台 job 与 shared quality gate 完成，失败不被忽略。
- Windows Artifact 实际可从 GitHub 下载，ZIP 通过内容、哈希、依赖和
  敏感信息检查。
- tag workflow 可创建带 ZIP 和 checksum 的 Release，未签名时不伪装成
  signed/stable release。
- DeepSeek 真实测试的 secret 不出现在 Git、workflow YAML、命令输出或
  artifact 中。

## 8. 明确不在本次范围

- 购买或申请 Windows/Apple 代码签名证书。
- 用 Linux CI 结果替代 Windows WebView2/Win32 或 macOS notarization 实机证据。
- 将云端服务凭据打包到桌面便携 ZIP。
