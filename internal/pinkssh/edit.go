package pinkssh

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var (
	ErrHostExists   = errors.New("host already exists")
	ErrHostNotFound = errors.New("host not found")
)

type HostConfig struct {
	Alias         string
	HostName      string
	User          string
	Port          string
	IdentityFiles []string
	ProxyJump     string
	ProxyCommand  string
}

func HostConfigFromHost(host Host) HostConfig {
	return HostConfig{
		Alias:         host.Alias,
		HostName:      host.HostName,
		User:          host.User,
		Port:          host.Port,
		IdentityFiles: append([]string(nil), host.IdentityFiles...),
		ProxyJump:     host.ProxyJump,
		ProxyCommand:  host.ProxyCommand,
	}
}

func ReadHostConfig(configPath, alias string) (HostConfig, error) {
	block, err := findHostBlock(configPath, alias)
	if err != nil {
		return HostConfig{}, err
	}
	return parseHostBlockConfig(block), nil
}

func AddHost(configPath string, spec HostConfig) error {
	if err := validateHostConfig(spec); err != nil {
		return err
	}
	if exists, err := hostExists(configPath, spec.Alias); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("%w: %s", ErrHostExists, spec.Alias)
	}

	path := expandPath(configPath, "")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	doc, err := readConfigDocument(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(doc.lines) > 0 && strings.TrimSpace(doc.lines[len(doc.lines)-1]) != "" {
		doc.lines = append(doc.lines, "")
	}
	doc.lines = append(doc.lines, renderHostBlock(spec)...)
	return doc.write()
}

func UpdateHost(configPath, alias string, spec HostConfig) error {
	if spec.Alias == "" {
		spec.Alias = alias
	}
	if err := validateHostConfig(spec); err != nil {
		return err
	}
	if !strings.EqualFold(alias, spec.Alias) {
		if exists, err := hostExists(configPath, spec.Alias); err != nil {
			return err
		} else if exists {
			return fmt.Errorf("%w: %s", ErrHostExists, spec.Alias)
		}
	}

	block, err := findHostBlock(configPath, alias)
	if err != nil {
		return err
	}
	replacement, err := updateHostBlockLines(block, alias, spec)
	if err != nil {
		return err
	}

	doc := block.doc
	doc.lines = replaceLines(doc.lines, block.start, block.end, replacement)
	return doc.write()
}

func DeleteHost(configPath, alias string) error {
	block, err := findHostBlock(configPath, alias)
	if err != nil {
		return err
	}

	aliases := append([]string(nil), block.patterns...)
	var remaining []string
	for _, pattern := range aliases {
		if strings.EqualFold(pattern, alias) {
			continue
		}
		remaining = append(remaining, pattern)
	}

	doc := block.doc
	if len(remaining) == 0 {
		doc.lines = replaceLines(doc.lines, block.start, block.end, nil)
	} else {
		doc.lines[block.start] = renderHostLine(remaining)
	}
	return doc.write()
}

type configDocument struct {
	path    string
	lines   []string
	newline string
}

func readConfigDocument(path string) (configDocument, error) {
	path = expandPath(path, "")
	data, err := os.ReadFile(path)
	doc := configDocument{path: path, newline: "\n"}
	if err != nil {
		return doc, err
	}
	raw := string(data)
	if strings.Contains(raw, "\r\n") {
		doc.newline = "\r\n"
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimSuffix(raw, "\n")
	if raw == "" {
		return doc, nil
	}
	doc.lines = strings.Split(raw, "\n")
	return doc, nil
}

func (d configDocument) write() error {
	if err := os.MkdirAll(filepath.Dir(d.path), 0o700); err != nil {
		return err
	}
	content := strings.Join(d.lines, d.newline)
	if content != "" {
		content += d.newline
	}
	return os.WriteFile(d.path, []byte(content), 0o600)
}

type hostBlock struct {
	doc      configDocument
	start    int
	end      int
	patterns []string
}

func findHostBlock(configPath, alias string) (hostBlock, error) {
	if configPath == "" {
		configPath = filepath.Join(SSHDir(), "config")
	}
	state := discoveryState{
		seenFiles: map[string]bool{},
		seenHosts: map[string]bool{},
	}
	block, found, err := findHostBlockInFile(expandPath(configPath, ""), alias, &state)
	if err != nil {
		return hostBlock{}, err
	}
	if !found {
		return hostBlock{}, fmt.Errorf("%w: %s", ErrHostNotFound, alias)
	}
	return block, nil
}

func findHostBlockInFile(path, alias string, state *discoveryState) (hostBlock, bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return hostBlock{}, false, err
	}
	abs = filepath.Clean(abs)
	if state.seenFiles[abs] {
		return hostBlock{}, false, nil
	}
	state.seenFiles[abs] = true

	doc, err := readConfigDocument(abs)
	if errors.Is(err, os.ErrNotExist) {
		return hostBlock{}, false, nil
	}
	if err != nil {
		return hostBlock{}, false, err
	}

	baseDir := filepath.Dir(abs)
	for i, line := range doc.lines {
		tokens, err := splitSSHWords(strings.TrimRight(line, "\r"))
		if err != nil {
			return hostBlock{}, false, err
		}
		if len(tokens) == 0 {
			continue
		}

		switch strings.ToLower(tokens[0]) {
		case "include":
			for _, include := range tokens[1:] {
				for _, match := range expandInclude(include, baseDir) {
					block, found, err := findHostBlockInFile(match, alias, state)
					if err != nil {
						return hostBlock{}, false, err
					}
					if found {
						return block, true, nil
					}
				}
			}
		case "host":
			patterns := tokens[1:]
			for _, pattern := range patterns {
				if !isConcreteHostPattern(pattern) {
					continue
				}
				key := strings.ToLower(pattern)
				if state.seenHosts[key] {
					continue
				}
				state.seenHosts[key] = true
				if strings.EqualFold(pattern, alias) {
					return hostBlock{
						doc:      doc,
						start:    i,
						end:      findHostBlockEnd(doc.lines, i),
						patterns: patterns,
					}, true, nil
				}
			}
		}
	}
	return hostBlock{}, false, nil
}

func findHostBlockEnd(lines []string, start int) int {
	for i := start + 1; i < len(lines); i++ {
		tokens, err := splitSSHWords(lines[i])
		if err != nil || len(tokens) == 0 {
			continue
		}
		switch strings.ToLower(tokens[0]) {
		case "host", "match":
			return i
		}
	}
	return len(lines)
}

func parseHostBlockConfig(block hostBlock) HostConfig {
	spec := HostConfig{Alias: firstConcretePattern(block.patterns)}
	for _, line := range block.doc.lines[block.start+1 : block.end] {
		tokens, err := splitSSHWords(line)
		if err != nil || len(tokens) < 2 {
			continue
		}
		value := strings.Join(tokens[1:], " ")
		switch strings.ToLower(tokens[0]) {
		case "hostname":
			spec.HostName = value
		case "user":
			spec.User = value
		case "port":
			spec.Port = value
		case "identityfile":
			spec.IdentityFiles = append(spec.IdentityFiles, value)
		case "proxyjump":
			if isSet(value) {
				spec.ProxyJump = value
			}
		case "proxycommand":
			if isSet(value) {
				spec.ProxyCommand = value
			}
		}
	}
	return spec
}

func updateHostBlockLines(block hostBlock, oldAlias string, spec HostConfig) ([]string, error) {
	lines := append([]string(nil), block.doc.lines[block.start:block.end]...)
	for i, pattern := range block.patterns {
		if strings.EqualFold(pattern, oldAlias) {
			block.patterns[i] = spec.Alias
			break
		}
	}
	lines[0] = renderHostLine(block.patterns)

	indent := detectOptionIndent(lines[1:])
	optionLines := map[string][]string{
		"hostname":     renderOptionLines(indent, "HostName", []string{spec.HostName}),
		"user":         renderOptionLines(indent, "User", []string{spec.User}),
		"port":         renderOptionLines(indent, "Port", []string{spec.Port}),
		"identityfile": renderOptionLines(indent, "IdentityFile", spec.IdentityFiles),
		"proxyjump":    renderOptionLines(indent, "ProxyJump", []string{spec.ProxyJump}),
		"proxycommand": renderOptionLines(indent, "ProxyCommand", []string{spec.ProxyCommand}),
	}
	order := []string{"hostname", "user", "port", "identityfile", "proxyjump", "proxycommand"}
	seen := map[string]bool{}

	var updated []string
	updated = append(updated, lines[0])
	for _, line := range lines[1:] {
		tokens, err := splitSSHWords(line)
		if err != nil || len(tokens) == 0 {
			updated = append(updated, line)
			continue
		}
		key := strings.ToLower(tokens[0])
		replacement, ok := optionLines[key]
		if !ok {
			updated = append(updated, line)
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		updated = append(updated, replacement...)
	}

	for _, key := range order {
		if seen[key] || len(optionLines[key]) == 0 {
			continue
		}
		updated = append(updated, optionLines[key]...)
	}
	return updated, nil
}

func renderHostBlock(spec HostConfig) []string {
	lines := []string{renderHostLine([]string{spec.Alias})}
	lines = append(lines, renderOptionLines("    ", "HostName", []string{spec.HostName})...)
	lines = append(lines, renderOptionLines("    ", "User", []string{spec.User})...)
	lines = append(lines, renderOptionLines("    ", "Port", []string{spec.Port})...)
	lines = append(lines, renderOptionLines("    ", "IdentityFile", spec.IdentityFiles)...)
	lines = append(lines, renderOptionLines("    ", "ProxyJump", []string{spec.ProxyJump})...)
	lines = append(lines, renderOptionLines("    ", "ProxyCommand", []string{spec.ProxyCommand})...)
	return lines
}

func renderHostLine(patterns []string) string {
	var quoted []string
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		quoted = append(quoted, quoteSSHConfigValue(pattern))
	}
	return "Host " + strings.Join(quoted, " ")
}

func renderOptionLines(indent, key string, values []string) []string {
	var lines []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		lines = append(lines, indent+key+" "+quoteSSHConfigValue(value))
	}
	return lines
}

func quoteSSHConfigValue(value string) string {
	if value == "" {
		return `""`
	}
	needsQuote := false
	for _, r := range value {
		if unicode.IsSpace(r) || r == '#' || r == '"' || r == '\'' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return value
	}
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func detectOptionIndent(lines []string) string {
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
			continue
		}
		return strings.TrimSuffix(line, trimmed)
	}
	return "    "
}

func replaceLines(lines []string, start, end int, replacement []string) []string {
	out := append([]string(nil), lines[:start]...)
	out = append(out, replacement...)
	out = append(out, lines[end:]...)
	return out
}

func validateHostConfig(spec HostConfig) error {
	if spec.Alias = strings.TrimSpace(spec.Alias); spec.Alias == "" {
		return errors.New("alias is required")
	}
	if !isConcreteHostPattern(spec.Alias) {
		return errors.New("alias must be a concrete Host name, not a wildcard or negated pattern")
	}
	for _, value := range []string{spec.Alias, spec.HostName, spec.User, spec.Port, spec.ProxyJump, spec.ProxyCommand} {
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("SSH config values cannot contain newlines")
		}
	}
	for _, value := range spec.IdentityFiles {
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("SSH config values cannot contain newlines")
		}
	}
	return nil
}

func hostExists(configPath, alias string) (bool, error) {
	discovery, err := DiscoverHosts(configPath)
	if err != nil {
		return false, err
	}
	for _, entry := range discovery.Entries {
		if strings.EqualFold(entry.Alias, alias) {
			return true, nil
		}
	}
	return false, nil
}

func firstConcretePattern(patterns []string) string {
	for _, pattern := range patterns {
		if isConcreteHostPattern(pattern) {
			return pattern
		}
	}
	return ""
}
