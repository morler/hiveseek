package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/agent"
	"github.com/usewhale/whale/internal/core"
	whalemcp "github.com/usewhale/whale/internal/mcp"
	"github.com/usewhale/whale/internal/plugins"
	"github.com/usewhale/whale/internal/plugins/memoryplugin"
)

func TestMemoryPluginToolsRegisteredByDefault(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()

	if app.toolRegistry.Get("remember") == nil || app.toolRegistry.Get("forget") == nil || app.toolRegistry.Get("recall_memory") == nil {
		t.Fatalf("memory tools not registered")
	}
	if app.baseToolRegistry.Get("remember") != nil {
		t.Fatal("memory tools should not be in base subagent registry")
	}
	if app.subagentToolRegistry.Get("remember") == nil {
		t.Fatal("memory tools should be available to subagents by exact selector")
	}
}

func TestMemoryPluginCanBeDisabledByConfig(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	if err := SaveConfigFile(GlobalConfigPath(cfg.DataDir), FileConfig{
		Plugins: FilePluginsConfig{"memory": {Enabled: boolPtr(false)}},
	}); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()
	if app.toolRegistry.Get("remember") != nil {
		t.Fatal("remember should not be registered when memory plugin is disabled")
	}
	handled, out, _, err := app.HandleLocalCommand("/memory")
	if err != nil || !handled {
		t.Fatalf("/memory disabled handled=%v err=%v", handled, err)
	}
	if !strings.Contains(out, "disabled") {
		t.Fatalf("expected disabled message, got %q", out)
	}
}

func TestMemoryPluginCanBeDisabledByExplicitConfig(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.Plugins = plugins.ConfigMap{"memory": {Enabled: boolPtr(false)}}
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()
	if app.toolRegistry.Get("remember") != nil {
		t.Fatal("remember should not be registered when explicit config disables memory plugin")
	}
}

func TestMemoryLocalCommandMatchesOnlyMemoryToken(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()

	handled, out, _, err := app.HandleLocalCommand("/memorybank")
	if err != nil {
		t.Fatalf("/memorybank err: %v", err)
	}
	if handled || strings.TrimSpace(out) != "" {
		t.Fatalf("/memorybank should not be handled as /memory: handled=%v out=%q", handled, out)
	}
}

func TestMCPRefreshPreservesMemoryTools(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()

	if err := app.refreshMCPTools(); err != nil {
		t.Fatalf("refreshMCPTools: %v", err)
	}
	if app.toolRegistry.Get("remember") == nil {
		t.Fatal("memory tool disappeared after MCP refresh")
	}
}

func TestPluginStatusesListOfficialPlugins(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()

	statuses := app.PluginStatuses()
	var got []string
	for _, st := range statuses {
		got = append(got, st.Manifest.ID)
	}
	out := strings.Join(got, "\n")
	if !strings.Contains(out, "memory") {
		t.Fatalf("plugin statuses missing memory:\n%s", out)
	}
	for _, hidden := range []string{"skills-improver", "local-indexer"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("plugin statuses should not list %s before it is ready:\n%s", hidden, out)
		}
	}
}

func TestPluginStatusesListInstalledLocalPlugins(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	workspace := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()
	pluginDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pluginDir, plugins.ManifestFileName), []byte(`id = "demo-local"
name = "Demo Local"
version = "0.1.0"
description = "Installed local plugin."

[components]
skills = "./skills"
mcp = "./mcp.json"
hooks = "./hooks.toml"
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "bin", "server"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write mcp server: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(`{
  "mcpServers": {
    "local": {
      "command": "./bin/server",
      "disabled_tools": ["from-manifest"]
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write mcp config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "hooks.toml"), []byte(`[[hooks.SessionStart]]
description = "Plugin startup marker"
command = "pwd > hook-cwd.txt"
timeout = 5
`), 0o600); err != nil {
		t.Fatalf("write hooks: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "skills", "demo-skill"), 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "skills", "demo-skill", "SKILL.md"), []byte(`---
name: demo-skill
description: Use this skill for plugin runtime tests.
---

# Demo Skill

Use the plugin-provided instructions.
`), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if _, err := plugins.InstallLocal(cfg.DataDir, pluginDir); err != nil {
		t.Fatalf("InstallLocal: %v", err)
	}
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()

	statuses := app.PluginStatuses()
	var found bool
	for _, st := range statuses {
		if st.Manifest.ID == "demo-local" {
			found = true
			if st.Enabled {
				t.Fatal("local installed plugin should not be runtime-enabled in phase 1")
			}
			if st.Manifest.Status != "installed" || st.Manifest.Components.Skills != "./skills" {
				t.Fatalf("unexpected local status: %+v", st)
			}
			if !containsString(st.Skills, "demo-skill") {
				t.Fatalf("installed plugin status missing skill: %+v", st)
			}
			if !containsString(st.Hooks, "Plugin startup marker") {
				t.Fatalf("installed plugin status missing hook: %+v", st)
			}
			if !hasService(st.Services, "mcp:local") {
				t.Fatalf("installed plugin status missing MCP service: %+v", st)
			}
		}
	}
	if !found {
		t.Fatalf("installed local plugin not listed: %+v", statuses)
	}
	if reportHasSkill(app.SkillReport(), "demo-skill") {
		t.Fatal("installed-only local plugin skill should not be active before enable")
	}
	if hasMCPState(app.MCPStates(), "demo-local.local") {
		t.Fatalf("installed-only local plugin MCP server should not be active before enable: %+v", app.MCPStates())
	}
	if hasHookSource(app.HookEntries(), "plugin:demo-local") {
		t.Fatalf("installed-only local plugin hook should not be active before enable: %+v", app.HookEntries())
	}

	if _, err := app.SetPluginEnabled("demo-local", true); err != nil {
		t.Fatalf("SetPluginEnabled local plugin: %v", err)
	}
	status, ok := app.pluginManager.Status("demo-local")
	if !ok || !status.Enabled {
		t.Fatalf("expected local plugin enabled in status after toggle, ok=%v status=%+v", ok, status)
	}
	if !reportHasSkill(app.SkillReport(), "demo-skill") {
		t.Fatalf("enabled local plugin skill missing from report: %+v", app.SkillReport().All())
	}
	if !hasMCPState(app.MCPStates(), "demo-local.local") {
		t.Fatalf("enabled local plugin MCP server missing from runtime states: %+v", app.MCPStates())
	}
	var hookEntry agent.HookListEntry
	for _, entry := range app.HookEntries() {
		if entry.Source == "plugin:demo-local" {
			hookEntry = entry
			break
		}
	}
	if hookEntry.Key == "" || !hookEntry.Managed || hookEntry.Trust != agent.HookTrustManaged || !hookEntry.Active {
		t.Fatalf("enabled plugin hook should be managed and active, got %+v", hookEntry)
	}
	skillPath := ""
	for _, view := range app.SkillReport().Ready {
		if view.Name == "demo-skill" {
			skillPath = view.SkillFilePath
			break
		}
	}
	if skillPath == "" {
		t.Fatal("enabled plugin skill path missing")
	}
	out, synthetic, err := app.buildSkillSyntheticPromptFromBinding("demo-skill", "with args", SkillBinding{Name: "demo-skill", SkillFilePath: skillPath})
	if err != nil {
		t.Fatalf("buildSkillSyntheticPromptFromBinding: %v", err)
	}
	if !strings.Contains(out, "loaded skill: demo-skill") || !strings.Contains(synthetic, "Use the plugin-provided instructions.") || !strings.Contains(synthetic, "with args") {
		t.Fatalf("unexpected synthetic skill prompt:\nout=%s\nsynthetic=%s", out, synthetic)
	}
	report := app.hookRunner.RunHook(t.Context(), agent.NewSessionStartPayload("s1", workspace))
	if len(report.Outcomes) != 1 || report.Outcomes[0].Decision != agent.HookDecisionPass {
		t.Fatalf("plugin hook did not run cleanly: %+v", report)
	}
	status, ok = app.pluginManager.Status("demo-local")
	if !ok {
		t.Fatal("enabled local plugin status missing")
	}
	cwdPath := filepath.Join(status.Paths["install"], "hook-cwd.txt")
	cwdBytes, err := os.ReadFile(cwdPath)
	if err != nil {
		t.Fatalf("read plugin hook cwd marker: %v", err)
	}
	if got := strings.TrimSpace(string(cwdBytes)); got != status.Paths["install"] {
		t.Fatalf("plugin hook should run from plugin root, got %q want %q", got, status.Paths["install"])
	}
}

func TestSetPluginEnabledUpdatesProjectLocalConfigAndRuntime(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	workspace := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()
	if err := SaveConfigFile(ProjectLocalConfigPath(workspace), FileConfig{
		Plugins: FilePluginsConfig{
			"memory": {
				MCPServers: map[string]plugins.MCPServerConfig{
					"local": {Enabled: boolPtr(false), DisabledTools: []string{"write_file"}},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save project local config: %v", err)
	}

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()
	if app.toolRegistry.Get("remember") == nil {
		t.Fatal("memory plugin should start enabled")
	}

	out, err := app.SetPluginEnabled("memory", false)
	if err != nil {
		t.Fatalf("SetPluginEnabled(false): %v", err)
	}
	if !strings.Contains(out, "disabled plugin: memory") {
		t.Fatalf("unexpected disable output: %q", out)
	}
	if app.toolRegistry.Get("remember") != nil {
		t.Fatal("memory tool should be removed after disabling plugin")
	}
	handled, out, _, err := app.HandleLocalCommand("/memory")
	if err != nil || !handled {
		t.Fatalf("/memory handled=%v err=%v", handled, err)
	}
	if !strings.Contains(out, "disabled") {
		t.Fatalf("expected memory disabled after toggle, got:\n%s", out)
	}

	cfgFile, loaded, err := LoadConfigFile(ProjectLocalConfigPath(app.workspaceRoot))
	if err != nil || !loaded {
		t.Fatalf("load project local config loaded=%v err=%v", loaded, err)
	}
	if cfgFile.Plugins["memory"].Enabled == nil || *cfgFile.Plugins["memory"].Enabled {
		t.Fatalf("expected memory disabled in config, got %+v", cfgFile.Plugins)
	}
	if serverCfg := cfgFile.Plugins["memory"].MCPServers["local"]; serverCfg.Enabled == nil || *serverCfg.Enabled || !containsString(serverCfg.DisabledTools, "write_file") {
		t.Fatalf("expected plugin MCP policy preserved in config, got %+v", cfgFile.Plugins)
	}
	if _, loaded, err := LoadConfigFile(ProjectConfigPath(app.workspaceRoot)); err != nil || loaded {
		t.Fatalf("shared project config should not be written by plugin toggle, loaded=%v err=%v", loaded, err)
	}

	out, err = app.SetPluginEnabled("memory", true)
	if err != nil {
		t.Fatalf("SetPluginEnabled(true): %v", err)
	}
	if !strings.Contains(out, "enabled plugin: memory") {
		t.Fatalf("unexpected enable output: %q", out)
	}
	if app.toolRegistry.Get("remember") == nil {
		t.Fatal("memory tool should be registered after enabling plugin")
	}
	cfgFile, _, err = LoadConfigFile(ProjectLocalConfigPath(app.workspaceRoot))
	if err != nil {
		t.Fatalf("load project local config: %v", err)
	}
	if cfgFile.Plugins["memory"].Enabled == nil || !*cfgFile.Plugins["memory"].Enabled {
		t.Fatalf("expected memory enabled in config, got %+v", cfgFile.Plugins)
	}
}

func TestSetPluginEnabledLocalEnableOverridesSharedDisabled(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	workspace := t.TempDir()
	if err := SaveConfigFile(ProjectConfigPath(workspace), FileConfig{
		Plugins: FilePluginsConfig{"memory": {Enabled: boolPtr(false)}},
	}); err != nil {
		t.Fatalf("save shared project config: %v", err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()
	if app.toolRegistry.Get("remember") != nil {
		t.Fatal("memory tool should start disabled from shared project config")
	}

	out, err := app.SetPluginEnabled("memory", true)
	if err != nil {
		t.Fatalf("SetPluginEnabled(true): %v", err)
	}
	if !strings.Contains(out, "enabled plugin: memory") {
		t.Fatalf("unexpected enable output: %q", out)
	}
	if app.toolRegistry.Get("remember") == nil {
		t.Fatal("memory tool should be registered after local enable")
	}
	cfgFile, loaded, err := LoadConfigFile(ProjectLocalConfigPath(app.workspaceRoot))
	if err != nil || !loaded {
		t.Fatalf("load project local config loaded=%v err=%v", loaded, err)
	}
	if cfgFile.Plugins["memory"].Enabled == nil || !*cfgFile.Plugins["memory"].Enabled {
		t.Fatalf("expected memory enabled in config, got %+v", cfgFile.Plugins)
	}
	reloaded, err := LoadAndApplyConfig(Config{DataDir: cfg.DataDir}, app.workspaceRoot)
	if err != nil {
		t.Fatalf("LoadAndApplyConfig: %v", err)
	}
	if reloaded.Plugins["memory"].Enabled == nil || !*reloaded.Plugins["memory"].Enabled {
		t.Fatalf("expected local enable to override shared disabled on reload, got %+v", reloaded.Plugins)
	}
}

func TestReloadPluginConfigReturnsApplyLoadedConfigError(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	if err := SaveConfigFile(ProjectConfigPath(dir), FileConfig{
		UI:      FileUIConfig{ViewMode: "invalid-view"},
		Plugins: FilePluginsConfig{"memory": {Enabled: boolPtr(false)}},
	}); err != nil {
		t.Fatalf("save project config: %v", err)
	}
	app := &App{workspaceRoot: dir, cfg: Config{DataDir: cfg.DataDir, Plugins: plugins.ConfigMap{"keep-plugin": {Enabled: boolPtr(false)}}}}

	err := app.reloadPluginConfig()
	if err == nil {
		t.Fatal("expected reload error")
	}
	if !strings.Contains(err.Error(), "invalid ui.view_mode") {
		t.Fatalf("expected invalid ui.view_mode error, got %v", err)
	}
	if app.cfg.Plugins["keep-plugin"].Enabled == nil || *app.cfg.Plugins["keep-plugin"].Enabled {
		t.Fatalf("expected in-memory plugin config to be preserved, got %+v", app.cfg.Plugins)
	}
}

func TestOfficialPluginCommands(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()

	handled, out, _, err := app.HandleLocalCommand("/skills-improver proposals")
	if err != nil || handled || strings.TrimSpace(out) != "" {
		t.Fatalf("/skills-improver should not be exposed as a slash command: handled=%v out=%q err=%v", handled, out, err)
	}
	handled, out, _, err = app.HandleLocalCommand("/local-indexer rebuild")
	if err != nil {
		t.Fatalf("/local-indexer rebuild err=%v", err)
	}
	if handled || strings.TrimSpace(out) != "" {
		t.Fatalf("/local-indexer should not be exposed before it is ready: handled=%v out=%q", handled, out)
	}
}

func TestSkillsImproverIsHiddenFromSkillReportAndTools(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()

	if reportHasSkill(app.SkillReport(), "skills-improver") {
		t.Fatalf("skills-improver should not be visible in skill report: %+v", app.SkillReport().All())
	}
	if app.toolRegistry.Get("save_skill_proposal") != nil {
		t.Fatal("save_skill_proposal should not be registered while skills-improver is hidden")
	}
	blocked, output, updated := app.RunUserPromptSubmitHook("下次 $demo skill 要更具体")
	if blocked || output != "" || updated != "下次 $demo skill 要更具体" {
		t.Fatalf("skills-improver hooks should not run while hidden: blocked=%v output=%q updated=%q", blocked, output, updated)
	}
	if out := app.RunStopHook("final answer", 1); out != "" {
		t.Fatalf("skills-improver stop hook should not run while hidden: %q", out)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "plugins", "skills-improver", "data", "evidence.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("skills-improver should not write evidence while hidden, stat err=%v", err)
	}
	if app.toolRegistry.Get("load_skill") == nil {
		t.Fatal("load_skill not registered")
	}
}

func TestMemoryLocalCommandListsAndShowsMemory(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	workspace := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()
	store := memoryplugin.NewStore(filepath.Join(app.cfg.DataDir, "plugins", "memory"), app.workspaceRoot)
	if _, err := store.Write(memoryplugin.WriteInput{Scope: "global", Type: "user", Name: "style", Description: "concise Chinese", Content: "Answer concisely in Chinese."}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	handled, out, _, err := app.HandleLocalCommand("/memory")
	if err != nil || !handled {
		t.Fatalf("/memory handled=%v err=%v", handled, err)
	}
	if !strings.Contains(out, "[style](style.md)") {
		t.Fatalf("/memory output missing index:\n%s", out)
	}

	handled, out, _, err = app.HandleLocalCommand("/memory show global/style")
	if err != nil || !handled {
		t.Fatalf("/memory show handled=%v err=%v", handled, err)
	}
	if !strings.Contains(out, "Answer concisely in Chinese.") {
		t.Fatalf("/memory show output missing body:\n%s", out)
	}
}

func TestMemoryForgetInvalidatesActiveAgent(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	workspace := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()
	store := memoryplugin.NewStore(filepath.Join(app.cfg.DataDir, "plugins", "memory"), app.workspaceRoot)
	if _, err := store.Write(memoryplugin.WriteInput{Scope: "project", Type: "project", Name: "roadmap", Description: "old fact", Content: "Use the old fact."}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := app.ensureAgent(); err != nil {
		t.Fatalf("ensureAgent: %v", err)
	}
	if app.a == nil {
		t.Fatal("expected active agent")
	}

	handled, out, _, err := app.HandleLocalCommand("/memory forget project/roadmap")
	if err != nil || !handled {
		t.Fatalf("/memory forget handled=%v err=%v", handled, err)
	}
	if !strings.Contains(out, "forgot memory") {
		t.Fatalf("unexpected forget output: %s", out)
	}
	if app.a != nil {
		t.Fatal("memory deletion should invalidate active agent startup context")
	}
}

func hasService(all []plugins.ServiceStatus, name string) bool {
	for _, service := range all {
		if service.Name == name {
			return true
		}
	}
	return false
}

func hasMCPState(all []whalemcp.ServerState, name string) bool {
	for _, state := range all {
		if state.Name == name {
			return true
		}
	}
	return false
}

func hasHookSource(all []agent.HookListEntry, source string) bool {
	for _, hook := range all {
		if hook.Source == source {
			return true
		}
	}
	return false
}

func TestSkillsImproverListedOnceEnabled(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.Plugins = plugins.ConfigMap{"skills-improver": {Enabled: boolPtr(true)}}
	// A default-disabled plugin also needs the session capability the user
	// granted. Only the user-level config file produces that grant, so it has
	// to be written where New() reads the global layer (M1-40/M1-42).
	writeGlobalConfig(t, cfg.DataDir, "[plugins.skills-improver]\nenabled = true\n")
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()

	var got []string
	for _, st := range app.PluginStatuses() {
		got = append(got, st.Manifest.ID)
	}
	if !strings.Contains(strings.Join(got, "\n"), "skills-improver") {
		t.Fatalf("enabled skills-improver should be listed: %v", got)
	}
	if app.toolRegistry.Get("save_skill_proposal") == nil {
		t.Fatal("enabled skills-improver should register save_skill_proposal")
	}
}

func TestSkillsImproverUsesSharedPluginEnableConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	workspace := t.TempDir()

	// The /plugins toggle path every plugin shares must be the only entry
	// point: same [plugins.<id>] table, no bespoke switch (M1-39).
	loaded, _, err := SetPluginEnabledConfig(cfg, workspace, "skills-improver", true)
	if err != nil {
		t.Fatalf("SetPluginEnabledConfig: %v", err)
	}
	if cfgEntry, ok := loaded.Plugins["skills-improver"]; !ok || cfgEntry.Enabled == nil || !*cfgEntry.Enabled {
		t.Fatalf("expected runtime config to enable skills-improver, got %+v", loaded.Plugins)
	}
	file, _, err := LoadConfigFile(ProjectLocalConfigPath(workspace))
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if entry, ok := file.Plugins["skills-improver"]; !ok || entry.Enabled == nil || !*entry.Enabled {
		t.Fatalf("expected shared [plugins.skills-improver] table, got %+v", file.Plugins)
	}

	// The shared table alone cannot enable it: a project-level config may only
	// tighten the capability the user granted (M1-42/M1-43).
	listed := func(ctx plugins.Context, config plugins.ConfigMap) bool {
		for _, st := range plugins.NewManager(ctx, config).Statuses() {
			if st.Manifest.ID == "skills-improver" {
				return true
			}
		}
		return false
	}
	ctx := plugins.Context{DataDir: loaded.DataDir, WorkspaceRoot: workspace}
	if listed(ctx, loaded.Plugins) {
		t.Fatal("project-level enable config alone must not list skills-improver")
	}
	ctx.UserGrants = plugins.NewSessionGrants("skills-improver")
	if !listed(ctx, loaded.Plugins) {
		t.Fatal("shared enable config plus a user grant should list skills-improver")
	}
}

func writeGlobalConfig(t *testing.T, dataDir, body string) {
	t.Helper()
	if err := os.WriteFile(GlobalConfigPath(dataDir), []byte(body), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
}

// TestSubagentAndWorkflowAgentsDoNotInheritSkillsImproverGrant covers the two
// in-process spawn paths: subagents and workflow agents both take their tools
// and skills from the grant-less plugin resolution, while the interactive
// session keeps the granted one (M1-44).
func TestSubagentAndWorkflowAgentsDoNotInheritSkillsImproverGrant(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	workspace := t.TempDir()
	writeGlobalConfig(t, cfg.DataDir, "[plugins.skills-improver]\nenabled = true\n")
	cfg.Plugins = plugins.ConfigMap{"skills-improver": {Enabled: boolPtr(true)}}
	cfg.UserGrants = plugins.NewSessionGrants("skills-improver")

	init, err := initAppTools(cfg, StartOptions{}, workspace)
	if err != nil {
		t.Fatalf("initAppTools: %v", err)
	}

	// The interactive session holds the capability and registers the tool.
	if got := init.pluginManager.Outcome().Tools; !hasToolNamed(got, "save_skill_proposal") {
		t.Fatalf("interactive session should register save_skill_proposal: %v", toolNames(got))
	}
	// Subagents and workflow agents do not: same registry, no grant.
	if init.subagentToolRegistry.Get("save_skill_proposal") != nil {
		t.Fatal("subagent registry must not inherit save_skill_proposal")
	}
	for _, skill := range init.subagentPluginSkills {
		if skill.Name == "skills-improver" {
			t.Fatal("subagent skill set must not inherit the skills-improver skill")
		}
	}
	for _, definition := range init.subagentPluginAgents {
		if strings.Contains(definition.Name, "skills-improver") {
			t.Fatalf("subagent agent set must not inherit %q", definition.Name)
		}
	}
}

func hasToolNamed(tools []core.Tool, name string) bool {
	for _, tool := range tools {
		if tool != nil && tool.Name() == name {
			return true
		}
	}
	return false
}

func toolNames(tools []core.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool != nil {
			names = append(names, tool.Name())
		}
	}
	return names
}

// TestDangerouslySkipPermissionsCannotGrantSkillsImprover pins M1-45: the
// bypass flag only relaxes permission prompts, it never becomes the session
// grant or the confirmation an apply/restore needs.
func TestDangerouslySkipPermissionsCannotGrantSkillsImprover(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	workspace := t.TempDir()

	// Exactly what --dangerously-skip-permissions does in the CLI.
	cfg.PermissionDefault = "allow"
	cfg.AutoAcceptPermissions = true
	cfg.Plugins = plugins.ConfigMap{"skills-improver": {Enabled: boolPtr(true)}}

	manager := plugins.NewManager(plugins.Context{
		DataDir:       cfg.DataDir,
		WorkspaceRoot: workspace,
		UserGrants:    cfg.UserGrants,
	}, cfg.Plugins)
	if st, ok := manager.Status("skills-improver"); !ok || st.Enabled {
		t.Fatalf("bypass flag must not enable skills-improver: ok=%v status=%+v", ok, st)
	}
	for _, tool := range manager.Outcome().Tools {
		if tool != nil && tool.Name() == "save_skill_proposal" {
			t.Fatal("bypass flag must not register the proposal tool")
		}
	}
}

// TestSessionGrantHasExactlyOneSource blocks a future edit from deriving the
// capability from permission state or from the merged plugin config (M1-41).
func TestSessionGrantHasExactlyOneSource(t *testing.T) {
	entries, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	writers := []string{}
	for _, entry := range entries {
		if strings.HasSuffix(entry, "_test.go") {
			continue
		}
		data, err := os.ReadFile(entry)
		if err != nil {
			t.Fatalf("read %s: %v", entry, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.Contains(trimmed, ".UserGrants =") {
				writers = append(writers, entry)
			}
		}
	}
	if len(writers) != 1 || writers[0] != "config_apply.go" {
		t.Fatalf("the session grant must only be built in config_apply.go, got %v", writers)
	}
}
