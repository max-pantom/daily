# daily

Lightweight CLI + tray to track long workdays. Commands:

- `daily start [--tag t --note msg]` / `daily stop` (tags are free text, repeat `--tag` to add more; `--note` is a short description)
- `daily status` / `daily today` / `daily history [days]`
- `daily sprint --work 50 --break 10 --cycles 4 [--tag ... --note ...]`
- `daily watch --idle 10 --interval 30s` (autopause on idle, macOS/Linux)
- `daily ui` (TUI) / `daily tray` (detached menu) / `daily install`
- `daily update` (pull latest from GitHub; `--version v0.1.3` to pin)

Updating:

- `daily update` runs `go install github.com/max-pantom/daily/cmd/daily@latest` and installs to `/usr/local/bin/daily`.
- To cut a release, tag the repo (e.g. `git tag v0.1.0 && git push origin v0.1.0`). `daily update --version v0.1.0` (coming soon) or `GOFLAGS=-ldflags=... go install github.com/max-pantom/daily/cmd/daily@v0.1.0` will fetch that tag.

Install/update from source:

```bash
go run ./cmd/daily install    # or: go run ./cmd/daily update
# or direct from GitHub (no repo checkout needed):
go install github.com/max-pantom/daily/cmd/daily@latest
```

No-Go install (prebuilt binary):

```bash
curl -fsSL https://raw.githubusercontent.com/max-pantom/daily/main/install.sh | bash
# or pin a version:
VERSION=v0.1.3 curl -fsSL https://raw.githubusercontent.com/max-pantom/daily/main/install.sh | bash
```

If the release binary is missing for your platform, the installer falls back to `go install` (requires Go). To build binaries yourself: `VERSION=v0.1.4.1 scripts/package.sh` (run on each target OS/arch or set TARGETS if your toolchain supports cross-build).

Release artifacts (for maintainers):

- Tag the repo: `git tag v0.1.3 && git push origin v0.1.3`
- Build tarballs/checksums: `VERSION=v0.1.3 scripts/package.sh` (outputs in `dist/` as `daily_<version>_<os>_<arch>.tar.gz` + `checksums.txt`).
- Upload artifacts to the GitHub Release matching the tag. The `daily update` binary fallback and `install.sh` expect this naming.

Quick use:

```bash
daily start --tag focus --note "chapter 3"
daily stop
daily tray     # stays after terminal closes
daily update   # pull latest
```

Notes: idle watch needs `ioreg` (mac) or `xprintidle` (Linux); notifications use `osascript`/`notify-send` if available.
