// Package ui holds all lipgloss styling plus the two bubbletea programs trackr
// uses: a spinner that runs while a scan executes, and an interactive list that
// works both for read-only browsing (scan) and for picking an item (where/remove).
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"trackr/internal/model"
)

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

var (
	ColorGreen  = lipgloss.Color("#3B6D11")
	ColorRed    = lipgloss.Color("#A32D2D")
	ColorYellow = lipgloss.Color("#854F0B")
	ColorDim    = lipgloss.Color("#888780")
	ColorAccent = lipgloss.Color("#185FA5")

	TitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	SectionStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	DividerStyle = lipgloss.NewStyle().Foreground(ColorDim)
	HelpStyle    = lipgloss.NewStyle().Foreground(ColorDim).Italic(true)
	NoteStyle    = lipgloss.NewStyle().Foreground(ColorDim)
	SpinnerStyle = lipgloss.NewStyle().Foreground(ColorAccent)

	CleanStyle  = lipgloss.NewStyle().Foreground(ColorGreen)
	YellowStyle = lipgloss.NewStyle().Foreground(ColorYellow)
	OrphanStyle = lipgloss.NewStyle().Foreground(ColorRed)
	PkgStyle    = lipgloss.NewStyle().Foreground(ColorDim)
	PlainStyle  = lipgloss.NewStyle()
	BoldStyle   = lipgloss.NewStyle().Bold(true)
)

// Tone hints select a colour for a row.
const (
	TonePlain  = "plain"
	ToneClean  = "clean"
	ToneYellow = "yellow"
	ToneOrphan = "orphan"
	TonePkg    = "pkg"
)

func toneStyle(tone string) lipgloss.Style {
	switch tone {
	case ToneClean:
		return CleanStyle
	case ToneYellow:
		return YellowStyle
	case ToneOrphan:
		return OrphanStyle
	case TonePkg:
		return PkgStyle
	default:
		return PlainStyle
	}
}

// ToneForStatus maps a model status string to a render tone.
func ToneForStatus(status string) string {
	switch status {
	case model.StatusClean:
		return ToneClean
	case model.StatusNoUninstall:
		return ToneYellow
	case model.StatusOrphan:
		return ToneOrphan
	case model.StatusPkg:
		return TonePkg
	default:
		return TonePlain
	}
}

// ---------------------------------------------------------------------------
// Scan result + spinner program
// ---------------------------------------------------------------------------

// ScanResult bundles everything a full system scan produces.
type ScanResult struct {
	Pkg            []model.Item
	Exe            []model.Item
	Folders        []model.Item
	RegistryGhosts []model.Item
	FolderGhosts   []model.Item
	Notes          []string
}

type statusMsg string
type scanDoneMsg struct{ res ScanResult }

type scanModel struct {
	sp       spinner.Model
	status   string
	statusCh chan string
	resultCh chan ScanResult
	res      ScanResult
	done     bool
}

func (m scanModel) Init() tea.Cmd {
	return tea.Batch(m.sp.Tick, waitForScan(m.statusCh, m.resultCh))
}

func waitForScan(statusCh chan string, resultCh chan ScanResult) tea.Cmd {
	return func() tea.Msg {
		s, ok := <-statusCh
		if !ok {
			return scanDoneMsg{res: <-resultCh}
		}
		return statusMsg(s)
	}
}

func (m scanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.status = string(msg)
		return m, waitForScan(m.statusCh, m.resultCh)
	case scanDoneMsg:
		m.res = msg.res
		m.done = true
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m scanModel) View() string {
	if m.done {
		return ""
	}
	return fmt.Sprintf("  %s %s\n", m.sp.View(), m.status)
}

// RunScan shows a spinner while scan executes in the background. The scan
// function reports progress by sending strings on the provided channel; it must
// return the final ScanResult. The channel is closed automatically afterwards.
func RunScan(scan func(status chan<- string) ScanResult) (ScanResult, error) {
	statusCh := make(chan string, 32)
	resultCh := make(chan ScanResult, 1)
	go func() {
		res := scan(statusCh)
		close(statusCh)
		resultCh <- res
	}()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = SpinnerStyle

	m := scanModel{
		sp:       sp,
		status:   "Starting scan...",
		statusCh: statusCh,
		resultCh: resultCh,
	}
	fm, err := tea.NewProgram(m).Run()
	if err != nil {
		return ScanResult{}, err
	}
	return fm.(scanModel).res, nil
}

// ---------------------------------------------------------------------------
// Interactive list / selection program
// ---------------------------------------------------------------------------

// Row is one line in the interactive list. A row is selectable only if Item is
// non-nil; Header and Separator rows are decorative.
type Row struct {
	Header    bool
	Separator bool
	Text      string
	Tone      string
	Item      *model.Item
}

type listModel struct {
	title       string
	rows        []Row
	cursorPos   int // position within the currently-visible row list
	top         int // scroll offset within the visible row list
	height      int
	width       int
	selectMode  bool
	chosen      *model.Item
	quit        bool
	searching   bool
	searchQuery string
	filtered    []int // row indices matching searchQuery; nil = no filter
}

func newListModel(title string, rows []Row, selectMode bool) listModel {
	m := listModel{
		title:      title,
		rows:       rows,
		height:     20,
		selectMode: selectMode,
	}
	if np := m.navPositions(); len(np) > 0 {
		m.cursorPos = np[0]
	}
	return m
}

func (m listModel) Init() tea.Cmd { return nil }

// visibleRowIndices returns the row indices currently shown, honoring any
// active search filter.
func (m *listModel) visibleRowIndices() []int {
	if m.filtered != nil {
		return m.filtered
	}
	out := make([]int, len(m.rows))
	for i := range m.rows {
		out[i] = i
	}
	return out
}

// navPositions returns the positions (within the visible list) that the cursor
// may land on — i.e. rows that are neither headers nor separators.
func (m *listModel) navPositions() []int {
	visible := m.visibleRowIndices()
	var pos []int
	for p, rowIdx := range visible {
		r := m.rows[rowIdx]
		if !r.Header && !r.Separator {
			pos = append(pos, p)
		}
	}
	return pos
}

func (m *listModel) ensureVisible() {
	if m.cursorPos < m.top {
		m.top = m.cursorPos
	}
	if m.cursorPos >= m.top+m.height {
		m.top = m.cursorPos - m.height + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

// moveCursor advances the cursor to the next/previous navigable position,
// skipping header and separator rows.
func (m *listModel) moveCursor(dir int) {
	np := m.navPositions()
	if len(np) == 0 {
		return
	}
	cur := -1
	for i, p := range np {
		if p == m.cursorPos {
			cur = i
			break
		}
	}
	if cur == -1 {
		m.cursorPos = np[0]
	} else if ni := cur + dir; ni >= 0 && ni < len(np) {
		m.cursorPos = np[ni]
	}
	m.ensureVisible()
}

func (m *listModel) applyFilter() {
	if m.searchQuery == "" {
		m.filtered = nil
	} else {
		q := strings.ToLower(m.searchQuery)
		m.filtered = nil
		for i, r := range m.rows {
			if r.Header || r.Separator {
				continue
			}
			if strings.Contains(strings.ToLower(r.Text), q) {
				m.filtered = append(m.filtered, i)
			}
		}
	}
	m.top = 0
	if np := m.navPositions(); len(np) > 0 {
		m.cursorPos = np[0]
	} else {
		m.cursorPos = 0
	}
}

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := msg.Height - 6
		if h < 5 {
			h = 5
		}
		m.height = h
		m.width = msg.Width
		m.ensureVisible()
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			m.quit = true
			return m, tea.Quit
		}

		// While typing a search query, capture keys for the query.
		if m.searching {
			switch key {
			case "esc":
				m.searching = false
				m.searchQuery = ""
				m.applyFilter()
			case "enter":
				m.searching = false // keep the filter applied, just exit input mode
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				}
				m.applyFilter()
			default:
				if len([]rune(key)) == 1 {
					m.searchQuery += key
					m.applyFilter()
				}
			}
			return m, nil
		}

		switch key {
		case "q":
			m.quit = true
			return m, tea.Quit
		case "esc":
			if m.filtered != nil {
				// Clear the active filter rather than quitting.
				m.searchQuery = ""
				m.applyFilter()
				return m, nil
			}
			m.quit = true
			return m, tea.Quit
		case "/":
			m.searching = true
			m.searchQuery = ""
			return m, nil
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
		case "enter":
			if m.selectMode {
				visible := m.visibleRowIndices()
				if m.cursorPos < len(visible) {
					if it := m.rows[visible[m.cursorPos]].Item; it != nil {
						m.chosen = it
						m.quit = true
						return m, tea.Quit
					}
				}
			}
		}
	}
	return m, nil
}

func (m listModel) renderRow(pos, rowIdx int) string {
	r := m.rows[rowIdx]
	switch {
	case r.Separator:
		return DividerStyle.Render("  " + strings.Repeat("─", 58))
	case r.Header:
		return SectionStyle.Render("  " + r.Text)
	}
	selected := pos == m.cursorPos
	prefix := "    "
	if m.selectMode && selected {
		prefix = "  ▸ "
	}
	if selected {
		// High-visibility position highlight (works in read-only lists too).
		return menuSelectedStyle.Render(prefix + r.Text)
	}
	return toneStyle(r.Tone).Render(prefix + r.Text)
}

func (m listModel) View() string {
	if m.quit {
		return ""
	}
	var b strings.Builder
	b.WriteString(TitleStyle.Render("  "+m.title) + "\n")

	if m.searching || m.searchQuery != "" {
		searchBar := fmt.Sprintf("  / %s█", m.searchQuery)
		if !m.searching {
			searchBar = fmt.Sprintf("  / %s  (esc to clear)", m.searchQuery)
		}
		b.WriteString(HelpStyle.Render(searchBar) + "\n")
	}
	b.WriteString("\n")

	visible := m.visibleRowIndices()
	end := m.top + m.height
	if end > len(visible) {
		end = len(visible)
	}
	for pos := m.top; pos < end; pos++ {
		b.WriteString(m.renderRow(pos, visible[pos]) + "\n")
	}
	if len(visible) == 0 {
		b.WriteString(NoteStyle.Render("  (no matches)") + "\n")
	}

	help := "  ↑/↓ scroll · / search · q quit"
	if m.selectMode {
		help = "  ↑/↓ navigate · enter select · / search · q quit"
	}

	// Build position indicator (cursor rank out of navigable rows).
	posIndicator := ""
	np := m.navPositions()
	if len(np) > 0 {
		rank := 0
		for i, p := range np {
			if p == m.cursorPos {
				rank = i + 1
				break
			}
		}
		posIndicator = fmt.Sprintf("[ %d/%d ]", rank, len(np))
	}

	helpLine := HelpStyle.Render(help)
	if posIndicator != "" {
		helpLine = helpLine + "   " + HelpStyle.Render(posIndicator)
	}
	b.WriteString("\n" + helpLine)
	return b.String()
}

// RunList displays an interactive list. When selectMode is true, the returned
// pointer is the item the user chose with Enter (nil if they cancelled).
func RunList(title string, rows []Row, selectMode bool) (*model.Item, error) {
	m := newListModel(title, rows, selectMode)
	fm, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}
	return fm.(listModel).chosen, nil
}
