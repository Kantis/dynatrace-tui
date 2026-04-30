# dttui

A small Go CLI / TUI for searching Dynatrace logs with DQL.

- `dttui query "<DQL>"` — run a query, print result records as JSON.
- `dttui` — open the interactive TUI: multi-line DQL editor, results table,
  JSON detail view, saved searches (with edit), parameter templates, JSON/CSV export.

## Getting started

### 1. Install Go

```sh
brew install golang
```

Make sure Go's bin directory is on your `PATH` — add this to `~/.zshrc`
(or `~/.bashrc`) once:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

### 2. Clone and install

```sh
git clone git@github.com:Kantis/dynatrace-tui.git
cd dynatrace-tui
go install ./cmd/dttui
```

### 3. Generate a config scaffold

```sh
dttui generate-config
```

This writes a starter `~/.config/dynatrace-tui/config.yaml` (mode `0600`)
with placeholders for one environment and commented-out examples for
additional environments and the time-range picker. Pass `--config <path>`
to write somewhere else, or `--force` to overwrite an existing file.

### 4. Create a Platform Token and fill in the config

Sign in to <https://myaccount.dynatrace.com/platformTokens> and create a
new Platform Token with these scopes:

- `storage:logs:read`
- `storage:buckets:read`

Copy the generated token — it looks like `dt0s16.XXXXXXXX.YYYY...`. You
only see it once. Open the config file and replace the two `REPLACE_ME`
placeholders under `environments.PROD`:

- `environment_id` — the prefix in `https://<env-id>.apps.dynatrace.com`
- `platform_token` — the token you just generated

### 5. Run a query

```sh
dttui query 'fetch logs, from:now()-15m | limit 5'
```

## CLI

```
dttui query [flags] <DQL>
```

| Flag | Description |
| --- | --- |
| `-t, --timeframe` | Convenience preset (`15m`, `1h`, `6h`, `24h`). Injected as `from:now()-<tf>` if the query doesn't already have a `from:` clause. |
| `-e, --env` | Environment name to use (overrides `default:` in config). |
| `--config` | Path to config file (default `~/.config/dynatrace-tui/config.yaml`). |

Output is a JSON array of records on stdout; diagnostics on stderr. `Ctrl-C`
during a running query calls `query:cancel` server-side before exiting.

## TUI

Run `dttui` with no arguments. The screen has an editor pane on top and a
results pane below.

The editor auto-prepends `fetch logs,` so you only type the body — e.g.
`from:now()-15m | filter loglevel == "ERROR"`. Lines that already start
with `fetch ...` (e.g. `fetch events`) or with `|` are handled correctly.

| Key | Action |
| --- | --- |
| `Alt-Enter` / `Ctrl-Enter`* | Run the current query |
| `Tab` / `Shift-Tab` | Cycle focus between editor and results |
| `↑` `↓` | Navigate result rows (when results focused) |
| `Enter` | Expand selected row as formatted JSON |
| `Esc` | Cancel running query · close detail view · close modal |
| `Ctrl-G` | Chart hits over time (re-runs query wrapped in `\| makeTimeseries count=count()`) |
| `Ctrl-T` | Time-range presets (15m / 1h / 6h / 24h) |
| `Alt-1` / `Alt-2` | Switch between Query view and Saved Searches view |
| `Ctrl-S` | Save current query to `~/.config/dynatrace-tui/searches.yaml` |
| `Ctrl-O` | Jump to the Saved Searches view |
| `Ctrl-P` | Fill `$placeholder` parameters in the current query |
| `Ctrl-E` | Switch environment (when more than one is configured) |
| `Ctrl-X` | Export results as JSON or CSV (all rows or current row) |
| `Ctrl-R` | Redo (vim-style) — pairs with `u` for undo in normal mode |
| `q` | Close detail view, or quit when results are focused |
| `Ctrl-C` | Cancel running query, or quit when idle |

\* Most terminals don't distinguish `Ctrl-Enter` from plain `Enter`
without an enhanced keyboard protocol enabled (kitty / iTerm2 with
`CSI u` mode). `Alt-Enter` works everywhere bubbletea runs.
`Ctrl-Space` also runs the query as a fallback.

### Editor (vim-style modal)

The query editor is a plain textarea by default. Set `vim_mode: true` at
the top of `~/.config/dynatrace-tui/config.yaml` to opt into the vim-style
modal editor described below.

With vim mode on, the editor starts in **insert** mode (type immediately).
Press `Esc` to enter **normal** mode; the tabs row flips to `[NORMAL]`.

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

### Detail view

`Enter` on a result row opens its formatted JSON (or, after `Ctrl-G`, the
chart) in a scrollable pane. While the detail pane is focused:

| Key | Action |
| --- | --- |
| `↑` `↓` / `j` `k` / `PgUp` `PgDn` | Scroll |
| `gg` | Jump to top |
| `G` | Jump to bottom |
| `q` / `Esc` | Close and return to results |

### Saved Searches view

`Alt-2` (or `Ctrl-O`) opens a dedicated view listing every saved search with a
preview of the selected query. From the list:

| Key | Action |
| --- | --- |
| `↑` `↓` (or `j` `k`) | Move selection |
| `Enter` | Load the selected query into the editor and switch back to the query view |
| `e` | Edit the selected entry (name + body) in place |
| `*` | Toggle the selected entry as the **default** — auto-loaded and run on startup. A `★` marker shows in the list. |
| `d` | Delete the selected entry |
| `Alt-1` | Switch back to the query view |

The default is persisted as a top-level `default: <name>` field in
`~/.config/dynatrace-tui/searches.yaml`. Deleting the default entry, or
renaming it via `e`, updates the field accordingly.

**Edit mode** — `Tab` switches between the **Name** input and the **Query**
editor (which uses the same vim-style modal editing as the main editor).
`Ctrl-S` saves and returns to the list; `Esc` cancels. Renaming replaces the
entry in place; the new name has to be unique.

### Time-range picker

`Ctrl-T` opens a modal with **two sections**: relative presets at the top, and
absolute From/To text inputs below. **Tab / Shift-Tab** cycle between them.

**Presets** — focus the preset list, pick with ↑/↓, **Enter** to apply.
Defaults: From shows `now()-15m`, `now()-1h`, `now()-6h`, `now()-24h`, and
the start of the current hour; To shows the moment the modal was opened.
Override either list under `time_picker:` in `config.yaml` (see below).
Substitution priority on selection:

1. Replace literal `$timeframe` if present.
2. Else rewrite an existing `now()-<duration>` clause.
3. Else inject `from:now()-<preset>` after the `fetch <table>`.

**Absolute date / range** — Tab into the From input. Accepted formats:

- `2026-04-28` (date only — defaults to start-of-day for *From*, end-of-day
  for *To*)
- `2026-04-28 09:00`
- `2026-04-28T09:00:00`
- Full RFC 3339 (`2026-04-28T09:00:00Z`)

Press **Enter** while in From or To to apply. Empty *To* means "from-only" —
existing `to:` clauses are left alone. Substitution priority is the same
three-step `$from` / `$to` placeholders → existing clauses → injection.

### Templates

Templates work by writing `$name` placeholders in the DQL and pressing
`Ctrl-P`, e.g.:

```
fetch logs, from:now()-1h
| filter dt.entity.host == "$host" and loglevel == "$level"
```

`$timeframe`, `$from`, and `$to` are reserved for the time-range picker —
`Ctrl-P` does not prompt for them.

### Export

`Ctrl-X` opens a four-option modal: all records or the current row, as JSON
or CSV. Files land in the current working directory as
`dttui-export-<timestamp>.<json|csv>`.

## Configuration

`~/.config/dynatrace-tui/config.yaml` — env vars override file values.

Each environment is authenticated with a Platform Token:

```yaml
environments:
  PROD:
    environment_id: abc12345
    platform_token: dt0s16.XXXX.YYYY
  TEST:
    environment_id: def67890
    platform_token: dt0s16.AAAA.BBBB

# Optional. The first environment in the file is used when neither
# `default:` nor `--env` is set.
default: PROD

# Optional. Enables the vim-style modal query editor. Off by default —
# when off the editor is a plain textarea.
vim_mode: true

# Optional. Override the Ctrl-T preset lists. When the block (or either
# inner list) is omitted, the built-in defaults are used. Each entry is
# either a `now()-<duration>` relative offset, a literal datetime, or one
# of these dynamic tokens (resolved when the modal opens):
#   start_of_hour  — the start of the current hour
#   start_of_day   — the start of the current day
#   now() / now    — the moment the modal opened
# Set `from: []` to render an empty list.
time_picker:
  from:
    - now()-5m
    - now()-30m
    - now()-1h
    - now()-12h
    - now()-7d
    - start_of_hour
    - start_of_day
  to:
    - now()
```

Pick an environment with `--env <name>` (or `-e <name>`); inside the TUI use
`Ctrl-E` to switch on the fly. The active environment shows up as a `[NAME]`
suffix on the Query and Results pane titles.

The legacy single-environment shape (top-level `environment_id` /
`platform_token`) is still accepted and is treated as a single environment
named `default`.

| Env var | Overrides |
| --- | --- |
| `DT_ENVIRONMENT_ID` | the active environment's `environment_id` |
| `DT_PLATFORM_TOKEN` | the active environment's `platform_token` |
