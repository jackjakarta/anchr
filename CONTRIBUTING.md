# Contributing to anchr

Thanks for your interest in contributing! anchr is a keyboard-driven terminal UI
for browsing AWS S3 and S3-compatible object stores, built in Go on
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Getting started

You'll need [Go](https://go.dev/dl/) installed (see `go.mod` for the minimum
version).

```sh
git clone https://github.com/jackjakarta/anchr.git
cd anchr
go build -o anchr            # local dev build
go run . --config <path>     # run from source
```

To run the TUI you need a config at `~/.config/anchr/config.yaml` (or pass
`--config`). Copy `config.example.yaml` as a starting point — `real-config.yaml`
is gitignored for local credentials.

## Development workflow

1. Fork the repo and create a branch off `main`.
2. Make your change.
3. Run the checks below and make sure they all pass.
4. Open a pull request with a clear description of what and why.

## Checks

CI (`.github/workflows/static-checks.yml`) runs gofmt, vet, build, and tests on
every push. Run them locally before opening a PR:

```sh
gofmt -w .        # format (code is gofmt'd on save)
go vet ./...      # vet
go build -o anchr # build
go test ./...     # unit tests live in the ui package
```

Tests live in `ui/*_test.go` — pure-function table tests plus `view_test.go`,
which renders `Model.View()` at several sizes and asserts the grid invariants.
Please add or update tests for behavior changes.

## Code style

- Keep code gofmt-clean (`gofmt -w`).
- Follow the existing conventions; the architecture is documented in
  [`CLAUDE.md`](CLAUDE.md) — worth a read before larger changes.
- Match the surrounding code's naming, comment density, and idioms.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/)
(`feat:`, `fix:`, `chore:`, …), matching the existing git history.

## Reporting issues

Open an issue describing the problem, what you expected, and steps to reproduce.
For security-sensitive reports, please avoid filing a public issue with exploit
details.
