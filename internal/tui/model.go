package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/fsnotify/fsnotify"

	"pinkssh/internal/pinkssh"
)

type model struct {
	opts          pinkssh.Options
	hosts         []pinkssh.Host
	watchPaths    []string
	selected      int
	offset        int
	width         int
	height        int
	loading       bool
	checking      bool
	busy          bool
	status        string
	spinner       spinner.Model
	keyPicker     *keyPicker
	search        *searchState
	form          *hostForm
	confirmDelete string
	lastLoadError error
}

type keyPicker struct {
	keys  []string
	index int
}

type layoutMetrics struct {
	bodyWidth       int
	panelStyleWidth int
	listRows        int
	modalBodyWidth  int
	modalStyleWidth int
}

type loadMsg struct {
	hosts      []pinkssh.Host
	watchPaths []string
	err        error
}

type statusesMsg struct {
	hosts []pinkssh.Host
}

type watchChangedMsg struct {
	err error
}

type connectDoneMsg struct {
	err error
}

type copyDoneMsg struct {
	alias string
	log   string
	err   error
}

type copyStartErrorMsg struct {
	err error
}

type configSavedMsg struct {
	action string
	alias  string
	err    error
}

type configDeletedMsg struct {
	alias string
	err   error
}

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e8e8e8")).
			Bold(true)
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#5f6368")).
			Padding(1, 2)
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#5f6368")).
			Padding(1, 2)
	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8a8a8a"))
	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#c9c9c9")).
				Bold(true)
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff4fd8")).
			Bold(true)
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff6b6b"))
)

func Run(opts pinkssh.Options) error {
	p := tea.NewProgram(newModel(opts), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newModel(opts pinkssh.Options) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#c9c9c9"))
	return model{
		opts:    opts,
		loading: true,
		status:  "loading",
		spinner: s,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, loadHostsCmd(m.opts))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.keepSelectionVisible()
		return m, nil

	case spinner.TickMsg:
		if m.loading || m.checking || m.busy {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case loadMsg:
		m.loading = false
		m.lastLoadError = msg.err
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.hosts = msg.hosts
		m.watchPaths = msg.watchPaths
		m.clampSelection()
		m.refreshSearch()
		m.keepSelectionVisible()
		m.status = hostCountStatus(len(m.hosts))
		m.checking = len(m.hosts) > 0

		var cmds []tea.Cmd
		if m.checking {
			cmds = append(cmds, statusesCmd(m.hosts, m.opts))
		}
		if m.opts.Watch {
			cmds = append(cmds, watchCmd(m.watchPaths))
		}
		if len(cmds) == 0 {
			return m, nil
		}
		return m, tea.Batch(cmds...)

	case statusesMsg:
		m.checking = false
		m.hosts = msg.hosts
		m.clampSelection()
		m.refreshSearch()
		m.keepSelectionVisible()
		m.status = hostCountStatus(len(m.hosts))
		return m, nil

	case watchChangedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.status = "reloading"
		}
		m.loading = true
		return m, loadHostsCmd(m.opts)

	case connectDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "ssh exited: " + msg.err.Error()
		} else {
			m.status = "returned from ssh"
		}
		return m, nil

	case copyDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "copy failed: " + msg.err.Error()
			if msg.log != "" {
				m.status = "copy failed; log " + msg.log
			}
			return m, nil
		}
		m.status = "copy complete: " + msg.alias
		m.checking = true
		return m, statusesCmd(m.hosts, m.opts)

	case copyStartErrorMsg:
		m.busy = false
		m.status = msg.err.Error()
		return m, nil

	case configSavedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.loading = true
		m.status = msg.action + ": " + msg.alias
		return m, loadHostsCmd(m.opts)

	case configDeletedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.loading = true
		m.status = "deleted: " + msg.alias
		return m, loadHostsCmd(m.opts)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.form != nil {
		return m.handleFormKey(msg)
	}
	if m.confirmDelete != "" {
		return m.handleDeleteConfirmKey(msg)
	}
	if m.keyPicker != nil {
		return m.handlePickerKey(msg)
	}
	if m.search != nil {
		return m.handleSearchKey(msg)
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "/":
		if m.busy {
			return m, nil
		}
		m.openSearch()
	case "up", "k":
		if m.selected > 0 {
			m.selected--
			m.keepSelectionVisible()
		}
	case "down", "j":
		if m.selected < len(m.hosts)-1 {
			m.selected++
			m.keepSelectionVisible()
		}
	case "r":
		m.loading = true
		m.status = "reloading"
		return m, loadHostsCmd(m.opts)
	case "a":
		if m.busy {
			return m, nil
		}
		m.form = newAddForm()
		m.status = "add host"
	case "e":
		if len(m.hosts) == 0 || m.busy {
			return m, nil
		}
		host := m.hosts[m.selected]
		spec, err := pinkssh.ReadHostConfig(m.opts.ConfigPath, host.Alias)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		spec.Alias = host.Alias
		m.form = newEditForm(host.Alias, spec)
		m.status = "edit host"
	case "d":
		if len(m.hosts) == 0 || m.busy {
			return m, nil
		}
		m.confirmDelete = m.hosts[m.selected].Alias
		m.status = "confirm delete"
	case "enter":
		if len(m.hosts) == 0 || m.busy {
			return m, nil
		}
		return startConnect(m, m.hosts[m.selected])
	case "c":
		if len(m.hosts) == 0 || m.busy {
			return m, nil
		}
		host := m.hosts[m.selected]
		keys := pinkssh.PublicKeyCandidates(host)
		if len(keys) == 0 {
			m.status = pinkssh.ErrNoPublicKeys.Error()
			return m, nil
		}
		if len(keys) == 1 {
			return startCopy(m, host, keys[0])
		}
		m.keyPicker = &keyPicker{keys: keys}
		m.status = "select public key"
	}
	return m, nil
}

func (m model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	search := m.search
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.closeSearch(true)
		return m, nil
	case "up", "k":
		if search.selected > 0 {
			search.selected--
			m.keepSearchSelectionVisible()
		}
		return m, nil
	case "down", "j":
		if search.selected < len(search.matches)-1 {
			search.selected++
			m.keepSearchSelectionVisible()
		}
		return m, nil
	case "enter":
		index, ok := search.currentHostIndex()
		if !ok || index < 0 || index >= len(m.hosts) || m.busy {
			return m, nil
		}
		host := m.hosts[index]
		m.selected = index
		m.search = nil
		m.keepSelectionVisible()
		return startConnect(m, host)
	}

	previousIndex, _ := search.currentHostIndex()
	var cmd tea.Cmd
	search.input, cmd = search.input.Update(msg)
	search.updateMatches(m.hosts, previousIndex)
	m.keepSearchSelectionVisible()
	return m, cmd
}

func (m model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.form = nil
		m.status = hostCountStatus(len(m.hosts))
		return m, nil
	case "tab", "down":
		if msg.String() == "down" && m.form.hasIdentityDropdown() {
			return m, m.form.update(msg)
		}
		m.form.nextField()
		return m, nil
	case "shift+tab", "up":
		if msg.String() == "up" && m.form.hasIdentityDropdown() {
			return m, m.form.update(msg)
		}
		m.form.previousField()
		return m, nil
	case "enter", "ctrl+s":
		form := m.form
		spec := form.spec()
		m.form = nil
		m.busy = true
		m.status = "saving"
		return m, saveHostCmd(m.opts, form.mode, form.original, spec)
	default:
		return m, m.form.update(msg)
	}
}

func (m model) handleDeleteConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n", "N":
		m.confirmDelete = ""
		m.status = hostCountStatus(len(m.hosts))
	case "y", "Y":
		alias := m.confirmDelete
		m.confirmDelete = ""
		m.busy = true
		m.status = "deleting: " + alias
		return m, deleteHostCmd(m.opts, alias)
	}
	return m, nil
}

func (m model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	picker := m.keyPicker
	switch msg.String() {
	case "esc", "q":
		m.keyPicker = nil
		m.status = hostCountStatus(len(m.hosts))
	case "up", "k":
		if picker.index > 0 {
			picker.index--
		}
	case "down", "j":
		if picker.index < len(picker.keys)-1 {
			picker.index++
		}
	case "enter":
		if len(m.hosts) == 0 || len(picker.keys) == 0 {
			return m, nil
		}
		key := picker.keys[picker.index]
		host := m.hosts[m.selected]
		m.keyPicker = nil
		return startCopy(m, host, key)
	}
	return m, nil
}

func startCopy(m model, host pinkssh.Host, pubkey string) (tea.Model, tea.Cmd) {
	cmd, err := pinkssh.NewCopyPublicKeyCommand(host, m.opts, pubkey)
	if err != nil {
		pinkssh.LogCopyDone(m.opts, host, err)
		return m, func() tea.Msg { return copyStartErrorMsg{err: err} }
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	pinkssh.LogCopyStart(m.opts, host, pubkey)
	m.busy = true
	m.status = "copying key: " + host.Alias
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		pinkssh.LogCopyDone(m.opts, host, err)
		return copyDoneMsg{alias: host.Alias, log: m.opts.LogPath, err: err}
	})
}

func startConnect(m model, host pinkssh.Host) (tea.Model, tea.Cmd) {
	cmd := pinkssh.NewConnectCommand(host.Alias, m.opts)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	m.busy = true
	m.status = "connecting: " + host.Alias
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return connectDoneMsg{err: err}
	})
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	layout := m.layout()
	content := m.renderPanel(layout)
	panel := panelStyle.Width(layout.panelStyleWidth).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func (m model) layout() layoutMetrics {
	maxPanelWidth := m.maxPanelWidth()
	maxBodyWidth := max(1, maxPanelWidth-panelStyle.GetHorizontalFrameSize())
	bodyWidth := clamp(m.preferredBodyWidth(maxBodyWidth), minBodyWidth(maxBodyWidth), maxBodyWidth)
	panelStyleWidth := bodyWidth + panelStyle.GetHorizontalPadding()

	modalOuterWidth := min(bodyWidth, max(34, bodyWidth-8))
	if bodyWidth < 44 {
		modalOuterWidth = bodyWidth
	}
	modalBodyWidth := max(1, modalOuterWidth-modalStyle.GetHorizontalFrameSize())
	modalStyleWidth := modalBodyWidth + modalStyle.GetHorizontalPadding()

	listRows := m.height - 11
	if m.keyPicker != nil {
		listRows -= min(8, len(m.keyPicker.keys)+3)
	}
	if m.search != nil {
		listRows -= 4
	}
	if m.form != nil {
		listRows -= len(fieldLabels) + m.form.extraRows(modalBodyWidth) + 4
	}
	if m.confirmDelete != "" {
		listRows -= 4
	}
	listRows = clamp(listRows, 1, 18)

	return layoutMetrics{
		bodyWidth:       bodyWidth,
		panelStyleWidth: panelStyleWidth,
		listRows:        listRows,
		modalBodyWidth:  modalBodyWidth,
		modalStyleWidth: modalStyleWidth,
	}
}

func (m model) maxPanelWidth() int {
	panelWidth := m.width
	switch {
	case m.width >= 118:
		panelWidth = 104
	case m.width >= 82:
		panelWidth = m.width - 10
	case m.width >= 48:
		panelWidth = m.width - 4
	case m.width > 2:
		panelWidth = m.width - 2
	}
	panelWidth = min(panelWidth, m.width)
	if m.width >= 20 {
		panelWidth = max(20, panelWidth)
	}
	return panelWidth
}

func (m model) preferredBodyWidth(maxBodyWidth int) int {
	status := m.status
	if m.loading || m.checking || m.busy {
		status = m.spinner.View() + " " + status
	}

	width := textWidth("pinkssh")
	if status != "" {
		width = max(width, textWidth("pinkssh  "+status))
	}

	if len(m.hosts) == 0 && m.form == nil {
		width = max(width, textWidth("No hosts found"))
	} else if len(m.hosts) > 0 {
		tableHosts := m.tableHosts()
		width = max(width, naturalTableColumns(tableHosts, m.checking).totalWidth())
		if m.search != nil && len(tableHosts) == 0 {
			width = max(width, textWidth("No matches"))
		}
	}

	if m.search != nil {
		width = max(width, min(58, maxBodyWidth))
	}

	if m.keyPicker != nil {
		width = max(width, textWidth("Select public key"))
		for _, key := range m.keyPicker.keys {
			width = max(width, min(textWidth(key)+1, maxBodyWidth))
		}
	}

	if m.form != nil {
		width = max(width, min(64, maxBodyWidth))
		for i, field := range m.form.fields {
			fieldWidth := textWidth(fieldLabels[i]) + 2 + min(textWidth(field.Value()), 48)
			width = max(width, min(fieldWidth, maxBodyWidth))
		}
	}

	if m.confirmDelete != "" {
		width = max(width, min(textWidth("Delete "+m.confirmDelete+"?"), maxBodyWidth))
		width = max(width, min(textWidth("Press y to delete, n or Esc to cancel"), maxBodyWidth))
	}

	return min(width, maxBodyWidth)
}

func minBodyWidth(maxBodyWidth int) int {
	return min(44, maxBodyWidth)
}

func (m model) renderPanel(layout layoutMetrics) string {
	var b strings.Builder
	header := m.renderHeader(layout.bodyWidth)
	b.WriteString(header)
	b.WriteByte('\n')
	b.WriteString(subtleStyle.Render(truncate(m.opts.ConfigPath, layout.bodyWidth)))
	b.WriteByte('\n')
	b.WriteByte('\n')

	if m.search != nil {
		b.WriteString(m.renderSearchBox(layout))
		b.WriteByte('\n')
		b.WriteByte('\n')
	}

	if m.lastLoadError != nil {
		b.WriteString(errorStyle.Render(truncate(m.lastLoadError.Error(), layout.bodyWidth)))
		return b.String()
	}

	if len(m.hosts) == 0 && m.form == nil {
		b.WriteString(subtleStyle.Render("No hosts found"))
		b.WriteByte('\n')
	} else if m.search != nil && len(m.search.matches) == 0 {
		b.WriteString(subtleStyle.Render("No matches"))
		b.WriteByte('\n')
	} else if len(m.hosts) > 0 {
		cols := m.columnWidths(layout.bodyWidth)
		b.WriteString(m.renderTableHeader(layout.bodyWidth, cols))
		b.WriteByte('\n')
		for _, line := range m.renderRows(layout.bodyWidth, layout.listRows, cols) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	modal := m.renderModal(layout)
	if modal != "" {
		b.WriteByte('\n')
		b.WriteString(lipgloss.PlaceHorizontal(layout.bodyWidth, lipgloss.Center, modal))
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(subtleStyle.Render(truncate(m.helpText(), layout.bodyWidth)))

	return strings.TrimRight(b.String(), "\n")
}

func (m model) renderSearchBox(layout layoutMetrics) string {
	if m.search == nil {
		return ""
	}
	searchInput := m.search.input
	searchInput.Width = max(1, layout.modalBodyWidth-textWidth(searchInput.Prompt))
	body := truncate(searchInput.View(), layout.modalBodyWidth)
	box := modalStyle.Width(layout.modalStyleWidth).Render(body)
	return lipgloss.PlaceHorizontal(layout.bodyWidth, lipgloss.Center, box)
}

func (m model) renderHeader(width int) string {
	if width <= 0 {
		return ""
	}

	title := "pinkssh"
	status := m.status
	if m.loading || m.checking || m.busy {
		status = m.spinner.View() + " " + status
	}
	titleWidth := textWidth(title)
	if status == "" || width <= titleWidth+1 {
		return titleStyle.Render(truncate(title, width))
	}

	statusWidth := width - titleWidth - 2
	if statusWidth <= 0 {
		return titleStyle.Render(truncate(title, width))
	}
	return titleStyle.Render(title) + "  " + subtleStyle.Render(truncate(status, statusWidth))
}

func (m model) renderModal(layout layoutMetrics) string {
	var body string
	switch {
	case m.keyPicker != nil:
		body = m.renderPicker(layout.modalBodyWidth)
	case m.form != nil:
		body = m.form.render(layout.modalBodyWidth)
	case m.confirmDelete != "":
		body = m.renderDeleteConfirm(layout.modalBodyWidth)
	}
	if body == "" {
		return ""
	}
	return modalStyle.Width(layout.modalStyleWidth).Render(body)
}

func (m model) helpText() string {
	if m.search != nil {
		return "Enter connect  Esc close  up/down select"
	}
	return "Enter connect  a add  e edit  d delete  c key  / search  r reload  q quit"
}

func (m model) renderTableHeader(width int, cols tableColumns) string {
	row := renderHostRow(cols, "Host", "Address", "Status", "Key", "Via")
	return tableHeaderStyle.Render(truncate(row, width))
}

func (m model) renderRows(width, rows int, cols tableColumns) []string {
	if m.search != nil {
		return m.renderSearchRows(width, rows, cols)
	}

	var out []string
	end := min(len(m.hosts), m.offset+rows)
	for i := m.offset; i < end; i++ {
		host := m.hosts[i]
		row := renderHostRow(
			cols,
			host.Alias,
			host.Address(),
			statusText(host, m.checking),
			keyText(host, m.checking),
			viaText(host),
		)
		row = padRight(truncate(row, width), width)
		if i == m.selected {
			row = selectedStyle.Width(width).Render(row)
		}
		out = append(out, row)
	}
	return out
}

func (m model) renderSearchRows(width, rows int, cols tableColumns) []string {
	if m.search == nil {
		return nil
	}

	var out []string
	end := min(len(m.search.matches), m.search.offset+rows)
	for pos := m.search.offset; pos < end; pos++ {
		index := m.search.matches[pos]
		if index < 0 || index >= len(m.hosts) {
			continue
		}
		host := m.hosts[index]
		row := renderHostRow(
			cols,
			host.Alias,
			host.Address(),
			statusText(host, m.checking),
			keyText(host, m.checking),
			viaText(host),
		)
		row = padRight(truncate(row, width), width)
		if pos == m.search.selected {
			row = selectedStyle.Width(width).Render(row)
		}
		out = append(out, row)
	}
	return out
}

func (m model) renderPicker(width int) string {
	if width <= 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(truncate("Select public key", width)))
	b.WriteByte('\n')
	for i, key := range m.keyPicker.keys {
		row := truncate(" "+key, width)
		if i == m.keyPicker.index {
			row = selectedStyle.Width(width).Render(padRight(row, width))
		}
		b.WriteString(row)
		if i < len(m.keyPicker.keys)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m model) renderDeleteConfirm(width int) string {
	line := "Delete " + m.confirmDelete + "?"
	help := "Press y to delete, n or Esc to cancel"
	return titleStyle.Render(truncate(line, width)) + "\n" + subtleStyle.Render(truncate(help, width))
}

func (m model) visibleRows() int {
	return m.layout().listRows
}

func (m *model) clampSelection() {
	if len(m.hosts) == 0 {
		m.selected = 0
		m.offset = 0
		return
	}
	if m.selected >= len(m.hosts) {
		m.selected = len(m.hosts) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *model) keepSelectionVisible() {
	rows := m.visibleRows()
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+rows {
		m.offset = m.selected - rows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *model) openSearch() {
	m.search = newSearchState(m.hosts, m.selected)
	m.keepSearchSelectionVisible()
}

func (m *model) closeSearch(preserveSelection bool) {
	if m.search == nil {
		return
	}
	if preserveSelection {
		if index, ok := m.search.currentHostIndex(); ok {
			m.selected = index
			m.clampSelection()
			m.keepSelectionVisible()
		}
	}
	m.search = nil
}

func (m *model) refreshSearch() {
	if m.search == nil {
		return
	}
	preserveIndex, _ := m.search.currentHostIndex()
	m.search.updateMatches(m.hosts, preserveIndex)
	m.keepSearchSelectionVisible()
}

func (m *model) keepSearchSelectionVisible() {
	if m.search == nil {
		return
	}
	rows := m.visibleRows()
	m.search.keepSelectionVisible(rows)
}

func loadHostsCmd(opts pinkssh.Options) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		hosts, watchPaths, err := pinkssh.LoadHosts(ctx, opts)
		return loadMsg{hosts: hosts, watchPaths: watchPaths, err: err}
	}
}

func statusesCmd(hosts []pinkssh.Host, opts pinkssh.Options) tea.Cmd {
	return func() tea.Msg {
		return statusesMsg{hosts: pinkssh.RefreshStatuses(context.Background(), hosts, opts)}
	}
}

func saveHostCmd(opts pinkssh.Options, mode formMode, original string, spec pinkssh.HostConfig) tea.Cmd {
	return func() tea.Msg {
		action := "added"
		var err error
		if mode == formEdit {
			action = "updated"
			err = pinkssh.UpdateHost(opts.ConfigPath, original, spec)
		} else {
			err = pinkssh.AddHost(opts.ConfigPath, spec)
		}
		return configSavedMsg{action: action, alias: spec.Alias, err: err}
	}
}

func deleteHostCmd(opts pinkssh.Options, alias string) tea.Cmd {
	return func() tea.Msg {
		return configDeletedMsg{alias: alias, err: pinkssh.DeleteHost(opts.ConfigPath, alias)}
	}
}

func watchCmd(paths []string) tea.Cmd {
	return func() tea.Msg {
		targets := newWatchTargets(paths)
		if len(targets.dirs) == 0 {
			return nil
		}

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return watchChangedMsg{err: err}
		}
		defer watcher.Close()

		for _, dir := range targets.dirs {
			if err := watcher.Add(dir); err != nil {
				return watchChangedMsg{err: err}
			}
		}

		for {
			select {
			case event := <-watcher.Events:
				if targets.relevant(event.Name) {
					time.Sleep(250 * time.Millisecond)
					return watchChangedMsg{}
				}
			case err := <-watcher.Errors:
				return watchChangedMsg{err: err}
			}
		}
	}
}

type watchTargets struct {
	dirs        []string
	files       map[string]bool
	sshDir      string
	includeDirs map[string]bool
}

func newWatchTargets(paths []string) watchTargets {
	targets := watchTargets{
		files:       map[string]bool{},
		includeDirs: map[string]bool{},
		sshDir:      filepath.Clean(pinkssh.SSHDir()),
	}
	dirSet := map[string]bool{}
	addDir := func(dir string) {
		if dir == "" {
			return
		}
		dir = filepath.Clean(dir)
		key := strings.ToLower(dir)
		if dirSet[key] {
			return
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			dirSet[key] = true
			targets.dirs = append(targets.dirs, dir)
		}
	}

	for _, path := range paths {
		path = filepath.Clean(path)
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			addDir(path)
			if strings.EqualFold(path, targets.sshDir) {
				continue
			}
			targets.includeDirs[strings.ToLower(path)] = true
			continue
		}
		targets.files[strings.ToLower(path)] = true
		addDir(filepath.Dir(path))
	}
	addDir(targets.sshDir)
	return targets
}

func (w watchTargets) relevant(path string) bool {
	path = filepath.Clean(path)
	lower := strings.ToLower(path)
	if w.files[lower] {
		return true
	}
	dir := strings.ToLower(filepath.Dir(path))
	if dir == strings.ToLower(w.sshDir) && strings.HasSuffix(lower, ".pub") {
		return true
	}
	return w.includeDirs[dir]
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "~")
}

type tableColumns struct {
	host    int
	address int
	status  int
	key     int
	via     int
}

func renderHostRow(cols tableColumns, host, address, status, key, via string) string {
	return padRight(host, cols.host) +
		padRight(address, cols.address) +
		padRight(status, cols.status) +
		padRight(key, cols.key) +
		padRight(via, cols.via)
}

func (m model) columnWidths(width int) tableColumns {
	return columnWidths(width, m.tableHosts(), m.checking)
}

func columnWidths(width int, hosts []pinkssh.Host, checking bool) tableColumns {
	if width <= 0 {
		return tableColumns{}
	}

	return fitColumns(width, naturalTableColumns(hosts, checking))
}

func naturalTableColumns(hosts []pinkssh.Host, checking bool) tableColumns {
	statusW := textWidth("Status")
	keyW := textWidth("Key")
	viaW := textWidth("Via")
	hostContentW := textWidth("Host")
	addressContentW := textWidth("Address")

	for _, host := range hosts {
		hostContentW = max(hostContentW, textWidth(host.Alias))
		addressContentW = max(addressContentW, textWidth(host.Address()))
		statusW = max(statusW, textWidth(statusText(host, checking)))
		keyW = max(keyW, textWidth(keyText(host, checking)))
		viaW = max(viaW, textWidth(viaText(host)))
	}

	hostW := hostContentW + 4
	addressW := addressContentW + 4
	statusW += 2
	keyW += 2
	viaW = max(viaW, 3)

	return tableColumns{
		host:    hostW,
		address: addressW,
		status:  statusW,
		key:     keyW,
		via:     viaW,
	}
}

func fitColumns(width int, cols tableColumns) tableColumns {
	over := cols.totalWidth() - width
	if over <= 0 {
		return cols
	}

	minAddress := min(cols.address, max(3, min(12, width/3)))
	minHost := min(cols.host, max(4, min(12, width/4)))
	minStatus := min(cols.status, 6)
	minKey := min(cols.key, 6)
	minVia := min(cols.via, 3)

	shrink := func(value *int, minimum int) {
		if over <= 0 || *value <= minimum {
			return
		}
		delta := min(over, *value-minimum)
		*value -= delta
		over -= delta
	}

	shrink(&cols.address, minAddress)
	shrink(&cols.host, minHost)
	shrink(&cols.key, minKey)
	shrink(&cols.status, minStatus)
	shrink(&cols.via, minVia)

	for over > 0 {
		switch {
		case cols.address > 1:
			cols.address--
		case cols.host > 1:
			cols.host--
		case cols.key > 1:
			cols.key--
		case cols.status > 1:
			cols.status--
		case cols.via > 1:
			cols.via--
		default:
			return cols
		}
		over--
	}
	return cols
}

func (c tableColumns) totalWidth() int {
	return c.host + c.address + c.status + c.key + c.via
}

func textWidth(value string) int {
	return ansi.StringWidth(value)
}

func hostCountStatus(count int) string {
	if count == 1 {
		return "1 host"
	}
	return fmt.Sprintf("%d hosts", count)
}

func statusText(host pinkssh.Host, checking bool) string {
	switch host.Status {
	case pinkssh.NetworkOnline:
		return "online"
	case pinkssh.NetworkOffline:
		return "offline"
	case pinkssh.NetworkProxy:
		return "proxy"
	default:
		if checking {
			return "checking"
		}
		return "unknown"
	}
}

func keyText(host pinkssh.Host, checking bool) string {
	switch host.Auth {
	case pinkssh.AuthKeyOK:
		return "accepted"
	case pinkssh.AuthCopyKey:
		return "needs copy"
	default:
		if checking {
			return "checking"
		}
		if host.LocalKey {
			return "local only"
		}
		return "no key"
	}
}

func viaText(host pinkssh.Host) string {
	switch {
	case valueSet(host.ProxyJump):
		return "jump"
	case valueSet(host.ProxyCommand):
		return "cmd"
	default:
		return "direct"
	}
}

func valueSet(value string) bool {
	switch value {
	case "", "none", "None", "NONE":
		return false
	default:
		return true
	}
}

func padRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = truncate(value, width)
	padding := width - textWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
