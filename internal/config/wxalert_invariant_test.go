package config_test

// This file is deliberately package config_test (black-box), not config —
// config.go itself never imports internal/wxalert (it is a leaf package;
// see config.DefaultWxInterruptEvents's own doc comment), but a test
// binary importing both to cross-check a literal is not a real dependency
// and does not create an import cycle (wxalert never imports config).

import (
	"reflect"
	"testing"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/wxalert"
)

// TestDefaultWxInterruptEventsMatchesWxalertPackage is cross-package
// invariant #3 (BUILD-PLAN §6): config.DefaultConfig's seed allowlist must
// stay byte-for-byte identical, in order, to wxalert.DefaultInterruptEvents
// — the two are separate literals only because config cannot import
// wxalert, not because they are allowed to drift.
func TestDefaultWxInterruptEventsMatchesWxalertPackage(t *testing.T) {
	got := config.DefaultWxInterruptEvents
	want := wxalert.DefaultInterruptEvents
	if !reflect.DeepEqual(got, want) {
		t.Errorf("config.DefaultWxInterruptEvents = %v\nwxalert.DefaultInterruptEvents = %v", got, want)
	}
}

// TestWxFloorTextMatchesPolicyDoc pins the exact floor sentence (BUILD-PLAN
// §4.1) that server.computeWxEffectivePolicy serves verbatim as
// policy.floorText — this is the copy the settings UI and the per-net
// watch-area sheet are required to render unedited.
func TestWxFloorTextMatchesPolicyDoc(t *testing.T) {
	want := "An Extreme-severity, Immediate-urgency alert inside the watch area always shows at least a toast. Nothing on this page can turn that off."
	if got := wxalert.FloorText(); got != want {
		t.Errorf("wxalert.FloorText() = %q, want %q", got, want)
	}
}
