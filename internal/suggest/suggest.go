// Package suggest offers "did you mean" candidates for a mistyped value.
package suggest

import (
	"sort"
	"strings"
	"unicode"
)

// Normalize lowercases and collapses internal whitespace, so "small  tent" and
// "Small Tent" compare equal.
func Normalize(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), unicode.IsSpace), " ")
}

type scored struct {
	value string
	rank  int // lower is better
	dist  int
}

// Closest returns candidates similar to input, best first.
//
// Substring containment is checked before edit distance on purpose: the
// recreation.gov vocabulary is full of multi-word names built around a shorter
// one ("Tent" inside "Small Tent" and "Large Tent Over 9X12`"), and those are
// far more useful suggestions than anything edit distance would surface for a
// short input.
func Closest(input string, candidates []string, limit int) []string {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	in := Normalize(input)
	if in == "" {
		return nil
	}

	maxDist := len(in) / 3
	if maxDist < 2 {
		maxDist = 2
	}

	var out []scored
	for _, c := range candidates {
		n := Normalize(c)
		switch {
		case n == in:
			out = append(out, scored{c, 0, 0})
		case strings.Contains(n, in) || strings.Contains(in, n):
			out = append(out, scored{c, 1, abs(len(n) - len(in))})
		default:
			if d := levenshtein(in, n); d <= maxDist {
				out = append(out, scored{c, 2, d})
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		if out[i].dist != out[j].dist {
			return out[i].dist < out[j].dist
		}
		return out[i].value < out[j].value
	})

	var names []string
	for _, s := range out {
		if len(names) == limit {
			break
		}
		names = append(names, s.value)
	}
	return names
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// levenshtein is the standard edit distance, two rows at a time. Cobra has one
// but does not export it.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}

	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
