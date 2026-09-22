package netcontrol

import (
	"testing"

	"github.com/narvel/nymeria/internal/netprofile"
)

// TestProfileCheckInCategoriesAreValid guards the netprofile registry from
// the netcontrol side: netprofile cannot import netcontrol (netcontrol
// imports netprofile), so this fence lives here instead.
func TestProfileCheckInCategoriesAreValid(t *testing.T) {
	for _, p := range netprofile.All() {
		for _, cat := range p.CheckInCategories {
			if !ValidCategories[cat] {
				t.Errorf("profile %q: check-in category %q is not a valid netcontrol category", p.ID, cat)
			}
		}
		if !ValidCategories[p.DefaultCheckInCategory] {
			t.Errorf("profile %q: DefaultCheckInCategory %q is not a valid netcontrol category", p.ID, p.DefaultCheckInCategory)
		}
	}
}

// TestARRLLadderIDsAreTrafficConstants pins netprofile's "arrl" ladder ids to
// the literal netcontrol.Traffic* constant values.
func TestARRLLadderIDsAreTrafficConstants(t *testing.T) {
	tiers := netprofile.LadderTiers(netprofile.LadderARRL)
	want := map[string]bool{
		TrafficEmergency: true,
		TrafficPriority:  true,
		TrafficWelfare:   true,
		TrafficRoutine:   true,
	}
	if len(tiers) != len(want) {
		t.Fatalf("arrl ladder len = %d, want %d", len(tiers), len(want))
	}
	for _, tier := range tiers {
		if !want[tier.ID] {
			t.Errorf("arrl ladder id %q is not a netcontrol.Traffic* constant", tier.ID)
		}
	}
}
