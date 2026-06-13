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

### Object actions & keybindings

`keys.go` is the single source of truth for bindings (a `bubbles/key` `keyMap`);
the status-bar hints in `statusHints` are a separate hand-maintained list, so keep
the two in sync. **Gotcha:** Shift-letter keys arrive as the uppercase rune, so
`D`/`Y`/`S` are bound to `"D"`/`"Y"`/`"S"`, not a modifier.

The browser-pane actions all follow the same shape in `model.go`: a `start*`
method guards the selection (`!ok || item.IsDir || item.Name == "../"`), sets a
transient `m.status`, and returns a `tea.Cmd` for the I/O. They are:

- `D` — download (native macOS save panel, see below)
- `p` — preview popup (async ranged fetch, 64 KB)
- `y` / `Y` — copy the object key / `s3://bucket/key` URI to the clipboard
- `u` — presigned GET URL (valid `presignExpiry`, 1h), copied to the clipboard
- `s` / `S` — cycle sort field / toggle direction

Clipboard writes go through `github.com/atotto/clipboard`. `m.status` is the
transient feedback line in the status bar; **any keypress clears it** (the first
line of `handleKey`), so it's for one-shot confirmations, not persistent state.

### V3 "Rich · Gruvbox" rendering

The UI is the V3 design: a top bar (`anchr` pill + breadcrumb + region), a
three-column body (sidebar | file list | always-on metadata pane), an
indeterminate transfer bar shown only while downloading, and a colored-key
status bar. Layout-relevant files:

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

All I/O (listing, downloading, the save dialog, presign, preview fetch) runs off
the UI thread as `tea.Cmd`s that return one of the message types in `messages.go`
(`ObjectsLoadedMsg`, `DownloadPathChosenMsg`, `FileDownloadedMsg`,
`PresignedURLGeneratedMsg`, `ObjectPreviewLoadedMsg`). The root `Update` switches
on these. When adding new async work, define a `Msg` type, return a `tea.Cmd`
closure that produces it, and handle it in `Update` — never block in `Update`
itself. (`BucketSelectedMsg`/`NavigateMsg` are defined but currently inert — not
dispatched anywhere.)

### macOS-only download

`chooseDownloadDest` in `model.go` shells out to `osascript` to show the native
macOS save panel. **Downloads currently only work on macOS.** Adding Linux
support means replacing this with a cross-platform path prompt.

## Release & deploy

- Tagging `*.*.*` triggers `.github/workflows/release.yml`, which runs `build.sh`
  and publishes the tarballs + `checksums.txt` as a GitHub release. The version
  is injected via `-ldflags "-X main.version=…"`.
- All pushes are mirrored to GitLab (`mirror-gitlab.yml`).
- `install.sh` is served at `anchr.jackjakarta.guru` (see `devops/nginx.conf`,
  `CNAME`); it downloads the matching release tarball and verifies checksums.

## Libraries

When working with libraries always use the context7 mcp tools, never guess APIs from memory.
