# Repository Guidelines

## Project Structure & Module Organization
Whale is a DeepSeek-native Go CLI/TUI coding agent; the module is `github.com/usewhale/whale` and all binaries live under `cmd/`:

- `cmd/whale` — main CLI/TUI binary; `main.go` first branches into the exec-boundary wrapper when `WHALE_EXEC_BOUNDARY_WRAPPER=1`.
- `cmd/whale-acp` — ACP server binary, built via `make build-acp` (not covered by `make build`).
- `cmd/dev` — cross-platform dev runner that every Makefile target delegates to.

Most application code lives under `internal/`:

- `internal/ui/cli/cmd` and `internal/tui` for Cobra commands and terminal UI; `internal/app` assembles runtime services and wires them to the UI.
- `internal/agent` for the turn loop, approvals, hooks, and stream classifier; `internal/tools` for built-in tools.
- `internal/session`, `internal/store`, and `internal/memory` for persistence; `internal/policy` for permission and effects rules.
- `internal/llm/deepseek` for the provider client; `internal/workflow` (QuickJS script orchestration, disabled by default), `internal/skills`, `internal/plugins`, `internal/mcp`, and `internal/lsp` for extension surfaces.
- `internal/evals` for offline eval and replay coverage; `internal/selfdev` for in-progress self-build work (design in `docs/self-evolution-plan.md`).
- `bench/` and `scripts/bench/` for cost and schema benchmarks; `scripts/smoke/` and `scripts/swebench/` for smoke checks and the SWE-bench-style harness.

Docs are bilingual where a pair exists: `docs/x.md` is Chinese and `docs/x.en.md` is English (`README.zh.md` pairs with `README.md`); some pages like `docs/lsp-configuration.md` are Chinese-only. Update both languages together.

Keep new packages focused and place tests next to the code they cover.

## Build, Test, and Development Commands

Each command routes through `go run ./cmd/dev <command>`; the Makefile targets are thin wrappers around it, and this runner also works on Windows where `make` does not: `build`, `run`, `fmt-check`, `vet`, `test`, `test-tui`, `test-evals`, `test-windows`, `clean`.

- `make build` produces `bin/whale`; `make run` builds and launches the TUI. `BIN`, `GOCACHE_DIR`, `VERSION`, and `LDFLAGS` env vars control the output path, cache dir, and version injected into `internal/build.Version`.
- `.gocache/` is a deliberate repo-local Go build cache; `make clean` removes `bin/` and `.gocache/` but refuses `GOCACHE_DIR` paths outside the repo.
- `make test` runs `go test ./...` and already includes `internal/evals`; `make test-evals` is a focused subset for iteration, not an additional quality gate.
- `make test-tui` covers only `./internal/tui` and `./internal/tui/render`.
- `make test-windows` runs the curated Windows-only subset plus a build; use it on Windows.
- CI runs `fmt-check`, then `vet`, then `test`, with a separate `test-windows` job (`.github/workflows/ci.yml`); match that order before pushing.
- `make build-acp` builds `bin/whale-acp` with a plain `go build`.
- `make bench-cost` stays offline; `make bench-cost-live` and the `real_*`/`mcp_tools` smoke scripts need a real `DEEPSEEK_API_KEY` (`scripts/smoke/local.sh` stays offline: Go suite plus mock deep-research E2E).

## Coding Style & Naming Conventions

Follow standard Go style: tabs for indentation, `gofmt` formatting, concise package names, and exported identifiers only when cross-package use is required. Prefer table-driven tests for behavior matrices. Name files by responsibility, such as `tool_repair.go`, `model_test.go`, or `preferences_test.go`.

## Testing Guidelines

Tests use Go's `testing` package. Name tests `TestXxx` and keep them adjacent to the implementation. Prefer offline tests with temp dirs, fake providers, and deterministic fixtures; live provider tests never run in normal builds — they are gated by the `livetest` build tag (`go test -tags livetest ...` with `DEEPSEEK_API_KEY`) or the `RUN_REAL_SMOKE=1` guard, and real network checks belong in `scripts/smoke/`. For user-visible CLI/TUI changes, add or update behavior-level tests, not just helper-unit coverage.

## Commit & Pull Request Guidelines

Recent history mixes short imperative subjects with scoped prefixes such as `feat(tui): ...`, `agent: ...`, and `chore: ...`. Keep commits small and behavior-focused. PRs should state what changed, why it changed, and what you tested; include terminal output or screenshots when CLI/TUI behavior changes. Open an issue before broad command-surface, session-format, or provider-scope changes. For these changes, a design document checked into `docs/` may be used instead of an issue, provided it records the decision, its status (draft or confirmed), and the baseline commit.

## Security & Configuration Tips

Do not commit `~/.whale/` data or project-local `.whale/` state: `config.local.toml`, session logs, usage data, credentials, or API keys (`.whale/config.toml` is the shared project config). Treat `.whale/config.toml` hook config as untrusted input because hooks execute shell commands.
