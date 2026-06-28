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
	title      string
	rows       []Row
	selIdx     []int
	cursor     int
	top        int
	height     int
	selectMode bool
	chosen     *model.Item
	quit       bool
}

func newListModel(title string, rows []Row, selectMode bool) listModel {
	var sel []int
	for i, r := range rows {
		if r.Item != nil {
			sel = append(sel, i)
		}
	}
	return listModel{
		title:      title,
		rows:       rows,
		selIdx:     sel,
		height:     20,
		selectMode: selectMode,
	}
}

func (m listModel) Init() tea.Cmd { return nil }

func (m *listModel) ensureVisible() {
	if len(m.selIdx) == 0 {
		return
	}
	target := m.selIdx[m.cursor]
	if target < m.top {
		m.top = target
	}
	if target >= m.top+m.height {
		m.top = target - m.height + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := msg.Height - 5
		if h < 5 {
			h = 5
		}
		m.height = h
		m.ensureVisible()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quit = true
			return m, tea.Quit
		case "up", "k":
			if len(m.selIdx) > 0 && m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			} else if len(m.selIdx) == 0 && m.top > 0 {
				m.top--
			}
		case "down", "j":
			if len(m.selIdx) > 0 && m.cursor < len(m.selIdx)-1 {
				m.cursor++
				m.ensureVisible()
			} else if len(m.selIdx) == 0 && m.top < len(m.rows)-m.height {
				m.top++
			}
		case "enter":
			if m.selectMode && len(m.selIdx) > 0 {
				it := m.rows[m.selIdx[m.cursor]].Item
				m.chosen = it
				m.quit = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m listModel) renderRow(idx int, r Row) string {
	switch {
	case r.Separator:
		return DividerStyle.Render("  " + strings.Repeat("─", 58))
	case r.Header:
		return SectionStyle.Render("  " + r.Text)
	}
	selected := m.selectMode && len(m.selIdx) > 0 && m.selIdx[m.cursor] == idx
	prefix := "    "
	if m.selectMode {
		prefix = "    "
		if selected {
			prefix = "  ▸ "
		}
	}
	if selected {
		// High-visibility selection: blue background, white text, bold.
		selectedStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#185FA5")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)
		return selectedStyle.Render(prefix + r.Text)
	}
	style := toneStyle(r.Tone)
	return style.Render(prefix + r.Text)
}

func (m listModel) View() string {
	if m.quit {
		return ""
	}
	var b strings.Builder
	b.WriteString(TitleStyle.Render("  "+m.title) + "\n\n")

	end := m.top + m.height
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.top; i < end; i++ {
		b.WriteString(m.renderRow(i, m.rows[i]) + "\n")
	}

	help := "  ↑/↓ scroll · q quit"
	if m.selectMode {
		help = "  ↑/↓ move · enter select · q cancel"
	}

	// Build position indicator.
	posIndicator := ""
	if len(m.selIdx) > 0 {
		// Show focused item position out of total selectable items.
		posIndicator = fmt.Sprintf("[ %d/%d ]", m.cursor+1, len(m.selIdx))
	} else if len(m.rows) > m.height {
		// No selectable items (read-only list) — show row position instead.
		posIndicator = fmt.Sprintf("[ %d/%d ]", m.top+1, len(m.rows))
	}

	helpLine := HelpStyle.Render(help)
	if posIndicator != "" {
		// Pad between help and indicator.
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
