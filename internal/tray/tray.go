package tray

import (
	"fmt"
	"time"

	"github.com/getlantern/systray"

	"github.com/max-pantom/daily/internal/state"
)

// Run starts a macOS/Linux system tray with quick actions.
func Run(statePath string) error {
	done := make(chan struct{})

	systray.Run(func() {
		title, tip := statusInfo(statePath)
		systray.SetTitle(title)
		systray.SetTooltip(tip)

		mStart := systray.AddMenuItem("Start", "Start tracking")
		mStop := systray.AddMenuItem("Stop", "Stop tracking")
		mBreak := systray.AddMenuItem("Break", "Start/stop break")
		mStatus := systray.AddMenuItem("Status", "Show current status")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Quit Daily tray")

		go func() {
			ticker := time.NewTicker(20 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					title, tip := statusInfo(statePath)
					systray.SetTitle(title)
					systray.SetTooltip(tip)
				case <-mStart.ClickedCh:
					_ = start(statePath)
					title, tip := statusInfo(statePath)
					systray.SetTitle(title)
					systray.SetTooltip(tip)
				case <-mStop.ClickedCh:
					_ = stop(statePath)
					title, tip := statusInfo(statePath)
					systray.SetTitle(title)
					systray.SetTooltip(tip)
				case <-mBreak.ClickedCh:
					_ = toggleBreak(statePath)
					title, tip := statusInfo(statePath)
					systray.SetTitle(title)
					systray.SetTooltip(tip)
				case <-mStatus.ClickedCh:
					_, tip := statusInfo(statePath)
					systray.SetTooltip(tip)
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {
		close(done)
	})

	<-done
	return nil
}

func statusInfo(path string) (string, string) {
	st, err := state.Load(path)
	if err != nil {
		return "Daily", "Daily Work Tracker"
	}
	now := time.Now()
	st.Normalize(now)
	work, active := st.TodaySummary(now)

	goal := st.GoalMinutes
	percent := 0
	if goal > 0 {
		percent = (work * 100) / goal
	}

	statusGlyph := progressGlyph(percent)
	if st.ActiveBreak != nil {
		statusGlyph = "☕"
	}

	title := fmt.Sprintf("%s %s", statusGlyph, state.HumanMinutes(work))
	if st.ActiveSession != nil {
		title += fmt.Sprintf(" (+%s)", state.HumanMinutes(active))
	}
	if st.ActiveBreak != nil {
		mins := int(now.Sub(st.ActiveBreak.Start).Minutes())
		title += fmt.Sprintf(" [break %s]", state.HumanMinutes(mins))
	}

	nextLabel, nextETA := nextMilestone(work, goal)
	goalStr := state.HumanMinutes(goal)
	tip := fmt.Sprintf("Work: %s | Goal: %s | %d%%", state.HumanMinutes(work), goalStr, percent)
	if nextLabel != "" && nextETA != "" {
		tip += fmt.Sprintf(" | Next: %s in %s", nextLabel, nextETA)
	}
	if st.ActiveBreak != nil {
		mins := int(now.Sub(st.ActiveBreak.Start).Minutes())
		tip += fmt.Sprintf(" | Break: %s", state.HumanMinutes(mins))
	}
	return title, tip
}

func progressGlyph(percent int) string {
	switch {
	case percent >= 100:
		return "●"
	case percent >= 80:
		return "◕"
	case percent >= 60:
		return "◑"
	case percent >= 40:
		return "◔"
	case percent >= 20:
		return "○"
	default:
		return "◌"
	}
}

var milestoneThresholds = []int{240, 360, 480, 600}

func nextMilestone(workMin, goalMin int) (string, string) {
	best := -1
	for _, th := range milestoneThresholds {
		if th > workMin {
			best = th
			break
		}
	}
	if best == -1 || (goalMin > 0 && goalMin < best) {
		best = goalMin
	}
	if best <= 0 || best <= workMin {
		return "", ""
	}
	etaMin := best - workMin
	return state.HumanMinutes(best), state.HumanMinutes(etaMin)
}

func start(path string) error {
	st, err := state.Load(path)
	if err != nil {
		return err
	}
	if st.ActiveBreak != nil {
		if _, err := st.StopBreak(time.Now()); err != nil {
			return err
		}
	}
	if err := st.StartSession(time.Now(), nil, ""); err != nil {
		return err
	}
	return st.Save(path)
}

func stop(path string) error {
	st, err := state.Load(path)
	if err != nil {
		return err
	}
	if st.ActiveSession == nil {
		return nil
	}
	if _, err := st.StopSession(time.Now()); err != nil {
		return err
	}
	return st.Save(path)
}

func toggleBreak(path string) error {
	st, err := state.Load(path)
	if err != nil {
		return err
	}
	now := time.Now()
	if st.ActiveBreak != nil {
		if _, err := st.StopBreak(now); err != nil {
			return err
		}
	} else {
		// End any running session before break.
		if st.ActiveSession != nil {
			if _, err := st.StopSession(now); err != nil {
				return err
			}
		}
		if err := st.StartBreak(now); err != nil {
			return err
		}
	}
	return st.Save(path)
}
