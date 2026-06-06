package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"pinkssh/internal/pinkssh"
)

type searchState struct {
	input    textinput.Model
	matches  []int
	selected int
	offset   int
}

func newSearchState(hosts []pinkssh.Host, selectedHost int) *searchState {
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "search"
	input.CharLimit = 256
	input.PromptStyle = subtleStyle
	input.TextStyle = selectedStyle
	input.Cursor.Style = selectedStyle
	input.Focus()

	search := &searchState{input: input}
	search.updateMatches(hosts, selectedHost)
	return search
}

func (s *searchState) updateMatches(hosts []pinkssh.Host, preserveHost int) {
	s.matches = fuzzyHostMatches(hosts, s.input.Value())
	s.selected = 0
	s.offset = 0

	if preserveHost >= 0 {
		for i, index := range s.matches {
			if index == preserveHost {
				s.selected = i
				return
			}
		}
	}
	s.clampSelection()
}

func (s *searchState) currentHostIndex() (int, bool) {
	if s == nil || len(s.matches) == 0 || s.selected < 0 || s.selected >= len(s.matches) {
		return 0, false
	}
	return s.matches[s.selected], true
}

func (s *searchState) clampSelection() {
	if len(s.matches) == 0 {
		s.selected = 0
		s.offset = 0
		return
	}
	if s.selected >= len(s.matches) {
		s.selected = len(s.matches) - 1
	}
	if s.selected < 0 {
		s.selected = 0
	}
}

func (s *searchState) keepSelectionVisible(rows int) {
	s.clampSelection()
	if rows < 1 {
		rows = 1
	}
	if s.selected < s.offset {
		s.offset = s.selected
	}
	if s.selected >= s.offset+rows {
		s.offset = s.selected - rows + 1
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

func (m model) tableHosts() []pinkssh.Host {
	if m.search == nil {
		return m.hosts
	}
	hosts := make([]pinkssh.Host, 0, len(m.search.matches))
	for _, index := range m.search.matches {
		if index >= 0 && index < len(m.hosts) {
			hosts = append(hosts, m.hosts[index])
		}
	}
	return hosts
}

type fuzzyResult struct {
	index int
	score int
}

func fuzzyHostMatches(hosts []pinkssh.Host, query string) []int {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(terms) == 0 {
		matches := make([]int, len(hosts))
		for i := range hosts {
			matches[i] = i
		}
		return matches
	}

	var results []fuzzyResult
	for i, host := range hosts {
		score, ok := fuzzyHostScore(host, terms)
		if !ok {
			continue
		}
		results = append(results, fuzzyResult{index: i, score: score})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].index < results[j].index
		}
		return results[i].score > results[j].score
	})

	matches := make([]int, len(results))
	for i, result := range results {
		matches[i] = result.index
	}
	return matches
}

func fuzzyHostScore(host pinkssh.Host, terms []string) (int, bool) {
	alias := strings.ToLower(host.Alias)
	address := strings.ToLower(host.Address())
	targets := []string{alias, address, alias + " " + address}

	total := 0
	for _, term := range terms {
		bestScore := 0
		matched := false
		for _, target := range targets {
			score, ok := fuzzyScore(term, target)
			if !ok {
				continue
			}
			if !matched || score > bestScore {
				bestScore = score
			}
			matched = true
		}
		if !matched {
			return 0, false
		}
		total += bestScore
	}
	return total, true
}

func fuzzyScore(query, target string) (int, bool) {
	if query == "" {
		return 0, true
	}
	if target == "" {
		return 0, false
	}

	queryRunes := []rune(query)
	targetRunes := []rune(target)
	position := 0
	lastMatch := -2
	score := 0

	for queryIndex, queryRune := range queryRunes {
		match := -1
		for i := position; i < len(targetRunes); i++ {
			if targetRunes[i] == queryRune {
				match = i
				break
			}
		}
		if match < 0 {
			return 0, false
		}

		score += 12
		if match == lastMatch+1 {
			score += 10
		} else if lastMatch >= 0 {
			score -= min(match-lastMatch-1, 8)
		}
		if isSearchBoundary(targetRunes, match) {
			score += 8
		}
		if match == queryIndex {
			score += 3
		}

		position = match + 1
		lastMatch = match
	}

	if strings.HasPrefix(target, query) {
		score += 40
	}
	if strings.Contains(target, query) {
		score += 20
	}
	score -= len(targetRunes) / 4
	return score, true
}

func isSearchBoundary(value []rune, index int) bool {
	if index <= 0 {
		return true
	}
	switch value[index-1] {
	case ' ', '-', '_', ':', '/', '.', '@':
		return true
	default:
		return false
	}
}
