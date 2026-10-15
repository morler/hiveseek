# M0 基线记录与核实报告

- 状态：草案，待维护者确认。确认前命令名、参数与配置键不是冻结接口。
- 机器可读记录：`docs/self-evolution/baseline-v1.json`（`schema_version: 1`、`gate_version: m0-baseline-v1`、`status: proposed`）
- 基线 commit：`676a0a228882c457ff4dc00d9d285d972aea4a43`（`v0.1.66`）
- 测量 commit：`0a7e808a24f92e2de85650d5eb9705e2b5a745b7`；相对基线只多出三份文档（`AGENTS.md`、`docs/self-evolution-plan.md`、`docs/self-evolution-tasks.md`），Go 代码与 `go.mod`／`go.sum` 零差异
- 阶段纪律：本报告只测量当前版本；`candidate_executed: false`，未执行任何模型生成的候选（M0-21）

## 1. 工具链与宿主（M0-02）

`go version go1.27.2 linux/amd64`；`GOOS=linux`、`GOARCH=amd64`、`CGO_ENABLED=1`、`GOAMD64=v1`。

宿主机器：Linux 7.2.6.p03.32-1.fc44.x86_64，13th Gen Intel(R) Core(TM) i9-13980HX，32 逻辑 CPU。

测量环境固定为离线可复现：`GOWORK=off`、`GOTOOLCHAIN=local`、`GOPROXY=off`、`GOFLAGS=-mod=readonly`、`GOCACHE=<repo>/.gocache`、临时 HOME、`LANG=C.UTF-8`。

## 2. 基线耗时（M0-03）

| 命令 | 墙钟耗时 | 退出码 |
| --- | --- | --- |
| `make fmt-check` | 0.176942s | 0 |
| `make vet` | 0.863897s | 0 |
| `go test -count=1 ./...` | 16.69998s | 0 |
| `go build ./cmd/whale` | 0.542735s | 0 |

四条合计 18.283554s，为热构建缓存下的下界；冷缓存与更慢机器显著更久。
### 2.1 环境依赖：Git 输出本地化

`internal/worktree/worktree_test.go:498` 的 `TestRemoveDoesNotForceDeleteUnmergedBranchByDefault` 断言 git 的英文提示文本；在 `LANG=zh_CN.UTF-8` 下 git 输出中文，断言失败：

```text
expected non-force branch delete warning, got "git branch -d worktree-unmerged: 错误：分支 'worktree-unmerged' 没有完全合并 ..."
```

`internal/worktree/**` 属受保护路径，M0 不改动它。现状结论：受信任验证门需在英文消息环境下执行（`LC_ALL=C`）才是确定性的；本报告所有耗时测量均在 `LANG=C.UTF-8` 下取得，`LC_ALL=C go test -count=1 ./...` 复核为绿色（17.035s）。是否顺带修正该断言由维护者决定。

### 2.2 `make test` 与 `make test-evals` 的关系（M0-12）

`Makefile` 的 `test`、`test-evals` 都经 `go run ./cmd/dev` 执行。`make test` 等价 `go test ./...`，已覆盖含 `internal/evals` 在内的全部包；`make test-evals` 只跑 `go test ./internal/evals/...`。

结论：受信任验证门只执行一次完整测试（`make test` 或等价的 `go test -count=1 ./...`）；`test-evals` 仅用于开发定位，不作为第二个独立质量门重复执行。

## 3. 冷启动到 ready（M0-04）

3 次采样，全部值（非平均值）：`0.014532s`、`0.014713s`、`0.013537s`。

测量方式：在 bwrap 隔离进程中启动 `whale app-server`，就绪信号为一条 `type=ready` 且同时含 `session_id` 与 `workspace_root` 的 JSON；随后以 `shutdown` 意图正常退出。

该信号是启动代理（startup proxy），不等价于未来的激活握手。

## 4. 超时常量（M0-05、M0-06、M0-07）

位置：`internal/selfdev/limits.go`。

| 常量 | 值 | 依据 |
| --- | --- | --- |
| `VerificationTimeout` | `15 * time.Minute` | 界定整个"构建 + 验证"操作（不是单步）；M0 实测 18.28s 为热缓存下界，保留 15 分钟给冷构建与更慢机器 |
| `ReadyTimeout` | `30 * time.Second` | 界定候选启动；M0 实测约 14ms，保留 30 秒 |

调整路径：只由维护者审阅代码改动调整，并同步更新本报告与 `baseline-v1.json`；不接受运行时输入，也不接受模型自定值。

## 5. 信任面清单

### 5.1 候选身份字段（M0-08）

与计划 §7.1 逐条对应，落于 `baseline-v1.json` 的 `identity_required_fields`（10 项）：

`source_root`、`candidate_commit`、`baseline_commit`、`go_toolchain`、`goos`、`goarch`、`build_args`、`dependency_digest`、`gate_version`、`binary_sha256`。

### 5.2 可信验证支撑文件（M0-09）

`trusted_files` 共 337 个文件，摘要 `sha256: 9e8ad3406e6ea1c9195766795b1f4ce6a600e9dc50aed35755cbca36709d8ba0`；role 分布：`baseline_test` 281、`baseline_fixture` 21、`trusted_support` 35。

哈希格式：`role <TAB> path <TAB> sha256`，路径排序，末行带 LF；依赖摘要同格式（`go.mod`、`go.sum`），`dependencies.sha256: f6fee80499385cbb9fd938ebf52de209099c129494b8517b4b513104adb218c3`。

`trusted_support`（runner、评分器、非 `_test.go` 的测试 helper，35 个）：

```text
bench/deferred_compare/main.go
bench/livecache/main.go
bench/livecache/report.go
bench/livecache/tasks.go
bench/livecache/types.go
bench/taubenchlite/db.go
bench/taubenchlite/main.go
bench/taubenchlite/report.go
bench/taubenchlite/tasks.go
bench/taubenchlite/tools.go
bench/taubenchlite/types.go
bench/taubenchlite/user_sim.go
bench/toolshape/main.go
cmd/dev/main.go
internal/evals/doc.go
internal/evals/harness.go
internal/evals/replay.go
internal/evals/result.go
scripts/acp_smoke_test.sh
scripts/bench/cost.sh
scripts/bench/live_cache.sh
scripts/bench/tau_bench_lite.sh
scripts/bench/tool_shape.sh
scripts/evals/autoreview_overeager/main.go
scripts/evals/autoreview_session/main.go
scripts/evals/autoreview_synthetic_exfil/main.go
scripts/smoke/deep_research_mock.sh
scripts/smoke/local.sh
scripts/smoke/mcp_tools.sh
scripts/smoke/real_cache.sh
scripts/smoke/real_stream.sh
scripts/smoke/responses_web_search.sh
scripts/swebench/instance_sets/lite_batch_01.txt
scripts/swebench/instance_tools.py
scripts/swebench/run_instance.sh
```

`baseline_test` 与 `baseline_fixture` 按规则从 git 基线快照枚举（`*_test.go` 与 testdata／fixtures），完整清单在 `baseline-v1.json`。

### 5.3 受保护审查路径（M0-10）

| 类别 | 路径 |
| --- | --- |
| 依赖 | `go.mod`、`go.sum` |
| 构建与发布 | `Makefile`、`cmd/dev/**`、`.github/workflows/**`、`npm/**`、`scripts/**` |
| tests | `**/*_test.go` |
| fixtures | `**/testdata/**`、`scripts/evals/**/fixtures/**` |
| evals | `internal/evals/**`、`scripts/evals/**`、`bench/**` |
| 权限与 hooks | `internal/agent/hook*.go`、`internal/policy/**`、`internal/execboundary/**`、`internal/agent/approval*.go` |
| 更新监督 | `cmd/whale-acp/**`、`internal/acp/**`、`internal/plugins/plugin.go`（插件根与项目数据目录） |
| 会话格式 | `internal/store/**`、`internal/session/**` |
| 自演化本身 | `docs/self-evolution/**`、`internal/selfdev/**` |

### 5.4 provider 请求入口（M0-11）

统一出口（含重试）：`internal/llm/deepseek/client.go:433` `sendStreamRequestWithKeyPath`；`internal/llm/deepseek/client.go:383` `streamWithRetriesAuth`（`requestAttempt`／`streamAttempt` 双层重试，策略来自 `llmretry.NormalizePolicy`）；实际 HTTP 发起点 `internal/llm/deepseek/client.go:446` `c.httpClient.Do(req)`。Responses 路径同经该出口：`internal/llm/deepseek/responses.go:451` → `:458`。接口契约：`internal/llm/provider.go:72-78` `Provider.StreamResponse`。

调用入口（每次请求都在此预占预算）：

| 入口 | 位置 |
| --- | --- |
| 主轮次循环 | `internal/agent/stream_ingest.go:44-54` |
| 压缩（compact） | `internal/agent/compact.go:104-118` |
| 强制摘要 | `internal/agent/force_summary.go:80-86` |
| side question | `internal/agent/side_question.go:29-61` |
| subagent | `internal/tasks/subagent_hooks.go:157-167` |
| 并行推理 | `internal/tasks/parallel_reason.go:132-145` |
| webfetch 抽取 | `internal/app/webfetch_extractor.go:29-72` |
| 后台任务运行时 | `internal/app/task_runtime.go:24-74` |

结论：入口收敛于少数几个调用点，均在 `sendStreamRequestWithKeyPath` 之下；重试不绕过该出口，可在此实现原子预占与统一截止时间。

### 5.5 私有数据目录约定（M0-13）

- 数据根（`internal/store/jsonl.go:20-69`）：环境变量 `WHALE_HOME` 优先；否则用户主目录下 `.whale`；Windows 取用户主目录，取不到时回退相对当前目录的 `.whale`。
- 插件数据（`internal/plugins/plugin.go`）：`Context{DataDir, WorkspaceRoot}`；插件根 `<DataDir>/plugins/<plugin-id>`，另有插件数据目录、缓存目录，以及按工作区哈希（`WorkspaceHash`，sha1 前 16 位十六进制；空工作区为 `no-workspace`）隔离的项目数据目录。
- skills-improver（`internal/plugins/skillsimprover/store.go:339-345`）：证据 `<DataDir>/evidence.jsonl`，提案 `<DataDir>/proposals/<id>.json`；目录与文件统一经 `internal/securefs/private.go`（`MkdirPrivate`、`WritePrivateFile`、`OpenPrivateAppend`）创建为私有权限。
- 候选、报告、备份、激活日志与 `current` 指针：现有代码中**不存在**。M2 起在 `<DataDir>/selfdev/<candidate-id>/` 新建，不复用 `proposals/`。

### 5.6 提案 schema 兼容策略（M0-14，草案）

新增字段一律带向后兼容默认值（零值即旧语义）；旧提案文件不迁移、不改写、不重定位；解析失败不回写，只报错。此结论待维护者确认后随 M1-26 落地。

### 5.7 候选只读落地点（M0-15，草案）

按固定 commit 以 `git archive` 导出到 `<DataDir>/selfdev/<id>/source/`（私有权限目录），验证时以 bwrap 只读绑定该目录、配独立临时 `GOCACHE` 运行；若宿主无可用沙箱，则按 M0-17 结论退化为只产出 diff、不激活。

## 6. 边界确认

### 6.1 开发安装与 npm 共存（M0-16，待决策）

建议：用户主动配置独立 managed 启动入口，不覆盖包管理器文件（见 M0-E 决策 2）；否则明确改为只交付 L2。待维护者决策。

### 6.2 沙箱运行时可用性（M0-17）

宿主可用：`/usr/bin/bwrap`（bubblewrap 0.12.0）。

已验证的组合：`--unshare-all --die-with-parent --new-session`、`--clearenv`、`--ro-bind /usr /usr`。在此配置下：宿主 HOME 不可见、凭据不可见、`/usr` 只读、CPU 时间受限、fd 上限 64、地址空间 256MiB、无网络路由；`whale app-server` 在其中完成 ready 与 shutdown。

结论：可用。若在目标机器不可用，L4 只生成 diff（不激活）。

### 6.3 授权来源边界（M0-18）

照抄计划 §4.2：会话能力在配置合并前，从用户级 Global 配置及真实用户启动入口生成，作为独立授权对象传递；不从最终 `cfg.Plugins` 反推来源。

## 7. 交付物与阶段纪律（M0-19、M0-20、M0-21）

- baseline 记录：`docs/self-evolution/baseline-v1.json`；`approval: null`，批准需绑定具体 hash。
- 离线一致性校验：`internal/selfdev/baseline_test.go` 校验计数、身份字段、超时常量与常量一致性、路径排序与哈希格式、`real_api_key_used=false`、`candidate_executed=false`。
- 阶段纪律：M0 期间未执行任何模型生成的候选；`/skills-improver` 等命令面仍为草案，未经维护者确认不得实现 M1-47 及之后的命令。

## 8. 需维护者确认

| # | 事项 | 现状 |
| --- | --- | --- |
| 1 | M0-E 决策 1–5（全局写入、共存方式、沙箱选择、批准流程、热激活） | 建议，待确认；权威状态与日期见 `self-evolution-tasks.md` 的 M0-E 决策表 |
| 2 | M0-E 决策 6（插件状态列表过滤） | 已确认（方案 A）；日期与依据见 M0-E 决策 6 |
| 3 | M0-16 开发安装与 npm 共存方式 | 待决策 |
| 4 | `baseline-v1.json` 由 `proposed` 转为批准，并绑定 hash | 待批准 |
| 5 | M1 开工许可 | 未批准 |
