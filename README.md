# pm

Launch profile manager for [Claude Code](https://claude.com/claude-code) and [Codex](https://github.com/openai/codex).

`pm` keeps multiple provider setups — endpoint, API key, model — as named profiles, and launches `claude` or `codex` with the right credentials for each one. Switch between a company gateway, a personal Anthropic key, AWS Bedrock, and your claude.ai subscription without editing environment variables or global settings files.

- **Interactive TUI** — bare `pm` opens a two-pane picker (profiles / models) with search and per-profile recent models
- **Secure key storage** — OS keychain, environment variable, shell command, or literal
- **Clean auth handoff** — the API key is delivered via a per-invocation `apiKeyHelper`, so your claude.ai login is untouched and Claude Code prints no auth-conflict warnings
- **Multiple subscription accounts** — `pm login <profile>` binds a claude.ai account to a profile via a long-lived OAuth token, so work and personal subscriptions can coexist on one machine
- **Per-directory model memory** — `pm` remembers which model you last used for each profile in each directory

## Install

Requires Go 1.26+ and `claude` (and/or `codex`) on your `PATH`.

```sh
go install github.com/boriswu0212/profile-manager/cmd/pm@latest
```

Or from a checkout:

```sh
git clone https://github.com/boriswu0212/profile-manager
cd profile-manager
go install ./cmd/pm
```

Prebuilt binaries for macOS and Linux (amd64/arm64) are attached to [GitHub Releases](https://github.com/boriswu0212/profile-manager/releases) — download the archive for your platform, extract, and put `pm` somewhere on your `PATH`. Binaries are unsigned; if macOS quarantines a browser download, clear it with `xattr -d com.apple.quarantine pm`.

## Quick start

```sh
pm add             # create a profile interactively
pm                 # open the TUI picker, Enter launches claude
pm run work        # or launch a profile directly
pm use work        # set the default profile
pm run             # launches the default profile
```

Pass arguments through to `claude` after `--`:

```sh
pm run work -- -p "explain this repo"
pm run work -m claude-opus-4-8 -- --continue
```

## Commands

| Command | Description |
|---|---|
| `pm` | Open the TUI profile/model picker |
| `pm add` | Add a profile interactively |
| `pm list` (`ls`) | List profiles (`*` marks the default) |
| `pm use <profile>` | Set the default profile |
| `pm run [profile] [-m model] [-- args...]` | Launch `claude`/`codex` with a profile; extra args go to the tool |
| `pm status` | Show the active profile and recent usage |
| `pm models [profile]` | List models available from the profile's endpoint |
| `pm login <profile> [--paste] [--account label]` | Bind a claude.ai subscription account to a profile (creates it if needed) |
| `pm logout <profile>` | Remove a profile's stored subscription token |
| `pm remove <profile>` (`rm`) | Remove a profile (and its `pm login` keychain entry, if any) |

All commands accept `--config <path>` to use a config file other than `~/.pm.yaml`. `pm --version` prints the build version.

## TUI

| Key | Action |
|---|---|
| `↑`/`k`, `↓`/`j` | Move (model list wraps around) |
| `←`/`h`, `→`/`l`/`m` | Switch between profile and model panes |
| `Enter` | Launch with the selected profile (and model, if in the model pane) |
| `/` | Search models |
| `s` | Set selected profile as default, or selected model as the profile's default |
| `d` | Delete the selected profile (profile pane only) |
| `q`, `Ctrl+C` | Quit |

While searching: type to filter, `↑`/`↓` to move, `Enter` to launch, `Ctrl+S` to set the filtered model as default, `Esc` to cancel.

The model pane pins up to three recently used models for the selected profile above the full list.

## Configuration

Profiles live in `~/.pm.yaml`. The file can contain literal API keys, so `pm` keeps it at mode `0600` and warns if the permissions are looser.

```yaml
default_profile: work
profiles:
  - name: work
    provider: anthropic
    base_url: https://gateway.example.com/v1
    api_key: keychain://work-gateway
    model: claude-sonnet-4-20250514
  - name: personal
    provider: subscription          # use the claude.ai login, no key needed
  - name: bedrock
    provider: bedrock
    region: us-east-1
    aws_profile: dev
    model: us.anthropic.claude-sonnet-4-20250514-v1:0
  - name: codex
    tool: codex
    provider: openai
    base_url: https://api.openai.com/v1
    api_key_cmd: op read op://vault/openai/key
    model: gpt-5.4-mini-codex
```

### Profile fields

| Field | Description |
|---|---|
| `name` | Profile identifier |
| `tool` | `claude` (default) or `codex` |
| `provider` | `anthropic`, `openai`, `bedrock`, or `subscription` |
| `base_url` | API endpoint (anthropic/openai). Keep the `/v1` suffix; `pm` strips it where Claude Code requires that |
| `api_key` | API key — see key sources below |
| `api_key_cmd` | Shell command whose stdout is the key (used when `api_key` is empty) |
| `model` | Default model for this profile |
| `region`, `aws_profile` | AWS settings (bedrock only) |
| `env_key` | Env var name the key is injected into for codex (default `PM_CODEX_API_KEY`) |

The same file also stores usage state maintained by `pm`: `recent` (last 10 profile+model launches, shown in the TUI and `pm status`) and `last_models` (per-directory last-used model, up to 50 entries).

### API key sources

`api_key` is interpreted by its form:

- `keychain://name` — OS keychain (macOS Keychain, Windows Credential Manager, or Linux Secret Service), service `pm`, account `name`. Seed it on macOS with `security add-generic-password -s pm -a name -w`
- `${ENV_VAR}` — read from the environment at launch time
- anything else — used literally

When `api_key` is empty, `api_key_cmd` is run via `sh -c` and its trimmed stdout becomes the key.

`bedrock` profiles need no key (AWS credentials are used). `subscription` profiles need none either — but can optionally hold an OAuth token bound by `pm login` (stored as `keychain://<profile>` in `api_key`), which pins the launch to a specific claude.ai account.

## Multiple subscription accounts

Claude Code itself keeps exactly one claude.ai login per machine (a single Keychain slot — logging into a second account overwrites the first). `pm` works around this with per-profile OAuth tokens:

```sh
pm login work        # browser signed into the work claude.ai account
pm login personal    # switch the browser to the personal account first
pm run work          # launches claude as the work account
pm run personal      # launches claude as the personal account
```

`pm login` runs `claude setup-token` (a browser OAuth flow that prints a 1-year token), stores the token in the OS keychain under `pm`'s own namespace, and injects it as `CLAUDE_CODE_OAUTH_TOKEN` at launch. Which account a login binds to is decided by whichever claude.ai account is active in your browser during the flow — use a second browser profile or a private window for the second account.

Notes:

- Requires a Pro, Max, Team, or Enterprise plan (`claude setup-token` rejects free accounts).
- Running `setup-token` again for the same account invalidates that account's previous token. To reuse a token minted elsewhere (e.g. on another machine), store it with `pm login <profile> --paste`.
- A subscription profile without a token keeps the old behavior: it uses the machine's shared claude.ai login.
- Rate limits are per account, not per profile.
- Token-authed sessions show a "Claude API" launch banner (and `claude auth status` reports `authMethod: oauth_token`) instead of the plan name. This is cosmetic — an `sk-ant-oat…` token always bills the subscription it belongs to, never pay-per-token API. Verify with `/usage` inside the session: plan-style rate-limit windows mean subscription billing.
- Claude Code's UI can't tell you *which* token a session runs on (setup-tokens carry only `user:inference` scope, so even the API refuses to identify them). pm compensates at launch: every run prints `pm ▸ profile "personal" · claude · subscription (you@example.com · token ab12cd34)` above the banner and exports `PM_PROFILE` for in-session checks (`!echo $PM_PROFILE`). The account label is what you declare at `pm login` (prompted, or `--account you@example.com`); the fingerprint is a live sha256 prefix of the token actually being used.
- Re-running `pm login` on a profile tells you exactly what happened to the token: `Token 5bf470b9 stored — replaces ccf4495b` (or `unchanged, same token as before`). `pm status` and `pm list` show the fingerprint, account label, and bind date.

## Model selection

The model for a launch is chosen in this order:

1. `-m`/`--model` flag (or the model picked in the TUI)
2. The model you last used for this profile **in the current directory**
3. The profile's `model` field

Every launch records its model per (directory, profile) in `~/.pm.yaml`, and that remembered model **becomes the effective default** the next time you run the same profile from the same directory — it takes precedence over the profile's `model` field until you override it again with `-m` or the TUI. Each project directory therefore sticks to its own model per profile.

## How launching works

`pm run` resolves credentials, prepares the environment, and replaces itself with `claude`/`codex` (`exec`), so there is no wrapper process left running.

- **anthropic / openai** — sets `ANTHROPIC_BASE_URL` (with any trailing `/v1` stripped, since Claude Code appends its own) and passes the key via `claude --settings '{"apiKeyHelper": ...}'`, along with `disableClaudeAiConnectors: true` (connectors can't work through a gateway). The helper is the hidden `pm _resolve-key <profile>` command. `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN` are cleared from the environment rather than set, and the settings only apply to that invocation — other sessions keep their claude.ai login. Note: any pre-existing `apiKeyHelper` entry in `~/.claude/settings.json` is removed before launch.
- **bedrock** — clears `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, and `ANTHROPIC_BASE_URL`, then sets `CLAUDE_CODE_USE_BEDROCK=1`, `AWS_REGION`, and `AWS_PROFILE`.
- **subscription** — clears `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL`, and Bedrock/Vertex overrides. With a token bound via `pm login`, sets `CLAUDE_CODE_OAUTH_TOKEN` so `claude` runs as that account; otherwise it also clears that variable and `claude` uses your claude.ai login.
- **codex** (`tool: codex`) — writes `~/.codex/config.toml` with a `pm` provider pointing at the profile's `base_url`, and injects the key via the `env_key` env var. Note: the previous `config.toml` is not restored after codex exits; keep a copy if you maintain one by hand.

## Development

```sh
go test ./...
go vet ./...
go build -o pm ./cmd/pm
```

See [CLAUDE.md](CLAUDE.md) for architecture notes.

## License

[MIT](LICENSE). `pm` only manages launch profiles and hands credentials to `claude`/`codex` (its sole API use is listing models for the picker) — it is provided as-is, without warranty of any kind.
