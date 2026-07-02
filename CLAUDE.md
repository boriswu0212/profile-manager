# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`pm` is a Go CLI that manages launch profiles (endpoint, API key, model) for Claude Code and Codex. Bare `pm` opens a bubbletea TUI (profile/model picker); `pm run [profile] [-- claude-args...]` resolves credentials and replaces the process with `claude` (or `codex`) via `syscall.Exec`.

## Commands

```sh
go test ./...                                   # all tests
go test ./internal/tui/ -run TestName -v       # single test
go vet ./...
go build -o pm ./cmd/pm                       # repo-root binary (used for stub E2E)
go install ./cmd/pm                             # REQUIRED after changes: the `pm` on PATH
                                                # is mise's go bin copy, not ./pm
```

There is no lint config beyond `go vet`.

Releases are tag-driven: pushing a `v*` tag runs `.github/workflows/release.yml` (GoReleaser; darwin/linux × amd64/arm64, version injected into `main.version` via ldflags). Merges to main only run CI. Version tags are semver `v0.x.y`: while pre-1.0, breaking CLI/config changes bump minor and fixes bump patch. Never tag `v2.0.0`+ without adding the `/v2` module-path suffix Go requires.

## Architecture

Flow: `cmd/pm/main.go` → `cmd/` (cobra) → `internal/runner` → exec `claude`/`codex`.

- `cmd/` — one file per subcommand (`run`, `add`, `use`, `remove`, `list`, `status`, `models`, hidden `_resolve-key`). Bare root command starts the TUI. `run` splits claude passthrough args with `cmd.ArgsLenAtDash()` — cobra strips the literal `--`, so never scan `args` for it.
- `internal/config` — `~/.pm.yaml` (must stay 0600). `Profile.Provider` ∈ `anthropic | openai | bedrock | subscription`; `Tool` ∈ `claude | codex`. API keys: `api_key` is interpreted as `keychain://name` (service `pm`), `${ENV_VAR}`, or a literal; `api_key_cmd` (shell command) is used only when `api_key` is empty. `env_key` names the env var a codex launch injects the key into (default `PM_CODEX_API_KEY`). The same file stores usage state: `recent` (feeds the TUI Recent section, deduped per profile+model, cap 10) and `last_models` (per-directory last-used model, deduped per dir+profile, cap 50).
- `internal/runner` — builds env vars and flags, then `syscall.Exec` replaces the process: code after exec never runs, so cleanup cannot rely on defers (this is why `apiKeyHelper` is passed via `--settings` instead of written to `~/.claude/settings.json`). Two persistent side effects exist by design: launch strips any pre-existing `apiKeyHelper` from `~/.claude/settings.json`, and `tool: codex` launches rewrite `~/.codex/config.toml` without restoring it after exit. Model precedence lives in `applyModelAndRecord`: `-m` flag > `last_models[(cwd, profile)]` > `profile.Model`; every launch records both recent and last-model.
- `internal/provider` — `ListModels` implementations (anthropic, openai, bedrock) for the TUI and `pm models`. `apiBase()` tolerates base URLs with or without a trailing `/v1`.
- `internal/tui` — bubbletea two-pane picker (profiles / models) with search (`/`), set-default (`s`, `Ctrl+S` while searching), delete profile (`d`), and wrap-around model selection. On Enter it quits and calls `runner.Run`.

Behavioral invariants for the runner and TUI are in `.claude/rules/` (loaded automatically).
