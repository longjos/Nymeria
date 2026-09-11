package w3w

import "testing"

// vector is one row of the shared normalize/validate test table. The
// TypeScript port of these rules (web/src/lib/w3w.ts) tests against the same
// cases — keep the two in sync when changing either.
type vector struct {
	name       string
	input      string
	normalized string // expected Normalize(input); "" means "same as input, trimmed/lowered" — checked explicitly per case
	isFull     bool
	isPartial  bool
}

var vectors = []vector{
	{name: "plain", input: "filled.count.soap", normalized: "filled.count.soap", isFull: true, isPartial: false},
	{name: "triple slash", input: "///filled.count.soap", normalized: "filled.count.soap", isFull: true, isPartial: false},
	{name: "single slash", input: "/filled.count.soap", normalized: "filled.count.soap", isFull: true, isPartial: false},
	{name: "mixed case and spaces", input: "  Filled.Count.Soap  ", normalized: "filled.count.soap", isFull: true, isPartial: false},
	{name: "trailing dot", input: "filled.count.", normalized: "filled.count.", isFull: false, isPartial: true},
	{name: "two components", input: "filled.count", normalized: "filled.count", isFull: false, isPartial: true},
	{name: "one dot", input: "filled.", normalized: "filled.", isFull: false, isPartial: true},
	{name: "one word no dot", input: "filled", normalized: "filled", isFull: false, isPartial: false},
	{name: "four components", input: "filled.count.soap.extra", normalized: "filled.count.soap.extra", isFull: false, isPartial: false},
	{name: "digit in word", input: "filled.count.s0ap", normalized: "filled.count.s0ap", isFull: false, isPartial: false},
	{name: "space separated", input: "filled count soap", normalized: "filled count soap", isFull: false, isPartial: false},
	{name: "unicode accents", input: "déjà.vu.vraiment", normalized: "déjà.vu.vraiment", isFull: true, isPartial: false},
	{name: "cjk separator", input: "旅。行。者", normalized: "旅.行.者", isFull: true, isPartial: false},
	{name: "empty", input: "", normalized: "", isFull: false, isPartial: false},
	{name: "annotation label not a 3wa", input: "Aid Station 3", normalized: "aid station 3", isFull: false, isPartial: false},
	{name: "annotation label with decimal", input: "Mile 22.5", normalized: "mile 22.5", isFull: false, isPartial: false},
	{name: "annotation label dash", input: "CP-1", normalized: "cp-1", isFull: false, isPartial: false},
	{name: "annotation label dot no second word", input: "N. Trailhead", normalized: "n. trailhead", isFull: false, isPartial: false},
	{name: "coordinates not a 3wa", input: "41.8781, -87.6298", normalized: "41.8781, -87.6298", isFull: false, isPartial: false},
	// Normalize strips every leading slash, not just up to three (a caller
	// who fat-fingers a few extra slashes still gets a valid resolve) — see
	// the comment on Normalize. fullRe's own "/{0,3}" is therefore never
	// reachable through Normalize; this vector pins the actual contract.
	{name: "many leading slashes", input: "/////filled.count.soap", normalized: "filled.count.soap", isFull: true, isPartial: false},
}

func TestNormalize(t *testing.T) {
	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			got := Normalize(v.input)
			if got != v.normalized {
				t.Errorf("Normalize(%q) = %q, want %q", v.input, got, v.normalized)
			}
		})
	}
}

func TestIsFullAddress(t *testing.T) {
	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			got := IsFullAddress(v.input)
			if got != v.isFull {
				t.Errorf("IsFullAddress(%q) = %v, want %v", v.input, got, v.isFull)
			}
		})
	}
}

func TestLooksLikePartial(t *testing.T) {
	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			got := LooksLikePartial(v.input)
			if got != v.isPartial {
				t.Errorf("LooksLikePartial(%q) = %v, want %v", v.input, got, v.isPartial)
			}
		})
	}
}

// A full address must never also be reported as partial — the combobox uses
// this to decide whether to show suggestion rows or resolve directly.
func TestFullAndPartialAreMutuallyExclusive(t *testing.T) {
	for _, v := range vectors {
		if v.isFull && v.isPartial {
			t.Fatalf("vector %q: isFull and isPartial both true — contradicts mutual exclusivity", v.name)
		}
	}
}
