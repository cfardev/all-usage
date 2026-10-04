package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Template is the commented configuration written by `all-usage init`. Every
// value equals the built-in default (enforced by a test), so the file only
// needs editing where you want something different.
const Template = `# all-usage configuration
# Precedence: built-in defaults < this file < ALL_USAGE_* env vars < CLI flags.
# Run "all-usage doctor" to check what is detected and why.

# How often the dashboard refreshes ("30s", "5m"; 0 disables auto-refresh; min 5s).
refresh_interval = "1m"
# Maximum time to wait for one provider on each refresh.
timeout = "20s"
# Display order. Providers not listed are shown after these, if enabled.
order = ["codex", "kiro", "cursor"]
# Under WSL, also look for credentials of Windows apps (e.g. /mnt/c/Users/<you>).
scan_windows = true
# Windows profile dir to use instead of auto-detecting it (WSL only).
windows_home = ""

[ui]
# auto | dark | light | dracula | nord | catppuccin | gruvbox | tokyonight | mono
theme = "auto"
# auto (fit as many columns as possible) | columns (one row) | rows (one column)
layout = "auto"
# Fixed number of columns (0 = automatic).
columns = 0
# Minimum card width used by the automatic layout.
card_width = 36
# One line per meter instead of label/bar/reset blocks.
compact = false
# Progress bar style: blocks | line | ascii | dots
bar_style = "blocks"
# Show percentages as "used" or "remaining".
percent = "used"
# Reset times: relative ("in 2h 5m") | absolute ("Tue 14:00") | both
reset_format = "relative"
# Clock for absolute times: 24h | 12h
clock = "24h"
# Usage percentage at which meters turn yellow / red.
warn_at = 70.0
critical_at = 90.0
# Show where data came from and when it was fetched.
show_source = true
# Show account e-mails (off by default so screenshots stay private).
show_account = false
# Show the key help line at the bottom of the dashboard.
show_help = true
# Colors: auto (only on terminals) | always | never. NO_COLOR is honored.
color = "auto"

# Override individual theme colors: "#RRGGBB", "#RGB" or ANSI index "0"-"255".
[ui.colors]
accent = ""
text = ""
muted = ""
border = ""
ok = ""
warn = ""
critical = ""
bar_empty = ""

[codex]
enabled = true
display_name = "Codex"
color = "#10A37F"
# Meters to show, in order (empty = all). IDs: primary (5h), secondary (weekly),
# credits, and "<limit_id>.primary"/"<limit_id>.secondary" for extra model limits.
meters = []
# auto: API with your local login -> "codex app-server" -> latest session log.
# Or force one of: api | app-server | sessions
source = "auto"
# Codex home (default: $CODEX_HOME or ~/.codex; Windows homes are added under WSL).
home = ""
# Codex executable, used for the app-server source.
binary = "codex"
# Let the official Codex app-server fetch usage (it refreshes expired logins itself).
use_app_server = true
# When live data is unavailable, show the last limits recorded in session logs.
sessions_fallback = true
base_url = "https://chatgpt.com/backend-api"

[kiro]
enabled = true
display_name = "Kiro"
color = "#9D6CFF"
# Meters to show (empty = all). IDs: credit, credit.bonus, credit.free_trial, credit.overage
meters = []
# auto: kiro-cli login, then Kiro IDE login. Or force: cli | ide
source = "auto"
# kiro-cli database (default: ~/.local/share/kiro-cli/data.sqlite3 or OS equivalent).
db_path = ""
# Kiro IDE token (default: ~/.aws/sso/cache/kiro-auth-token.json).
ide_token_file = ""
# kiro-cli executable, used to refresh an expired kiro-cli login.
binary = "kiro-cli"
# Run "kiro-cli whoami" (official client) to refresh an expired kiro-cli token.
refresh_with_cli = true
# API region (default: from your Kiro profile ARN) and endpoint override.
region = ""
endpoint = ""
# Profile ARN override (default: the profile selected in kiro-cli).
profile_arn = ""

[cursor]
enabled = true
display_name = "Cursor"
color = "#4C9AFF"
# Meters to show (empty = all). IDs: total, auto, api, on_demand, team_on_demand, requests
meters = []
# auto: Cursor IDE login, then cursor-agent CLI login. Or force: ide | cli
source = "auto"
# Cursor IDE state database (default: <app data>/Cursor/User/globalStorage/state.vscdb).
state_db = ""
# cursor-agent credentials (default: ~/.config/cursor/auth.json).
auth_file = ""
api_base = "https://api2.cursor.sh"
web_base = "https://cursor.com"
`

// WriteTemplate writes Template to path, creating parent directories. It
// refuses to overwrite an existing file unless force is set.
func WriteTemplate(path string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(Template), 0o644)
}

// Encode renders the effective configuration as TOML.
func (c *Config) Encode() (string, error) {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(c); err != nil {
		return "", err
	}
	return buf.String(), nil
}
