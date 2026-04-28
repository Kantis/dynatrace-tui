# CLAUDE.md

A Go CLI/TUI for searching Dynatrace logs with DQL. The user-facing docs
live in `README.md` — this file captures only what's useful for working in
the repo and isn't obvious from reading the code.

## Build & test

```sh
go build ./...
go test -race ./...
go install ./cmd/dttui   # binary lands in $(go env GOPATH)/bin
```

CI runs `go build ./...` then `go test -race ./...` on PRs and `main`
(`.github/workflows/ci.yml`).

## Layout

- `cmd/dttui/main.go` — entrypoint; everything else hangs off `cmd.Root()`.
- `cmd/{root,query,tui}.go` — Cobra wiring. `query` is the headless
  subcommand; running `dttui` with no args boots the TUI.
- `internal/config` — YAML config loader. Supports both the multi-env
  shape (`environments:` map, ordered) and the legacy single-env shape
  (top-level fields synthesised into a `default` env). `DT_*` env vars
  override resolved values.
- `internal/auth` — `auth.Static` (Platform Token) and `auth.New` (OAuth
  client-credentials). Both implement `grail.TokenProvider`.
- `internal/grail` — Grail client: `Execute` (POST query), `PollUntilDone`
  (GET poll), `Cancel` (best-effort `query:cancel` on Ctrl-C).
- `internal/dql` — Pure string transforms on DQL. `PrependFetch` /
  `StripFetch` are the editor↔storage boundary; `ApplyTimeframe`,
  `SubstituteTimeframe`, `SubstituteAbsolute`, `MakeTimeseries`,
  `Placeholders`/`Substitute` cover the time-range picker, chart command,
  and `$param` templates. Heavily unit-tested — extend the tests when
  changing the regex/clause logic.
- `internal/tui` — Bubble Tea app. Single `Model` in `app.go`; subsystems
  (editor, table, modals, saved searches, chart, export, env switch) are
  split across sibling files. Modal dispatch goes through `m.modal`; view
  switching (`Alt-1` / `Alt-2`) goes through `m.currentView`.

## Conventions worth knowing

- The editor auto-prepends `fetch logs,` so the body is what users edit and
  what gets stored in `searches.yaml`. Always go through `dql.PrependFetch`
  before sending to Grail and `dql.StripFetch` when loading into the
  editor — don't reimplement the prefix logic.
- `populateTable` clears rows *before* setting columns. The bubbles table
  panics if column count shrinks under existing rows; preserve that order.
- The query editor is a custom vim-modal wrapper around `bubbles/textarea`
  (`internal/tui/editor.go`). Insert mode is the default; tests in
  `editor_test.go` cover the motions and undo grouping.
- Config secrets: `~/.config/dynatrace-tui/config.yaml` holds platform
  tokens. Don't log it, don't echo it, don't add it to error strings.
- Saved searches live at `~/.config/dynatrace-tui/searches.yaml` (mode
  0600, dir 0755).

## Don't

- Don't add visual mode, `:`-commands, or search-in-editor without asking —
  the README explicitly notes they're unimplemented and the user may want
  to scope them.
- Don't introduce a third config shape; the legacy/single-env path is a
  back-compat shim, not a parallel design.
