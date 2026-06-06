package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pinkssh/internal/pinkssh"
)

func TestLayoutAdaptsToTerminalWidth(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		minInner  int
		maxBorder int
	}{
		{name: "wide stays compact without table content", width: 160, minInner: 44, maxBorder: 58},
		{name: "medium stays compact without table content", width: 96, minInner: 44, maxBorder: 58},
		{name: "narrow fits shell", width: 40, minInner: 20, maxBorder: 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(pinkssh.DefaultOptions())
			m.width = tt.width
			m.height = 32
			layout := m.layout()
			renderedWidth := layout.bodyWidth + panelStyle.GetHorizontalFrameSize()
			if renderedWidth > tt.maxBorder {
				t.Fatalf("rendered width = %d, want <= %d", renderedWidth, tt.maxBorder)
			}
			if renderedWidth > tt.width {
				t.Fatalf("rendered width = %d exceeds terminal width %d", renderedWidth, tt.width)
			}
			if layout.bodyWidth < tt.minInner {
				t.Fatalf("body width = %d, want >= %d", layout.bodyWidth, tt.minInner)
			}
		})
	}
}

func TestLayoutShrinksToNaturalTableWidth(t *testing.T) {
	m := newModel(pinkssh.DefaultOptions())
	m.width = 160
	m.height = 32
	m.loading = false
	m.status = "2 hosts"
	m.hosts = []pinkssh.Host{
		{
			Alias:    "router",
			HostName: "192.0.2.254",
			User:     "root",
			Port:     "22",
			Status:   pinkssh.NetworkOnline,
			Auth:     pinkssh.AuthCopyKey,
		},
		{
			Alias:    "workstation",
			HostName: "workstation.local",
			User:     "alice",
			Port:     "22",
			Status:   pinkssh.NetworkOnline,
			Auth:     pinkssh.AuthKeyOK,
		},
	}

	layout := m.layout()
	want := naturalTableColumns(m.hosts, false).totalWidth()
	if layout.bodyWidth != want {
		t.Fatalf("body width = %d, want natural table width %d", layout.bodyWidth, want)
	}
}

func TestTableDoesNotWrapViaColumn(t *testing.T) {
	m := newModel(pinkssh.DefaultOptions())
	m.width = 120
	m.height = 28
	m.loading = false
	m.status = "7 hosts"
	m.hosts = []pinkssh.Host{
		{Alias: "Lab: PVE", HostName: "pve.example.test", User: "root", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthKeyOK},
		{Alias: "Lab: Docker", HostName: "docker.example.test", User: "alice", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthKeyOK},
		{Alias: "Workstation", HostName: "workstation.local", User: "alice", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthCopyKey},
		{Alias: "Workstation Admin", HostName: "workstation.local", User: "admin", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthCopyKey},
		{Alias: "Remote: PVE", HostName: "198.51.100.20", User: "root", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthKeyOK},
		{Alias: "Cloud VM", HostName: "203.0.113.10", User: "alice", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthKeyOK},
		{Alias: "Gateway", HostName: "192.0.2.254", User: "root", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthCopyKey},
	}

	view := m.View()
	if strings.Contains(view, "\n│  Via") || strings.Contains(view, "\n│  direct") {
		t.Fatalf("Via column wrapped onto its own line:\n%s", view)
	}
	for _, alias := range []string{"Lab: PVE", "Gateway"} {
		line := renderedLineContaining(view, alias)
		if line == "" {
			t.Fatalf("missing rendered line for %q:\n%s", alias, view)
		}
		if !strings.Contains(line, "direct") {
			t.Fatalf("row for %q does not keep Via value on same line: %q", alias, line)
		}
	}
}

func TestColumnWidthsFitAvailableWidth(t *testing.T) {
	for _, width := range []int{12, 28, 40, 60, 90, 120} {
		cols := columnWidths(width, nil, false)
		if cols.totalWidth() > width {
			t.Fatalf("columns exceed width %d: %#v", width, cols)
		}
	}
}

func TestHostAndAddressColumnsFitContentPlusFour(t *testing.T) {
	hosts := []pinkssh.Host{
		{
			Alias:    "pve",
			HostName: "coal",
			User:     "root",
			Port:     "22",
			Status:   pinkssh.NetworkOnline,
			Auth:     pinkssh.AuthKeyOK,
		},
		{
			Alias:    "development",
			HostName: "devbox.internal",
			User:     "alice",
			Port:     "2222",
			Status:   pinkssh.NetworkOnline,
			Auth:     pinkssh.AuthKeyOK,
		},
	}

	cols := columnWidths(140, hosts, false)
	if cols.host != textWidth("development")+4 {
		t.Fatalf("host width = %d, want %d", cols.host, textWidth("development")+4)
	}
	wantAddress := textWidth("alice@devbox.internal:2222") + 4
	if cols.address != wantAddress {
		t.Fatalf("address width = %d, want %d", cols.address, wantAddress)
	}
}

func TestHeaderUsesCompactStatusSpacing(t *testing.T) {
	m := newModel(pinkssh.DefaultOptions())
	m.loading = false
	m.status = "12 hosts"

	header := m.renderHeader(80)
	wantWidth := textWidth("pinkssh  12 hosts")
	if got := textWidth(header); got != wantWidth {
		t.Fatalf("header width = %d, want compact width %d: %q", got, wantWidth, header)
	}
}

func TestHostCountStatusPluralizes(t *testing.T) {
	if got := hostCountStatus(0); got != "0 hosts" {
		t.Fatalf("hostCountStatus(0) = %q", got)
	}
	if got := hostCountStatus(1); got != "1 host" {
		t.Fatalf("hostCountStatus(1) = %q", got)
	}
	if got := hostCountStatus(2); got != "2 hosts" {
		t.Fatalf("hostCountStatus(2) = %q", got)
	}
}

func TestViewFitsTerminalWidth(t *testing.T) {
	for _, width := range []int{38, 80, 132} {
		m := newModel(pinkssh.DefaultOptions())
		m.width = width
		m.height = 28
		m.status = "1 host"
		m.hosts = []pinkssh.Host{{
			Alias:    "very-long-development-host-alias",
			HostName: "very-long-hostname.example.internal",
			User:     "alice",
			Port:     "2222",
			Status:   pinkssh.NetworkOnline,
			Auth:     pinkssh.AuthKeyOK,
			LocalKey: true,
		}}
		view := m.View()
		for i, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width %d line %d rendered at %d columns: %q", width, i+1, got, line)
			}
		}
	}
}

func TestModalViewsFitTerminalWidth(t *testing.T) {
	for _, width := range []int{18, 24, 36, 64} {
		t.Run(fmt.Sprintf("form width %d", width), func(t *testing.T) {
			m := modelWithOneHost(width)
			form := newAddForm()
			form.identityChoices = []string{"~/.ssh/id_ed25519", "~/.ssh/work"}
			form.focusField(fieldIdentityFiles)
			m.form = form
			assertViewFitsWidth(t, m, width)
		})

		t.Run(fmt.Sprintf("picker width %d", width), func(t *testing.T) {
			m := modelWithOneHost(width)
			m.keyPicker = &keyPicker{keys: []string{"C:/Users/example/.ssh/id_ed25519.pub", "C:/Users/example/.ssh/work.pub"}}
			assertViewFitsWidth(t, m, width)
		})

		t.Run(fmt.Sprintf("delete width %d", width), func(t *testing.T) {
			m := modelWithOneHost(width)
			m.confirmDelete = "very-long-development-host-alias"
			assertViewFitsWidth(t, m, width)
		})
	}
}

func TestRenderHelpersRespectCellWidths(t *testing.T) {
	if got := textWidth(truncate(selectedStyle.Render("abcdef"), 4)); got > 4 {
		t.Fatalf("truncate rendered %d cells, want <= 4", got)
	}
	if got := textWidth(padRight("abc", 8)); got != 8 {
		t.Fatalf("padRight rendered %d cells, want 8", got)
	}
}

func TestFuzzyHostMatches(t *testing.T) {
	hosts := searchTestHosts()

	tests := []struct {
		name  string
		query string
		want  []int
	}{
		{name: "empty", query: "", want: []int{0, 1, 2}},
		{name: "exact host", query: "workstation", want: []int{1}},
		{name: "case insensitive subsequence", query: "cldck", want: []int{0}},
		{name: "address term", query: "192254", want: []int{2}},
		{name: "multi term", query: "root 254", want: []int{2}},
		{name: "no match", query: "missing", want: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyHostMatches(hosts, tt.query)
			if !sameInts(got, tt.want) {
				t.Fatalf("fuzzyHostMatches(%q) = %#v, want %#v", tt.query, got, tt.want)
			}
		})
	}
}

func TestFuzzyHostMatchesStableTieOrder(t *testing.T) {
	hosts := []pinkssh.Host{
		{Alias: "box", HostName: "one"},
		{Alias: "box", HostName: "two"},
	}
	if got := fuzzyHostMatches(hosts, "box"); !sameInts(got, []int{0, 1}) {
		t.Fatalf("tie order = %#v, want original order", got)
	}
}

func TestSearchOpensAndFiltersHostTable(t *testing.T) {
	m := modelWithSearchHosts()
	m = handleTestKey(m, keyRunes("/"))
	if m.search == nil {
		t.Fatal("search did not open")
	}

	m = typeSearchText(m, "docker")
	if !sameInts(m.search.matches, []int{0}) {
		t.Fatalf("matches = %#v, want Docker only", m.search.matches)
	}

	view := m.View()
	if !strings.Contains(view, "Lab: Docker") {
		t.Fatalf("filtered view missing match:\n%s", view)
	}
	if strings.Contains(view, "Workstation") || strings.Contains(view, "Gateway") {
		t.Fatalf("filtered view includes non-matches:\n%s", view)
	}
}

func TestSearchNavigationAndEscapePreserveSelectedHost(t *testing.T) {
	m := modelWithSearchHosts()
	m.selected = 0
	m = handleTestKey(m, keyRunes("/"))
	m = handleTestKey(m, keyType(tea.KeyDown))
	if m.search == nil || m.search.selected != 1 {
		t.Fatalf("search selected = %v, want 1", m.search)
	}

	m = handleTestKey(m, keyType(tea.KeyEsc))
	if m.search != nil {
		t.Fatal("search did not close")
	}
	if m.selected != 1 {
		t.Fatalf("selected host = %d, want 1", m.selected)
	}
}

func TestSearchEnterConnectsSelectedMatch(t *testing.T) {
	m := modelWithSearchHosts()
	m = handleTestKey(m, keyRunes("/"))
	m = typeSearchText(m, "gateway")

	next, cmd := m.handleKey(keyType(tea.KeyEnter))
	m = next.(model)
	if cmd == nil {
		t.Fatal("enter did not return connect command")
	}
	if m.search != nil {
		t.Fatal("search stayed open after connect")
	}
	if !m.busy || m.status != "connecting: Gateway" {
		t.Fatalf("connect state = busy %v status %q", m.busy, m.status)
	}
	if m.selected != 2 {
		t.Fatalf("selected host = %d, want 2", m.selected)
	}
}

func TestSearchNoMatchesRendersCleanly(t *testing.T) {
	m := modelWithSearchHosts()
	m = handleTestKey(m, keyRunes("/"))
	m = typeSearchText(m, "zzzzz")

	if len(m.search.matches) != 0 {
		t.Fatalf("matches = %#v, want none", m.search.matches)
	}
	view := m.View()
	if !strings.Contains(view, "No matches") {
		t.Fatalf("no-match view missing message:\n%s", view)
	}
	assertViewFitsWidth(t, m, m.width)
}

func TestSearchViewFitsTerminalWidth(t *testing.T) {
	for _, width := range []int{24, 40, 80} {
		m := modelWithSearchHosts()
		m.width = width
		m = handleTestKey(m, keyRunes("/"))
		m = typeSearchText(m, "root 254")
		assertViewFitsWidth(t, m, width)
	}
}

func TestKeyTextStates(t *testing.T) {
	tests := []struct {
		name     string
		host     pinkssh.Host
		checking bool
		want     string
	}{
		{name: "checking", checking: true, want: "checking"},
		{name: "accepted", host: pinkssh.Host{Auth: pinkssh.AuthKeyOK}, want: "accepted"},
		{name: "needs copy", host: pinkssh.Host{Auth: pinkssh.AuthCopyKey}, want: "needs copy"},
		{name: "local only", host: pinkssh.Host{LocalKey: true}, want: "local only"},
		{name: "no key", host: pinkssh.Host{}, want: "no key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyText(tt.host, tt.checking); got != tt.want {
				t.Fatalf("keyText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func searchTestHosts() []pinkssh.Host {
	return []pinkssh.Host{
		{Alias: "Lab: Docker", HostName: "docker.example.test", User: "alice", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthKeyOK},
		{Alias: "Workstation", HostName: "workstation.local", User: "alice", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthCopyKey},
		{Alias: "Gateway", HostName: "192.0.2.254", User: "root", Port: "22", Status: pinkssh.NetworkOnline, Auth: pinkssh.AuthCopyKey},
	}
}

func modelWithSearchHosts() model {
	m := newModel(pinkssh.DefaultOptions())
	m.width = 96
	m.height = 28
	m.loading = false
	m.status = "3 hosts"
	m.hosts = searchTestHosts()
	return m
}

func handleTestKey(m model, key tea.KeyMsg) model {
	next, _ := m.handleKey(key)
	return next.(model)
}

func typeSearchText(m model, text string) model {
	for _, r := range text {
		m = handleTestKey(m, keyRunes(string(r)))
	}
	return m
}

func keyRunes(value string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}

func keyType(value tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: value}
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func modelWithOneHost(width int) model {
	m := newModel(pinkssh.DefaultOptions())
	m.width = width
	m.height = 24
	m.loading = false
	m.status = "1 host"
	m.hosts = []pinkssh.Host{{
		Alias:    "very-long-development-host-alias",
		HostName: "very-long-hostname.example.internal",
		User:     "alice",
		Port:     "2222",
		Status:   pinkssh.NetworkOnline,
		Auth:     pinkssh.AuthKeyOK,
		LocalKey: true,
	}}
	return m
}

func assertViewFitsWidth(t *testing.T, m model, width int) {
	t.Helper()
	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("width %d line %d rendered at %d columns: %q", width, i+1, got, line)
		}
	}
}

func renderedLineContaining(view, fragment string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, fragment) {
			return line
		}
	}
	return ""
}
