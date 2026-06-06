package pinkssh

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type HostEntry struct {
	Alias  string
	Source string
	Line   int
}

type Discovery struct {
	Entries []HostEntry
	Files   []string
}

func DiscoverHosts(configPath string) (Discovery, error) {
	if configPath == "" {
		configPath = filepath.Join(SSHDir(), "config")
	}

	var discovery Discovery
	state := discoveryState{
		seenFiles: map[string]bool{},
		seenHosts: map[string]bool{},
	}

	err := parseConfigFile(expandPath(configPath, ""), &discovery, &state)
	if errors.Is(err, os.ErrNotExist) {
		return discovery, nil
	}
	return discovery, err
}

type discoveryState struct {
	seenFiles map[string]bool
	seenHosts map[string]bool
}

func parseConfigFile(path string, discovery *Discovery, state *discoveryState) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	if state.seenFiles[abs] {
		return nil
	}
	state.seenFiles[abs] = true

	content, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	discovery.Files = append(discovery.Files, abs)

	baseDir := filepath.Dir(abs)
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		tokens, err := splitSSHWords(strings.TrimRight(line, "\r"))
		if err != nil {
			return err
		}
		if len(tokens) == 0 {
			continue
		}

		keyword := strings.ToLower(tokens[0])
		switch keyword {
		case "include":
			for _, include := range tokens[1:] {
				for _, match := range expandInclude(include, baseDir) {
					if err := parseConfigFile(match, discovery, state); err != nil && !errors.Is(err, os.ErrNotExist) {
						return err
					}
				}
			}
		case "host":
			for _, pattern := range tokens[1:] {
				if !isConcreteHostPattern(pattern) {
					continue
				}
				key := strings.ToLower(pattern)
				if state.seenHosts[key] {
					continue
				}
				state.seenHosts[key] = true
				discovery.Entries = append(discovery.Entries, HostEntry{
					Alias:  pattern,
					Source: abs,
					Line:   i + 1,
				})
			}
		}
	}
	return nil
}

func expandInclude(pattern, baseDir string) []string {
	pattern = expandPath(pattern, baseDir)
	matches, err := filepath.Glob(pattern)
	if err == nil && len(matches) > 0 {
		for i := range matches {
			matches[i] = filepath.Clean(matches[i])
		}
		return matches
	}
	return []string{pattern}
}

func isConcreteHostPattern(pattern string) bool {
	if pattern == "" || strings.HasPrefix(pattern, "!") {
		return false
	}
	return !strings.ContainsAny(pattern, "*?")
}

func splitSSHWords(line string) ([]string, error) {
	var tokens []string
	var b strings.Builder
	var quote rune
	escaped := false
	inToken := false

	flush := func() {
		if !inToken {
			return
		}
		tokens = append(tokens, b.String())
		b.Reset()
		inToken = false
	}

	for _, r := range line {
		if escaped {
			if quote == '"' && r != '"' && r != '\\' {
				b.WriteRune('\\')
			}
			b.WriteRune(r)
			escaped = false
			inToken = true
			continue
		}

		if quote != 0 {
			if r == quote {
				quote = 0
				inToken = true
				continue
			}
			if r == '\\' && quote == '"' {
				escaped = true
				inToken = true
				continue
			}
			b.WriteRune(r)
			inToken = true
			continue
		}

		switch {
		case r == '#':
			flush()
			return tokens, nil
		case r == '\'' || r == '"':
			quote = r
			inToken = true
		case r == '\\':
			escaped = true
			inToken = true
		case unicode.IsSpace(r):
			flush()
		default:
			b.WriteRune(r)
			inToken = true
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote in SSH config")
	}
	flush()
	return tokens, nil
}

func expandPath(path, baseDir string) string {
	path = filepath.FromSlash(os.ExpandEnv(path))
	home := HomeDir()
	if home != "" {
		switch {
		case path == "~":
			path = home
		case strings.HasPrefix(path, "~"+string(filepath.Separator)):
			path = filepath.Join(home, strings.TrimPrefix(path, "~"+string(filepath.Separator)))
		case strings.HasPrefix(path, "~/"):
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		case strings.HasPrefix(path, `~\`):
			path = filepath.Join(home, strings.TrimPrefix(path, `~\`))
		}
	}
	if baseDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Clean(path)
}

func WatchPaths(configPath string, parsedFiles []string) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		key := strings.ToLower(path)
		if seen[key] {
			return
		}
		seen[key] = true
		paths = append(paths, path)
	}

	if configPath != "" {
		add(expandPath(configPath, ""))
	}
	for _, file := range parsedFiles {
		add(file)
	}
	add(SSHDir())
	return paths
}
