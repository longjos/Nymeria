package w3w

import (
	"regexp"
	"strings"
)

// what3words addresses are three dot-separated components of Unicode letters
// (plus the apostrophe variants a few languages use). "///filled.count.soap"
// is the canonical display form, with any number of leading slashes
// tolerated on input — Normalize strips them all before either regex below
// ever runs, so neither one needs to (or can) match a slash itself.
var (
	// fullRe matches a syntactically complete three-word address, once
	// Normalize has already removed any leading slashes.
	fullRe = regexp.MustCompile(`^[\p{L}'’]{1,}[.][\p{L}'’]{1,}[.][\p{L}'’]{1,}$`)
	// partialRe matches an address on its way to being complete: at least
	// "a." typed, with an optional second and third component.
	partialRe = regexp.MustCompile(`^[\p{L}'’]{1,}[.][\p{L}'’]*([.][\p{L}'’]*)?$`)
)

// separatorReplacer converts the CJK what3words separators (。 and ・) to the
// ASCII '.' used everywhere else, so downstream matching only deals with one
// separator character.
var separatorReplacer = strings.NewReplacer("。", ".", "・", ".")

// Normalize lowercases, trims surrounding whitespace, strips every leading
// slash (not just up to three — a few fat-fingered extra slashes still
// resolve), and converts 。/・ separators to '.'. It does NOT validate —
// call IsFullAddress or LooksLikePartial for that.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "/")
	s = separatorReplacer.Replace(s)
	return strings.ToLower(s)
}

// IsFullAddress reports whether s (after normalization) is a syntactically
// complete three-word address. It does not verify the words actually
// resolve to a square — that requires a Resolve call.
func IsFullAddress(s string) bool {
	return fullRe.MatchString(Normalize(s))
}

// LooksLikePartial reports whether s is on its way to being a three-word
// address (i.e. worth calling Suggest for) but is not yet a complete one.
// A syntactically complete address is never also "partial".
func LooksLikePartial(s string) bool {
	n := Normalize(s)
	if fullRe.MatchString(n) {
		return false
	}
	return partialRe.MatchString(n)
}
