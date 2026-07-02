---
paths:
  - "internal/runner/**"
---

# Runner: auth and base URL invariants

- **Never export `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` when launching
  claude.** Either one alongside a claude.ai login makes Claude Code print an
  auth-conflict warning banner on every start ("Both claude.ai and X set" /
  "X overriding Claude subscription login"). The key is delivered through
  `--settings '{"apiKeyHelper":"<pm> _resolve-key <profile>", ...}'` —
  apiKeyHelper is the only auth source Claude Code exempts from those
  warnings, and the `--settings` flag is per-invocation so pm's own key never
  persists into `~/.claude/settings.json` and other sessions keep their
  claude.ai login. (One persistent write does happen: any pre-existing
  `apiKeyHelper` in `~/.claude/settings.json` is stripped before launch so a
  stale helper can't shadow the per-invocation one.) `disableClaudeAiConnectors: true` rides along to silence the
  "connectors are disabled" notice (they can't work through a gateway anyway).
- **`ANTHROPIC_BASE_URL` must not end in `/v1`.** Claude Code's SDK appends
  `/v1/messages` itself; a base URL with `/v1` produces `/v1/v1/...`, which
  API gateways reject (often as a misleading 401 "token lacks permission to
  access the API"). Profiles keep OpenAI-convention URLs *with* `/v1` because
  codex requires them — `anthropicBaseURL()` strips it for claude only. Keep
  the two conventions separate.
- **`syscall.Exec` replaces the process.** No defer, signal handler, or
  restore logic placed after it will ever run. Any state that must be undone
  after claude exits cannot be undone here — design so nothing needs undoing
  (per-invocation flags over persistent settings writes).
