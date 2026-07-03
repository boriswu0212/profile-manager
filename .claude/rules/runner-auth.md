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
  (per-invocation flags over persistent settings writes). Windows has no
  exec: `execProcess` emulates it by ignoring `os.Interrupt` (which also
  cancels any pre-exec `signal.Notify` handler, matching how exec would wipe
  it), waiting on a child process, and `os.Exit`-ing with its code — so the
  same "nothing after it runs" rule holds on both platforms.
- **`CLAUDE_CODE_OAUTH_TOKEN` is the one credential pm deliberately exports —
  subscription profiles only.** It is Claude Code's documented subscription
  auth (below `ANTHROPIC_AUTH_TOKEN`/`ANTHROPIC_API_KEY`/`apiKeyHelper` in
  credential precedence, which is why `runSubscription` clears all of those
  and strips stale `apiKeyHelper` from settings). Resolve it with
  `config.ResolveOAuthToken`, never `ResolveAPIKey` — the latter must keep
  returning nothing for subscription profiles so the hidden `pm _resolve-key`
  can never print an OAuth token. Every non-subscription path (and a
  subscription profile with no token bound) must actively `Unsetenv` it so a
  token exported in the shell can't hijack a launch. Upstream caveats:
  claude-code #37512 reported (v2.1.81, closed not-planned) that launching
  with this env var can delete the shared `Claude Code-credentials` Keychain
  login on exit — not reproduced on 2.1.199 (entry survived interactive and
  `claude auth status` runs); re-verify on claude upgrades. Headless `-p`
  runs don't refresh the token's embedded access token (#28827), so long
  headless sessions can 401 while interactive ones are fine. Cosmetic:
  token-authed sessions show a "Claude API" launch banner and
  `authMethod: oauth_token` in `claude auth status` instead of the
  subscription name — usage still bills the subscription; only
  `ANTHROPIC_API_KEY` (which the runner clears) would cause pay-per-token
  billing.
