package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// State is the persisted application state.
// Days are keyed by local date in YYYY-MM-DD.
// All timestamps are stored in RFC3339 with local time zone.
type State struct {
	GoalMinutes          int                `json:"goal_minutes"`
	BreakIntervalMinutes int                `json:"break_interval_minutes"`
	ActiveSession        *Session           `json:"active_session,omitempty"`
	ActiveBreak          *Session           `json:"active_break,omitempty"`
	Days                 map[string]*DayLog `json:"days"`
}

type Session struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end,omitempty"`
	Tags  []string   `json:"tags,omitempty"`
	Note  string     `json:"note,omitempty"`
}

type DayLog struct {
	Date              string    `json:"date"`
	Sessions          []Session `json:"sessions"`
	TotalWorkMinutes  int       `json:"total_work_minutes"`
	TotalWorkSeconds  int       `json:"total_work_seconds,omitempty"`
	TotalBreakMinutes int       `json:"total_break_minutes"`
	BreakCount        int       `json:"break_count"`
	GoalMinutes       int       `json:"goal_minutes"`
}

const (
	defaultGoalMinutes          = 12 * 60
	defaultBreakIntervalMinutes = 120
)

// Load loads state from disk or returns defaults when missing.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		st := defaults()
		if err := st.Save(path); err != nil {
			return nil, err
		}
		return st, nil
	}
	if err != nil {
		return nil, err
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	st.ensureDefaults()
	return &st, nil
}

// Save writes state to disk atomically.
func (s *State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// StartSession sets an active session if none is running.
func (s *State) StartSession(now time.Time, tags []string, note string) error {
	if s.ActiveSession != nil {
		return fmt.Errorf("session already running since %s", s.ActiveSession.Start.Format(time.Kitchen))
	}
	s.ActiveSession = &Session{Start: now, Tags: tags, Note: note}
	return nil
}

// StopSession closes the active session and records it to today's log.
func (s *State) StopSession(now time.Time) (int, error) {
	if s.ActiveSession == nil {
		return 0, errors.New("no active session")
	}
	if now.Before(s.ActiveSession.Start) {
		return 0, errors.New("stop time is before start time")
	}

	seconds := int(now.Sub(s.ActiveSession.Start).Seconds())
	end := now
	sess := Session{Start: s.ActiveSession.Start, End: &end, Tags: s.ActiveSession.Tags, Note: s.ActiveSession.Note}

	dayKey := dateKey(now)
	log := s.dayLog(dayKey)
	if log.TotalWorkSeconds == 0 && log.TotalWorkMinutes > 0 {
		log.TotalWorkSeconds = log.TotalWorkMinutes * 60
	}
	log.Sessions = append(log.Sessions, sess)
	log.TotalWorkSeconds += seconds
	log.TotalWorkMinutes = log.TotalWorkSeconds / 60
	log.GoalMinutes = s.GoalMinutes
	s.Days[dayKey] = log

	s.ActiveSession = nil
	return seconds / 60, nil
}

// StartBreak starts a break; if a work session is running, it is ended first.
func (s *State) StartBreak(now time.Time) error {
	if s.ActiveBreak != nil {
		return errors.New("break already running")
	}
	if s.ActiveSession != nil {
		if _, err := s.StopSession(now); err != nil {
			return err
		}
	}
	s.ActiveBreak = &Session{Start: now}
	return nil
}

// StopBreak ends the active break and records its duration.
func (s *State) StopBreak(now time.Time) (int, error) {
	if s.ActiveBreak == nil {
		return 0, errors.New("no active break")
	}
	if now.Before(s.ActiveBreak.Start) {
		return 0, errors.New("break end before start")
	}
	minutes := int(now.Sub(s.ActiveBreak.Start).Minutes())
	dayKey := dateKey(now)
	log := s.dayLog(dayKey)
	log.BreakCount++
	log.TotalBreakMinutes += minutes
	// We keep break totals only; per-break list can be added later if needed.
	s.Days[dayKey] = log

	s.ActiveBreak = nil
	return minutes, nil
}

// TodaySummary returns the accumulated minutes for today including the running session.
func (s *State) TodaySummary(now time.Time) (workMinutes int, activeMinutes int) {
	dayKey := dateKey(now)
	if log, ok := s.Days[dayKey]; ok {
		workMinutes += log.TotalWorkMinutes
	}
	if s.ActiveSession != nil {
		activeMinutes = int(now.Sub(s.ActiveSession.Start).Minutes())
		workMinutes += activeMinutes
	}
	// Breaks are tracked separately; workMinutes excludes break minutes.
	return workMinutes, activeMinutes
}

func (s *State) dayLog(key string) *DayLog {
	if s.Days == nil {
		s.Days = make(map[string]*DayLog)
	}
	if log, ok := s.Days[key]; ok {
		return log
	}
	log := &DayLog{Date: key, GoalMinutes: s.GoalMinutes}
	s.Days[key] = log
	return log
}

func (s *State) ensureDefaults() {
	if s.GoalMinutes == 0 {
		s.GoalMinutes = defaultGoalMinutes
	}
	if s.BreakIntervalMinutes == 0 {
		s.BreakIntervalMinutes = defaultBreakIntervalMinutes
	}
	if s.Days == nil {
		s.Days = make(map[string]*DayLog)
	}
}

func defaults() *State {
	return &State{
		GoalMinutes:          defaultGoalMinutes,
		BreakIntervalMinutes: defaultBreakIntervalMinutes,
		Days:                 make(map[string]*DayLog),
	}
}

// Normalize ensures active session/break don’t span days; it splits at midnight.
func (s *State) Normalize(now time.Time) {
	// Normalize active work session across day boundary.
	if s.ActiveSession != nil && !sameDate(s.ActiveSession.Start, now) {
		mid := midnight(now)
		if mid.After(s.ActiveSession.Start) {
			dur := mid.Sub(s.ActiveSession.Start)
			if dur > 0 {
				s.addWorkSpan(s.ActiveSession.Start, mid, s.ActiveSession.Tags, s.ActiveSession.Note)
			}
			s.ActiveSession.Start = mid
		}
	}

	// Normalize active break across day boundary.
	if s.ActiveBreak != nil && !sameDate(s.ActiveBreak.Start, now) {
		mid := midnight(now)
		if mid.After(s.ActiveBreak.Start) {
			dur := mid.Sub(s.ActiveBreak.Start)
			if dur > 0 {
				s.addBreakSpan(s.ActiveBreak.Start, mid)
			}
			s.ActiveBreak.Start = mid
		}
	}
}

func (s *State) addWorkSpan(start, end time.Time, tags []string, note string) {
	seconds := int(end.Sub(start).Seconds())
	if seconds <= 0 {
		return
	}
	dayKey := dateKey(start)
	log := s.dayLog(dayKey)
	if log.TotalWorkSeconds == 0 && log.TotalWorkMinutes > 0 {
		log.TotalWorkSeconds = log.TotalWorkMinutes * 60
	}
	log.Sessions = append(log.Sessions, Session{Start: start, End: &end, Tags: tags, Note: note})
	log.TotalWorkSeconds += seconds
	log.TotalWorkMinutes = log.TotalWorkSeconds / 60
	log.GoalMinutes = s.GoalMinutes
	s.Days[dayKey] = log
}

func (s *State) addBreakSpan(start, end time.Time) {
	minutes := int(end.Sub(start).Minutes())
	if minutes <= 0 {
		return
	}
	dayKey := dateKey(start)
	log := s.dayLog(dayKey)
	log.TotalBreakMinutes += minutes
	log.BreakCount++
	s.Days[dayKey] = log
}

// dateKey returns the local date in YYYY-MM-DD.
func dateKey(t time.Time) string {
	y, m, d := t.Date()
	return fmt.Sprintf("%04d-%02d-%02d", y, int(m), d)
}

func sameDate(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// ParseGoalMinutes interprets an input value as either hours (< 24) or minutes.
func ParseGoalMinutes(input int) int {
	if input <= 0 {
		return defaultGoalMinutes
	}
	if input <= 24 {
		return input * 60
	}
	return input
}

// HumanMinutes renders minutes as HhMm string.
func HumanMinutes(total int) string {
	if total < 60 {
		return fmt.Sprintf("%dm", total)
	}
	hours := total / 60
	mins := total % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%02dm", hours, mins)
}

// Copy writes the state JSON to an io.Writer, mainly for debugging.
func (s *State) Copy(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}
