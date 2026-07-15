# 工作区布局编辑设计

状态：**已确认，待实施**  
记录日期：2026-07-15（Asia/Shanghai）  
上位需求：`docs/superpowers/specs/2026-07-13-mvp-closure-roadmap-design.md` P1-A

## 1. 目标

让用户在 Flutter 主工作区中直接拖动和缩放卡片，所有结果吸附到现有 12 列网格，确定性地避让碰撞，并在 300ms 防抖后写入 SQLite。写入失败时必须恢复到最后一次已持久化布局，并显示可理解的非敏感错误。

本设计只关闭 P1-A 的“拖动、缩放、吸附网格、碰撞处理、可恢复自动落位和布局持久化”范围。复制、删除、卸载以及同卡升级/回滚分别进入后续小计划，避免把 UI 手势、数据删除和云端协议混在一个不可独立验证的改动中。

## 2. 现状与约束

- `CardPlacement` 已是 Surface 与 SQLite 的共同布局类型；主工作区用 `x/y/width/height` 表示网格单元。
- `LocalDatabase.moveInstance` 已能更新 surface 与 placement，但当前没有错误反馈、事务结果检查或调用它的工作区编辑路径。
- `_WorkspaceCanvas` 已按 12 列和 80px 行高渲染，但卡片没有拖动或缩放手势。
- Surface adapter 只能存在于 adapter/协调层；纯布局决策不能依赖平台插件。
- NativeCard 与 CodeCard 的运行时状态不得因布局编辑而重建或丢失。
- Linux/headless widget test 是本阶段事实证据；Windows DPI、多屏和原生窗口行为仍由设备闸门证明。

## 3. 方案比较

### 3.1 纯网格布局引擎加受控 UI 手势——采用

新增无 Flutter 平台依赖的布局引擎，负责 snap、clamp、碰撞检测和 first-fit 自动落位；`WorkspaceController` 负责内存状态、防抖写入和失败回滚；Widget 只把像素 delta 转成网格候选值。

优点是每层单一职责、算法可用普通 Dart 单测证明，拖动过程中不触碰 SQLite。缺点是首版只支持矩形网格和确定性向下避让，不提供自由像素布局。

### 3.2 直接在 Widget 中计算和写库——不采用

实现文件少，但布局规则会和渲染尺寸耦合，手势每帧可能写库，失败回滚也难以独立测试。

### 3.3 引入第三方可拖拽 Dashboard 包——不采用

能快速获得交互，但会引入新的布局模型和依赖供应链，并可能破坏现有 Surface、runtime budget 与 card state 生命周期，不符合当前 KISS/YAGNI 边界。

## 4. 布局模型

### 4.1 固定规则

- 网格固定 12 列，横向坐标和宽度为整数网格单位。
- 最小尺寸为 2 列 × 2 行；最大宽度为 12 列；高度至少 2 行，不预设产品级最大高度。
- `x` 限制在 `0..12-width`，`y` 不小于 0。
- 输入候选值使用四舍五入吸附；所有输出均为有限整数值。NaN、Infinity 或非正尺寸直接拒绝。
- 两个矩形边界接触不算碰撞，面积相交才算碰撞。

### 4.2 碰撞处理

用户拖动或缩放产生候选 placement 后，布局引擎先 snap/clamp，再忽略当前实例检测其他工作区卡片。如果碰撞，从候选 `y` 开始逐行向下寻找第一个无碰撞位置，`x/width/height` 保持不变。

新实例自动落位复用同一 first-fit 规则，从 `(0, 0)` 开始按行、再按列扫描。算法设置与实例数量相关的有限搜索上限；达到上限属于内部错误，不返回部分布局。

该策略稳定、可预测且不会级联移动其他卡片。首版不实现推挤整列、交换位置或自由重叠。

## 5. 状态与持久化

`WorkspaceController` 保存每个实例最后一次成功持久化的 placement：

1. 手势更新时，控制器立即发布已解析的内存 placement，使 UI 跟手。
2. 同一实例的连续更新合并为一次 300ms 防抖写入。
3. 写入成功后，新 placement 成为回滚基线，并清除该实例错误。
4. 写入失败后，控制器恢复该实例最后成功 placement，通知 UI，并暴露稳定中文错误“布局保存失败，已恢复上次位置”。
5. 新更新到来时，旧计时器取消；控制器 dispose 时取消未开始的写入，不在关闭后通知监听器。

持久化端口只接收完整的 `instanceId/surfaceId/placement`，生产实现调用 `LocalDatabase.moveInstance`。控制器不直接依赖 SQLite，单测使用可控端口验证延时、合并和失败回滚。

同一实例在写入进行中又发生编辑时，不允许旧写入成功覆盖更新的内存状态：每次编辑带单调 revision，只有当前 revision 的结果能更新基线或触发回滚。

## 6. Flutter 交互

- 卡片标题区作为拖动把手，鼠标和触控拖动均通过 `GestureDetector` 进入同一逻辑。
- 右下角提供带 tooltip 和稳定 key 的缩放把手。
- 手势开始时记录 placement；每次 update 用 canvas 的列宽和 80px 行高换算累计 delta，再交给布局引擎。
- 手势只操作主工作区实例，不改变 `versionId`、`stateNamespace`、卡片状态或 capability grants。
- 编辑中的卡片提升视觉层级；错误通过工作区顶部的可关闭提示显示，不将数据库异常正文呈现给用户。
- 空状态“生成卡片”按钮接通现有 Agent Studio 展开动作，消除当前空回调。

## 7. 错误与安全边界

- 未知实例、非法 placement 和无法解析的布局是编程/数据错误，操作被拒绝且不写库。
- 用户可见错误只使用稳定文案；原始 SQLite/文件异常不进入 UI。
- 布局写入不修改卡片制品、签名、权限或 state namespace。
- UI 不执行生成代码，不新增平台插件和网络访问。

## 8. 测试与验收

### 8.1 纯 Dart/Flutter 单测

- snap、12 列 clamp、最小尺寸、非法数值拒绝。
- 边界接触、面积碰撞、忽略当前实例和确定性向下 first-fit。
- controller 立即更新、300ms 合并写入、成功基线、失败回滚、revision 防旧结果覆盖及 dispose 取消。
- SQLite production port 写入后重开数据库仍恢复 placement。

### 8.2 Widget test

- 拖动标题区后卡片吸附并只调用一次持久化。
- 缩放把手遵守最小尺寸和碰撞规则。
- 写入失败后位置恢复并显示稳定错误。
- 拖动/缩放不重建卡片 state namespace；空状态按钮打开 Agent Studio。

### 8.3 阶段结论

Go 不受本小计划影响。完成后运行 `flutter analyze`、完整 `flutter test`、共享安全门和 `git diff --check`。这些证据只能标记为 `P1-A WORKSPACE HEADLESS PASS`；Windows 实际鼠标、触控、DPI 和多屏体验保持 `DEVICE NOT RUN`。

## 9. 非目标

- 多选、框选、撤销栈、键盘微调、自由旋转或层级叠放。
- 工作区之间的协同布局或云同步。
- detached/overlay 原生窗口的实时拖拽实现。
- 复制、删除、卸载和数据保留选择。
- 卡片版本生成、升级、回滚或状态 schema 迁移。
