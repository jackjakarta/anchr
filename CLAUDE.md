# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`anchr` is a keyboard-driven terminal UI for browsing AWS S3 and S3-compatible
object stores (MinIO, Supabase, …). It ships as a single static Go binary built
on [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Commands

```sh
go build -o anchr            # local dev build
go run . --config <path>     # run from source
go vet ./...                 # vet
go test ./...                # unit tests live in the ui package
./build.sh <version>         # cross-compile release tarballs for linux/darwin × amd64/arm64
```

Tests live in `ui/*_test.go` (pure-function table tests plus `view_test.go`,
which renders `Model.View()` at several sizes and asserts the grid invariants —
exactly `height` lines, each exactly `width` cells). Code is gofmt'd on save
(`.vscode/settings.json` uses the `golang.go` formatter); run `gofmt -w` before
committing. CI (`.github/workflows/static-checks.yml`) runs gofmt/vet/build/test.

To run the TUI you need a config at `~/.config/anchr/config.yaml` (or pass
`--config`). `config.example.yaml` is a template; `real-config.yaml` is
gitignored for local credentials.

## Architecture

The program follows the Elm Architecture (Bubble Tea). Three packages, wired
together in `main.go`:

- **`config`** — loads/validates YAML into `[]BucketConfig`. Resolves the config
  path from `--config` → `XDG_CONFIG_HOME` → `~/.config/anchr/config.yaml`,
  applies defaults (`name` falls back to `bucket`, `region` to `us-east-1`), and
  fails fast with a copy-pasteable bootstrap message if the file is missing.
- **`s3client`** — one `Client` per configured bucket, each wrapping an
  aws-sdk-go-v2 `s3.Client`. Credentials come from explicit `access_key`/`secret_key`
  if set, otherwise the default AWS credential chain. `endpoint` + `path_style`
  support S3-compatible stores. `ListObjects` uses `Delimiter: "/"` so listings
  are one "directory" level at a time (CommonPrefixes become `IsDir` items) and
  pages through every result via the v2 paginator. Beyond listing it exposes
  `DownloadObject` (streams to a local path), `PreviewObject` (ranged GET of the
  first N bytes, also capped with `io.LimitReader` in case the store ignores the
  `Range` header), and `PresignGetObject` (mints a presigned GET URL).
- **`ui`** — the Bubble Tea program. `main.go` builds a `[]*s3client.Client`
  (parallel to `cfg.Buckets`), and the sidebar cursor indexes into both slices.

### UI model structure

`ui.Model` (`model.go`) is the single root `tea.Model`. It owns two focusable
sub-views, `sidebar` and `browser`, plus a `preview` popup — all **plain structs
with methods, not nested `tea.Model`s** — the root `Update`/`View` calls their
methods directly rather than delegating via `Update(msg)`. Pointer receivers
mutate cursor/scroll state; the root holds them by value, so mutations only stick
when done on `m.sidebar`/`m.browser` before returning `m`.

- `focus` (sidebar vs. browser) decides which pane key events drive.
- The `browser` tracks navigation with `prefix` + `prefixStack`; entering a
  folder pushes the current prefix, `goBack` pops it. A synthetic `"../"` item is
  prepended when `canGoBack()` is true — index math in `selectedItem`/`renderItem`
  must account for this offset.
- The `browser` sorts in-memory by name/size/modified (`s` cycles the field, `S`
  toggles direction); directories always sort first, and `restoreCursor` keeps the
  selection pinned to the same key across re-sorts.
- `/` filters the listing in memory (see "The `/` filter" below). `items` always
  holds the full listing; `rows()` is what the list actually renders from.

### Object actions & keybindings

`keys.go` is the single source of truth for bindings (a `bubbles/key` `keyMap`);
the status-bar hints in `statusHints` are a separate hand-maintained list, so keep
the two in sync. **Gotcha:** Shift-letter keys arrive as the uppercase rune, so
`D`/`Y`/`S` are bound to `"D"`/`"Y"`/`"S"`, not a modifier.

The browser-pane actions all follow the same shape in `model.go`: a `start*`
method guards the selection (`!ok || item.IsDir || item.Name == "../"`), sets a
transient `m.status`, and returns a `tea.Cmd` for the I/O (`D` opens the save
prompt first and starts its `tea.Cmd`s on confirm). They are:

- `D` — download (in-TUI save-path prompt + progress bar, see below)
- `p` — preview popup (async ranged fetch, 64 KB)
- `y` / `Y` — copy the object key / `s3://bucket/key` URI to the clipboard
- `u` — presigned GET URL (valid `presignExpiry`, 1h), copied to the clipboard
- `s` / `S` — cycle sort field / toggle direction
- `/` — filter the listing (input mode, see below)

Clipboard writes go through `github.com/atotto/clipboard`. `m.status` is the
transient feedback line in the status bar; **any keypress clears it** (the first
line of `handleKey`), so it's for one-shot confirmations, not persistent state.

### The `/` filter

`filter.go` owns the filter. `browser.items` always keeps the full listing and
`browser.matches` holds the narrowed projection, so clearing the filter restores
everything without a refetch. **Anything that indexes the listing must go through
`browser.rows()`** — `itemCount`, `selectedItem`, `renderItem`, `totalSize`,
`statusPosition` and `restoreCursor` all do. The one deliberate exception is
`fileCount`, kept on `items` because the sidebar badge describes the bucket, not
the filtered view.

Matching is smart-case substring (`matchIndex`): an all-lowercase query is
case-insensitive, any uppercase rune makes it case-sensitive. It returns a
**rune** index, which `renderNameCell` uses to colour the matched run — that
helper must return exactly `nameW` cells or the whole grid shifts, so it is
covered by both `TestRenderNameCellWidth` and the filter states in
`TestViewGridInvariants`.

**Gotchas:**

- `applySort` reorders `items` in place, so `cycleSort`/`toggleReverse` must call
  `applyFilter()` afterwards or `matches` goes stale.
- `setItems` clears the filter — that single reset is what drops it on folder
  entry, `goBack` and bucket switch.
- `m.browser.filtering` captures *every* key at the top of `handleKey` (mirroring
  the `m.preview.active` and `m.savePrompt.active` blocks), otherwise
  `s`/`y`/`D`/`q` fire mid-word.
- `keys.Back` also binds `h` and backspace, so the "first esc clears the filter,
  a second navigates up" case is keyed on `tea.KeyEsc` directly and sits *before*
  the `keys.Back` case. `h` keeps meaning "go up" unconditionally.

### V3 "Rich · Gruvbox" rendering

The UI is the V3 design: a top bar (`anchr` pill + breadcrumb + region), a
three-column body (sidebar | file list | always-on metadata pane), a transfer
bar shown only while downloading, and a colored-key status bar. Layout-relevant
files:

- **`styles.go`** — the Gruvbox palette (`c*` color vars) and every `lipgloss`
  style, grouped by region. Each pane paints its own background, so leaf styles
  derive from a region *base* (`appBase`/`panelBase`/`darkBase`/`elevBase`) to
  carry the bg — a wrapper background does **not** bleed through inner styled
  segments (each emits its own SGR reset). Rows are assembled from bg-styled
  fixed-width fields (no bare spacers). **Never use `Inline(true)` for badges** —
  in lipgloss v1.1.0 it strips `Padding`.
- **`kinds.go`** — `kindFor(item)` → icon/badge-color/label/mime, the single
  source of truth for the file-list row *and* the preview pane's "Type".
- **`metapane.go`** — `renderMetaPane`, the stateless right-hand pane rendered
  from the current selection (`ETag`/`StorageClass` come from the listing).
- **`model.go`** — `updateLayout()` resolves responsive widths into `m.layout`
  (preview hides below 90 cols, sidebar narrows below 70). `fitBar` pads/truncates
  the full-width bars to exactly the terminal width (using `Width()` would
  soft-wrap an overflowing bar onto a second line).
- **`preview.go`** — the `preview` popup (`p`) overlays the body for reading file
  content. Opening it shows a spinner, then an async ranged fetch fills it in;
  `isBinary` (invalid UTF-8 or a NUL byte) swaps text for a "preview unavailable"
  note. The metadata pane is separate and always visible (when wide enough).

### Async and messages

All I/O (listing, downloading, presign, preview fetch) runs off the UI thread as
`tea.Cmd`s that return one of the message types in `messages.go`
(`ObjectsLoadedMsg`, `DownloadProgressMsg`, `FileDownloadedMsg`,
`PresignedURLGeneratedMsg`, `ObjectPreviewLoadedMsg`). The root `Update` switches
on these. When adding new async work, define a `Msg` type, return a `tea.Cmd`
closure that produces it, and handle it in `Update` — never block in `Update`
itself. (`BucketSelectedMsg`/`NavigateMsg` are defined but currently inert — not
dispatched anywhere.)

### Downloads

Downloads are pure Go and work on every platform — there is no `osascript` save
panel any more. Three pieces, all in `ui/download.go` unless noted:

- **`saveprompt.go`** — `D` opens a centered "Save as" popup (a plain struct with
  methods, like `preview`) wrapping a `bubbles/textinput`. It is prefilled from
  `defaultDownloadPath` (`$XDG_DOWNLOAD_DIR` → `~/Downloads` → `~`) and captures
  every key at the top of `handleKey`, exactly like the preview and `/` blocks.
  `resolveDest` expands `~`, appends the object name when the path names a
  directory, rejects a missing parent inline, and asks for a second `enter`
  before overwriting an existing file (any edit clears that confirmation).
  The input's styles must all derive from `darkBase` — it renders its own SGR
  reset, so a popup background does not reach it — and its cursor is set to
  `cursor.CursorStatic` so the root `Update` needs no `cursor.BlinkMsg` case.
- **`transfer`** — the in-flight download's state, held as `Model.transfer`
  (nil = idle). **Always by pointer:** it holds `atomic.Int64`s, which embed
  `noCopy`, and `Model` is copied by value on every `Update`. The download
  goroutine only stores into the atomics (`transfer.note`, handed to
  `s3client.DownloadObject` as its `ProgressFunc`) and `pollProgress` — a
  `tea.Tick` re-armed from the `DownloadProgressMsg` handler — only loads them.
  Rate and ETA are computed in that handler, not in `View`, so rendering stays a
  pure function of the model.
- **`renderTransferBar`** — determinate when the sample carries a total
  (percentage + throughput + ETA), falling back to the old sweeping block when
  the store omitted `Content-Length`. Segment order matters: `fitBar` truncates
  from the right, so the least important information disappears first on a narrow
  terminal. `ctrl+x` (`keys.CancelDL`) aborts via `transfer.abort()`; `esc` is
  deliberately left alone so "go back" keeps working during a transfer.

`s3client.DownloadObject` streams into `<dest>.part` and renames onto `dest` only
on success, removing the partial file on any error or cancellation.

## Release & deploy

- Tagging `*.*.*` triggers `.github/workflows/release.yml`, which runs `build.sh`
  and publishes the tarballs + `checksums.txt` as a GitHub release. The version
  is injected via `-ldflags "-X main.version=…"`.
- All pushes are mirrored to GitLab (`mirror-gitlab.yml`).
- `install.sh` is served straight from the repo root by GitHub Pages (Pages
  source: `main` / `/`); the custom domain comes from the `CNAME` file. The
  script downloads the matching release tarball and verifies checksums.

## Libraries

When working with libraries always use the context7 mcp tools, never guess APIs from memory.
