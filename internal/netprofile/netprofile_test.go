package netprofile

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

func TestRegistryShape(t *testing.T) {
	all := All()
	if len(all) != 2 {
		t.Fatalf("All() len = %d, want 2", len(all))
	}
	if all[0].ID != ProfileGeneral || all[1].ID != ProfileBikeRide {
		t.Fatalf("All() order = [%s, %s], want [general, bike-ride]", all[0].ID, all[1].ID)
	}

	for _, p := range all {
		if len(p.Panels) == 0 {
			t.Errorf("%s: Panels empty", p.ID)
		}
		if len(p.AnnotationCategories) == 0 {
			t.Errorf("%s: AnnotationCategories empty", p.ID)
		}
		if len(p.CheckInCategories) == 0 {
			t.Errorf("%s: CheckInCategories empty", p.ID)
		}
		if len(p.PriorityTiers) == 0 {
			t.Errorf("%s: PriorityTiers empty", p.ID)
		}
		found := false
		for _, c := range p.CheckInCategories {
			if c == p.DefaultCheckInCategory {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: DefaultCheckInCategory %q not in CheckInCategories %v", p.ID, p.DefaultCheckInCategory, p.CheckInCategories)
		}
	}

	general, _ := Get(ProfileGeneral)
	if general.PriorityLadderID != LadderARRL {
		t.Errorf("general.PriorityLadderID = %q, want %q", general.PriorityLadderID, LadderARRL)
	}
	if general.HasRideConfig {
		t.Error("general.HasRideConfig = true, want false")
	}

	bikeRide, _ := Get(ProfileBikeRide)
	if bikeRide.PriorityLadderID != LadderMarinARS {
		t.Errorf("bike-ride.PriorityLadderID = %q, want %q", bikeRide.PriorityLadderID, LadderMarinARS)
	}
	if !bikeRide.HasRideConfig {
		t.Error("bike-ride.HasRideConfig = false, want true")
	}
}

func TestGetReturnsCopy(t *testing.T) {
	first, ok := Get(ProfileGeneral)
	if !ok {
		t.Fatal("Get(general) not found")
	}
	if len(first.Panels) == 0 {
		t.Fatal("Panels empty")
	}
	first.Panels[0] = "MUTATED"

	second, ok := Get(ProfileGeneral)
	if !ok {
		t.Fatal("Get(general) not found (second)")
	}
	if second.Panels[0] == "MUTATED" {
		t.Error("mutating Get() result affected the registry")
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "general"},
		{"General ", "general"},
		{"BIKE-RIDE", "bike-ride"},
		{"sar", "sar"},
	}
	for _, tt := range tests {
		if got := Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"general", true},
		{"bike-ride", true},
		{"", false},
		{"Bike-Ride", false},
		{"sar", false},
	}
	for _, tt := range tests {
		if got := Valid(tt.in); got != tt.want {
			t.Errorf("Valid(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestProfileGeneralLiteralMatchesStore(t *testing.T) {
	if ProfileGeneral != "general" {
		t.Errorf("ProfileGeneral = %q, want %q (store.go hardcodes this literal)", ProfileGeneral, "general")
	}
}

func TestMarinLadderLoadBearingDistinctions(t *testing.T) {
	tiers := LadderTiers(LadderMarinARS)
	if len(tiers) != 5 {
		t.Fatalf("marin ladder len = %d, want 5", len(tiers))
	}
	wantIDs := []string{"emergency", "priority", "high", "medium", "low"}
	byID := map[string]store.PriorityTier{}
	for i, tier := range tiers {
		if tier.ID != wantIDs[i] {
			t.Errorf("tiers[%d].ID = %q, want %q", i, tier.ID, wantIDs[i])
		}
		if tier.Rank != i+1 {
			t.Errorf("tiers[%d].Rank = %d, want %d", i, tier.Rank, i+1)
		}
		byID[tier.ID] = tier
		if len(tier.Examples) == 0 {
			t.Errorf("tier %q has no examples", tier.ID)
		}
		if tier.Description == "" {
			t.Errorf("tier %q has no description", tier.ID)
		}
	}

	if !anyContains(byID["high"].Examples, "OUT of water") {
		t.Error(`"high" tier missing an "OUT of water" example`)
	}
	if !anyContains(byID["medium"].Examples, "RUNNING LOW") {
		t.Error(`"medium" tier missing a "RUNNING LOW" example`)
	}
	if !anyContains(byID["high"].Examples, "FROM THE COURSE") {
		t.Error(`"high" tier missing a "FROM THE COURSE" example`)
	}
	if !anyContains(byID["medium"].Examples, "FROM A CHECKPOINT") {
		t.Error(`"medium" tier missing a "FROM A CHECKPOINT" example`)
	}
}

func anyContains(examples []string, substr string) bool {
	for _, e := range examples {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func TestARRLLadderMatchesNetcontrolConstants(t *testing.T) {
	tiers := LadderTiers(LadderARRL)
	want := []string{"emergency", "priority", "welfare", "routine"}
	if len(tiers) != len(want) {
		t.Fatalf("arrl ladder len = %d, want %d", len(tiers), len(want))
	}
	for i, tier := range tiers {
		if tier.ID != want[i] {
			t.Errorf("tiers[%d].ID = %q, want %q", i, tier.ID, want[i])
		}
	}
}

func TestEffectiveTiers(t *testing.T) {
	custom := []store.PriorityTier{{ID: "only", Label: "Only", Rank: 1}}

	tests := []struct {
		name     string
		profile  string
		override []store.PriorityTier
		wantLen  int
		wantID0  string
	}{
		{"general nil override", ProfileGeneral, nil, 4, "emergency"},
		{"bike-ride nil override", ProfileBikeRide, nil, 5, "emergency"},
		{"bike-ride empty override", ProfileBikeRide, []store.PriorityTier{}, 5, "emergency"},
		{"bike-ride custom override", ProfileBikeRide, custom, 1, "only"},
		{"unknown profile", "unknown", nil, 4, "emergency"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveTiers(tt.profile, tt.override)
			if got == nil {
				t.Fatal("EffectiveTiers returned nil")
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tt.wantLen)
			}
			if got[0].ID != tt.wantID0 {
				t.Errorf("got[0].ID = %q, want %q", got[0].ID, tt.wantID0)
			}
		})
	}

	// Copy semantics: mutating the result must not affect the source slice.
	src := []store.PriorityTier{{ID: "a", Label: "A", Rank: 1, Examples: []string{"x"}}}
	result := EffectiveTiers(ProfileBikeRide, src)
	result[0].ID = "mutated"
	result[0].Examples[0] = "mutated"
	if src[0].ID == "mutated" {
		t.Error("EffectiveTiers did not copy the override slice (ID)")
	}
	if src[0].Examples[0] == "mutated" {
		t.Error("EffectiveTiers did not deep-copy Examples")
	}
}

func TestValidateTiers(t *testing.T) {
	valid5 := []store.PriorityTier{
		{ID: "emergency", Label: "Emergency", Rank: 1},
		{ID: "priority", Label: "Priority", Rank: 2},
		{ID: "high", Label: "High", Rank: 3},
		{ID: "medium", Label: "Medium", Rank: 4},
		{ID: "low", Label: "Low", Rank: 5},
	}

	nineTiers := make([]store.PriorityTier, 9)
	for i := range nineTiers {
		nineTiers[i] = store.PriorityTier{ID: "t" + string(rune('a'+i)), Label: "T", Rank: i + 1}
	}

	tests := []struct {
		name    string
		in      []store.PriorityTier
		wantSub string
	}{
		{"empty", nil, ""},
		{"valid 5", valid5, ""},
		{"9 tiers", nineTiers, "at most 8"},
		{"id capitalized", []store.PriorityTier{{ID: "High", Label: "H", Rank: 1}}, "lowercase slug"},
		{"id empty", []store.PriorityTier{{ID: "", Label: "H", Rank: 1}}, "lowercase slug"},
		{"id too long", []store.PriorityTier{{ID: "a-very-long-id-over-24-characters-x", Label: "H", Rank: 1}}, "lowercase slug"},
		{"duplicate id", []store.PriorityTier{
			{ID: "a", Label: "A", Rank: 1}, {ID: "a", Label: "A2", Rank: 2},
		}, "duplicate tier id"},
		{"ranks 1,2,2", []store.PriorityTier{
			{ID: "a", Label: "A", Rank: 1}, {ID: "b", Label: "B", Rank: 2}, {ID: "c", Label: "C", Rank: 2},
		}, "no gaps or repeats"},
		{"ranks 1,3", []store.PriorityTier{
			{ID: "a", Label: "A", Rank: 1}, {ID: "b", Label: "B", Rank: 3},
		}, "no gaps or repeats"},
		{"ranks 0,1", []store.PriorityTier{
			{ID: "a", Label: "A", Rank: 0}, {ID: "b", Label: "B", Rank: 1},
		}, "no gaps or repeats"},
		{"label empty", []store.PriorityTier{{ID: "a", Label: "", Rank: 1}}, "needs a label"},
		{"ranks out of order but complete", []store.PriorityTier{
			{ID: "a", Label: "A", Rank: 2}, {ID: "b", Label: "B", Rank: 1},
		}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTiers(tt.in)
			if tt.wantSub == "" {
				if err != nil {
					t.Errorf("ValidateTiers() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateTiers() = nil, want error containing %q", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("ValidateTiers() = %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func TestNormalizeRideConfig(t *testing.T) {
	c := store.NetRideConfig{
		AgencyName:    "  Marin Cyclists  ",
		Routes:        nil,
		PriorityTiers: nil,
	}
	NormalizeRideConfig(&c)
	if c.Routes == nil {
		t.Error("Routes = nil, want empty slice")
	}
	if c.PriorityTiers == nil {
		t.Error("PriorityTiers = nil, want empty slice")
	}
	if c.AgencyName != "Marin Cyclists" {
		t.Errorf("AgencyName = %q, want trimmed", c.AgencyName)
	}

	c2 := store.NetRideConfig{
		Routes: []store.RideRoute{
			{ID: " 100 ", Name: "100"},
			{ID: "Metric", Name: "Metric"},
		},
		PriorityTiers: []store.PriorityTier{
			{ID: "a", Label: "A", Examples: nil},
		},
	}
	NormalizeRideConfig(&c2)
	if c2.Routes[0].ID != "100" {
		t.Errorf("Routes[0].ID = %q, want %q", c2.Routes[0].ID, "100")
	}
	if c2.Routes[1].ID != "metric" {
		t.Errorf("Routes[1].ID = %q, want %q", c2.Routes[1].ID, "metric")
	}
	if c2.PriorityTiers[0].Examples == nil {
		t.Error("PriorityTiers[0].Examples = nil, want empty slice")
	}
}

func TestValidateRideConfig(t *testing.T) {
	fourRoutes := func() []store.RideRoute {
		return []store.RideRoute{
			{ID: "100", Name: "100 Mile", DistanceMiles: 100},
			{ID: "75", Name: "75 Mile", DistanceMiles: 75},
			{ID: "55", Name: "55 Mile", DistanceMiles: 55},
			{ID: "48", Name: "48 Mile", DistanceMiles: 48},
		}
	}

	tests := []struct {
		name    string
		build   func() store.NetRideConfig
		wantSub string
	}{
		{"default config", func() store.NetRideConfig { return DefaultRideConfig("n1") }, ""},
		{"4 routes", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", Routes: fourRoutes()}
		}, ""},
		{"13 routes", func() store.NetRideConfig {
			routes := make([]store.RideRoute, 13)
			for i := range routes {
				routes[i] = store.RideRoute{ID: "r" + string(rune('a'+i)), Name: "R", DistanceMiles: 10}
			}
			return store.NetRideConfig{NetID: "n1", Routes: routes}
		}, "at most 12 routes"},
		{"distance zero", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", Routes: []store.RideRoute{{ID: "a", Name: "A", DistanceMiles: 0}}}
		}, "distance"},
		{"distance negative", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", Routes: []store.RideRoute{{ID: "a", Name: "A", DistanceMiles: -5}}}
		}, "distance"},
		{"distance too large", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", Routes: []store.RideRoute{{ID: "a", Name: "A", DistanceMiles: 1001}}}
		}, "distance"},
		{"duplicate route id", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", Routes: []store.RideRoute{
				{ID: "a", Name: "A", DistanceMiles: 10}, {ID: "a", Name: "A2", DistanceMiles: 20},
			}}
		}, "duplicate route id"},
		{"route name empty", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", Routes: []store.RideRoute{{ID: "a", Name: "", DistanceMiles: 10}}}
		}, "needs a name"},
		{"route id with space", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", Routes: []store.RideRoute{{ID: "100 Mile", Name: "A", DistanceMiles: 10}}}
		}, "lowercase slug"},
		{"eventDate bad format", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", EventDate: "2026-6-1"}
		}, "eventDate"},
		{"eventDate good format", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", EventDate: "2026-06-13"}
		}, ""},
		{"opens after closes", func() store.NetRideConfig {
			opens := mustTime("2026-06-13T10:00:00Z")
			closes := mustTime("2026-06-13T09:00:00Z")
			return store.NetRideConfig{NetID: "n1", Cutoff: store.RideCutoffPolicy{CourseOpensAt: &opens, CourseClosesAt: &closes}}
		}, "courseOpensAt must be before"},
		{"route cutoff after course close", func() store.NetRideConfig {
			closes := mustTime("2026-06-13T18:00:00Z")
			routeCutoff := mustTime("2026-06-13T19:00:00Z")
			return store.NetRideConfig{
				NetID:  "n1",
				Routes: []store.RideRoute{{ID: "a", Name: "A", DistanceMiles: 10, CutoffAt: &routeCutoff}},
				Cutoff: store.RideCutoffPolicy{CourseClosesAt: &closes},
			}
		}, "cutoffAt"},
		{"invalid tiers propagate", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", PriorityTiers: []store.PriorityTier{{ID: "a", Label: "A", Rank: 1}, {ID: "a", Label: "A2", Rank: 2}}}
		}, "duplicate tier id"},
		{"agencyName too long", func() store.NetRideConfig {
			return store.NetRideConfig{NetID: "n1", AgencyName: strings.Repeat("x", 121)}
		}, "agencyName"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.build()
			err := ValidateRideConfig(&cfg)
			if tt.wantSub == "" {
				if err != nil {
					t.Errorf("ValidateRideConfig() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateRideConfig() = nil, want error containing %q", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("ValidateRideConfig() = %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func mustTime(s string) time.Time {
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestDefaultRideConfig(t *testing.T) {
	cfg := DefaultRideConfig("net-42")
	if cfg.NetID != "net-42" {
		t.Errorf("NetID = %q, want net-42", cfg.NetID)
	}
	if cfg.Routes == nil || len(cfg.Routes) != 0 {
		t.Errorf("Routes = %v, want empty non-nil slice", cfg.Routes)
	}
	if cfg.PriorityTiers == nil || len(cfg.PriorityTiers) != 0 {
		t.Errorf("PriorityTiers = %v, want empty non-nil slice", cfg.PriorityTiers)
	}
	if !cfg.Cutoff.DeclinedSAGIsUnsupported {
		t.Error("Cutoff.DeclinedSAGIsUnsupported = false, want true")
	}
	if cfg.Cutoff.MandatorySAGAfterCutoff {
		t.Error("Cutoff.MandatorySAGAfterCutoff = true, want false")
	}
	if cfg.WithholdBibOnSevereInjury {
		t.Error("WithholdBibOnSevereInjury = true, want false")
	}
}

func TestJSONShape(t *testing.T) {
	cfg := DefaultRideConfig("net-1")
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"routes":[]`) {
		t.Errorf("missing routes:[], got %s", s)
	}
	if !strings.Contains(s, `"priorityTiers":[]`) {
		t.Errorf("missing priorityTiers:[], got %s", s)
	}
	if strings.Contains(s, `"division"`) {
		t.Errorf("division should be omitted when empty, got %s", s)
	}

	pb, err := json.Marshal(All()[1])
	if err != nil {
		t.Fatalf("Marshal profile: %v", err)
	}
	ps := string(pb)
	for _, key := range []string{`"priorityLadderId"`, `"hasRideConfig"`, `"checkInCategories"`} {
		if !strings.Contains(ps, key) {
			t.Errorf("profile JSON missing key %s, got %s", key, ps)
		}
	}
}
