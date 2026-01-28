package tray

import (
	"fmt"
	"time"

	"github.com/getlantern/systray"

	"daily/internal/state"
)

// Run starts a macOS/Linux system tray with quick actions.
func Run(statePath string) error {
	done := make(chan struct{})

	systray.Run(func() {
		systray.SetTitle("Daily")
		systray.SetTooltip("Daily Work Tracker")

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
					systray.SetTitle(statusTitle(statePath))
				case <-mStart.ClickedCh:
					_ = start(statePath)
					systray.SetTitle(statusTitle(statePath))
				case <-mStop.ClickedCh:
					_ = stop(statePath)
					systray.SetTitle(statusTitle(statePath))
				case <-mBreak.ClickedCh:
					_ = toggleBreak(statePath)
					systray.SetTitle(statusTitle(statePath))
				case <-mStatus.ClickedCh:
					systray.SetTooltip(statusTitle(statePath))
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

func statusTitle(path string) string {
	st, err := state.Load(path)
	if err != nil {
		return "Daily"
	}
	now := time.Now()
	work, active := st.TodaySummary(now)
	label := fmt.Sprintf("Daily %s", state.HumanMinutes(work))
	if st.ActiveSession != nil {
		label += fmt.Sprintf(" (+%s)", state.HumanMinutes(active))
	}
	if st.ActiveBreak != nil {
		mins := int(now.Sub(st.ActiveBreak.Start).Minutes())
		label += fmt.Sprintf(" [break %s]", state.HumanMinutes(mins))
	}
	return label
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
