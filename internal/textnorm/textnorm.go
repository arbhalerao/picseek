package textnorm

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	Version     = "1"
	maxFragment = 3
)

var standalone = map[string]struct{}{}

func init() {
	for _, w := range strings.Fields(`a i an as at be by do go he hi if in is it me my no of oh ok on or so to up us we
		all and any are but can did for get got had has her him his how its may new not now off one our out own
		see the too two use was way who why yes you`) {
		standalone[w] = struct{}{}
	}
}

func Fold(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "rn", "m")
	var b strings.Builder
	b.Grow(len(s))
	prev := rune(-1)
	for _, r := range s {
		r = foldRune(r)
		if r == prev && !unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

func foldRune(r rune) rune {
	switch r {
	case '0':
		return 'o'
	case '1', 'i', '|', '!':
		return 'l'
	case '5':
		return 's'
	case '8':
		return 'b'
	}
	return r
}

func Compact(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = Fold(joinFragments(strings.Fields(line)))
	}
	return strings.Join(lines, "\n")
}

func joinFragments(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(tokens[0])
	for i := 1; i < len(tokens); i++ {
		if !joinable(tokens[i-1], tokens[i]) {
			b.WriteByte(' ')
		}
		b.WriteString(tokens[i])
	}
	return b.String()
}

func joinable(a, b string) bool {
	if !isWord(a) || !isWord(b) {
		return false
	}
	shortA, shortB := utf8.RuneCountInString(a) <= maxFragment, utf8.RuneCountInString(b) <= maxFragment
	if !shortA && !shortB {
		return false
	}
	if shortA && isStandalone(a) || shortB && isStandalone(b) {
		return false
	}
	return true
}

func isStandalone(w string) bool {
	_, ok := standalone[strings.ToLower(w)]
	return ok
}

func isWord(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}
