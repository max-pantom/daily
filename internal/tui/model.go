package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/max-pantom/daily/internal/state"
)

type model struct {
	statePath string
	loaded    bool
	err       error
	notice    string

	dayKey  string
	summary summary
	view    string

	tickRate time.Duration
	width    int
	height   int

	selected int
	actions  []string

	spin int

	lastMilestone int
	lastDay       string

	game gameState
}

type summary struct {
	workMinutes   int
	workSeconds   int
	activeMinutes int
	activeSeconds int
	goalMinutes   int
	breakMinutes  int
	breaksCount   int
	activeSince   *time.Time
	onBreak       bool
	sessions      []state.Session
}

// milestoneTheme controls palette shifts at certain work thresholds.
// Edit the entries in milestoneThemes to customize colors per milestone.
type milestoneTheme struct {
	Name         string
	ThresholdMin int
	Accent       lipgloss.Color // primary accent for title/arrows/notice
	Muted        lipgloss.Color // secondary text
	SelectedBg   lipgloss.Color // background for selected menu item
}

type tickMsg time.Time

const (
	actionStart  = "start"
	actionStop   = "stop"
	actionStatus = "status"
	actionBreak  = "break"
	actionRelax  = "relax"

	goalStepMinutes  = 30
	breakStepMinutes = 5
	minGoalMinutes   = 30
	minBreakMinutes  = 5
)

const statusBarHeight = 2

// milestoneThemes defines color themes by work-time thresholds (minutes).
// Customize the colors here to update the TUI look at each milestone.
var milestoneThemes = []milestoneTheme{
	{ // baseline
		Name:         "base",
		ThresholdMin: 0,
		Accent:       lipgloss.Color("#8aa788"),
		Muted:        lipgloss.Color("#6f7a70"),
		SelectedBg:   lipgloss.Color("#2b312a"),
	},
	{ // 4h blueish
		Name:         "deep-blue",
		ThresholdMin: 240,
		Accent:       lipgloss.Color("#7fb3ff"),
		Muted:        lipgloss.Color("#5c6b80"),
		SelectedBg:   lipgloss.Color("#1f2b3a"),
	},
	{ // 6h blackish
		Name:         "night-mode",
		ThresholdMin: 360,
		Accent:       lipgloss.Color("#dfe5dd"),
		Muted:        lipgloss.Color("#4a4f4a"),
		SelectedBg:   lipgloss.Color("#151515"),
	},
	{ // 8h amber
		Name:         "deep-amber",
		ThresholdMin: 480,
		Accent:       lipgloss.Color("#FFA132"),
		Muted:        lipgloss.Color("#6f7a70"),
		SelectedBg:   lipgloss.Color("#3a2b1f"),
	},
	{ // 10h red
		Name:         "alert-red",
		ThresholdMin: 600,
		Accent:       lipgloss.Color("#ff4d4d"),
		Muted:        lipgloss.Color("#6f7a70"),
		SelectedBg:   lipgloss.Color("#3a1f1f"),
	},
}

func newModel(path string) model {
	m := model{
		statePath: path,
		tickRate:  450 * time.Millisecond,
		actions:   []string{actionStart, actionStop, actionStatus, actionBreak, actionRelax},
		view:      "main",
	}
	m.game = newGameState()
	m.reload(time.Now())
	return m
}

func (m model) Init() tea.Cmd {
	return tick(m.tickRate)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "w":
			if m.view == "main" {
				m.view = "week"
			} else {
				m.view = "main"
			}
			return m, nil
		case "esc":
			if m.view == "game" {
				m.view = "main"
			} else {
				m.view = "main"
			}
			return m, nil
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "enter", " ":
			if m.view == "game" {
				m.game.launch()
			} else {
				m.execute(time.Now())
			}
			return m, tick(m.tickRate)
		case "left", "h", "a":
			if m.view == "game" {
				m.game.movePaddle(-1)
				return m, nil
			}
		case "right", "l", "d":
			if m.view == "game" {
				m.game.movePaddle(1)
				return m, nil
			}
		case "r":
			if m.view == "game" {
				m.game.reset()
				return m, nil
			}

			m.view = "game"
			m.notice = "Relax mode: Block Breaker"
			m.game.reset()
			return m, nil

		case "+":
			m.notice, m.err = changeGoal(m.statePath, goalStepMinutes)
			m.reload(time.Now())
			return m, tick(m.tickRate)
		case "-":
			m.notice, m.err = changeGoal(m.statePath, -goalStepMinutes)
			m.reload(time.Now())
			return m, tick(m.tickRate)
		case "[":
			m.notice, m.err = changeBreak(m.statePath, -breakStepMinutes)
			m.reload(time.Now())
			return m, tick(m.tickRate)
		case "]":
			m.notice, m.err = changeBreak(m.statePath, breakStepMinutes)
			m.reload(time.Now())
			return m, tick(m.tickRate)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		m.spin = (m.spin + 1) % len(spinnerRunFrames)
		if m.view == "game" {
			m.game.tick()
		} else {
			m.reload(time.Time(msg))
		}
		return m, tick(m.tickRate)
	}
	return m, nil
}

func (m *model) move(delta int) {
	m.selected = (m.selected + delta + len(m.actions)) % len(m.actions)
}

func (m *model) execute(now time.Time) {
	m.notice = ""
	m.err = nil

	switch m.actions[m.selected] {
	case actionStart:
		// If currently on a break, end it before resuming work.
		if m.summary.onBreak {
			if note, err := stopBreak(m.statePath, now); err != nil {
				m.err = err
				break
			} else {
				m.notice = note
			}
		}
		note, err := startSession(m.statePath, now, nil, "")
		m.err = err
		if note != "" {
			m.notice = note
		}
	case actionStop:
		note, err := stopSession(m.statePath, now)
		m.err = err
		m.notice = note
	case actionStatus:
		m.notice = fmt.Sprintf("Today %s (active %s)", state.HumanMinutes(m.summary.workMinutes), state.HumanMinutes(m.summary.activeMinutes))
	case actionBreak:
		if m.summary.onBreak {
			note, err := stopBreak(m.statePath, now)
			m.err = err
			m.notice = note
		} else {
			note, err := startBreak(m.statePath, now)
			m.err = err
			m.notice = note
		}
	case actionRelax:
		m.view = "game"
		m.notice = "Relax mode: Block Breaker"
		m.game.reset()
	}

	m.reload(now)
}

func (m *model) reload(now time.Time) {
	st, err := state.Load(m.statePath)
	if err != nil {
		m.err = err
		return
	}
	st.Normalize(now)
	work, active := st.TodaySummary(now)
	m.dayKey = now.Format("2006-01-02")
	if m.dayKey != m.lastDay {
		m.lastDay = m.dayKey
		m.lastMilestone = 0
	}
	m.summary = summary{
		workMinutes:   work,
		activeMinutes: active,
		goalMinutes:   st.GoalMinutes,
		breakMinutes:  st.BreakIntervalMinutes,
	}
	if st.ActiveSession != nil {
		m.summary.activeSince = &st.ActiveSession.Start
		m.summary.activeSeconds = int(now.Sub(st.ActiveSession.Start).Seconds()) % 60
	}
	if st.ActiveBreak != nil {
		m.summary.onBreak = true
		breakDuration := now.Sub(st.ActiveBreak.Start)
		m.summary.activeMinutes = int(breakDuration.Minutes())
		m.summary.activeSeconds = int(breakDuration.Seconds()) % 60
	}
	if log, ok := st.Days[m.dayKey]; ok {
		m.summary.sessions = log.Sessions
		m.summary.breakMinutes = log.TotalBreakMinutes
		m.summary.breaksCount = log.BreakCount
		if log.TotalWorkSeconds > 0 {
			m.summary.workSeconds = log.TotalWorkSeconds
		} else if log.TotalWorkMinutes > 0 {
			m.summary.workSeconds = log.TotalWorkMinutes * 60
		}
	}
	if m.summary.activeSince != nil {
		m.summary.workSeconds += int(now.Sub(*m.summary.activeSince).Seconds())
	}

	for _, theme := range milestoneThemes {
		if m.summary.workMinutes >= theme.ThresholdMin && theme.ThresholdMin > m.lastMilestone {
			m.lastMilestone = theme.ThresholdMin
			m.notice = fmt.Sprintf("Milestone reached: %s (%s)", state.HumanMinutes(theme.ThresholdMin), theme.Name)
		}
	}
	m.loaded = true
	m.err = nil
}

func (m model) View() string {
	if !m.loaded {
		return "daily\nloading..."
	}

	if m.view == "week" {
		return m.renderWeek()
	}
	if m.view == "game" {
		return m.renderGame()
	}

	th := themeForMinutes(m.summary.workMinutes)

	localTitle := titleStyle.Foreground(th.Accent)
	localArrow := arrowStyle.Foreground(th.Accent)
	localSelected := selectedStyle.Background(th.SelectedBg)
	localNotice := noticeStyle.Foreground(th.Accent)
	localHint := hintStyle.Foreground(th.Muted)

	title := localTitle.Render(renderBigTitle())

	var noticeLine string
	if m.err != nil {
		noticeLine = errorStyle.Render(fmt.Sprintf("error: %v", m.err))
	} else if m.notice != "" {
		noticeLine = localNotice.Render(m.notice)
	} else {
		noticeLine = ""
	}

	// Fixed label width for alignment; arrows only on the selected row.
	maxLabel := 0
	for _, act := range m.actions {
		w := lipgloss.Width(actionLabel(act))
		if w > maxLabel {
			maxLabel = w
		}
	}
	labelWidth := maxLabel + 4 // breathing room

	menuLines := make([]string, 0, len(m.actions))
	for i, act := range m.actions {
		label := actionLabel(act)
		if i == m.selected {
			box := localSelected.Width(labelWidth).Align(lipgloss.Center).Render(label)
			menuLines = append(menuLines, lipgloss.JoinHorizontal(lipgloss.Center,
				localArrow.Render("◀"),
				box,
				localArrow.Render("▶"),
			))
		} else {
			box := menuStyle.Width(labelWidth).Align(lipgloss.Center).Render(label)
			menuLines = append(menuLines, box)
		}
	}

	hints := localHint.Render("+/- goal   [/] break   r relax   TAB week   ENTER select   q quit")

	body := lipgloss.JoinVertical(lipgloss.Center,
		title,
		noticeLine,
		lipgloss.JoinVertical(lipgloss.Center, menuLines...),
		hints,
	)

	content := body
	if m.width > 0 && m.height > 0 {
		content = lipgloss.Place(m.width, m.height-statusBarHeight, lipgloss.Center, lipgloss.Center, body)
	}

	bottom := m.renderStatusBar()
	if m.width > 0 {
		bottom = lipgloss.Place(m.width, statusBarHeight, lipgloss.Center, lipgloss.Center, bottom)
	}

	view := lipgloss.JoinVertical(lipgloss.Left, content, bottom)
	if m.width > 0 && m.height > 0 {
		return baseStyle.Width(m.width).Height(m.height).Render(view)
	}
	return baseStyle.Render(view)
}

func (m model) renderGame() string {
	th := themeForMinutes(m.summary.workMinutes)
	title := titleStyle.Foreground(th.Accent).Render("BLOCK BREAKER")
	subtitle := hintStyle.Foreground(th.Muted).Render("←/→ move  SPACE launch  r reset  esc back")

	gameBoard := m.game.render()
	body := lipgloss.JoinVertical(lipgloss.Center, title, subtitle, gameBoard)
	if m.width > 0 && m.height > 0 {
		body = lipgloss.Place(m.width, m.height-statusBarHeight, lipgloss.Center, lipgloss.Center, body)
	}
	bottom := m.renderStatusBar()
	if m.width > 0 {
		bottom = lipgloss.Place(m.width, statusBarHeight, lipgloss.Center, lipgloss.Center, bottom)
	}
	view := lipgloss.JoinVertical(lipgloss.Left, body, bottom)
	if m.width > 0 && m.height > 0 {
		return baseStyle.Width(m.width).Height(m.height).Render(view)
	}
	return baseStyle.Render(view)
}

func renderBigTitle() string {
	lines := []string{
		"██████╗  █████╗ ██╗██╗  ██╗   ██╗",
		"██╔══██╗██╔══██╗██║██║  ╚██╗ ██╔╝",
		"██║  ██║███████║██║██║   ╚████╔╝ ",
		"██║  ██║██╔══██║██║██║    ╚██╔╝  ",
		"██████╔╝██║  ██║██║███████╗██║   ",
		"╚═════╝ ╚═╝  ╚═╝╚═╝╚══════╝╚═╝   ",
	}
	return strings.Join(lines, "\n")
}

type spinnerPalette struct {
	bright lipgloss.Color
	mid    lipgloss.Color
	dim    lipgloss.Color
}

func buildSpinnerFrames(palette spinnerPalette) []string {
	// Create a 2x2 spinner with a bright block and a trailing mid block.
	frames := make([]string, 4)
	for i := 0; i < 4; i++ {
		colors := [4]lipgloss.Color{
			palette.dim,
			palette.dim,
			palette.dim,
			palette.dim,
		}
		colors[i] = palette.bright
		colors[(i+1)%4] = palette.mid
		frames[i] = renderSpinnerGrid(colors)
	}
	return frames
}

func buildDimSpinnerFrame(dimColor lipgloss.Color) string {
	colors := [4]lipgloss.Color{dimColor, dimColor, dimColor, dimColor}
	return renderSpinnerGrid(colors)
}

func renderSpinnerGrid(colors [4]lipgloss.Color) string {
	left := lipgloss.NewStyle().
		Foreground(colors[0]).
		Background(colors[3]).
		Render("▀")
	right := lipgloss.NewStyle().
		Foreground(colors[1]).
		Background(colors[2]).
		Render("▀")
	return left + right
}

func (m model) renderStatusBar() string {
	running := m.summary.activeMinutes > 0 || m.summary.activeSince != nil
	statusText := "PAUSED"
	statusStyle := statusDim
	spin := spinnerDimFrame

	if m.summary.onBreak {
		statusText = "BREAK"
		statusStyle = statusBreak
		spin = spinnerDimFrame
	} else if running {
		statusText = "RUNNING"
		statusStyle = statusRun
		spin = spinnerRunFrames[m.spin]
	}

	workHours := m.summary.workSeconds / 3600
	workMinutes := (m.summary.workSeconds % 3600) / 60
	seconds := m.summary.workSeconds % 60
	workStr := fmt.Sprintf("^ %d HOURS", workHours)
	activeStr := fmt.Sprintf("^ %d MIN", workMinutes)
	secText := fmt.Sprintf("~ %02d SEC", seconds)
	secStr := statusHalf.Render(secText)
	breakStr := fmt.Sprintf("%d BREAKS", m.summary.breaksCount)

	return lipgloss.JoinHorizontal(lipgloss.Center,
		spin,
		" ",
		statusStyle.Render(statusText),
		"  ",
		statusDim.Render(workStr),
		"  ",
		statusDim.Render(activeStr),
		"  ",
		secStr,
		"  ",
		statusDim.Render(breakStr),
	)
}

func (m model) renderWeek() string {
	st, err := state.Load(m.statePath)
	if err != nil {
		return baseStyle.Render(errorStyle.Render(err.Error()))
	}
	keys := make([]string, 0, len(st.Days))
	for k := range st.Days {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return baseStyle.Render("no history yet (TAB to main)")
	}
	sort.Strings(keys)
	if len(keys) > 7 {
		keys = keys[len(keys)-7:]
	}

	maxWork := 0
	for _, k := range keys {
		if st.Days[k].TotalWorkMinutes > maxWork {
			maxWork = st.Days[k].TotalWorkMinutes
		}
	}
	if maxWork == 0 {
		maxWork = 1
	}

	barWidth := 24
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		log := st.Days[k]
		th := themeForMinutes(log.TotalWorkMinutes)
		dateStyle := weekDateStyle.Foreground(th.Accent)
		barStyle := weekBarStyle.Foreground(th.Accent)
		valueStyle := weekValueStyle.Foreground(th.Muted)

		barLen := int(float64(log.TotalWorkMinutes) / float64(maxWork) * float64(barWidth))
		if barLen < 1 && log.TotalWorkMinutes > 0 {
			barLen = 1
		}
		bar := barStyle.Render(strings.Repeat("█", barLen))
		info := valueStyle.Render(fmt.Sprintf("%s  %d breaks  %s brk",
			state.HumanMinutes(log.TotalWorkMinutes),
			log.BreakCount,
			state.HumanMinutes(log.TotalBreakMinutes),
		))
		line := lipgloss.JoinHorizontal(lipgloss.Left,
			dateStyle.Render(k),
			bar,
			info,
		)
		lines = append(lines, line)
	}

	hints := hintStyle.Render("TAB back   q quit")
	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	if m.width > 0 && m.height > 0 {
		body = lipgloss.Place(m.width, m.height-statusBarHeight, lipgloss.Center, lipgloss.Center, body)
	}
	view := lipgloss.JoinVertical(lipgloss.Left, body, hints)
	return baseStyle.Render(view)
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func startSession(path string, now time.Time, tags []string, note string) (string, error) {
	st, err := state.Load(path)
	if err != nil {
		return "", err
	}
	if err := st.StartSession(now, tags, note); err != nil {
		return "", err
	}
	if err := st.Save(path); err != nil {
		return "", err
	}
	return fmt.Sprintf("Started at %s", now.Format(time.Kitchen)), nil
}

func stopSession(path string, now time.Time) (string, error) {
	st, err := state.Load(path)
	if err != nil {
		return "", err
	}
	mins, err := st.StopSession(now)
	if err != nil {
		return "", err
	}
	if err := st.Save(path); err != nil {
		return "", err
	}
	return fmt.Sprintf("Stopped (%s)", state.HumanMinutes(mins)), nil
}

func startBreak(path string, now time.Time) (string, error) {
	st, err := state.Load(path)
	if err != nil {
		return "", err
	}
	if err := st.StartBreak(now); err != nil {
		return "", err
	}
	if err := st.Save(path); err != nil {
		return "", err
	}
	return fmt.Sprintf("Break started %s", now.Format(time.Kitchen)), nil
}

func stopBreak(path string, now time.Time) (string, error) {
	st, err := state.Load(path)
	if err != nil {
		return "", err
	}
	mins, err := st.StopBreak(now)
	if err != nil {
		return "", err
	}
	if err := st.Save(path); err != nil {
		return "", err
	}
	return fmt.Sprintf("Break ended (%s)", state.HumanMinutes(mins)), nil
}

// themeForMinutes selects the active theme based on minutes worked.
// Edit milestoneThemes to customize colors per threshold.
func themeForMinutes(mins int) milestoneTheme {
	best := milestoneThemes[0]
	for _, th := range milestoneThemes {
		if mins >= th.ThresholdMin && th.ThresholdMin >= best.ThresholdMin {
			best = th
		}
	}
	return best
}

func changeGoal(path string, delta int) (string, error) {
	st, err := state.Load(path)
	if err != nil {
		return "", err
	}
	newVal := st.GoalMinutes + delta
	if newVal < minGoalMinutes {
		newVal = minGoalMinutes
	}
	st.GoalMinutes = newVal
	if err := st.Save(path); err != nil {
		return "", err
	}
	return fmt.Sprintf("Goal set to %s", state.HumanMinutes(newVal)), nil
}

func changeBreak(path string, delta int) (string, error) {
	st, err := state.Load(path)
	if err != nil {
		return "", err
	}
	newVal := st.BreakIntervalMinutes + delta
	if newVal < minBreakMinutes {
		newVal = minBreakMinutes
	}
	st.BreakIntervalMinutes = newVal
	if err := st.Save(path); err != nil {
		return "", err
	}
	return fmt.Sprintf("Break every %s", state.HumanMinutes(newVal)), nil
}

func actionLabel(action string) string {
	switch action {
	case actionStart:
		return "START"
	case actionStop:
		return "STOP"
	case actionStatus:
		return "STATUS"
	case actionBreak:
		return "BREAK"
	case actionRelax:
		return "RELAX"
	default:
		return action
	}
}

type gameState struct {
	width        int
	height       int
	paddleX      int
	paddleWidth  int
	ballX        int
	ballY        int
	ballVX       int
	ballVY       int
	ballLaunched bool
	bricks       [][]bool
	score        int
	lives        int
	message      string
}

func newGameState() gameState {
	gs := gameState{
		width:       30,
		height:      14,
		paddleWidth: 6,
		lives:       3,
	}
	gs.reset()
	return gs
}

func (g *gameState) reset() {
	g.score = 0
	g.lives = 3
	g.message = "Press SPACE to launch"
	g.initBricks()
	g.resetBall()
}

func (g *gameState) initBricks() {
	rows := 4
	cols := 10
	g.bricks = make([][]bool, rows)
	for r := range g.bricks {
		g.bricks[r] = make([]bool, cols)
		for c := range g.bricks[r] {
			g.bricks[r][c] = true
		}
	}
}

func (g *gameState) resetBall() {
	g.paddleX = g.width/2 - g.paddleWidth/2
	g.ballX = g.paddleX + g.paddleWidth/2
	g.ballY = g.height - 2
	g.ballVX = 1
	g.ballVY = -1
	g.ballLaunched = false
}

func (g *gameState) launch() {
	if g.lives == 0 {
		g.reset()
		return
	}
	if !g.ballLaunched {
		g.ballLaunched = true
		g.message = ""
	}
}

func (g *gameState) movePaddle(dir int) {
	if g.lives == 0 {
		return
	}
	g.paddleX += dir * 2
	if g.paddleX < 1 {
		g.paddleX = 1
	}
	maxX := g.width - g.paddleWidth - 1
	if g.paddleX > maxX {
		g.paddleX = maxX
	}
	if !g.ballLaunched {
		g.ballX = g.paddleX + g.paddleWidth/2
	}
}

func (g *gameState) tick() {
	if !g.ballLaunched || g.lives == 0 {
		return
	}
	nextX := g.ballX + g.ballVX
	nextY := g.ballY + g.ballVY

	if nextX <= 1 || nextX >= g.width-2 {
		g.ballVX *= -1
		nextX = g.ballX + g.ballVX
	}
	if nextY <= 1 {
		g.ballVY *= -1
		nextY = g.ballY + g.ballVY
	}

	if nextY >= g.height-2 {
		g.lives--
		if g.lives == 0 {
			g.message = "Game over. Press r to reset"
			return
		}
		g.message = "Missed! Press SPACE"
		g.resetBall()
		return
	}

	if nextY == g.height-3 && nextX >= g.paddleX && nextX <= g.paddleX+g.paddleWidth-1 {
		g.ballVY *= -1
		nextY = g.ballY + g.ballVY
	}

	if g.checkBrickCollision(nextX, nextY) {
		g.ballVY *= -1
		nextY = g.ballY + g.ballVY
	}

	g.ballX = nextX
	g.ballY = nextY
}

func (g *gameState) checkBrickCollision(x, y int) bool {
	brickTop := 2
	brickHeight := 1
	brickWidth := 3
	for r, row := range g.bricks {
		for c, alive := range row {
			if !alive {
				continue
			}
			bx := 1 + c*brickWidth
			by := brickTop + r*brickHeight
			if x >= bx && x < bx+brickWidth && y == by {
				g.bricks[r][c] = false
				g.score += 10
				if g.allBricksCleared() {
					g.message = "You cleared all bricks! Press r"
					g.ballLaunched = false
				}
				return true
			}
		}
	}
	return false
}

func (g *gameState) allBricksCleared() bool {
	for _, row := range g.bricks {
		for _, alive := range row {
			if alive {
				return false
			}
		}
	}
	return true
}

func (g *gameState) render() string {
	board := make([][]rune, g.height)
	for y := range board {
		board[y] = make([]rune, g.width)
		for x := range board[y] {
			if y == 0 || y == g.height-1 {
				board[y][x] = '─'
			} else if x == 0 || x == g.width-1 {
				board[y][x] = '│'
			} else {
				board[y][x] = ' '
			}
		}
		board[y][0] = '│'
		board[y][g.width-1] = '│'
	}
	board[0][0] = '┌'
	board[0][g.width-1] = '┐'
	board[g.height-1][0] = '└'
	board[g.height-1][g.width-1] = '┘'

	for r, row := range g.bricks {
		for c, alive := range row {
			if !alive {
				continue
			}
			x := 1 + c*3
			y := 2 + r
			for i := 0; i < 3; i++ {
				board[y][x+i] = '█'
			}
		}
	}

	for i := 0; i < g.paddleWidth; i++ {
		board[g.height-2][g.paddleX+i] = '▂'
	}

	if g.ballLaunched || g.message != "" {
		board[g.ballY][g.ballX] = '●'
	}

	lines := make([]string, 0, g.height+2)
	for _, row := range board {
		lines = append(lines, string(row))
	}
	scoreLine := fmt.Sprintf("Score %d  Lives %d", g.score, g.lives)
	if g.message != "" {
		scoreLine = fmt.Sprintf("%s  •  %s", scoreLine, g.message)
	}
	lines = append(lines, scoreLine)
	return strings.Join(lines, "\n")
}

var (
	baseStyle        = lipgloss.NewStyle().Background(lipgloss.Color("#222222"))
	titleStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#8aa788")).Bold(true).MarginBottom(2)
	menuStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#e4e4e4")).PaddingLeft(2).PaddingRight(2)
	selectedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#e4e4e4")).Background(lipgloss.Color("#2b312a")).PaddingLeft(2).PaddingRight(2)
	arrowStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#8aa788")).PaddingLeft(1).PaddingRight(1)
	arrowDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#656D65")).PaddingLeft(1).PaddingRight(1)
	noticeStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#8aa788")).MarginBottom(1)
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb347")).MarginBottom(1)
	hintStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#6f7a70")).MarginTop(1)
	statusDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("#656D65"))
	statusBreak      = lipgloss.NewStyle().Foreground(lipgloss.Color("#656D65")).Bold(true)
	statusRun        = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA132"))
	statusHalf       = lipgloss.NewStyle().Foreground(lipgloss.Color("#4a504a"))
	weekDateStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8aa788")).PaddingRight(1)
	weekValueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#dfe5dd")).PaddingLeft(1)
	weekBarStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#8aa788"))
	spinnerRunFrames = buildSpinnerFrames(spinnerPalette{
		bright: lipgloss.Color("#FFA132"),
		mid:    lipgloss.Color("#90612A"),
		dim:    lipgloss.Color("#5B330E"),
	})
	spinnerDimFrame = buildDimSpinnerFrame(lipgloss.Color("#3E423E"))
)
