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
go vet ./...                 # vet (no test suite exists yet)
./build.sh <version>         # cross-compile release tarballs for linux/darwin × amd64/arm64
```

There are no tests. Code is gofmt'd on save (`.vscode/settings.json` uses the
`golang.go` formatter); run `gofmt -w` before committing.

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
  are one "directory" level at a time (CommonPrefixes become `IsDir` items).
- **`ui`** — the Bubble Tea program. `main.go` builds a `[]*s3client.Client`
  (parallel to `cfg.Buckets`), and the sidebar cursor indexes into both slices.

### UI model structure

`ui.Model` (`model.go`) is the single root `tea.Model`. It owns two sub-views,
`sidebar` and `browser`, which are **plain structs with methods, not nested
`tea.Model`s** — the root `Update`/`View` calls their methods directly rather
than delegating via `Update(msg)`. Pointer receivers mutate cursor/scroll state;
the root holds them by value, so mutations only stick when done on `m.sidebar`/
`m.browser` before returning `m`.

- `focus` (sidebar vs. browser) decides which pane key events drive.
- The `browser` tracks navigation with `prefix` + `prefixStack`; entering a
  folder pushes the current prefix, `goBack` pops it. A synthetic `"../"` item is
  prepended when `canGoBack()` is true — index math in `selectedItem`/`renderItem`
  must account for this offset.

### Async and messages

All I/O (listing, downloading, the save dialog) runs off the UI thread as
`tea.Cmd`s that return one of the message types in `messages.go`
(`ObjectsLoadedMsg`, `DownloadPathChosenMsg`, `FileDownloadedMsg`). The root
`Update` switches on these. When adding new async work, define a `Msg` type,
return a `tea.Cmd` closure that produces it, and handle it in `Update` — never
block in `Update` itself.

### macOS-only download

`chooseDownloadDest` in `model.go` shells out to `osascript` to show the native
macOS save panel. **Downloads currently only work on macOS.** Adding Linux
support means replacing this with a cross-platform path prompt.

## Release & deploy

- Tagging `*.*.*` triggers `.github/workflows/release.yml`, which runs `build.sh`
  and publishes the tarballs + `checksums.txt` as a GitHub release. The version
  is injected via `-ldflags "-X main.version=…"`.
- All pushes are mirrored to GitLab (`mirror-gitlab.yml`).
- `install.sh` is served at `anchr.jackjakarta.xyz` (see `devops/nginx.conf`,
  `CNAME`); it downloads the matching release tarball and verifies checksums.

## Libraries

When working with libraries always use the context7 mcp tools, never guess APIs from memory.
