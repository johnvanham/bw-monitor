package model

import (
	"context"
	"net"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/johnvanham/bw-monitor/tui-go/internal/redis"
	"github.com/johnvanham/bw-monitor/tui-go/internal/ui"
)

type viewState int

const (
	viewReportsList viewState = iota
	viewReportDetail
	viewBansList
	viewBanDetail
)

const pollInterval = 2 * time.Second

// Model is the root bubbletea model.
type Model struct {
	// Data
	allReports   []redis.BlockReport
	filteredIdx  []int
	redisClient  *redis.Client
	reconnector  *redis.Reconnector
	totalReports int

	// View state
	currentView  viewState
	detailReport *redis.BlockReport

	// List viewports (content rendered manually, viewport just displays)
	reportsViewport viewport.Model
	reportsCursor   int

	bansViewport viewport.Model
	bansCursor   int

	// Detail viewports (viewport handles scrolling natively)
	detailViewport viewport.Model

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
	loading    bool
	maxEntries int

	// DNS lookup cache and state
	dnsCache     map[string][]string // ip -> hostnames
	dnsLookingUp string              // IP currently being looked up (empty = idle)

	// Bans
	bans           []redis.Ban
	filteredBanIdx []int
	detailBan      *redis.Ban

	// IP exclusions
	excludes           *ExcludeList
	excludeModalOpen   bool
	excludeModalCursor int
}

// detailKeyMap returns a KeyMap for detail viewports that only binds
// arrow keys, j/k, pgup/pgdown — avoids conflicts with app keys like f, space, b.
func detailKeyMap() viewport.KeyMap {
	return viewport.KeyMap{
		Up: key.NewBinding(key.WithKeys("up", "k")),
		Down: key.NewBinding(key.WithKeys("down", "j")),
		PageUp: key.NewBinding(key.WithKeys("pgup")),
		PageDown: key.NewBinding(key.WithKeys("pgdown")),
		HalfPageUp: key.NewBinding(key.WithKeys("ctrl+u")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
	}
}

// emptyKeyMap returns a KeyMap with no bindings, used for list viewports
// where we handle navigation manually.
func emptyKeyMap() viewport.KeyMap {
	return viewport.KeyMap{}
}

// New creates a new Model.
func New(redisClient *redis.Client, reconnector *redis.Reconnector, maxEntries int) Model {
	// Create filter inputs
	ipInput := textinput.New()
	ipInput.Placeholder = "e.g. 192.168.1"
	ipInput.SetWidth(50)

	countryInput := textinput.New()
	countryInput.Placeholder = "e.g. GB or GB,IN,US"
	countryInput.SetWidth(50)

	serverInput := textinput.New()
	serverInput.Placeholder = "e.g. www.ter-europe.org"
	serverInput.SetWidth(50)

	dateFromInput := textinput.New()
	dateFromInput.Placeholder = "YYYY-MM-DD HH:MM"
	dateFromInput.SetWidth(50)

	dateToInput := textinput.New()
	dateToInput.Placeholder = "YYYY-MM-DD HH:MM"
	dateToInput.SetWidth(50)

	// Create list viewports with empty keymaps (we handle keys manually)
	reportsVP := viewport.New()
	reportsVP.KeyMap = emptyKeyMap()

	bansVP := viewport.New()
	bansVP.KeyMap = emptyKeyMap()

	// Create detail viewport with custom keymap
	detailVP := viewport.New()
	detailVP.KeyMap = detailKeyMap()

	var filter Filter
	filter.Load()

	return Model{
		redisClient:     redisClient,
		reconnector:     reconnector,
		loading:         true,
		following:       true,
		maxEntries:      maxEntries,
		filter:          filter,
		filterInputs:    []textinput.Model{ipInput, countryInput, serverInput, dateFromInput, dateToInput},
		dnsCache:        make(map[string][]string),
		excludes:        NewExcludeList(),
		reportsViewport: reportsVP,
		bansViewport:    bansVP,
		detailViewport:  detailVP,
	}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		reports, err := m.redisClient.LoadInitial(context.Background(), m.maxEntries)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return InitialLoadMsg{Reports: reports}
	}
}

// dataRows returns the number of rows available for list data display.
func (m *Model) dataRows() int {
	rows := m.height - 4 // minus title bar, column header, status bar, help bar
	if rows < 1 {
		return 1
	}
	return rows
}

func (m *Model) updateViewportSizes() {
	dataRows := m.dataRows()

	m.reportsViewport.SetWidth(m.width)
	m.reportsViewport.SetHeight(dataRows)

	m.bansViewport.SetWidth(m.width)
	m.bansViewport.SetHeight(dataRows)

	// Detail viewport: height minus title bar (1) and help bar (1)
	detailHeight := m.height - 2
	if detailHeight < 1 {
		detailHeight = 1
	}
	m.detailViewport.SetWidth(m.width)
	m.detailViewport.SetHeight(detailHeight)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewportSizes()
		m.rebuildReportsContent()
		m.rebuildBansContent()
		return m, nil

	case InitialLoadMsg:
		m.loading = false
		m.allReports = msg.Reports
		m.totalReports = len(msg.Reports)
		m.refilter()
		m.scrollToNewest()
		m.rebuildReportsContent()
		return m, m.pollTick()

	case NewReportsMsg:
		if len(msg.Reports) > 0 {
			if m.paused {
				m.pendingReports = append(msg.Reports, m.pendingReports...)
			} else {
				oldFilteredLen := len(m.filteredIdx)
				m.allReports = append(msg.Reports, m.allReports...)
				m.totalReports = len(m.allReports)
				m.refilter()
				newVisibleCount := len(m.filteredIdx) - oldFilteredLen
				if m.following {
					m.scrollToNewest()
				} else if newVisibleCount > 0 {
					// Shift cursor down by the number of new *visible* entries
					wasAtEnd := m.reportsCursor >= oldFilteredLen-1

					m.reportsCursor += newVisibleCount

					if wasAtEnd {
						m.reportsCursor = len(m.filteredIdx) - 1
						if m.reportsCursor < 0 {
							m.reportsCursor = 0
						}
					}
				}
				m.rebuildReportsContent()
				m.syncReportsViewportOffset()
			}
		}
		m.lastErr = nil
		return m, m.pollTick()

	case PollTickMsg:
		return m, m.doPoll()

	case BansLoadedMsg:
		m.bans = msg.Bans
		m.refilterBans()
		m.rebuildBansContent()
		return m, nil

	case BansPollTickMsg:
		return m, m.doLoadBans()

	case ErrMsg:
		m.loading = false
		m.lastErr = msg.Err
		// Attempt reconnection on error
		if m.reconnector != nil {
			return m, func() tea.Msg {
				if err := m.reconnector.Reconnect(); err != nil {
					return ReconnectFailedMsg{Err: err}
				}
				return ReconnectedMsg{}
			}
		}
		return m, m.pollTick()

	case ReconnectedMsg:
		m.redisClient = m.reconnector.Client()
		m.lastErr = nil
		return m, m.pollTick()

	case ReconnectFailedMsg:
		m.lastErr = msg.Err
		return m, m.pollTick()

	case DNSResultMsg:
		if msg.Err == nil && len(msg.Names) > 0 {
			m.dnsCache[msg.IP] = msg.Names
		} else {
			m.dnsCache[msg.IP] = []string{"(no rDNS)"}
		}
		m.dnsLookingUp = ""
		// Rebuild detail content if we're in a detail view
		m.rebuildDetailContent()
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

	// Exclude modal captures input when open
	if m.excludeModalOpen {
		switch key {
		case "escape", "esc":
			m.excludeModalOpen = false
		case "up", "k":
			if m.excludeModalCursor > 0 {
				m.excludeModalCursor--
			}
		case "down", "j":
			ips := m.excludes.List()
			if m.excludeModalCursor < len(ips)-1 {
				m.excludeModalCursor++
			}
		case "delete", "backspace":
			ips := m.excludes.List()
			if len(ips) > 0 && m.excludeModalCursor < len(ips) {
				m.excludes.Remove(ips[m.excludeModalCursor])
				if m.excludeModalCursor >= len(m.excludes.List()) {
					m.excludeModalCursor = len(m.excludes.List()) - 1
				}
				if m.excludeModalCursor < 0 {
					m.excludeModalCursor = 0
				}
				m.refilter()
				m.rebuildReportsContent()
			}
			if m.excludes.Count() == 0 {
				m.excludeModalOpen = false
			}
		}
		if isEscape {
			m.excludeModalOpen = false
		}
		return m, nil
	}

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
	case viewReportsList:
		return m.handleListKey(key)
	case viewReportDetail:
		if isEscape || key == "q" {
			m.currentView = viewReportsList
			m.detailReport = nil
			return m, nil
		}
		// Pass to detail viewport for native scrolling
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return m, cmd
	case viewBansList:
		return m.handleBansListKey(key)
	case viewBanDetail:
		if isEscape || key == "q" {
			m.currentView = viewBansList
			m.detailBan = nil
			return m, nil
		}
		// Pass to detail viewport for native scrolling
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case " ", "space":
		m.paused = !m.paused
		if !m.paused && len(m.pendingReports) > 0 {
			m.allReports = append(m.pendingReports, m.allReports...)
			m.pendingReports = nil
			m.totalReports = len(m.allReports)
			m.refilter()
			m.rebuildReportsContent()
		}
	case "up", "k":
		if m.reportsCursor > 0 {
			m.reportsCursor--
			m.following = false
			m.rebuildReportsContent()
			m.syncReportsViewportOffset()
		}
	case "down", "j":
		if m.reportsCursor < len(m.filteredIdx)-1 {
			m.reportsCursor++
			m.following = false
			m.rebuildReportsContent()
			m.syncReportsViewportOffset()
		}
	case "pgup":
		dataRows := m.dataRows()
		m.reportsCursor -= dataRows
		if m.reportsCursor < 0 {
			m.reportsCursor = 0
		}
		m.following = false
		m.rebuildReportsContent()
		m.syncReportsViewportOffset()
	case "pgdown":
		dataRows := m.dataRows()
		m.reportsCursor += dataRows
		if m.reportsCursor >= len(m.filteredIdx) {
			m.reportsCursor = len(m.filteredIdx) - 1
		}
		if m.reportsCursor < 0 {
			m.reportsCursor = 0
		}
		m.following = false
		m.rebuildReportsContent()
		m.syncReportsViewportOffset()
	case "home":
		m.following = true
		m.scrollToNewest()
		m.rebuildReportsContent()
		m.reportsViewport.GotoTop()
	case "end":
		m.reportsCursor = len(m.filteredIdx) - 1
		if m.reportsCursor < 0 {
			m.reportsCursor = 0
		}
		m.following = false
		m.rebuildReportsContent()
		m.syncReportsViewportOffset()
	case "enter":
		if m.reportsCursor >= 0 && m.reportsCursor < len(m.filteredIdx) {
			idx := m.filteredIdx[m.reportsCursor]
			m.detailReport = &m.allReports[idx]
			m.currentView = viewReportDetail
			m.rebuildDetailContent()
			m.detailViewport.GotoTop()

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
		m.filter.Delete()
		m.refilter()
		m.reportsCursor = 0
		m.rebuildReportsContent()
		m.reportsViewport.GotoTop()
	case "x":
		// Exclude the IP of the currently selected report
		if m.reportsCursor >= 0 && m.reportsCursor < len(m.filteredIdx) {
			idx := m.filteredIdx[m.reportsCursor]
			ip := m.allReports[idx].IP
			m.excludes.Add(ip)
			m.refilter()
			if m.reportsCursor >= len(m.filteredIdx) {
				m.reportsCursor = len(m.filteredIdx) - 1
			}
			if m.reportsCursor < 0 {
				m.reportsCursor = 0
			}
			m.rebuildReportsContent()
			m.syncReportsViewportOffset()
		}
	case "X":
		// Open exclude list modal
		m.excludeModalOpen = true
		m.excludeModalCursor = 0
	case "2":
		m.currentView = viewBansList
		m.bansCursor = 0
		m.rebuildBansContent()
		m.bansViewport.GotoTop()
		return m, m.doLoadBans()
	}
	return m, nil
}

func (m Model) handleBansListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.bansCursor > 0 {
			m.bansCursor--
			m.rebuildBansContent()
			m.syncBansViewportOffset()
		}
	case "down", "j":
		if m.bansCursor < len(m.filteredBanIdx)-1 {
			m.bansCursor++
			m.rebuildBansContent()
			m.syncBansViewportOffset()
		}
	case "pgup":
		dataRows := m.dataRows()
		m.bansCursor -= dataRows
		if m.bansCursor < 0 {
			m.bansCursor = 0
		}
		m.rebuildBansContent()
		m.syncBansViewportOffset()
	case "pgdown":
		dataRows := m.dataRows()
		m.bansCursor += dataRows
		if m.bansCursor >= len(m.filteredBanIdx) {
			m.bansCursor = len(m.filteredBanIdx) - 1
		}
		if m.bansCursor < 0 {
			m.bansCursor = 0
		}
		m.rebuildBansContent()
		m.syncBansViewportOffset()
	case "f":
		m.openFilter()
	case "c":
		m.filter.Clear()
		m.filter.Delete()
		m.refilterBans()
		m.bansCursor = 0
		m.rebuildBansContent()
		m.bansViewport.GotoTop()
	case "x":
		if m.bansCursor >= 0 && m.bansCursor < len(m.filteredBanIdx) {
			idx := m.filteredBanIdx[m.bansCursor]
			m.excludes.Add(m.bans[idx].IP)
			m.refilterBans()
			if m.bansCursor >= len(m.filteredBanIdx) {
				m.bansCursor = len(m.filteredBanIdx) - 1
			}
			if m.bansCursor < 0 {
				m.bansCursor = 0
			}
			m.rebuildBansContent()
		}
	case "X":
		m.excludeModalOpen = true
		m.excludeModalCursor = 0
	case "enter":
		if m.bansCursor >= 0 && m.bansCursor < len(m.filteredBanIdx) {
			idx := m.filteredBanIdx[m.bansCursor]
			m.detailBan = &m.bans[idx]
			m.currentView = viewBanDetail
			m.rebuildDetailContent()
			m.detailViewport.GotoTop()
			ip := m.detailBan.IP
			if _, ok := m.dnsCache[ip]; !ok {
				m.dnsLookingUp = ip
				return m, func() tea.Msg {
					names, err := net.LookupAddr(ip)
					return DNSResultMsg{IP: ip, Names: names, Err: err}
				}
			}
		}
	case "r":
		return m, m.doLoadBans()
	case "1":
		m.currentView = viewReportsList
		m.rebuildReportsContent()
		m.syncReportsViewportOffset()
	}
	return m, nil
}

// syncReportsViewportOffset ensures the viewport offset keeps the cursor visible.
func (m *Model) syncReportsViewportOffset() {
	dataRows := m.dataRows()
	offset := m.reportsViewport.YOffset()

	if m.reportsCursor < offset {
		offset = m.reportsCursor
	} else if m.reportsCursor >= offset+dataRows {
		offset = m.reportsCursor - dataRows + 1
	}

	m.reportsViewport.SetYOffset(offset)
}

// syncBansViewportOffset ensures the bans viewport offset keeps the cursor visible.
func (m *Model) syncBansViewportOffset() {
	dataRows := m.dataRows()
	offset := m.bansViewport.YOffset()

	if m.bansCursor < offset {
		offset = m.bansCursor
	} else if m.bansCursor >= offset+dataRows {
		offset = m.bansCursor - dataRows + 1
	}

	m.bansViewport.SetYOffset(offset)
}

func (m Model) doLoadBans() tea.Cmd {
	return func() tea.Msg {
		bans, err := m.redisClient.LoadBans(context.Background())
		if err != nil {
			return ErrMsg{Err: err}
		}
		return BansLoadedMsg{Bans: bans}
	}
}

func (m *Model) openFilter() {
	m.filterOpen = true
	m.filterFocus = 0
	m.filterInputs[0].SetValue(m.filter.IP)
	m.filterInputs[1].SetValue(m.filter.Country)
	m.filterInputs[2].SetValue(m.filter.Server)
	if !m.filter.DateFrom.IsZero() {
		m.filterInputs[3].SetValue(m.filter.DateFrom.Format("2006-01-02 15:04"))
	} else {
		m.filterInputs[3].SetValue("")
	}
	if !m.filter.DateTo.IsZero() {
		m.filterInputs[4].SetValue(m.filter.DateTo.Format("2006-01-02 15:04"))
	} else {
		m.filterInputs[4].SetValue("")
	}
	m.filterInputs[m.filterFocus].Focus()
}

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "escape", "esc":
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
		m.rebuildReportsContent()
		m.reportsViewport.GotoTop()
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
	m.filter.Server = strings.TrimSpace(m.filterInputs[2].Value())

	if v := strings.TrimSpace(m.filterInputs[3].Value()); v != "" {
		if t, err := time.Parse("2006-01-02 15:04", v); err == nil {
			m.filter.DateFrom = t
		}
	} else {
		m.filter.DateFrom = time.Time{}
	}

	if v := strings.TrimSpace(m.filterInputs[4].Value()); v != "" {
		if t, err := time.Parse("2006-01-02 15:04", v); err == nil {
			m.filter.DateTo = t
		}
	} else {
		m.filter.DateTo = time.Time{}
	}

	m.filter.SetActive()
	m.filter.Save()
	m.refilter()
	m.reportsCursor = 0
	m.bansCursor = 0
	m.rebuildBansContent()
	m.filterOpen = false
	m.filterInputs[m.filterFocus].Blur()
}

// scrollToNewest moves the cursor to the top of the list (newest entries)
// and resets the viewport to show the top.
func (m *Model) scrollToNewest() {
	m.reportsCursor = 0
	m.reportsViewport.GotoTop()
}

func (m *Model) refilter() {
	if !m.filter.IsActive() && m.excludes.Count() == 0 {
		m.filteredIdx = make([]int, len(m.allReports))
		for i := range m.allReports {
			m.filteredIdx[i] = i
		}
	} else {
		m.filteredIdx = nil
		for i := range m.allReports {
			if m.excludes.Contains(m.allReports[i].IP) {
				continue
			}
			if m.filter.IsActive() && !m.filter.Matches(&m.allReports[i]) {
				continue
			}
			m.filteredIdx = append(m.filteredIdx, i)
		}
	}
	m.refilterBans()
}

func (m *Model) refilterBans() {
	if !m.filter.IsActive() && m.excludes.Count() == 0 {
		m.filteredBanIdx = make([]int, len(m.bans))
		for i := range m.bans {
			m.filteredBanIdx[i] = i
		}
	} else {
		m.filteredBanIdx = nil
		for i := range m.bans {
			if m.excludes.Contains(m.bans[i].IP) {
				continue
			}
			if m.filter.IsActive() && !m.filter.MatchesBan(&m.bans[i]) {
				continue
			}
			m.filteredBanIdx = append(m.filteredBanIdx, i)
		}
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

// rebuildDetailContent rebuilds the detail viewport content based on current view.
func (m *Model) rebuildDetailContent() {
	switch m.currentView {
	case viewReportDetail:
		if m.detailReport != nil {
			dnsNames := m.dnsCache[m.detailReport.IP]
			dnsLoading := m.dnsLookingUp == m.detailReport.IP
			content := BuildDetailContent(m.detailReport, m.width, dnsNames, dnsLoading)
			m.detailViewport.SetContent(content)
		}
	case viewBanDetail:
		if m.detailBan != nil {
			dnsNames := m.dnsCache[m.detailBan.IP]
			dnsLoading := m.dnsLookingUp == m.detailBan.IP
			content := BuildBanDetailContent(m.detailBan, m.width, dnsNames, dnsLoading)
			m.detailViewport.SetContent(content)
		}
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
	case viewReportsList:
		content = m.renderReportsListView()
	case viewReportDetail:
		content = m.renderDetailView("Block Detail")
	case viewBansList:
		content = m.renderBansListView()
	case viewBanDetail:
		content = m.renderDetailView("Ban Detail")
	}

	// Overlay modals using lipgloss.Place
	if m.filterOpen {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderFilterModal())
	}
	if m.excludeModalOpen {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderExcludeModal())
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderReportsListView renders the full reports list view with title, header, viewport, status, help.
func (m Model) renderReportsListView() string {
	var b strings.Builder

	b.WriteString(RenderTitleBar("BW Monitor", "Live View", m.width))
	b.WriteString("\n")

	// Header row
	header := ui.HeaderStyle.Render(ui.PadRight(ui.FormatHeaderRow(m.width), m.width))
	b.WriteString(header)
	b.WriteString("\n")

	// Viewport content
	b.WriteString(m.reportsViewport.View())
	b.WriteString("\n")

	// Status bar
	b.WriteString(RenderReportsStatusBar(m.filteredIdx, m.totalReports, m.paused, &m.filter, m.excludes.Count(), m.lastErr, m.width))
	b.WriteString("\n")

	// Help bar
	help := "[1] Reports  [2] Bans  [Space] Pause  [Enter] Detail  [f] Filter  [c] Clear  [x] Exclude IP  [X] Excludes  [q] Quit"
	b.WriteString(ui.HelpStyle.Render(help))

	return b.String()
}

// renderBansListView renders the full bans list view.
func (m Model) renderBansListView() string {
	var b strings.Builder

	b.WriteString(RenderTitleBar("BW Monitor", "Active Bans", m.width))
	b.WriteString("\n")

	// Header
	header := RenderBansHeader(m.width)
	b.WriteString(ui.HeaderStyle.Render(ui.PadRight(header, m.width)))
	b.WriteString("\n")

	// Viewport content
	b.WriteString(m.bansViewport.View())
	b.WriteString("\n")

	// Status bar
	b.WriteString(RenderBansStatusBar(len(m.filteredBanIdx), len(m.bans), m.excludes.Count(), &m.filter, m.lastErr, m.width))
	b.WriteString("\n")

	// Help bar
	help := "[1] Reports  [2] Bans  [Enter] Detail  [f] Filter  [c] Clear  [x] Exclude IP  [X] Excludes  [r] Refresh  [q] Quit"
	b.WriteString(ui.HelpStyle.Render(help))

	return b.String()
}

// renderDetailView renders a detail view with title bar, viewport, and help bar.
func (m Model) renderDetailView(contextLabel string) string {
	var b strings.Builder

	b.WriteString(RenderTitleBar("BW Monitor", contextLabel, m.width))
	b.WriteString("\n")

	b.WriteString(m.detailViewport.View())
	b.WriteString("\n")

	help := ui.HelpStyle.Render("[Esc] Back  [Up/Down] Scroll  [PgUp/PgDn] Page")
	b.WriteString(help)

	return b.String()
}

func (m Model) renderFilterModal() string {
	var b strings.Builder

	labels := []string{"IP:", "Country:", "Server:", "From:", "To:"}

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
		Width(66).
		Render(b.String())
}

func (m Model) renderExcludeModal() string {
	var b strings.Builder

	b.WriteString(ui.TitleStyle.Render("  Excluded IPs"))
	b.WriteString("\n\n")

	ips := m.excludes.List()

	if len(ips) == 0 {
		b.WriteString(ui.DimStyle.Render("  No excluded IPs"))
		b.WriteString("\n")
	} else {
		for i, ip := range ips {
			prefix := "  "
			if i == m.excludeModalCursor {
				prefix = "> "
			}
			ipColour := ui.ColourForIP(ip)
			style := lipgloss.NewStyle().Foreground(ipColour)
			if i == m.excludeModalCursor {
				style = style.Bold(true).Background(lipgloss.Color("#333333"))
			}
			b.WriteString(prefix)
			b.WriteString(style.Render(ui.PadRight(ip, 40)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(ui.HelpStyle.Render("  [Del] Remove  [Esc] Close"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#569CD6")).
		Padding(1, 2).
		Width(66).
		Render(b.String())
}
