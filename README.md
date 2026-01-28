# daily

Lightweight CLI + tray to track long workdays. Commands:

- `daily start [--tag t --note msg]` / `daily stop`
- `daily status` / `daily today` / `daily history [days]`
- `daily sprint --work 50 --break 10 --cycles 4 [--tag ... --note ...]`
- `daily watch --idle 10 --interval 30s` (autopause on idle, macOS/Linux)
- `daily ui` (TUI) / `daily tray` (detached menu) / `daily install`
- `daily update` (pull latest from GitHub)

Install/update from source:

```bash
go run ./cmd/daily install    # or: go run ./cmd/daily update
```

Quick use:

```bash
daily start --tag focus --note "chapter 3"
daily stop
daily tray     # stays after terminal closes
```

Notes: idle watch needs `ioreg` (mac) or `xprintidle` (Linux); notifications use `osascript`/`notify-send` if available.
