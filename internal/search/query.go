package search

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const minTrigram = 3

type Query struct {
	Raw     string
	Words   []string
	Phrases []string
	Short   []string
	Exclude []string
	Folder  string
	After   time.Time
	Before  time.Time
	Type    string
}

func (q Query) HasText() bool {
	return len(q.Words) > 0 || len(q.Phrases) > 0 || len(q.Short) > 0
}

func (q Query) HasFilter() bool {
	return q.Folder != "" || !q.After.IsZero() || !q.Before.IsZero() || q.Type != ""
}

var ErrEmptyQuery = errors.New("empty query")

type QueryError struct {
	Err error
}

func (e *QueryError) Error() string { return e.Err.Error() }

func (e *QueryError) Unwrap() error { return e.Err }

func Parse(raw string, now time.Time) (Query, error) {
	q := Query{Raw: raw}
	tokens, err := tokenize(raw)
	if err != nil {
		return Query{}, &QueryError{err}
	}
	for _, tok := range tokens {
		if tok.negated {
			if tok.text != "" {
				q.Exclude = append(q.Exclude, tok.text)
			}
			continue
		}
		if colon := strings.IndexByte(tok.text, ':'); colon > 0 && (tok.quoteAt < 0 || colon < tok.quoteAt) {
			if key, value, ok := strings.Cut(tok.text, ":"); ok && value != "" {
				handled, err := q.applyFilter(strings.ToLower(key), value, now)
				if err != nil {
					return Query{}, &QueryError{err}
				}
				if handled {
					continue
				}
			}
		}
		q.addText(tok.text, tok.quoted)
	}
	if !q.HasText() && !q.HasFilter() {
		return Query{}, &QueryError{ErrEmptyQuery}
	}
	return q, nil
}

func (q *Query) addText(text string, quoted bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	switch {
	case utf8.RuneCountInString(text) < minTrigram:
		q.Short = append(q.Short, text)
	case quoted && strings.ContainsFunc(text, unicode.IsSpace):
		q.Phrases = append(q.Phrases, text)
	default:
		q.Words = append(q.Words, text)
	}
}

func (q *Query) applyFilter(key, value string, now time.Time) (bool, error) {
	switch key {
	case "folder":
		q.Folder = value
	case "type":
		q.Type = strings.ToLower(value)
	case "on":
		start, end, err := dateRange(value, now)
		if err != nil {
			return true, err
		}
		q.After, q.Before = start, end
	case "since":
		start, _, err := dateRange(value, now)
		if err != nil {
			return true, err
		}
		q.After = start
	case "until":
		start, _, err := dateRange(value, now)
		if err != nil {
			return true, err
		}
		q.Before = start
	default:
		return false, nil
	}
	return true, nil
}

type token struct {
	text    string
	quoted  bool
	negated bool
	quoteAt int
}

func tokenize(raw string) ([]token, error) {
	var tokens []token
	runes := []rune(raw)
	for i := 0; i < len(runes); {
		if unicode.IsSpace(runes[i]) {
			i++
			continue
		}
		tok := token{quoteAt: -1}
		if runes[i] == '-' && i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
			tok.negated = true
			i++
		}
		start := i
		var b strings.Builder
		for i < len(runes) && !unicode.IsSpace(runes[i]) {
			if runes[i] == '"' {
				end := indexRune(runes, '"', i+1)
				if end < 0 {
					return nil, fmt.Errorf("unclosed quote at position %d", i+1)
				}
				if !tok.quoted {
					tok.quoteAt = b.Len()
				}
				b.WriteString(string(runes[i+1 : end]))
				tok.quoted = true
				i = end + 1
				continue
			}
			b.WriteRune(runes[i])
			i++
		}
		if i == start {
			continue
		}
		tok.text = b.String()
		tokens = append(tokens, tok)
	}
	return tokens, nil
}

func indexRune(runes []rune, r rune, from int) int {
	for i := from; i < len(runes); i++ {
		if runes[i] == r {
			return i
		}
	}
	return -1
}

var months = map[string]time.Month{
	"jan": time.January, "january": time.January,
	"feb": time.February, "february": time.February,
	"mar": time.March, "march": time.March,
	"apr": time.April, "april": time.April,
	"may": time.May,
	"jun": time.June, "june": time.June,
	"jul": time.July, "july": time.July,
	"aug": time.August, "august": time.August,
	"sep": time.September, "sept": time.September, "september": time.September,
	"oct": time.October, "october": time.October,
	"nov": time.November, "november": time.November,
	"dec": time.December, "december": time.December,
}

func dateRange(value string, now time.Time) (time.Time, time.Time, error) {
	v := strings.ToLower(value)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch v {
	case "today":
		return day, day.AddDate(0, 0, 1), nil
	case "yesterday":
		return day.AddDate(0, 0, -1), day, nil
	case "week":
		return day.AddDate(0, 0, -6), day.AddDate(0, 0, 1), nil
	case "month":
		return day.AddDate(0, 0, -29), day.AddDate(0, 0, 1), nil
	case "year":
		return day.AddDate(0, 0, -364), day.AddDate(0, 0, 1), nil
	}
	if m, ok := months[v]; ok {
		year := now.Year()
		if m > now.Month() {
			year--
		}
		start := time.Date(year, m, 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 1, 0), nil
	}
	parts := strings.Split(v, "-")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("unknown date %q (try today, week, march, 2026, 2026-08 or 2026-08-01)", value)
		}
		nums[i] = n
	}
	switch len(nums) {
	case 1:
		start := time.Date(nums[0], 1, 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(1, 0, 0), nil
	case 2:
		if nums[1] < 1 || nums[1] > 12 {
			break
		}
		start := time.Date(nums[0], time.Month(nums[1]), 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 1, 0), nil
	case 3:
		start := time.Date(nums[0], time.Month(nums[1]), nums[2], 0, 0, 0, 0, now.Location())
		if start.Month() != time.Month(nums[1]) || start.Day() != nums[2] {
			break
		}
		return start, start.AddDate(0, 0, 1), nil
	}
	return time.Time{}, time.Time{}, fmt.Errorf("invalid date %q", value)
}
