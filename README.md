# all-usage

Codex, Kiro, and Cursor subscription usage at a glance, in the terminal.

```
 ◆ all-usage                                         ↻ 56s · updated 21:06

╭────────────────────────────────────────────────────────────────────────╮
│ ● Codex                                                           Plus │
│                                                                        │
│ 5-hour limit                                                       34% │
│ ━━━━━━━━━━━━━━━━━━━━━━━╸────────────────────────────────────────────── │
│ resets in 2h 14m                                                       │
│                                                                        │
│ Weekly limit                                                       61% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸─────────────────────────── │
│ resets in 3d 5h                                                        │
│                                                                        │
│ chatgpt.com API · just now                                             │
╰────────────────────────────────────────────────────────────────────────╯

╭────────────────────────────────────────────────────────────────────────╮
│ ● Kiro                                                        Kiro Pro │
│                                                                        │
│ Credits                                                          41.6% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸──────────────────────────────────────── │
│ 416.11 / 1,000 credits                                                 │
│ resets in 27d                                                          │
│                                                                        │
│ kiro-cli login · just now                                              │
╰────────────────────────────────────────────────────────────────────────╯

╭────────────────────────────────────────────────────────────────────────╮
│ ● Cursor                                                           Pro │
│                                                                        │
│ Total usage                                                      72.4% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸─────────────────── │
│ resets in 12d                                                          │
│ $14.48 spent                                                           │
│                                                                        │
│ Auto + Composer                                                  58.3% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸───────────────────────────── │
│ resets in 12d                                                          │
│                                                                        │
│ API models                                                       91.2% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸────── │
│ resets in 12d                                                          │
│                                                                        │
│ Cursor IDE login · just now                                            │
╰────────────────────────────────────────────────────────────────────────╯

 r refresh • c compact • t theme • u used/left • ? more keys • q quit
```

<sub>Sample data, `mono` theme. In a terminal, meters turn green, yellow, or red based on usage.</sub>

- No setup required. It finds logins from the official apps on its own (Codex CLI, kiro-cli, Kiro IDE, Cursor IDE, and cursor-agent). On WSL it also finds the Windows apps.
- One screen for Codex's 5-hour and weekly windows, Kiro credits, and Cursor's included usage, with time left until each limit resets.
- A dashboard with auto-refresh, 10 themes, compact mode, used or remaining percent, and a layout that fits the terminal width.
- Script-friendly output: a table, a one-line status bar, JSON, and Go templates.
- Configurable with a commented TOML file, environment variables, and flags.
- Read-only: it never refreshes or modifies your credentials.

## Install

Requires Go 1.24 or later.

```sh
go install github.com/cfardev/all-usage@latest
```

From a clone:

```sh
go build -o ~/.local/bin/all-usage .

# with a version number
go build -ldflags "-s -w -X github.com/cfardev/all-usage/internal/buildinfo.Version=v0.1.0" \
  -o ~/.local/bin/all-usage .
```

Pure Go (no CGO), so cross-compile with `GOOS`/`GOARCH`, for example `GOOS=darwin GOARCH=arm64 go build .`. Tested on Linux and WSL. It builds on macOS and Windows and looks for credentials in the usual paths there, but those platforms are not tested yet.

## Usage

```sh
all-usage                  # live dashboard
all-usage -p codex,kiro    # only those providers, in that order
all-usage -r 30s           # refresh every 30s (0 disables auto-refresh)
all-usage --theme nord     # another theme (all-usage themes lists them)
all-usage show             # print once and exit
all-usage doctor           # what was detected, and how to fix problems
```

### Dashboard

| Key | Action |
|---|---|
| `r` / `F5` | Refresh now |
| `c` | Compact mode |
| `t` | Cycle theme |
| `u` | Toggle used % / remaining % |
| `↑` `↓` / `k` `j`, `PgUp` `PgDn`, `Ctrl+u` `Ctrl+d` | Scroll when the dashboard does not fit |
| `?` | Show every key |
| `q` / `Esc` / `Ctrl+c` | Quit |

Cards wrap into as many columns as fit. The border turns yellow or red when a meter crosses `warn_at` or `critical_at`. A failed refresh keeps the last good data and shows the error. When the data is not live, the card shows how old it is (`as of …`).

Compact mode (`c` or `--compact`):

```
 ◆ all-usage                                         ↻ 55s · updated 21:06

╭────────────────────────────────────────────────────────────────────────╮
│ ● Codex                                                           Plus │
│ 5-hour limit    ━━━━━━━━━━━━━━╸───────────────────────────   34% 2h14m │
│ Weekly limit    ━━━━━━━━━━━━━━━━━━━━━━━━━╸────────────────   61%  3d5h │
│                                                                        │
│ chatgpt.com API · just now                                             │
╰────────────────────────────────────────────────────────────────────────╯

╭────────────────────────────────────────────────────────────────────────╮
│ ● Kiro                                                        Kiro Pro │
│ Credits         ━━━━━━━━━━━━━━━━━╸──────────────────────── 41.6%   27d │
│                                                                        │
│ kiro-cli login · just now                                              │
╰────────────────────────────────────────────────────────────────────────╯

╭────────────────────────────────────────────────────────────────────────╮
│ ● Cursor                                                           Pro │
│ Total usage     ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸─────────── 72.4%   12d │
│ Auto + Composer ━━━━━━━━━━━━━━━━━━━━━━━━╸───────────────── 58.3%   12d │
│ API models      ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸─── 91.2%   12d │
│                                                                        │
│ Cursor IDE login · just now                                            │
╰────────────────────────────────────────────────────────────────────────╯

 r refresh • c compact • t theme • u used/left • ? more keys • q quit
```

### Without the TUI: `all-usage show`

Prints usage once. This is what `all-usage` does when stdout is not a terminal.

```
● Codex  Plus  chatgpt.com API · 0.4s
  5-hour limit     ━━━━━━╸─────────────    34%  resets in 2h 14m
  Weekly limit     ━━━━━━━━━━━━╸───────    61%  resets in 3d 5h

● Kiro  Kiro Pro  kiro-cli login · 0.3s
  Credits          ━━━━━━━━╸───────────  41.6%  416.11 / 1,000 credits · resets in 27d

● Cursor  Pro  Cursor IDE login · 0.3s
  Total usage      ━━━━━━━━━━━━━━╸─────  72.4%  resets in 12d · $14.48 spent
  Auto + Composer  ━━━━━━━━━━━╸────────  58.3%  resets in 12d
  API models       ━━━━━━━━━━━━━━━━━━╸─  91.2%  resets in 12d
```

| Option | Output |
|---|---|
| `-f table` (default) | The table above |
| `-f short` | One line for status bars: `Codex 5h 34% · wk 61% \| Kiro 41.6% \| Cursor 72.4%` |
| `--json` | JSON for scripts |
| `-t '<template>'` | Custom format with a Go `text/template` |

In `short`, `*` marks data that is not live and `✗` marks a provider that could not be read. `show` exits 1 if it could not read any provider. `doctor` exits 1 if any provider fails.

Templates receive `.Now` and `.Providers`. Each provider has `.ID`, `.Name`, `.Plan`, `.Account`, `.Source`, `.OK`, `.Stale`, `.Error`, `.Hint`, `.Meters`, and `.Headline` (its primary meters). Each meter has `.ID`, `.Label`, `.Short`, `.Value`, `.Detail`, `.Percent`, `.Used`, `.Limit`, `.Unit`, and `.ResetsAt`. Functions: `pct`, `used`, `left`, `reset`, `bar <meter> <width>`, `money`, `num`, `upper`, `lower`, and `join`.

```sh
all-usage show -t '{{range .Providers}}{{.Name}}{{range .Headline}} {{pct .}}{{end}}  {{end}}'
# Codex 34% 61%  Kiro 41.6%  Cursor 72.4%

all-usage show --json | jq -r '.providers[] | select(.ok) | "\(.name): \(.usage.meters[0].percent // 0 | floor)%"'
# Codex: 34%
# Kiro: 41%
# Cursor: 72%
```

### Shell completion

```sh
source <(all-usage completion bash)    # or zsh; for fish: all-usage completion fish | source
```

`all-usage completion <shell> --help` explains how to install it permanently.

## Configuration

Everything is optional. `all-usage init` writes a file with every option commented and set to its default:

```sh
all-usage init          # creates ~/.config/all-usage/config.toml
all-usage config edit   # opens it in $VISUAL/$EDITOR and validates on save
all-usage config show   # effective config (defaults + file + env + flags)
all-usage config path   # where the file is
all-usage themes        # preview the themes
```

The file lives at `$XDG_CONFIG_HOME/all-usage/config.toml` (`~/.config/all-usage/config.toml`, including on macOS) or `%APPDATA%\all-usage\config.toml` on Windows. Point at another file with `-c` or `ALL_USAGE_CONFIG`.

Precedence: built-in defaults < file < `ALL_USAGE_*` environment variables < flags. Unknown keys produce a warning. Invalid values produce one error that lists every problem.

Example:

```toml
refresh_interval = "30s"
order = ["cursor", "codex", "kiro"]

[ui]
theme = "catppuccin"
percent = "remaining"   # what you have left, instead of what you used
reset_format = "both"   # "in 2h 5m · 14:00"
warn_at = 60.0
critical_at = 85.0

[ui.colors]
accent = "#FF79C6"

[codex]
meters = ["primary", "secondary"]   # hide credits and per-model limits

[kiro]
display_name = "Kiro (work)"

[cursor]
meters = ["total"]   # total only; enabled = false hides the provider
```

### General and UI options

| Key | Default | Description |
|---|---|---|
| `refresh_interval` | `"1m"` | How often the dashboard refreshes (`"30s"`, `"5m"`). `0` disables it; minimum 5s. |
| `timeout` | `"20s"` | Maximum time per provider on each refresh. |
| `order` | `["codex", "kiro", "cursor"]` | Card order. |
| `scan_windows` | `true` | On WSL, also look for Windows app logins. |
| `windows_home` | `""` | Windows profile to use instead of detecting one (for example `/mnt/c/Users/ana`). |
| `ui.theme` | `"auto"` | `auto`, `dark`, `light`, `dracula`, `nord`, `catppuccin`, `gruvbox`, `tokyonight`, `material-ocean`, `mono`. |
| `ui.layout` | `"auto"` | `auto` (as many columns as fit), `columns` (one row), `rows` (one column). |
| `ui.columns` | `0` | Fixed column count (0 = automatic). |
| `ui.card_width` | `36` | Minimum card width for the automatic layout. |
| `ui.compact` | `false` | One line per meter. |
| `ui.bar_style` | `"blocks"` | `blocks`, `line`, `ascii`, `dots`. |
| `ui.percent` | `"used"` | `used` or `remaining`. |
| `ui.reset_format` | `"relative"` | `relative` (`in 2h 5m`), `absolute` (`Tue 14:00`), or `both`. |
| `ui.clock` | `"24h"` | `24h` or `12h`. |
| `ui.warn_at` / `ui.critical_at` | `70` / `90` | Used % at which a meter turns yellow / red. |
| `ui.show_source` | `true` | Show where the data came from and how old it is. |
| `ui.show_account` | `false` | Show each account's email. |
| `ui.show_help` | `true` | Show the key hint at the bottom. |
| `ui.color` | `"auto"` | `auto` (terminals only), `always`, or `never`. Honors `NO_COLOR`. |
| `ui.colors.*` | `""` | Override theme colors: `accent`, `text`, `muted`, `border`, `ok`, `warn`, `critical`, `bar_empty`. Accepts `"#RRGGBB"`, `"#RGB"`, or an ANSI index `"0"`–`"255"`. |

### Per-provider options

`[codex]`, `[kiro]`, and `[cursor]` share these keys:

| Key | Default | Description |
|---|---|---|
| `enabled` | `true` | Show the provider. |
| `display_name` | `"Codex"`, `"Kiro"`, `"Cursor"` | Name on the card. |
| `color` | `"#10A37F"`, `"#9D6CFF"`, `"#4C9AFF"` | Color of the card's dot. |
| `meters` | `[]` | Meter IDs to show, in that order (empty = all). |
| `source` | `"auto"` | Where to read data from (see below). |

And these are specific to each provider:

| Key | Default | Description |
|---|---|---|
| `codex.source` | `"auto"` | `auto` tries the API with your login, then `codex app-server`, then the latest session log. Force one with `api`, `app-server`, or `sessions`. |
| `codex.home` | `""` | Codex directory (default `$CODEX_HOME` or `~/.codex`). |
| `codex.binary` | `"codex"` | Codex executable. |
| `codex.use_app_server` | `true` | Use `codex app-server` when the API fails. |
| `codex.sessions_fallback` | `true` | Show the latest session-log snapshot when live data is unavailable. |
| `codex.base_url` | `"https://chatgpt.com/backend-api"` | API URL. |
| `kiro.source` | `"auto"` | `auto` tries kiro-cli, then the Kiro IDE. Force one with `cli` or `ide`. |
| `kiro.db_path` | `""` | kiro-cli database. |
| `kiro.ide_token_file` | `""` | Kiro IDE token file. |
| `kiro.binary` | `"kiro-cli"` | kiro-cli executable. |
| `kiro.refresh_with_cli` | `true` | Run `kiro-cli whoami` so an expired token gets refreshed. |
| `kiro.region`, `kiro.endpoint`, `kiro.profile_arn` | `""` | Overrides. Defaults come from your Kiro profile. |
| `cursor.source` | `"auto"` | `auto` tries the Cursor IDE, then cursor-agent. Force one with `ide` or `cli`. |
| `cursor.state_db` | `""` | IDE database (`state.vscdb`). |
| `cursor.auth_file` | `""` | cursor-agent login file. |
| `cursor.api_base`, `cursor.web_base` | `"https://api2.cursor.sh"`, `"https://cursor.com"` | API URLs. |

Meter IDs for `meters` (`all-usage show --json` prints each meter's `id`):

| Provider | IDs |
|---|---|
| `codex` | `primary` (5-hour), `secondary` (weekly), `credits`, and for extra limits such as a model, `<limit_id>.primary` / `<limit_id>.secondary` |
| `kiro` | `credit`, `credit.bonus`, `credit.free_trial`, `credit.overage` |
| `cursor` | `total`, `auto`, `api`, `on_demand`, `team_on_demand`, `requests` |

### Environment variables

| Variable | Effect |
|---|---|
| `ALL_USAGE_CONFIG` | Config file path. |
| `ALL_USAGE_PROVIDERS` | Providers to show, in order: `codex,cursor`. |
| `ALL_USAGE_REFRESH` | Refresh interval: `30s`. |
| `ALL_USAGE_TIMEOUT` | Per-provider timeout. |
| `ALL_USAGE_THEME` | Theme. |
| `ALL_USAGE_WINDOWS_HOME` | Windows profile to search from WSL. |
| `ALL_USAGE_CODEX_TOKEN` (+ `ALL_USAGE_CODEX_ACCOUNT_ID`) | Use this Codex token instead of discovering one. |
| `ALL_USAGE_KIRO_TOKEN` (+ `ALL_USAGE_KIRO_PROFILE_ARN`) | Use this Kiro token instead of discovering one. |
| `ALL_USAGE_CURSOR_TOKEN` | Use this Cursor token instead of discovering one. |
| `NO_COLOR` | Disable colors. |

## Where the data comes from

Codex
- Credentials: `auth.json` in `$CODEX_HOME` or `~/.codex`, and from WSL also `/mnt/c/Users/<user>/.codex`.
- Source: `GET https://chatgpt.com/backend-api/wham/usage`, the same endpoint Codex uses for its limits.
- If the login has expired, it asks `codex app-server`, which refreshes its own login. If that fails too, it shows the latest snapshot in `~/.codex/sessions`, marked stale.
- Shows the 5-hour window, the weekly window, extra limits (for example per model), and credits.

Kiro
- Credentials: the kiro-cli database (`~/.local/share/kiro-cli/data.sqlite3`; on macOS, `~/Library/Application Support/kiro-cli/`) or the Kiro IDE token (`~/.aws/sso/cache/kiro-auth-token.json`).
- Source: `GET https://q.<region>.amazonaws.com/getUsageLimits`, Kiro's usage-limits API.
- kiro-cli tokens last about an hour. When one is expired, all-usage runs `kiro-cli whoami`, which lets the official client refresh it, then reads it again.
- Shows the month's credits (used / limit) and the reset. Also bonuses, free trial, and overage when they exist.

Cursor
- Credentials: the Cursor IDE database (`state.vscdb` under `~/.config/Cursor/User/globalStorage/`, `~/Library/Application Support/Cursor/User/globalStorage/`, or `%APPDATA%\Cursor\User\globalStorage\`; from WSL, the Windows one) or the cursor-agent login (`~/.config/cursor/auth.json`). The database is opened read-only with point queries, so reading stays fast even when the file is several GB.
- Source: `https://cursor.com/api/usage-summary`. If that fails, the IDE API (`api2.cursor.sh`, `DashboardService`).
- Shows the percent of included usage in the billing cycle (total, Auto + Composer, and API models), spend, on-demand, and, on older plans, premium requests.

Privacy and security
- Read-only: it never writes, refreshes, or copies credentials. Refreshing a token itself can invalidate the refresh token and sign you out of the official app, so refresh is left to the official clients.
- Each token is sent only to the service that issued it (OpenAI, AWS, or Cursor). There is no telemetry and no other server.
- `doctor` never prints tokens. Emails stay hidden unless `--show-account` or `ui.show_account = true`.

## Status bars

tmux:

```tmux
set -g status-interval 60
set -g status-right '#(all-usage show -f short)'
```

Waybar:

```json
"custom/ai-usage": {
  "exec": "all-usage show -f short",
  "interval": 120
}
```

## Troubleshooting

- Start with `all-usage doctor`: it lists each login it found, whether it is expired, and tests each provider.
- `Session expired`: sign in again with `codex login` or `kiro-cli login`, or open Cursor or run `cursor-agent login`, depending on the provider.
- Codex shows `as of …` or a `*`: your Codex login expired and you are seeing the latest session-log snapshot. `codex login` fixes it.
- WSL: profiles under `/mnt/c/Users` are detected automatically. Pin one with `windows_home` (or `ALL_USAGE_WINDOWS_HOME`); turn the search off with `scan_windows = false`.
- Colors: they are omitted when stdout is not a terminal. `ui.color = "always"` forces them; `--no-color` or `NO_COLOR` removes them.

## Development

```sh
go test -race ./...
go vet ./...
gofmt -l .
```

```
main.go
internal/
  cli/        commands (cobra)
  config/     schema, defaults, and the commented template
  core/       shared model: meters, usage, errors, and concurrent fetch
  providers/  codex/, kiro/, and cursor/
  tui/        dashboard (Bubble Tea + Lip Gloss)
  render/     table, short, json, and template output
  ui/         themes, bars, and formatting
  sys/        OS paths, WSL, read-only SQLite, and JWT
```

To add a provider:

1. Implement `core.Provider` (`ID`, `Name`, `Fetch`, and `Doctor`) in `internal/providers/<name>`.
2. Register it in `internal/providers/providers.go`.
3. Add its section in `internal/config`: the struct, defaults, `AllProviders`, `Common`, `SelectProviders`, `Validate`, and the template. A test fails if the template drifts from the defaults.

## Disclaimer

all-usage is not an official product and is not affiliated with OpenAI, AWS, Kiro, or Cursor. It uses undocumented internal APIs that can change without notice.
