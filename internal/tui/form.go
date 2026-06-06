package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pinkssh/internal/pinkssh"
)

type formMode int

const (
	formAdd formMode = iota
	formEdit
)

const (
	fieldAlias = iota
	fieldHostName
	fieldUser
	fieldPort
	fieldIdentityFiles
	fieldProxyJump
	fieldProxyCommand
)

var fieldLabels = []string{
	"Alias",
	"HostName",
	"User",
	"Port",
	"IdentityFile",
	"ProxyJump",
	"ProxyCommand",
}

type hostForm struct {
	mode                formMode
	original            string
	focus               int
	fields              []textinput.Model
	identityChoices     []string
	identityChoiceIndex int
}

func newAddForm() *hostForm {
	return newHostForm(formAdd, "", pinkssh.HostConfig{})
}

func newEditForm(original string, spec pinkssh.HostConfig) *hostForm {
	return newHostForm(formEdit, original, spec)
}

func newHostForm(mode formMode, original string, spec pinkssh.HostConfig) *hostForm {
	identityChoices := pinkssh.IdentityFileChoices()
	values := []string{
		spec.Alias,
		spec.HostName,
		spec.User,
		spec.Port,
		strings.Join(spec.IdentityFiles, "; "),
		spec.ProxyJump,
		spec.ProxyCommand,
	}
	if mode == formAdd && values[fieldIdentityFiles] == "" && len(identityChoices) > 0 {
		values[fieldIdentityFiles] = identityChoices[0]
	}
	identityChoices = mergeIdentityChoices(values[fieldIdentityFiles], identityChoices)

	fields := make([]textinput.Model, len(fieldLabels))
	for i := range fields {
		input := textinput.New()
		input.Placeholder = fieldLabels[i]
		input.CharLimit = 512
		input.Width = 44
		input.Prompt = ""
		input.SetValue(values[i])
		input.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f2f2f2"))
		input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4fd8"))
		fields[i] = input
	}

	form := &hostForm{
		mode:                mode,
		original:            original,
		fields:              fields,
		identityChoices:     identityChoices,
		identityChoiceIndex: indexOfString(identityChoices, values[fieldIdentityFiles]),
	}
	if form.identityChoiceIndex < 0 {
		form.identityChoiceIndex = 0
	}
	form.focusField(0)
	return form
}

func (f *hostForm) focusField(index int) {
	if index < 0 {
		index = len(f.fields) - 1
	}
	if index >= len(f.fields) {
		index = 0
	}
	for i := range f.fields {
		if i == index {
			f.fields[i].Focus()
		} else {
			f.fields[i].Blur()
		}
	}
	f.focus = index
}

func (f *hostForm) nextField() {
	f.focusField(f.focus + 1)
}

func (f *hostForm) previousField() {
	f.focusField(f.focus - 1)
}

func (f *hostForm) update(msg tea.KeyMsg) tea.Cmd {
	if f.focus == fieldIdentityFiles && len(f.identityChoices) > 0 {
		switch msg.String() {
		case "up", "left":
			f.selectIdentity(f.identityChoiceIndex - 1)
			return nil
		case "down", "right":
			f.selectIdentity(f.identityChoiceIndex + 1)
			return nil
		}
	}
	var cmd tea.Cmd
	f.fields[f.focus], cmd = f.fields[f.focus].Update(msg)
	if f.focus == fieldIdentityFiles {
		f.identityChoiceIndex = indexOfString(f.identityChoices, strings.TrimSpace(f.fields[fieldIdentityFiles].Value()))
		if f.identityChoiceIndex < 0 {
			f.identityChoiceIndex = 0
		}
	}
	return cmd
}

func (f *hostForm) hasIdentityDropdown() bool {
	return f.focus == fieldIdentityFiles && len(f.identityChoices) > 0
}

func (f *hostForm) extraRows(width int) int {
	rows := 0
	if formUsesStackedFields(width) {
		rows += len(f.fields)
	}
	if f.hasIdentityDropdown() {
		rows += min(6, len(f.identityChoices))
	}
	return rows
}

func (f *hostForm) selectIdentity(index int) {
	if len(f.identityChoices) == 0 {
		return
	}
	if index < 0 {
		index = len(f.identityChoices) - 1
	}
	if index >= len(f.identityChoices) {
		index = 0
	}
	f.identityChoiceIndex = index
	f.fields[fieldIdentityFiles].SetValue(f.identityChoices[index])
}

func (f hostForm) spec() pinkssh.HostConfig {
	return pinkssh.HostConfig{
		Alias:         strings.TrimSpace(f.fields[fieldAlias].Value()),
		HostName:      strings.TrimSpace(f.fields[fieldHostName].Value()),
		User:          strings.TrimSpace(f.fields[fieldUser].Value()),
		Port:          strings.TrimSpace(f.fields[fieldPort].Value()),
		IdentityFiles: splitIdentityField(f.fields[fieldIdentityFiles].Value()),
		ProxyJump:     strings.TrimSpace(f.fields[fieldProxyJump].Value()),
		ProxyCommand:  strings.TrimSpace(f.fields[fieldProxyCommand].Value()),
	}
}

func (f hostForm) title() string {
	if f.mode == formEdit {
		return "Edit host"
	}
	return "Add host"
}

func (f hostForm) render(width int) string {
	width = max(1, width)

	var b strings.Builder
	b.WriteString(titleStyle.Render(truncate(f.title(), width)))
	b.WriteByte('\n')
	labelWidth := formLabelWidth(width)
	stacked := formUsesStackedFields(width)
	inputWidth := width
	if !stacked {
		inputWidth = max(1, width-labelWidth-2)
	}
	labelRenderWidth := labelWidth
	if stacked {
		labelRenderWidth = width
	}

	for i := range f.fields {
		field := f.fields[i]
		field.Width = inputWidth
		label := fieldLabels[i]
		if i == f.focus {
			label = selectedStyle.Render(truncate(label, max(1, labelRenderWidth)))
		} else {
			label = subtleStyle.Render(truncate(label, max(1, labelRenderWidth)))
		}

		if stacked {
			b.WriteString(label)
			b.WriteByte('\n')
			b.WriteString(truncate(field.View(), width))
		} else {
			label = padRight(label, labelWidth)
			line := " " + label + " " + field.View()
			b.WriteString(truncate(line, width))
		}

		if i == fieldIdentityFiles && f.focus == fieldIdentityFiles && len(f.identityChoices) > 0 {
			b.WriteByte('\n')
			b.WriteString(f.renderIdentityDropdown(width, labelWidth))
		}
		if i < len(f.fields)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (f hostForm) renderIdentityDropdown(width, labelWidth int) string {
	width = max(1, width)
	maxRows := min(6, len(f.identityChoices))
	start := f.identityChoiceIndex - maxRows/2
	if start < 0 {
		start = 0
	}
	if start+maxRows > len(f.identityChoices) {
		start = max(0, len(f.identityChoices)-maxRows)
	}
	end := min(len(f.identityChoices), start+maxRows)

	var b strings.Builder
	indent := strings.Repeat(" ", min(max(0, width-1), labelWidth+3))
	contentWidth := max(1, width-textWidth(indent))
	for i := start; i < end; i++ {
		prefix := "  "
		style := subtleStyle
		if i == f.identityChoiceIndex {
			prefix = "> "
			style = selectedStyle
		}
		line := indent + style.Render(truncate(prefix+f.identityChoices[i], contentWidth))
		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func formLabelWidth(width int) int {
	if formUsesStackedFields(width) {
		return 0
	}
	if width < 38 {
		return 9
	}
	return 12
}

func formUsesStackedFields(width int) bool {
	return width < 24
}

func splitIdentityField(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ";") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func mergeIdentityChoices(current string, choices []string) []string {
	current = strings.TrimSpace(current)
	if current == "" {
		return choices
	}
	if indexOfString(choices, current) >= 0 {
		return choices
	}
	return append([]string{current}, choices...)
}

func indexOfString(values []string, target string) int {
	for i, value := range values {
		if strings.EqualFold(value, target) {
			return i
		}
	}
	return -1
}
