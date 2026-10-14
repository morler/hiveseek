# Whale 自构建与自改进开发计划

> 状态：开发方案草案，尚未实现，也不是发布承诺。
> 基线：Whale `676a0a2`；参考项目 `../jcode`，版本 `2f7039193`。
> 范围：技能改进、自身源码改进、候选构建与验证、受控激活与会话接续。
> 本次交付仅为计划文档，不开放隐藏功能，不改运行时代码，不运行候选程序。

## 1. 目标与交付边界
本计划的核心判断是：先把“能提出改动”和“能安全替换自己”分开，默认只交付可审查、可回退的能力。

### 1.1 产品目标

让 Whale 在用户授权下形成可审查的闭环：

```text
实际任务中的失败／用户纠正
  → 保存最少必要证据
  → 生成技能提案或隔离源码改动
  → 人工审查＋确定性验证
  → 应用技能或构建不可变候选
  → 人工确认激活
  → 就绪验证／失败回滚
  → 接续原会话并记录结果
```

“自改进”指能提出、验证和追踪改动，不等于模型证明自己变好了。“自构建”指有受控的源码、构建、验证和产物管理链路，不是给 shell 增加一条 `go build` 提示词。

### 1.2 分层交付

| 层级 | 用户获得的能力 | 激活规则 |
| --- | --- | --- |
| L1 技能改进 | 证据、SKILL.md 提案、差异审查、人工应用与恢复 | 用户明确选择启用；每次应用确认 |
| L2 自构建 | 隔离改动、固定测试门、候选二进制与验证报告 | 不切换运行版本 |
| L3 自更新 | Unix 开发安装下的受监督激活、失败回滚、会话恢复 | 用户逐次确认；仅已验证候选 |
| L4 有界自主改进 | 用户启动的一次改进任务，自动执行至提案／候选报告 | 不自动合并、推送或激活 |

每层可以独立停止。L1 不依赖 L2；L3 依赖 L2；L4 必须在 L1/L2 安全与测量链路完成后再做。

### 1.3 不做的事

- 不复制 jcode 的共享 daemon、ambient 调度、overnight 永久在线循环或服务器多通道部署。
- 不新增模型供应商、自动训练模型、跨项目通用记忆库或另一套 workflow 调度器。
- 不自动修改用户项目、全局配置、权限策略、hooks、API key 或会话存储格式。
- 不替换 npm／包管理器拥有的可执行文件，不在 Windows 上承诺原地自动换版或透明重连 ACP。
- 不把“编译成功”“版本字符串匹配”或模型自评分作为质量提升证明。

## 2. 仓库现状与复用清单

以下是代码核实结果；“已有实现”不代表它当前已向用户开放。

| 能力 | 已核实入口 | 计划中的用法／缺口 |
| --- | --- | --- |
| Skill 发现与解析 | `internal/skills/skills.go`：`DefaultRoots`、`ParseContent`、`Validate` | 复用格式与发现规则，不新增 skill 格式 |
| Skill 改进后端 | `internal/plugins/skillsimprover/plugin.go`、`internal/plugins/skillsimprover/store.go` | 已有 hooks、`save_skill_proposal`、`SaveProposal`、`ApplyProposal`；复用而非重写 |
| 当前隐藏策略 | `internal/app/plugins_test.go`：`TestSkillsImproverIsHiddenFromSkillReportAndTools`；`internal/app/commands_test.go` | 明确要求工具、技能、命令及证据写入不出现；默认行为必须保留 |
| 插件装配与 hooks | `internal/plugins/plugin.go`、`internal/app/app_tools_init.go` | 复用插件 Outcome 和 HookRunner；开放入口必须显式 opt-in |
| 技能 UI 与加载 | `internal/app/commands_skills.go`、`internal/app/commands_skill_config.go`、`internal/tui/skills.go`、`internal/tools` | 应用后更新发现结果；验证 disabled 与覆盖优先级不变 |
| Workflow | `internal/workflow/tool.go`、`internal/workflow/library.go` | 复用人工确认与结构化结果模式；不把任意 workflow 脚本变成可信测试门 |
| Git worktree | `internal/worktree/worktree.go`、`internal/ui/cli/cmd/root.go` | 复用隔离路径／命名；worktree 不是进程安全沙箱 |
| 开发命令与版本 | `cmd/dev/main.go`、`Makefile`、`internal/build/version.go` | 构建目标是 `./cmd/whale`；`CurrentVersion` 已读 Go VCS 信息，需补独立机器可读身份 |
| 会话存储与恢复 | `internal/session`、`internal/store`、`internal/app/resume.go`、`internal/ui/cli/cmd/commands.go` | 复用 `whale resume [id]`；新增激活日志，不迁移现有会话格式 |
| TUI 与服务入口 | `internal/ui/cli/cmd/root.go`、`internal/app/appserver/server.go`、`internal/app/service`、`internal/ui/tuiadapter` | 已有进程内／stdio app-server 和 ready 消息；不能据此假定已有跨版本监督进程 |
| 执行边界 | `internal/execboundary/wrapper_unix.go`、`internal/execboundary/wrapper_other.go` | Unix wrapper 使用 `syscall.Exec`，用途是命令执行边界，不是更新管理器 |
| 离线验证 | `internal/evals`、`cmd/dev/main.go` | `make test` 为 `go test ./...`，已包括 evals；需加受信任的候选二进制行为检查 |
| 项目记忆 | `internal/memory/project_memory.go`：`ReadProjectMemory` | 项目记忆入口是读取；本计划不增加通用记忆写工具 |
| 私有存储 | `internal/securefs/private.go` 及平台实现 | 复用权限处理；`WritePrivateFile` 直接写文件，不提供原子提交保证 |

### 2.1 对前期可行性分析的修正

1. 不是“完全没有 skill 写入实现”。已有 `ApplyProposal`，但当前没有开放的产品入口，隐藏状态受到测试保护。
2. 不需要先造 `skill_save`。第一阶段应复用并加固现有提案后端，只支持修改现有项目技能；新建技能仍可用普通文件工具经用户审查完成。
3. `make test` 与 `make test-evals` 不能当作两个独立质量门重复执行；后者只用于开发时快速定位 eval 问题。
4. 单进程 `syscall.Exec` 替换后，旧进程已不存在，不能在新版本立即崩溃时自行回滚。L3 必须有独立、不会被候选替换的监督者。
5. “Go 比 Rust 容易构建”不等于“热更新容易”。会话落盘、终端所有权、子进程清理和回滚才是主要工作量。

## 3. 借鉴 jcode 的边界

| 参考机制 | 参考源码 | Whale 决策 |
| --- | --- | --- |
| 构建队列与相同来源去重 | `../jcode/crates/jcode-app-core/src/tool/selfdev/build_queue.rs` | 首版单安装一次只允许一个构建；按候选身份识别已有产物，不做通用任务队列 |
| 源码状态与漂移检查 | `../jcode/crates/jcode-build-support/src/source_state.rs` | 首版只接收用户指定的干净 commit；构建与验证使用固定快照 |
| 不可变版本与发布验证 | `../jcode/crates/jcode-build-support/src/lib.rs`：`publish_local_current_build_for_source` | 复用思想；候选验证不更新 `current`，只在独立激活事务中更新 |
| 待激活记录、ack／ready、失败回滚 | `../jcode/crates/jcode-app-core/src/tool/selfdev/reload.rs` | 为单机 CLI 增加稳定监督者，不移植共享 server 信号机制 |
| 会话接续上下文 | `../jcode/crates/jcode-app-core/src/tool/selfdev/reload.rs` 与 `../jcode/tests/test_selfdev_reload.py` | 持久化激活 ID 与接续事件；禁止重复执行工具或自动发送用户 prompt |

已检查的 jcode 构建／发布路径包含源码身份与二进制版本冒烟检查；测试动作是独立入口。不能把这一观察扩大为“jcode 没有测试”，但 Whale 的激活资格必须额外强制绑定完整测试与回归报告，不能仅凭编译和版本冒烟。

## 4. 默认设计决定
默认设计优先保证失控时不扩大影响，因此宁可停在候选报告，也不让 agent 直接操作版本指针或激活状态。

### 4.1 最小架构

```text
用户确认／普通 agent + 现有 worktree、文件与 shell 工具
        │
        ├─ 技能提案 → 现有 skillsimprover Store → 人工应用／恢复
        │
        └─ 源码改动 → 干净候选 commit
                           │
                    受信任 selfdev 控制器
                           │
                  隔离构建＋固定验证门
                           │
                  候选目录＋验证报告
                           │
                       人工确认
                           │
                稳定监督者 → 候选 TUI 子进程
                           │
                      ready 或回滚
```

新增一个聚焦的 `internal/selfdev` 包管理候选、报告和激活状态，不增加新的通用插件框架。CLI/app 做适配，agent 不直接操作版本指针或 ready 状态。

### 4.2 授权与信任

- 全部默认关闭。项目 `.whale/config.toml`、`.whale/config.local.toml` 不能单独授予自改／激活能力；项目可收紧用户已授予的权限，不能提升权限。
- 用户通过可信 CLI 启动参数或用户级配置选择功能，再由当前会话显式取得能力。只持久化审计事实，不把会话授权保存为“下次自动许可”。
- 自构建来源必须是用户明确指定的 Whale 源码 checkout，验证模块标识 `github.com/usewhale/whale`、仓库实际根和目标 commit；不能把用户正在工作的任意项目当作 Whale 源码。
- 不为 subagent、workflow agent 或 ACP 连接默认继承激活权限；`--dangerously-skip-permissions` 不能取消本功能专用的激活确认和资格校验。
- 同 UID 的任意 shell 仍可能改写文件，不能声称普通文件权限或 worktree 能防住恶意候选。自主执行必须进入独立进程隔离环境；宿主控制器和批准材料不挂载为可写。

会话能力在配置合并前从用户级 Global 配置及真实用户启动入口生成，作为独立授权对象传递；不从最终 `cfg.Plugins` 反推来源。普通 agent、shell、tool 或 workflow 调 CLI 只能请求操作，不能创建能力或批准 ID。技能应用批准由现有 app 的直接用户确认通道取得，版本激活批准由监督者持有的直接用户确认通道取得，不接受工具 stdin、环境值或候选自报的“用户已确认”；技能批准绑定 session、目标路径及原／新 SHA；激活批准绑定 session、候选完整 hash、旧版与唯一批准 ID。该边界防止正常工具路径绕过，不声称能防同 UID 恶意进程。

### 4.3 平台与安装

L1、L2 设计为跨平台；L3 首版只针对 Linux/macOS 下用户主动配置的开发安装。Windows 可生成验证报告并在用户退出后按说明手动换版，不承诺替换运行中的 `.exe`。

ACP、stdio app-server、非交互 `exec` 模式首版不支持运行中激活：返回明确的不支持原因，保留连接和原进程。TUI 使用 app-server 的事实不意味着 ACP 可以透明重连。

## 5. 分阶段工作与依赖
实施顺序不是按功能炫技排序，而是按信任边界排序：先确认基线和授权，再做验证，最后才讨论运行中替换。

### M0：确认边界与基线（开发前置）

| 工作项 | 交付 | 完成条件 |
| --- | --- | --- |
| 形成设计决策 | 安装模式、授权、命令面与回滚决策，见 `docs/self-evolution-tasks.md` 的 M0-E | 在广泛变更命令面之前，维护者确认范围；本计划不代替维护者做决定 |
| 测量当前验证 | 构建、fmt、vet、离线测试结果与耗时 | 不使用真实 key；记录原有失败与环境问题，不用“允许失败”绕过门 |
| 定义受信任基线 | 基线 commit、工具链、回归样本清单 | 由用户确认；基线不由候选代码重写 |
| 检查兼容性 | 会话恢复、终端清理、后台任务、wrapper、app-server 入口清单 | 明确 safe-point；未满足时禁用激活 |
| 确定隔离方式 | 手动验证与自主验证的执行边界 | 自主模式缺少可用沙箱时只能产出 diff，不能执行候选代码 |

退出条件：基线可重现、安装边界明确、授权方案审查通过。L1 可独立推进；L3 不能以“之后再补监督者”为理由提前上线。

### M1：项目技能改进闭环

1. 加固 `skillsimprover.Store`：项目范围限制、原子写、原文备份、应用状态恢复、证据最小化与并发控制。
2. 用现有插件装配路径显式注册 opt-in 的 skill、工具和 hooks；默认仍隐藏且不写证据。不重新实现提案工具。
3. 开放人工查看证据、提案 diff、应用和恢复入口。保存提案可以经普通工具审批；应用和恢复只能由用户显式操作触发。
4. 应用成功后刷新技能目录与相关 app/toolset 状态；新内容从下一轮生效，不在正在流式执行的轮次改变 system prompt。
5. 补后端、app、service／TUI 行为测试及用户文档。

验收：用户启用后可以从一条失败证据走到提案和人工应用；关闭时旧测试仍通过；修改后的 skill 可被 `load_skill` 正确发现；原文可恢复。

### M2：固定快照自构建与验证

1. 增加 `internal/selfdev` 的候选状态与固定命令执行；保留原来的普通 `make build` 行为。
2. 来源使用用户选定的 clean commit；dirty 工作区只提示整理改动，不自动 `stash`、`reset` 或 `clean`。未跟踪的源码输入也不能悄悄纳入构建。
3. 在独立 checkout／受控 worktree 构建。候选源码只能作为输入，控制器、固定验证程序和最终资格报告来自受信任版本。
4. 执行第 7 节完整验证链路，产出二进制哈希、来源与测试报告；任何失败均阻止成为可激活候选。
5. CLI 提供候选检查和状态查看，不提供运行中切换；agent 首版使用现有 shell 经审批调用，不另加自动 build tool。

验收：正确候选生成可复核报告；编译失败、测试失败、来源漂移、报告缺失、测试／门禁篡改均不能获得激活资格；运行中的 Whale 完全不变。

### M3：受监督激活与会话接续

1. 增加用户 opt-in 的 managed 开发启动模式：稳定监督者持有控制状态，版本化 Whale 子进程持有交互终端。
2. 在安全点完成工具结果、会话和接续事件落盘；关停旧 UI、恢复终端，监督者重新检查资格并启动候选。
3. 候选完成受约束的启动检查并通过专用通道发送匹配的 ready；监督者外部检查 PID、版本、会话和结果。
4. ready 之前失败、超时或退出，恢复旧指针并启动已知良好版本；保留错误报告，不做无限重试。
5. 完成进程级集成测试、PTY 终端验证、跨版本兼容与人工演练后，才开放 `activate`。

验收：成功后恢复原 session；故障注入能恢复旧版本；不会重复执行工具、丢失用户消息、留下后台进程或破坏终端。没有监督者的普通启动返回“请用开发管理入口重启”，不退化成裸 `exec`。

### M4：一次性的有界自主改进（后续扩展）

1. 用户提供一个目标和明确预算，复用当前 agent／worktree／task 能力执行一次任务，不新增常驻调度器。
2. agent 只编辑自己的隔离源码并追加测试；外部控制器执行门禁。拒绝自行修改基线、审批或回滚组件来获准激活。
3. 提供 before/after 的客观结果、成本和不确定性；测试通过只标记“验证通过”，没有指标证据时不得标记“质量提升”。
4. 用户审阅提案或 diff；源码合并、推送、候选激活各自需要确认。首版任务结束于报告，不自动激活。
5. 补取消、预算耗尽、重复提案、失败恢复与敏感数据测试；测量结果支持再讨论 ambient/overnight，不能直接开启。

验收：单次任务在预算内结束且能随时取消；没有用户确认时主工作区、版本指针、全局技能和远端仓库均不变。

### 5.1 自主任务预算与提升判定

M4 启动必须绑定墙钟时间、模型调用次数、候选尝试次数三项硬预算。建议初值为 **30 分钟、20 次模型调用、最多 2 个候选**，均在用户开始任务时确认；所有子任务计入同一总账，不允许通过 subagent 分摊绕过。缺少预算或无法计量时拒绝启动，取消／耗尽后终止整个任务树并保留未完成报告，不偷偷续跑。

新增任务级共享预算对象，不把现有 session 美元预算当作调用次数总账。所有 provider 请求入口（含重试、压缩、side question、subagent）在发起前原子预占额度，失败请求也计数；候选在开始准备前预占尝试额度，失败和重复请求不返还。统一截止时间与取消信号覆盖全部衍生任务，使用并发抢最后额度、重试耗尽及子任务取消测试验证；入口清单在 M0 核实。

每个目标先声明指标与接受条件。例如 bug 修复要求指定重现样本从失败变为通过且基线零新增失败；性能改进按相同环境交替运行 baseline／candidate 各 **5 次**，报告中位耗时与全部测量值，建议要求中位耗时至少降低 **10%** 且固定功能样本零回归。这些是初始实验标准，不是统计显著性证明；LLM 质量任务需要独立固定评分规则／盲评样本，不能由生成改动的模型自评。

## 6. 技能证据、提案与应用设计
技能改进的首要风险不是生成失败，而是把错误、秘密或不可恢复的写入变成长期行为；所以应用必须以证据、差异和恢复能力为前提。

### 6.1 复用与加固

现有 `SaveProposal` 已解析并校验名称、检查目标是否为发现到的 skill、比较原文 SHA-256，拒绝 `plugin://`，限制提案大小。已有 `ApplyProposal` 会重新验证来源，但直接 `os.WriteFile` 目标后才保存 applied 状态。

应修复的缺口：

- `skills.DefaultRoots` 包括项目和用户 home；首版写入仅允许当前项目 `.whale/skills` 和 `.agents/skills` 内的已有普通文件，不修改 home 和插件技能。
- `normalizedExistingPath` 在 `EvalSymlinks` 失败时回退；写入信任边界必须 fail closed，并验证所有父目录。首版拒绝目标或父目录存在 symlink 的写入，不能用字符串前缀判断包含关系。
- 按真实项目根与目标串行化应用，防止两份提案同时过 SHA 检查；保存后再次解析确认，保持现有文件权限。
- 原文私有备份先落盘，目标通过同目录临时文件、flush／sync、rename 原子替换；applied 日志最后提交。权限 helper 不能被误当作原子写 helper。
- 若替换成功但状态写入失败，下次依据原 SHA／目标 SHA 恢复事务，不重复写入、不谎报“未发生变更”。人工恢复同样执行当前 SHA 校验，拒绝覆盖后来人工修改。

对跨进程／外部编辑竞争，应使用平台可用的锁与安全文件打开方式。不能完全消除的竞争必须明确报告；不得宣称能防住同 UID 的恶意进程。

### 6.2 证据采集与隐私

已有 hooks 覆盖用户反馈、工具失败、turn summary；关键词提取是线索，不是事实判断。保留这些来源，不额外扫描全部聊天历史。

默认只记 skill、错误码、最短必要摘要、会话关联与证据 ID。不保存完整 tool args、环境、请求头、key、prompt 或模型输出；确有必要的文本由用户确认后保留，并在截断前脱敏。现有每段 `2000` 字符与提案 `200000` 字符限制只能控制体积，不能代替脱敏。

记录写入失败对普通任务不致命，但必须在功能状态中可见，不继续静默忽略。支持查看、按用户确认清除及停止采集；不上传证据，不把私有数据目录提交到仓库。

### 6.3 新技能与记忆

M1 只改已有项目技能，不新增自动 skill 创建、批量改写、全局 skill 发布或通用 memory 写接口。用户创建新技能仍走已有文件工具与解析校验；project memory 不纳入本轮改进。

## 7. 自构建与质量门
任何候选都不能自己定义“通过”：质量门必须由候选之外的固定验证材料和控制器决定。

### 7.1 来源身份

首版候选身份绑定：规范化源码根、候选 commit、受信任 baseline commit、Go 工具链、GOOS/GOARCH、构建参数、依赖摘要、门禁版本与最终二进制 SHA-256。

要求候选 checkout 干净；只用 commit 短 hash 不足以标识脏工作区或构建参数。源码／依赖变化必须生成新候选，之前报告不能复用。构建快照无项目 secrets、git credential、用户数据或可写宿主控制文件。

校验 `go.mod`、`go.sum`、构建脚本、测试、fixtures、evals、权限／hooks、更新监督与会话格式的差异。它们属于受保护审查面：出现变化时停止自动资格授予，允许生成报告并标记人工审查要求，不能把“需要人工审查”当作验证成功。

### 7.2 固定验证顺序

| 顺序 | 验证 | 门禁条件 |
| --- | --- | --- |
| 1 | 来源、模块、平台、依赖与受保护差异 | 固定输入可复核；未审批的受保护变更拒绝激活 |
| 2 | 格式与静态检查 | 受信任控制器执行对应 `fmt-check` 和 `go vet ./...`；不修改源码 |
| 3 | 离线完整测试 | 固定命令 `go test -count=1 ./...` 成功；不能只运行修改相关包或复用旧测试缓存 |
| 4 | 固定基线回归 | 可信基线 tests／fixtures／evals 对候选执行，且基线测试集合不允许缩减 |
| 5 | 构建与二进制行为 | `go build` 目标 `./cmd/whale`；外部可信 smoke 使用刚构建的二进制验证身份、启动、session、拒绝动作及退出 |

`make fmt-check`、`make vet`、`make test` 可用于开发者日常验证；生产门禁不能执行候选改写后的 `Makefile` 或 `cmd/dev` 来决定自己是否通过。固定命令使用参数数组，不接收模型提供的 shell 命令。

完整源码测试与候选二进制检查是两类证据：前者不保证正在发布的文件就是被验证的二进制。最终报告必须绑定 smoke 使用过的二进制 SHA-256，激活前重新核对。

### 7.3 基线测试不可篡改

基线回归使用独立验证目录：候选生产代码加只读的受信任基线 tests／fixtures／eval 清单，保留 baseline 中的旧包与测试；该目录不混入候选同名测试，避免重复函数或文件覆盖。候选自己的完整测试在第 3 步单独运行，新增测试可以补证，但不能替代基线。若删除包或 API 变化导致基线无法编译，先标记不兼容并人工审查，不自动忽略测试。

M0 另外固定可信验证支撑文件清单，包括 runner、评分器及非 `_test.go` 的测试 helper；例如 `internal/evals/harness.go` 不能仅因是普通 Go 文件就用候选版本。按路径显式区分候选被测实现与可信验证代码，冲突或归属不明时拒绝组合；报告绑定文件清单与 hash。添加篡改候选 harness／评分回调的负例，验证固定回归的执行语义未被候选替换。

合并候选后，不立即把它当作新可信基线。只有人工审阅并更新基线记录，之后任务才可使用新基线。首版禁止 agent 自行修改基线资格记录。

### 7.4 沙箱与执行安全

`go test` 会执行任意候选代码，`go build` 也不能当作安全解析器。M2 手动验证在用户明确接受源码信任后执行；M4 自动执行必须使用隔离环境、只读输入、临时 HOME／DataDir、无凭据、默认禁网、进程与资源限制。

依赖提前从受信任步骤准备；运行测试时不临时下载包。新依赖、cgo、外部执行程序和网络需求必须人工确认。沙箱不存在或规则无法落实时 fail closed，不能改为宿主自动运行。

控制器固定可执行工具链和最小环境：清理候选／项目提供的 `GOFLAGS`、`GOWORK`、`GOTOOLCHAIN`、`LD_PRELOAD` 等执行影响项，使用受信任 PATH、`GOWORK=off`、`GOTOOLCHAIN=local` 和只读依赖模式。GOOS/GOARCH、cgo 策略、标签与 ldflags 来自批准的构建配置并记入身份；不运行 `go generate`，不通过项目 hooks 或 shell profile 初始化门禁。

### 7.5 运行控制与结果

首版固定为单安装单构建，使用跨进程锁，不开发队列。建议构建加验证总截止时间为 **15 分钟**，启动 ready 截止时间为 **30 秒**；M0 用真实基线耗时确认这两个初值，必要时由维护者调整常量，不开放模型自定无限超时。

取消／超时清理整个进程组或平台等价机制；失败报告区分 cancel、timeout 与工具退出码。输出先脱敏再截断，摘要只展示第一个阻塞步骤，完整私有日志有路径可查。

没有 quick／skip-tests 激活通道。开发者可以跑子集定位问题，但子集报告从不具备激活资格。

## 8. 激活、就绪与回滚
运行中替换最危险的时刻不是编译，而是旧进程已经退出、新进程还没有证明自己可用；因此监督者和回滚是硬前提。

### 8.1 为什么必须保留监督者

监督者必须是已知良好的、独立路径上的稳定程序，不运行候选更新逻辑，不在本次激活中自我替换。它负责锁、启动、ready 超时、退出监测和版本指针回退。

可以从已知良好 Whale 复制一个私有 supervisor 产物，使用受限内部启动模式；不需要外加系统服务，也不为执行边界 wrapper 加更新职责。监督者升级另走人工管理路径，候选修改其代码不能影响当前监督者。

### 8.2 安全点

安全点分为无损预检与提交阶段。预检只检查当前轮结束、tool call 已写终态、审批队列为空、前台／后台 shell、task、workflow、subagent 均已结束，以及 session 可落盘；不关闭旧 UI，也不要求先清理终端。批准后暂停新输入并再次检查，成功持久化 pending 后才提交最终快照、关闭旧 UI和恢复终端。pending 或快照提交失败时解除暂停，保留旧 UI 可用，不切指针。

首版不自动杀任务来凑安全点；只返回阻塞列表。用户主动取消后再次请求激活。源工作目录、session ID、worktree 身份、模型与模式需明确记录；不把整个环境或凭据写入激活日志。

### 8.3 激活事务顺序

1. 监督者持有安装锁，验证来源、候选报告、完整门禁、二进制 hash 和当前指针；用户确认绑定候选 ID、旧版本与源根，批准过期／来源变化必须重新确认。
2. 旧 app 暂停新输入并重新做无损预检；私有 pending 激活日志持久化 activation ID、旧新路径／hash、session ID、工作目录、基线身份、状态和请求时间。预检或落盘失败均解除暂停并保留旧 UI，不关闭旧进程、不切换。
3. 旧 app 提交最终 session 快照及恢复材料，然后关闭 UI、恢复终端；快照提交失败则标记本次中止、解除暂停且不切换。监督者保持存活，受管理的 `current` 指针以同目录临时 symlink 加 rename 切换；启动新子进程用精确版本路径而非重新解析可变指针。
4. 候选在受约束启动阶段读取冻结的 session 快照，不能写正式会话／用户数据，不调用真实模型、不执行 hooks 或恢复未决工具。只读存储或临时副本的具体落地点在 M0 确认；做不到就禁止激活。通过继承的专用 IPC 发送 activation ID、PID、候选身份、session 和 ready；监督者外部验证匹配及最小行为。ready 前故障仅丢弃临时数据，不污染旧版恢复源；版本命令退出成功不能替代 ready。
5. ready 成功后先原子提交 active，再由监督者发送开始运行许可；候选才可绑定正式存储并开放 UI 输入。向会话追加一次带 activation ID 的接续事件，不自动重发上一条用户 prompt。ready 前失败／超时则恢复旧指针，启动旧版恢复同一会话并显示失败原因。提交或许可中断时依据激活日志恢复，不并行启动两份 session 写者。

接续事件按 activation ID 去重，并在原有会话事件能力内实现；不宣称对任意外部副作用有“恰好一次”保证。禁止未落盘工具重放，恢复遇到未决状态时只提示人工处理。

### 8.4 回滚保证与限制

| 故障 | 行为 |
| --- | --- |
| 构建／测试／smoke 失败 | 不改 current；保留报告 |
| pending 日志失败或安全点不满足 | 不关闭旧 app，不切换 |
| 候选无法启动、panic、ready 超时 | 监督者恢复旧指针并启动旧版，自动回退一次 |
| 监督者崩溃／断电 | 下次 managed 启动持锁检查存活子进程；pending／starting／ready 未提交 active 时恢复旧版，不再尝试失败候选；已提交 active 时按已生效版本恢复，不把可能已有的新数据误当作未激活 |
| 指针回退失败、旧版本损坏 | 停止自动动作；保留所有数据，显示旧版精确路径与 `resume <id>` 恢复命令 |

候选若在 ready 后才崩溃，保留人工 rollback 入口和诊断，但不能承诺所有运行期故障自动判定或回退。启动回滚不等于应用数据回滚；M3 禁止自动应用会话格式迁移。候选必须兼容旧版读取，否则不具备可回滚资格。

旧版失败时不在两版本之间循环。监督者不删除候选源码、不做 git 回滚，不覆盖用户改动。

## 9. 数据布局、状态与命令契约
状态、产物和激活日志必须让控制器在崩溃后重新判断事实，而不是依赖候选留下的自报成功标记。

### 9.1 私有数据布局（拟议）

所有运行期数据放在用户 `DataDir` 下，不能默认放在项目仓库；技能原文备份使用私有权限。

```text
<DataDir>/selfdev/
  supervisor/                    # 人工配置的已知良好监督产物
  versions/<candidate-id>/
    whale[.exe]
    source.json                  # 来源、工具链、门禁身份
    verification.json            # 步骤结果、资格、二进制 hash
    build.log                    # 已脱敏输出
  activations/<activation-id>.json
  current                        # Unix managed 安装专用指针

<skills-improver 私有数据目录>/
  evidence.jsonl
  proposals/<proposal-id>.json
  backups/<proposal-id>/SKILL.md
```

复用 `Store` 已有的实际路径约定，不重定位旧提案。新增日志带 `schema_version: 1`；现有提案新增字段采用兼容默认值。旧的无备份提案不直接自动应用，先经重新验证补齐事务材料。

不可变候选目录不被覆盖；输入相同且报告完整可以复用，验证参数或门禁变化必须生成新身份。默认不自动删除产物；后续如有清理功能，只能删除未引用的候选，不能删除 current、回滚版、未完成激活或人工指定保留项。

### 9.2 状态

```text
技能：evidence → proposed → applying → applied
                         ↘ rejected     ↘ restored

候选：prepared → verifying → verified
                     ↘ failed／cancelled／needs_review

激活：pending → starting → ready → active
                      ↘ rolling_back → rolled_back／recovery_required
```

状态只能由控制器根据可核实事实推进，不能接受模型输入 `verified=true` 或 `ready=true`。未知 schema、损坏日志和越界路径均拒绝继续；报告“需人工审查”不等于通过。

### 9.3 命令面（拟议，当前不可用）

| 阶段 | 入口草案 | 行为 |
| --- | --- | --- |
| M1 | 显式启用后 `/skills-improver status`、`proposals`、`show <id>` | 状态、提案、diff；默认隐藏 |
| M1 | `/skills-improver apply <id>`、`restore <id>` | 必须用户确认目标与 hash；agent 不注册 apply tool |
| M2 | `whale selfdev check --source <path> --commit <sha>` | 构建与完整验证，不切换，不自动提交 |
| M2 | `whale selfdev status` | 显示当前、候选资格、最后一次失败与报告路径 |
| M3 | `whale selfdev activate <id>`、`rollback` | 请求当前 managed 会话的监督者；跨会话请求／busy／不支持模式拒绝 |

命令名、参数和配置键需 M0 设计决策（`docs/self-evolution-tasks.md` 的 M0-E）定稿，上表不是已实现 API。不新增 `build-reload` 一键跳过确认入口。配置优先复用插件 enable 机制，但必须保留用户来源信息，不能通过普通配置合并把项目启用误当用户授权。

### 9.4 错误与观察性

拟议结构化错误：`source_untrusted`、`source_changed`、`protected_change`、`verification_failed`、`verification_cancelled`、`candidate_mismatch`、`busy`、`unsupported_mode`、`activation_timeout`、`rollback_failed`。

每个报告保留开始／结束时间、步骤、退出码、失败摘要、来源、hash、批准标识和日志路径。不记录环境值或 key。TUI 展示当前步骤、候选 ID 和阻塞原因，不输出未经清理的控制字符；未知错误不能默默按成功处理。

## 10. 文件接入与影响范围

以下为预期触达点，不要求每个文件都改；实现时按符号确认最小 diff。

| 子系统 | 实现与调用方 | 测试／文档／兼容性 |
| --- | --- | --- |
| 技能后端 | `internal/plugins/skillsimprover/plugin.go`、`internal/plugins/skillsimprover/store.go` | 现有 `evidence_test.go`、`store_test.go` 加事务／隐私／路径案例；旧提案兼容 |
| 插件和授权 | `internal/plugins/plugin.go`、`internal/app/app_tools_init.go`、`internal/app/app_types.go`、`internal/app/config_file.go`、`internal/app/config_apply.go` | 隐藏默认测试、配置来源测试、subagent 不继承能力测试；现有 plugin Outcome 不改协议 |
| 命令与 UI | `internal/commands/parser.go`、`internal/app/command_handlers.go`、`internal/app/service`、`internal/tui`、`internal/app/commands_skills.go` | 默认无命令／工具；opt-in 确认与取消、目录刷新、disabled 与覆盖优先级 |
| 构建与身份 | 新 `internal/selfdev`、`internal/build/version.go`、`internal/ui/cli/cmd`；必要时 `cmd/dev/main.go`、`Makefile` | 机器可读身份测试、假 runner、真实最小 Go fixture；普通版本输出兼容 |
| 启动与恢复 | `cmd/whale/main.go`、`internal/ui/cli/cmd/root.go`、`internal/app/resume.go`、`internal/ui/tuiadapter`、`internal/app/service` | safe-point、进程故障、PTY、session 去重；不改变 execboundary 的权限语义 |
| M4 共享预算 | `internal/agent/turn_loop.go`、`internal/agent/side_question.go`、`internal/tasks/subagent.go` 及 M0 核实的全部 provider 调用入口 | 并发预占、重试／压缩计数、子任务总账、截止时间与取消；不替换现有 session 美元预算 |
| 回归与说明 | `internal/evals`、CLI／TUI 测试、smoke fixtures、`docs/skills.md`、`docs/skills.en.md`、`docs/plugins.md`、`docs/plugins.en.md`、`docs/configuration.md`、`docs/configuration.en.md` | 新功能上线时同步双语用户说明与 help；live API 验证仍放 `scripts/smoke/` |

基线、fixtures、exports 和调用者都属于 scope。新增 selfdev API 仅在跨包调用需要时导出；避免 import cycle，不让底层 selfdev 依赖 app/TUI。

## 11. 测试计划与验收矩阵

| 类别 | 必须覆盖 | 判定 |
| --- | --- | --- |
| 默认兼容 | 未启用时隐藏、无证据写入；普通 build、resume、权限、workflow、ACP 不变 | 原有行为级测试仍通过 |
| 授权与预算 | 项目配置覆盖、伪造用户来源、tool 调 CLI、同意后候选变化、并发争抢最后额度、重试耗尽 | 无独立用户确认不能获准；共享硬预算不被绕过 |
| 技能输入安全 | 无效 frontmatter、名称不符、过大、越界、home、plugin、symlink、SHA 漂移 | 原文件与元数据不受损 |
| 技能事务 | 并发应用、目标写失败、状态写失败、磁盘满、重复应用、恢复冲突、证据脱敏 | 原文可恢复；不重复应用；错误可见 |
| 验证可信度 | dirty／模块错误、源码漂移、修改测试／Makefile／依赖／fixtures、陈旧报告、二进制被替换 | 不授予激活资格；固定验证器仍执行 |
| 进程控制 | 编译失败、测试失败、超时、取消、衍生子进程、超大输出 | 无存活孤儿进程；报告与退出码准确 |
| 激活安全 | busy、pending 失败后旧 UI 仍可用、快照失败、ready 前拒绝正式写入、损坏日志、错误 token／PID、缺失旧版、超时、panic、断电恢复 | 旧版保留；自动回退一次；终端可用 |
| 会话正确性 | 同 session／cwd／worktree／模式恢复、接续事件去重、拒绝未决工具重放、旧版可读 | 不丢消息、不重复模型请求或工具动作 |
| 平台与安装 | Linux/macOS managed；Windows 编译／手动路径；npm 安装拒绝改写；ACP／exec 拒绝热激活 | 行为明确，无降级为危险路径 |
| 改进证据 | 固定样本 before/after、保持种子／参数、成本与失败记录 | 零基线行为回归；提升声明有对应指标 |

单元测试优先临时目录、fake runner／provider／clock；状态机错误注入不需要真实模型。真实候选二进制测试使用无 key 的临时 DataDir，除非特定 test fixture 明确用 fake key；不读取真实用户会话。

进程和 PTY 测试作为 M3 必需检查，不以单元测试替代。可选现场 API smoke 不影响默认离线套件，失败时不能宣称 live 路径已验证。

开发者检查命令：

```bash
make fmt-check
make vet
make test
make build
make build-acp
# 快速定位，不是附加门：
make test-evals
# 在 Windows runner 上：
make test-windows
```

## 12. 安全底线与故障处理

| 风险 | 防线 | 剩余限制 |
| --- | --- | --- |
| agent 修改门禁来过关 | 受信任外部控制器、只读 baseline、受保护差异审查 | 有权限的用户仍可改控制器；不防宿主全面失陷 |
| 证据带入秘密／恶意指令 | opt-in、最小化、脱敏、私有目录、将证据视为数据 | 脱敏非绝对保证，用户可检查和清除 |
| 候选测试执行恶意代码 | 自动执行沙箱、无 key／宿主写权限、禁网、资源限制 | worktree 本身无隔离保障；手动宿主运行有信任前提 |
| 技能改坏未来行为 | diff 审查、hash 比较、备份、原子应用、下一轮生效 | 语法合法不等于提示词正确 |
| 新版不可启动 | 稳定监督者、pending 日志、ready 外部检查、旧版保留 | ready 后业务错误需要回归样本与人工 rollback |
| 源码／二进制身份伪造 | 固定来源、完整 hash、报告绑定、激活重检 | 候选自报版本不是安全证明 |
| 会话数据损坏 | safe-point、只增激活日志、不做自动格式迁移、旧版可读 | 外部副作用不能通过二进制回滚撤销 |
| 多会话竞争与升级冲突 | 单安装锁、绑定会话、检测 current 变化、拒绝 busy | 首版不协调多会话同时换版 |

不运行 `git reset --hard`、`git clean`、自动 stash、强制推送或未确认删除。合并源码与激活二进制是两件事：自动回滚二进制不回滚 Git，也不改主工作区。

## 13. 交付节奏、估算与发布门

估算前提：一名熟悉仓库的 Go 开发者，现有离线测试可运行，不新增依赖／模型供应商。包含测试与审阅，不含外部安全审计；M0 实测后重新估算。

| 里程碑 | 工作量区间 | 发布门 |
| --- | --- | --- |
| M0 设计与基线 | 1–2 个工作日 | 设计决策文档与来源／安全点／隔离选择确认 |
| M1 技能闭环 | 3–5 个工作日 | 默认隐藏、隐私、事务、恢复和 UI 行为通过 |
| M2 自构建验证 | 4–7 个工作日 | 固定 baseline 与候选身份完整；无自动切换 |
| M3 监督激活 | 7–12 个工作日 | Unix 进程／PTY／回滚／会话故障演练通过 |
| M4 有界自主任务 | 5–8 个工作日 | 沙箱与预算可执行；结束于报告 |

M0–M2 共 **8–14 个工作日**；包含 M3 共 **15–26 个工作日**。M4 为单独决策，不属于前述时间范围。未知风险主要是 TUI 生命周期与会话 safe-point，不再沿用前期半天或 2–4 天的粗略自更新估算。

按里程碑拆 PR，后端加固先于入口开放，验证器先于激活，监督者先于 ready 切换。各 PR 写明行为变化、为什么需要、离线验证与未覆盖项；CLI／TUI 变更附确认／错误／恢复界面的输出。

### 13.1 Definition of Done

- 计划对应的产品行为可演示，调用方、测试、fixtures、help 与双语用户说明同步；默认行为不变。
- 所有激活必要的验证项通过，报告对应最终二进制；受保护变更有明确人工审查结果。
- 故障注入证明不丢技能原文、session 或回滚产物；拒绝动作有可执行原因。
- 无私有数据、key、日志或生成二进制入库；无新循环依赖或未说明的架构回退。
- 提升结论附原始指标定义与 before/after，不将测试通过写成性能或智能提升。

## 14. 未决问题与建议答案

| M0 需确认的问题 | 建议默认答案 | 未确认时 |
| --- | --- | --- |
| 首版是否包含技能全局写入／新建？ | 不包含；只改项目中已有技能 | 保持只读或普通文件编辑流程 |
| 开发安装怎样与 npm 版本共存？ | 用户主动配置独立 managed 启动入口，不覆盖包管理文件 | 只交付 L2 |
| 自主验证用哪种沙箱？ | 选宿主已可用的隔离运行时，禁止回退宿主自动执行 | L4 只生成 diff |
| 何时批准受保护变更和更新 baseline？ | 分开人工审阅 diff 与新基线；任何一次批准只绑定具体 hash | 标记 needs_review，拒绝激活 |
| 是否需要 ACP 运行中重连或常驻自改？ | 不在首版；另开设计与兼容性评估 | 明确拒绝热激活，不复制 daemon |

以上不阻塞本计划文档交付，但安装模式、授权来源和沙箱必须在对应实现阶段开始前确认。实际命令面须先由维护者在 `docs/self-evolution-tasks.md` 的 M0-E 中确认，不把草案直接当冻结接口。

## 15. 开始实施的最小步骤

1. 审阅第 4 节默认设计和第 14 节建议，先确认开发安装与授权边界。
2. 在 `docs/self-evolution-tasks.md` 的 M0-E 中完成决策并建立受信任 baseline；只测量当前版本，不执行模型生成的候选。
3. 先做 M1 Store 加固与测试，保留隐藏入口；通过后再做 opt-in 装配。
4. 独立交付 M2 验证器和报告；没有 M3 时停止于候选，不临时用 `syscall.Exec` 顶替监督者。
5. 在 M3 故障演练通过后开放逐次确认激活；M4 另行批准。

## 附录：本次计划的验证范围

本次已核对 Whale 的技能提案后端、隐藏测试、插件装配、构建／版本、resume、worktree、execboundary 与 app-server 的关键入口，并参考 jcode 构建发布与 reload 实现。

本次未运行两仓库的测试或真实构建，未进行 Unix 终端／断电演练、Windows 运行测试、ACP 热重连或 live API 检查，也未审计 jcode ambient／overnight 全部实现。时间、超时与安装方案仍需 M0 验证；本文是可执行的开发方案，不是已验证的运行期保证。
