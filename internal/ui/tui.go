// This file implements the full-screen interactive dashboard shown when the
// user runs `trackr` with no subcommand. The dashboard itself only owns the
// main menu and the small text-input prompts; the actual work (scan, where,
// remove, log) is delegated to injected Handlers so this package never needs to
// import the cmd package (which would create an import cycle).
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Handlers carries the self-contained action callbacks the dashboard invokes.
// Each callback performs its whole flow (including any spinner/list sub-views)
// and returns when the user is done with that screen.
type Handlers struct {
	Scan    func() error
	Orphans func() error
	Where   func(name string) error
	Remove  func(name string) error
	Log     func() error
}

type menuAction int

const (
	actNone menuAction = iota
	actScan
	actWhere
	actRemove
	actOrphans
	actLog
	actQuit
)

type menuResult struct {
	action menuAction
	input  string
}

const (
	screenMenu  = 0
	screenInput = 1
)

var menuItems = []struct {
	label  string
	action menuAction
}{
	{"Scan everything", actScan},
	{"Where is...", actWhere},
	{"Remove something", actRemove},
	{"View orphans", actOrphans},
	{"Install log", actLog},
	{"Quit", actQuit},
}

const trackrBanner = `  ████████╗██████╗  █████╗  ██████╗██╗  ██╗██████╗
  ╚══██╔══╝██╔══██╗██╔══██╗██╔════╝██║ ██╔╝██╔══██╗
     ██║   ██████╔╝███████║██║     █████╔╝ ██████╔╝
     ██║   ██╔══██╗██╔══██║██║     ██╔═██╗ ██╔══██╗
     ██║   ██║  ██║██║  ██║╚██████╗██║  ██╗██║  ██║
     ╚═╝   ╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝╚═╝  ╚═╝`

var bannerStyle = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

var menuSelectedStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#185FA5")).
	Foreground(lipgloss.Color("#FFFFFF")).
	Bold(true)

type tuiModel struct {
	screen        int
	menuCursor    int
	ti            textinput.Model
	pendingAction menuAction
	prompt        string
	result        menuResult
}

func newTUIModel() tuiModel {
	ti := textinput.New()
	ti.CharLimit = 120
	ti.Width = 40
	return tuiModel{screen: screenMenu, ti: ti}
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.result = menuResult{action: actQuit}
			return m, tea.Quit
		}
		if m.screen == screenInput {
			return m.updateInput(msg)
		}
		return m.updateMenu(msg)
	}
	if m.screen == screenInput {
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m tuiModel) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.result = menuResult{action: actQuit}
		return m, tea.Quit
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < len(menuItems)-1 {
			m.menuCursor++
		}
	case "enter":
		action := menuItems[m.menuCursor].action
		switch action {
		case actWhere:
			m.pendingAction = actWhere
			m.prompt = "Enter package/software name:"
			m.screen = screenInput
			m.ti.SetValue("")
			m.ti.Focus()
			return m, textinput.Blink
		case actRemove:
			m.pendingAction = actRemove
			m.prompt = "Enter name to remove:"
			m.screen = screenInput
			m.ti.SetValue("")
			m.ti.Focus()
			return m, textinput.Blink
		default:
			m.result = menuResult{action: action}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m tuiModel) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Back to menu, discard input.
		m.screen = screenMenu
		m.ti.Blur()
		return m, nil
	case "enter":
		m.result = menuResult{action: m.pendingAction, input: strings.TrimSpace(m.ti.Value())}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m tuiModel) View() string {
	if m.screen == screenInput {
		var b strings.Builder
		b.WriteString(bannerStyle.Render(trackrBanner) + "\n\n")
		b.WriteString(TitleStyle.Render("  "+m.prompt) + "\n\n")
		b.WriteString("  " + m.ti.View() + "\n\n")
		b.WriteString(HelpStyle.Render("  enter confirm · esc back · ctrl+c quit"))
		return b.String()
	}

	var b strings.Builder
	b.WriteString(bannerStyle.Render(trackrBanner) + "\n\n")
	b.WriteString(NoteStyle.Render("  caveman tool. me show you all thing. me find. me kill.") + "\n")
	b.WriteString(DividerStyle.Render("  "+strings.Repeat("─", 53)) + "\n\n")

	for i, it := range menuItems {
		if i == m.menuCursor {
			b.WriteString(menuSelectedStyle.Render("  ❯ "+it.label) + "\n")
		} else {
			b.WriteString("    " + it.label + "\n")
		}
	}

	b.WriteString("\n" + HelpStyle.Render("  ↑/↓ navigate · enter select · q quit"))
	return b.String()
}

// runMenuProgram shows the menu (and any input prompt) and returns the user's
// chosen action plus optional typed input.
func runMenuProgram() (menuResult, error) {
	fm, err := tea.NewProgram(newTUIModel(), tea.WithAltScreen()).Run()
	if err != nil {
		return menuResult{action: actQuit}, err
	}
	return fm.(tuiModel).result, nil
}

func pauseForMenu() {
	fmt.Print("\n  Press Enter to return to menu...")
	r := bufio.NewReader(os.Stdin)
	_, _ = r.ReadString('\n')
}

// RunTUI launches the dashboard loop. It keeps returning to the main menu after
// every action until the user chooses Quit (or presses q/esc on the menu, or
// ctrl+c anywhere).
func RunTUI(h Handlers) error {
	for {
		res, err := runMenuProgram()
		if err != nil {
			return err
		}
		switch res.action {
		case actScan:
			if h.Scan != nil {
				_ = h.Scan()
			}
		case actOrphans:
			if h.Orphans != nil {
				_ = h.Orphans()
			}
		case actWhere:
			if res.input != "" && h.Where != nil {
				_ = h.Where(res.input)
				pauseForMenu()
			}
		case actRemove:
			if res.input != "" && h.Remove != nil {
				_ = h.Remove(res.input)
				pauseForMenu()
			}
		case actLog:
			if h.Log != nil {
				_ = h.Log()
				pauseForMenu()
			}
		case actQuit, actNone:
			return nil
		}
	}
}
