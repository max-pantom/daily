package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"daily/internal/idle"
	"daily/internal/notify"
	"daily/internal/state"
	"daily/internal/tray"
	"daily/internal/tui"
)

func main() {
	if len(os.Args) < 2 {
		runUI()
		return
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	now := time.Now()
	st, err := state.Load(statePath())
	if err != nil {
		exitErr(err)
	}
	st.Normalize(now)

	switch cmd {
	case "start":
		tags, note := parseTagsAndNote(args)
		if err := st.StartSession(now, tags, note); err != nil {
			exitErr(err)
		}
		if err := st.Save(statePath()); err != nil {
			exitErr(err)
		}
		fmt.Printf("Started session at %s", now.Format(time.Kitchen))
		if len(tags) > 0 {
			fmt.Printf(" [tags: %s]", strings.Join(tags, ","))
		}
		if note != "" {
			fmt.Printf(" note: %s", note)
		}
		fmt.Println()

	case "stop":
		minutes, err := st.StopSession(now)
		if err != nil {
			exitErr(err)
		}
		if err := st.Save(statePath()); err != nil {
			exitErr(err)
		}
		fmt.Printf("Stopped session. Logged %s.\n", state.HumanMinutes(minutes))

	case "status":
		work, active := st.TodaySummary(now)
		fmt.Printf("Today: %s logged", state.HumanMinutes(work))
		if active > 0 {
			fmt.Printf(" (active %s)", state.HumanMinutes(active))
		}
		fmt.Println()
		if st.ActiveSession != nil {
			fmt.Printf("Running since %s\n", st.ActiveSession.Start.Format(time.Kitchen))
		}
		fmt.Printf("Goal: %s | Break interval: %s\n", state.HumanMinutes(st.GoalMinutes), state.HumanMinutes(st.BreakIntervalMinutes))

	case "today":
		showToday(st, now)

	case "history":
		days := 7
		if len(args) == 1 {
			if v := parseSingleInt(args); v > 0 {
				days = v
			}
		}
		showHistory(st, days)

	case "sprint":
		if err := runSprint(args); err != nil {
			exitErr(err)
		}

	case "set-goal":
		goalMinutes := parseSingleInt(args)
		st.GoalMinutes = state.ParseGoalMinutes(goalMinutes)
		if err := st.Save(statePath()); err != nil {
			exitErr(err)
		}
		fmt.Printf("Daily goal set to %s\n", state.HumanMinutes(st.GoalMinutes))

	case "set-breaks":
		interval := parseSingleInt(args)
		if interval <= 0 {
			exitErr(errors.New("break interval must be > 0 minutes"))
		}
		st.BreakIntervalMinutes = interval
		if err := st.Save(statePath()); err != nil {
			exitErr(err)
		}
		fmt.Printf("Break reminder set to every %s\n", state.HumanMinutes(interval))

	case "ui":
		runUI()

	case "tray":
		if maybeDetachTray() {
			fmt.Println("tray launched in background")
			return
		}
		if err := tray.Run(statePath()); err != nil {
			exitErr(err)
		}

	case "install":
		target := installPath()
		if err := buildLatest(target); err != nil {
			fmt.Printf("build failed (%v); falling back to copying current binary\n", err)
			if err2 := copySelf(target); err2 != nil {
				exitErr(fmt.Errorf("build error: %v; copy error: %w", err, err2))
			}
		}
		fmt.Printf("installed daily to %s\n", target)

	case "watch":
		if err := runWatch(args); err != nil {
			exitErr(err)
		}

	case "update":
		if err := runUpdate(); err != nil {
			exitErr(err)
		}

	case "help", "-h", "--help":
		usage()
	default:
		fmt.Printf("unknown command: %s\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("daily - track your work hours")
	fmt.Println("Usage:")
	fmt.Println("  daily start           Start tracking")
	fmt.Println("  daily stop            Stop current session")
	fmt.Println("  daily status          Show today status")
	fmt.Println("  daily today           Show today sessions")
	fmt.Println("  daily history [days]  Show recent days summary (default 7)")
	fmt.Println("  daily sprint          Run work/break cycles with notifications")
	fmt.Println("  daily watch           Auto-pause active session when idle (macOS/Linux)")
	fmt.Println("  daily set-goal <h|m>  Set daily goal in hours (<=24) or minutes")
	fmt.Println("  daily set-breaks <m>  Set break reminder interval (minutes)")
	fmt.Println("  daily ui              Open live terminal dashboard")
	fmt.Println("  daily tray            Launch macOS/Linux tray menu")
	fmt.Println("  daily install         Copy binary to /usr/local/bin/daily")
	fmt.Println("  daily update          Fetch latest from GitHub and install")
}

func runUI() {
	if err := tui.Run(statePath()); err != nil {
		exitErr(err)
	}
}

func parseSingleInt(args []string) int {
	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.SetOutput(os.Stdout)
	if err := fs.Parse(args); err != nil {
		return 0
	}
	if fs.NArg() != 1 {
		fmt.Println("expected one integer argument")
		os.Exit(1)
	}
	var val int
	_, err := fmt.Sscanf(fs.Arg(0), "%d", &val)
	if err != nil {
		fmt.Println("argument must be an integer")
		os.Exit(1)
	}
	return val
}

func parseTagsAndNote(args []string) ([]string, string) {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	var tags multiString
	var note string
	fs.Var(&tags, "tag", "tag for the session (repeatable)")
	fs.StringVar(&note, "note", "", "note for the session")
	fs.Parse(args)
	return tags, note
}

func runSprint(args []string) error {
	fs := flag.NewFlagSet("sprint", flag.ExitOnError)
	fs.SetOutput(os.Stdout)
	work := fs.Int("work", 50, "work minutes")
	brk := fs.Int("break", 10, "break minutes")
	cycles := fs.Int("cycles", 4, "cycles")
	var tags multiString
	var note string
	fs.Var(&tags, "tag", "tag for sprint sessions")
	fs.StringVar(&note, "note", "", "note for sprint sessions")
	fs.Parse(args)

	if *work <= 0 || *brk <= 0 || *cycles <= 0 {
		return errors.New("work, break, and cycles must be > 0")
	}

	for i := 1; i <= *cycles; i++ {
		now := time.Now()
		st, err := state.Load(statePath())
		if err != nil {
			return err
		}
		if err := st.StartSession(now, tags, note); err != nil {
			return err
		}
		_ = st.Save(statePath())
		fmt.Printf("Cycle %d/%d: work %d min\n", i, *cycles, *work)
		notify.Send("Daily Sprint", fmt.Sprintf("Cycle %d work started", i))
		time.Sleep(time.Duration(*work) * time.Minute)

		st, _ = state.Load(statePath())
		if st.ActiveSession != nil {
			if _, err := st.StopSession(time.Now()); err != nil {
				return err
			}
			_ = st.Save(statePath())
		}
		notify.Send("Daily Sprint", fmt.Sprintf("Cycle %d break", i))

		// Break
		st, _ = state.Load(statePath())
		if err := st.StartBreak(time.Now()); err != nil {
			return err
		}
		_ = st.Save(statePath())
		time.Sleep(time.Duration(*brk) * time.Minute)
		st, _ = state.Load(statePath())
		if st.ActiveBreak != nil {
			if _, err := st.StopBreak(time.Now()); err != nil {
				return err
			}
			_ = st.Save(statePath())
		}
	}

	notify.Send("Daily Sprint", "Sprint finished")
	fmt.Println("Sprint finished")
	return nil
}

func runWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	fs.SetOutput(os.Stdout)
	idleMin := fs.Int("idle", 10, "idle minutes before auto-pause")
	interval := fs.Duration("interval", 30*time.Second, "poll interval")
	fs.Parse(args)

	if *idleMin <= 0 {
		return errors.New("idle minutes must be > 0")
	}
	idleDur := time.Duration(*idleMin) * time.Minute
	for {
		time.Sleep(*interval)
		st, err := state.Load(statePath())
		if err != nil {
			fmt.Println("watch: load error", err)
			continue
		}
		now := time.Now()
		st.Normalize(now)
		if st.ActiveSession == nil {
			continue
		}
		idleDurNow, err := idle.Duration()
		if err != nil {
			fmt.Println("watch: idle check unsupported", err)
			return err
		}
		if idleDurNow >= idleDur {
			if _, err := st.StopSession(now); err != nil {
				fmt.Println("watch: stop error", err)
				continue
			}
			_ = st.Save(statePath())
			notify.Send("Daily", fmt.Sprintf("Auto-paused after %s idle", idleDur))
			fmt.Printf("Auto-paused session after idle %s\n", idleDur)
		}
	}
}

func showToday(st *state.State, now time.Time) {
	dayKey := now.Format("2006-01-02")
	fmt.Printf("Today: %s\n", dayKey)
	log, ok := st.Days[dayKey]
	if !ok || len(log.Sessions) == 0 {
		fmt.Println("  no logged sessions yet")
	} else {
		for i, sess := range log.Sessions {
			end := "--"
			if sess.End != nil {
				end = sess.End.Format(time.Kitchen)
			}
			note := ""
			if sess.Note != "" {
				note = fmt.Sprintf(" note:%s", sess.Note)
			}
			tags := ""
			if len(sess.Tags) > 0 {
				tags = fmt.Sprintf(" tags:%s", strings.Join(sess.Tags, ","))
			}
			fmt.Printf("  #%d %s -> %s (%s)%s%s\n", i+1, sess.Start.Format(time.Kitchen), end, sessionDuration(sess, now), tags, note)
		}
		fmt.Printf("  total: %s\n", state.HumanMinutes(log.TotalWorkMinutes))
	}
	if st.ActiveSession != nil {
		fmt.Printf("  active since %s (%s so far)\n", st.ActiveSession.Start.Format(time.Kitchen), state.HumanMinutes(int(now.Sub(st.ActiveSession.Start).Minutes())))
	}
}

func showHistory(st *state.State, days int) {
	if days <= 0 {
		days = 7
	}
	keys := make([]string, 0, len(st.Days))
	for k := range st.Days {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] > keys[j] })

	if len(keys) == 0 {
		fmt.Println("no history yet")
		return
	}

	if days < len(keys) {
		keys = keys[:days]
	}
	for _, k := range keys {
		log := st.Days[k]
		fmt.Printf("%s  work: %s  breaks: %s (%d)\n",
			k,
			state.HumanMinutes(log.TotalWorkMinutes),
			state.HumanMinutes(log.TotalBreakMinutes),
			log.BreakCount,
		)
	}
}

func sessionDuration(s state.Session, now time.Time) string {
	end := s.End
	if end == nil {
		tmp := now
		end = &tmp
	}
	mins := int(end.Sub(s.Start).Minutes())
	if mins < 1 {
		mins = 1
	}
	return state.HumanMinutes(mins)
}

func statePath() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil || cfgDir == "" {
		home, hErr := os.UserHomeDir()
		if hErr != nil {
			exitErr(errors.New("cannot determine config directory"))
		}
		cfgDir = filepath.Join(home, ".config")
	}
	return filepath.Join(cfgDir, "daily", "state.json")
}

func maybeDetachTray() bool {
	if os.Getenv("DAILY_TRAY_DETACHED") == "1" {
		return false
	}
	cmd := exec.Command(os.Args[0], "tray")
	cmd.Env = append(os.Environ(), "DAILY_TRAY_DETACHED=1")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return false
	}
	return true
}

func installPath() string {
	return "/usr/local/bin/daily"
}

func copySelf(dest string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, data, 0o755); err != nil {
		return err
	}
	return nil
}

func runUpdate() error {
	binDir := os.Getenv("GOBIN")
	if binDir == "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			home, _ := os.UserHomeDir()
			gopath = filepath.Join(home, "go")
		}
		binDir = filepath.Join(gopath, "bin")
	}
	// go install latest from GitHub
	cmd := exec.Command("go", "install", "github.com/max-pantom/daily/cmd/daily@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	src := filepath.Join(binDir, "daily")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("did not find built binary at %s", src)
	}
	if err := copyFile(src, installPath()); err != nil {
		return err
	}
	fmt.Printf("updated daily from GitHub to %s\n", installPath())
	return nil
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o755)
}

func buildLatest(dest string) error {
	cwd, _ := os.Getwd()
	root, err := findModuleRoot(cwd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", dest, "./cmd/daily")
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findModuleRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found from %s", start)
}

type multiString []string

func (m *multiString) String() string {
	return strings.Join(*m, ",")
}

func (m *multiString) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func exitErr(err error) {
	msg := err.Error()
	msg = strings.TrimSuffix(msg, "\n")
	fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	os.Exit(1)
}
