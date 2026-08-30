package core

import "strings"

// NormalizeValue folds the spelling differences that a hand-maintained,
// operator-authored vocabulary accumulates.
//
// recreation.gov has a data-entry typo: the equipment name it actually returns
// is "Large Tent Over 9X12`", with a trailing backtick, on 105 of 1047 sampled
// sites. The clean spelling appears nowhere. camply carried both in its own
// vocabulary list and advertised the clean one in help and suggestions, so
// `--equipment-types "Large Tent Over 9X12"` matched nothing while the
// backticked spelling matched 133 records.
//
// Normalizing at the comparison makes the two one key, deletes the duplicate
// entry, and makes user input forgiving in the same stroke. It folds case,
// collapses internal whitespace and strips surrounding punctuation — it does not
// do substring matching, which for attribute names would sweep unrelated
// concepts together.
func NormalizeValue(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), " ")
	return strings.Trim(s, "`'\".,;:*-_")
}

// EqualValue reports whether two vocabulary values name the same thing.
func EqualValue(a, b string) bool { return NormalizeValue(a) == NormalizeValue(b) }
