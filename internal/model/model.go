package model

import (
	"context"
	"net"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/johnvanham/bw-monitor/internal/redis"
	"github.com/johnvanham/bw-monitor/internal/ui"
)

type viewState int

const (
	viewList viewState = iota
	viewDetail
)

const pollInterval = 2 * time.Second

// Model is the root bubbletea model.
type Model struct {
	// Data
	allReports     []redis.BlockReport
	filteredIdx    []int
	redisClient    *redis.Client
	totalReports   int

	// View state
	currentView  viewState
	cursor       int
	offset       int
	detailReport *redis.BlockReport
	detailOffset int // scroll position within detail view

	// Filter modal
	filterOpen   bool
	filterInputs []textinput.Model
	filterFocus  int
	filter       Filter

	// Stream control
	paused         bool
	pendingReports []redis.BlockReport
	following      bool // true = auto-scroll to show latest entries

	// Dimensions
	width, height int

	// Errors
	lastErr error

	// Loading
	loading bool

	// DNS lookup cache and state
	dnsCache   map[string][]string // ip -> hostnames
	dnsLookingUp string            // IP currently being looked up (empty = idle)
}

// New creates a new Model.
func New(redisClient *redis.Client, maxEntries int) Model {
	// Create filter inputs
	ipInput := textinput.New()
	ipInput.Placeholder = "e.g. 192.168.1"
	ipInput.SetWidth(30)

	countryInput := textinput.New()
	countryInput.Placeholder = "e.g. GB"
	countryInput.SetWidth(30)

	dateFromInput := textinput.New()
	dateFromInput.Placeholder = "YYYY-MM-DD HH:MM"
	dateFromInput.SetWidth(30)

	dateToInput := textinput.New()
	dateToInput.Placeholder = "YYYY-MM-DD HH:MM"
	dateToInput.SetWidth(30)

	return Model{
		redisClient:  redisClient,
		loading:      true,
		following:    true,
		filterInputs: []textinput.Model{ipInput, countryInput, dateFromInput, dateToInput},
		dnsCache:     make(map[string][]string),
	}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		reports, err := m.redisClient.LoadInitial(context.Background(), 1000)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return InitialLoadMsg{Reports: reports}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case InitialLoadMsg:
		m.loading = false
		m.allReports = msg.Reports
		m.totalReports = len(msg.Reports)
		m.refilter()
		m.scrollToBottom()
		return m, m.pollTick()

	case NewReportsMsg:
		if len(msg.Reports) > 0 {
			if m.paused {
				m.pendingReports = append(msg.Reports, m.pendingReports...)
			} else {
				m.allReports = append(msg.Reports, m.allReports...)
				m.totalReports = len(m.allReports)
				m.refilter()
				if m.following {
					m.scrollToBottom()
				}
			}
		}
		m.lastErr = nil
		return m, m.pollTick()

	case PollTickMsg:
		return m, m.doPoll()

	case ErrMsg:
		m.loading = false
		m.lastErr = msg.Err
		return m, m.pollTick()

	case DNSResultMsg:
		if msg.Err == nil && len(msg.Names) > 0 {
			m.dnsCache[msg.IP] = msg.Names
		} else {
			m.dnsCache[msg.IP] = []string{"(no rDNS)"}
		}
		m.dnsLookingUp = ""
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Pass through to filter inputs if modal is open
	if m.filterOpen {
		return m.updateFilterInputs(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// Escape key — check by code as well as string (bubbletea v2 may vary)
	isEscape := key == "escape" || key == "esc" || msg.Code == tea.KeyEscape

	// Filter modal captures all input when open
	if m.filterOpen {
		if isEscape {
			m.filterOpen = false
			m.filterInputs[m.filterFocus].Blur()
			return m, nil
		}
		return m.handleFilterKey(msg)
	}

	switch m.currentView {
	case viewList:
		return m.handleListKey(key)
	case viewDetail:
		if isEscape || key == "q" {
			m.currentView = viewList
			m.detailReport = nil
			m.detailOffset = 0
			return m, nil
		}
		switch key {
		case "up", "k":
			if m.detailOffset > 0 {
				m.detailOffset--
			}
		case "down", "j":
			m.detailOffset++
		case "pgup":
			m.detailOffset -= m.height - 3
			if m.detailOffset < 0 {
				m.detailOffset = 0
			}
		case "pgdown":
			m.detailOffset += m.height - 3
		case "home":
			m.detailOffset = 0
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case " ":
		m.paused = !m.paused
		if !m.paused && len(m.pendingReports) > 0 {
			m.allReports = append(m.pendingReports, m.allReports...)
			m.pendingReports = nil
			m.totalReports = len(m.allReports)
			m.refilter()
		}
	case "up", "k":
		m.following = false
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
		}
	case "down", "j":
		if m.cursor < len(m.filteredIdx)-1 {
			m.cursor++
			dataRows := m.dataRows()
			if m.cursor >= m.offset+dataRows {
				m.offset = m.cursor - dataRows + 1
			}
		}
		// Re-enable following if cursor is at the bottom
		if m.cursor >= len(m.filteredIdx)-1 {
			m.following = true
		}
	case "pgup":
		m.following = false
		dataRows := m.dataRows()
		m.cursor -= dataRows
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.offset = m.cursor
	case "pgdown":
		dataRows := m.dataRows()
		m.cursor += dataRows
		if m.cursor >= len(m.filteredIdx) {
			m.cursor = len(m.filteredIdx) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor >= m.offset+dataRows {
			m.offset = m.cursor - dataRows + 1
		}
		if m.cursor >= len(m.filteredIdx)-1 {
			m.following = true
		}
	case "home":
		m.following = false
		m.cursor = 0
		m.offset = 0
	case "end":
		m.following = true
		m.scrollToBottom()
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.filteredIdx) {
			idx := m.filteredIdx[m.cursor]
			m.detailReport = &m.allReports[idx]
			m.currentView = viewDetail
			m.detailOffset = 0

			// Trigger async DNS lookup if not cached
			ip := m.detailReport.IP
			if _, ok := m.dnsCache[ip]; !ok {
				m.dnsLookingUp = ip
				return m, func() tea.Msg {
					names, err := net.LookupAddr(ip)
					return DNSResultMsg{IP: ip, Names: names, Err: err}
				}
			}
		}
	case "f":
		m.openFilter()
	case "c":
		m.filter.Clear()
		m.refilter()
		m.cursor = 0
		m.offset = 0
	}
	return m, nil
}

func (m *Model) openFilter() {
	m.filterOpen = true
	m.filterFocus = 0
	m.filterInputs[0].SetValue(m.filter.IP)
	m.filterInputs[1].SetValue(m.filter.Country)
	if !m.filter.DateFrom.IsZero() {
		m.filterInputs[2].SetValue(m.filter.DateFrom.Format("2006-01-02 15:04"))
	} else {
		m.filterInputs[2].SetValue("")
	}
	if !m.filter.DateTo.IsZero() {
		m.filterInputs[3].SetValue(m.filter.DateTo.Format("2006-01-02 15:04"))
	} else {
		m.filterInputs[3].SetValue("")
	}
	m.filterInputs[m.filterFocus].Focus()
}

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "escape":
		m.filterOpen = false
		m.filterInputs[m.filterFocus].Blur()
		return m, nil
	case "tab", "down":
		m.filterInputs[m.filterFocus].Blur()
		m.filterFocus = (m.filterFocus + 1) % len(m.filterInputs)
		m.filterInputs[m.filterFocus].Focus()
		return m, nil
	case "shift+tab", "up":
		m.filterInputs[m.filterFocus].Blur()
		m.filterFocus = (m.filterFocus - 1 + len(m.filterInputs)) % len(m.filterInputs)
		m.filterInputs[m.filterFocus].Focus()
		return m, nil
	case "enter":
		m.applyFilter()
		return m, nil
	}

	// Pass to focused input
	return m.updateFilterInputs(msg)
}

func (m Model) updateFilterInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for i := range m.filterInputs {
		var cmd tea.Cmd
		m.filterInputs[i], cmd = m.filterInputs[i].Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) applyFilter() {
	m.filter.IP = strings.TrimSpace(m.filterInputs[0].Value())
	m.filter.Country = strings.TrimSpace(m.filterInputs[1].Value())

	if v := strings.TrimSpace(m.filterInputs[2].Value()); v != "" {
		if t, err := time.Parse("2006-01-02 15:04", v); err == nil {
			m.filter.DateFrom = t
		}
	} else {
		m.filter.DateFrom = time.Time{}
	}

	if v := strings.TrimSpace(m.filterInputs[3].Value()); v != "" {
		if t, err := time.Parse("2006-01-02 15:04", v); err == nil {
			m.filter.DateTo = t
		}
	} else {
		m.filter.DateTo = time.Time{}
	}

	m.filter.SetActive()
	m.refilter()
	m.cursor = 0
	m.offset = 0
	m.filterOpen = false
	m.filterInputs[m.filterFocus].Blur()
}

func (m *Model) dataRows() int {
	rows := m.height - 4 // minus title bar, column header, status bar, help bar
	if rows < 1 {
		return 1
	}
	return rows
}

func (m *Model) scrollToBottom() {
	m.cursor = len(m.filteredIdx) - 1
	if m.cursor < 0 {
		m.cursor = 0
	}
	dataRows := m.dataRows()
	if m.cursor >= dataRows {
		m.offset = m.cursor - dataRows + 1
	} else {
		m.offset = 0
	}
}

func (m *Model) refilter() {
	if !m.filter.IsActive() {
		m.filteredIdx = make([]int, len(m.allReports))
		for i := range m.allReports {
			m.filteredIdx[i] = i
		}
	} else {
		m.filteredIdx = m.filter.Apply(m.allReports)
	}
}

func (m Model) pollTick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		return PollTickMsg{}
	})
}

func (m Model) doPoll() tea.Cmd {
	return func() tea.Msg {
		reports, err := m.redisClient.PollNew(context.Background())
		if err != nil {
			return ErrMsg{Err: err}
		}
		return NewReportsMsg{Reports: reports}
	}
}

func (m Model) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("Initializing...")
		v.AltScreen = true
		return v
	}

	if m.loading {
		v := tea.NewView(ui.TitleStyle.Render("  Loading reports from Redis..."))
		v.AltScreen = true
		return v
	}

	var content string

	switch m.currentView {
	case viewList:
		content = RenderList(m.allReports, m.filteredIdx, m.cursor, m.offset, m.width, m.height, m.paused, &m.filter, m.totalReports, m.lastErr)
	case viewDetail:
		if m.detailReport != nil {
			dnsNames := m.dnsCache[m.detailReport.IP]
			dnsLoading := m.dnsLookingUp == m.detailReport.IP
			content = RenderDetail(m.detailReport, m.width, m.height, m.detailOffset, dnsNames, dnsLoading)
		}
	}

	// Overlay filter modal if open
	if m.filterOpen {
		modal := m.renderFilterModal()
		modalHeight := 14
		x := (m.width - 50) / 2
		y := (m.height - modalHeight) / 2
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}

		lines := strings.Split(content, "\n")
		modalLines := strings.Split(modal, "\n")
		for i, ml := range modalLines {
			row := y + i
			if row < len(lines) {
				if x+len(ml) < m.width {
					padding := strings.Repeat(" ", x)
					lines[row] = padding + ml
				}
			}
		}
		content = strings.Join(lines, "\n")
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) renderFilterModal() string {
	var b strings.Builder

	labels := []string{"IP:", "Country:", "From:", "To:"}

	b.WriteString(ui.TitleStyle.Render("  Filter Reports"))
	b.WriteString("\n\n")

	for i, label := range labels {
		focus := ""
		if i == m.filterFocus {
			focus = " > "
		} else {
			focus = "   "
		}
		b.WriteString(focus)
		b.WriteString(ui.LabelStyle.Render(ui.PadRight(label, 10)))
		b.WriteString(m.filterInputs[i].View())
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(ui.HelpStyle.Render("  [Tab] Next field  [Enter] Apply  [Esc] Cancel"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#569CD6")).
		Padding(1, 2).
		Width(46).
		Render(b.String())
}
