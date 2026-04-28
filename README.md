# dttui

A small Go CLI / TUI for searching Dynatrace logs with DQL.

- `dttui query "<DQL>"` — run a query, print result records as JSON.
- `dttui` — open the interactive TUI: multi-line DQL editor, results table,
  JSON detail view, saved searches, parameter templates, JSON/CSV export.

## Getting started

### 1. Create a Platform Token

Sign in to <https://myaccount.dynatrace.com/platformTokens> and create a new
Platform Token with these scopes:

- `storage:logs:read`
- `storage:buckets:read`

Copy the generated token — it looks like `dt0s16.XXXXXXXX.YYYY...`. You only
see it once.

### 2. Write a config file

Create `~/.config/dynatrace-tui/config.yaml`:

```yaml
environment_id: abc12345    # the prefix in https://<env-id>.apps.dynatrace.com
platform_token: dt0s16.XXXXXXXX.YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY
```

Tighten permissions since the file holds a secret:

```sh
chmod 600 ~/.config/dynatrace-tui/config.yaml
```

### 3. Install and run

```sh
go install .
dttui query 'fetch logs, from:now()-15m | limit 5'
```

## CLI

```
dttui query [flags] <DQL>
```

| Flag | Description |
| --- | --- |
| `-t, --timeframe` | Convenience preset (`15m`, `1h`, `6h`, `24h`). Injected as `from:now()-<tf>` if the query doesn't already have a `from:` clause. |
| `--config` | Path to config file (default `~/.config/dynatrace-tui/config.yaml`). |

Output is a JSON array of records on stdout; diagnostics on stderr. `Ctrl-C`
during a running query calls `query:cancel` server-side before exiting.

## TUI

Run `dttui` with no arguments. The screen has an editor pane on top and a
results pane below.

| Key | Action |
| --- | --- |
| `Alt-Enter` / `Ctrl-Enter`* | Run the current query |
| `Tab` / `Shift-Tab` | Cycle focus between editor and results |
| `↑` `↓` | Navigate result rows (when results focused) |
| `Enter` | Expand selected row as formatted JSON |
| `Esc` | Cancel running query · close detail view · close modal |
| `Ctrl-T` | Time-range presets (15m / 1h / 6h / 24h) |
| `Ctrl-S` | Save current query to `~/.config/dynatrace-tui/searches.yaml` |
| `Ctrl-O` | Open saved searches (`Enter` to load, `d` to delete) |
| `Ctrl-P` | Fill `$placeholder` parameters in the current query |
| `Ctrl-E` | Export results as JSON or CSV (all rows or current row) |
| `Ctrl-R` | Redo (vim-style) — pairs with `u` for undo in normal mode |
| `q` | Quit (when not editing text) |
| `Ctrl-C` | Cancel running query, or quit when idle |

\* Most terminals don't distinguish `Ctrl-Enter` from plain `Enter`
without an enhanced keyboard protocol enabled (kitty / iTerm2 with
`CSI u` mode). `Alt-Enter` works everywhere bubbletea runs.
`Ctrl-Space` also runs the query as a fallback.

### Editor (vim-style modal)

The query editor starts in **insert** mode (type immediately). Press `Esc`
to enter **normal** mode; the title flips to `Query [NORMAL]`.

| Normal-mode key | Action |
| --- | --- |
| `i` `I` `a` `A` | Insert at cursor / line start / right of cursor / line end |
| `o` `O` | Open new line below / above and insert |
| `h` `j` `k` `l` | Move left / down / up / right |
| `w` `b` | Word forward / backward |
| `0` `$` | Line start / end |
| `gg` `G` | First / last line |
| `x` | Delete character |
| `D` | Delete from cursor to end of line |
| `dd` `yy` `p` | Delete / yank / paste line |
| `dw` `db` | Delete word forward / backward |
| `yw` `yb` | Yank word forward / backward |
| `u` | Undo last edit-group |
| `Ctrl-R` | Redo (works in either mode) |

An *edit-group* is one undo step. A whole insert session — from
`i`/`a`/`I`/`A`/`o`/`O` until `Esc` — collapses into a single
group, matching vim's default behavior.

No visual mode, ex commands (`:w`/`:q`), or search yet — ask if you
want them.

### Templates

Templates work by writing `$name` placeholders in the DQL and pressing
`Ctrl-P`, e.g.:

```
fetch logs, from:now()-1h
| filter dt.entity.host == "$host" and loglevel == "$level"
```

Exports land in the current working directory as
`dttui-export-<timestamp>.<json|csv>`.

## Configuration

`~/.config/dynatrace-tui/config.yaml` — env vars override file values.

```yaml
environment_id: abc12345

# Either a Platform Token (recommended for personal use)…
platform_token: dt0s16.XXXX.YYYY

# …or an OAuth client (machine-to-machine).
# oauth:
#   client_id: dt0s02.XXXX
#   client_secret: dt0s02.XXXX.YYYY
#   scopes:
#     - storage:logs:read
#     - storage:buckets:read
```

| Env var | Overrides |
| --- | --- |
| `DT_ENVIRONMENT_ID` | `environment_id` |
| `DT_PLATFORM_TOKEN` | `platform_token` |
| `DT_OAUTH_CLIENT_ID` | `oauth.client_id` |
| `DT_OAUTH_CLIENT_SECRET` | `oauth.client_secret` |
