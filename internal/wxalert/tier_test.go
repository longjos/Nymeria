package wxalert

import (
	"strings"
	"testing"
)

// TestTierForEvent covers all 111 canonical names plus the trap rows from
// spec-tests.md §2.1/§2.2. The suffix rule decides the tier; the override
// table only kicks in for names with no tier suffix (or where the suffix
// would mislead, per the traps below).
func TestTierForEvent(t *testing.T) {
	type row struct {
		name  string
		event string
		want  Tier
		known bool
	}
	var tests []row

	warning := []string{
		"Ashfall Warning", "Avalanche Warning", "Blizzard Warning", "Blowing Dust Warning",
		"Civil Danger Warning", "Coastal Flood Warning", "Dust Storm Warning", "Earthquake Warning",
		"Extreme Heat Warning", "Extreme Cold Warning", "Extreme Wind Warning", "Fire Warning",
		"Flash Flood Warning", "Flood Warning", "Freeze Warning", "Gale Warning",
		"Hazardous Materials Warning", "Hazardous Seas Warning", "Heavy Freezing Spray Warning",
		"High Surf Warning", "High Wind Warning", "Hurricane Force Wind Warning", "Hurricane Warning",
		"Ice Storm Warning", "Lake Effect Snow Warning", "Lakeshore Flood Warning",
		"Law Enforcement Warning", "Nuclear Power Plant Warning", "Radiological Hazard Warning",
		"Red Flag Warning", "Severe Thunderstorm Warning", "Shelter In Place Warning",
		"Snow Squall Warning", "Special Marine Warning", "Storm Surge Warning", "Storm Warning",
		"Tornado Warning", "Tropical Storm Warning", "Tsunami Warning", "Typhoon Warning",
		"Volcano Warning", "Winter Storm Warning",
		"Child Abduction Emergency", "Civil Emergency Message", "Evacuation Immediate",
		"Extreme Fire Danger", "Local Area Emergency", "Blue Alert",
	}
	watch := []string{
		"Avalanche Watch", "Coastal Flood Watch", "Extreme Heat Watch", "Extreme Cold Watch",
		"Fire Weather Watch", "Flash Flood Watch", "Flood Watch", "Freeze Watch", "Gale Watch",
		"Hazardous Seas Watch", "Heavy Freezing Spray Watch", "High Wind Watch",
		"Hurricane Force Wind Watch", "Hurricane Watch", "Lakeshore Flood Watch",
		"Severe Thunderstorm Watch", "Storm Surge Watch", "Storm Watch", "Tornado Watch",
		"Tropical Storm Watch", "Tsunami Watch", "Typhoon Watch", "Winter Storm Watch",
	}
	advisory := []string{
		"Air Stagnation Advisory", "Ashfall Advisory", "Avalanche Advisory", "Blowing Dust Advisory",
		"Brisk Wind Advisory", "Coastal Flood Advisory", "Cold Weather Advisory", "Dense Fog Advisory",
		"Dense Smoke Advisory", "Dust Advisory", "Flood Advisory", "Freezing Fog Advisory",
		"Freezing Spray Advisory", "Frost Advisory", "Heat Advisory", "High Surf Advisory",
		"Lake Wind Advisory", "Lakeshore Flood Advisory", "Low Water Advisory",
		"Small Craft Advisory", "Tsunami Advisory", "Wind Advisory", "Winter Weather Advisory",
		"Air Quality Alert",
	}
	statement := []string{
		"Beach Hazards Statement", "Coastal Flood Statement", "Flash Flood Statement",
		"Flood Statement", "Lakeshore Flood Statement", "Marine Weather Statement",
		"Rip Current Statement", "Severe Weather Statement", "Special Weather Statement",
		"Tropical Cyclone Local Statement",
		"911 Telephone Outage", "Administrative Message", "Hazardous Weather Outlook",
		"Hydrologic Outlook", "Short Term Forecast", "Test",
	}

	for _, e := range warning {
		tests = append(tests, row{e, e, TierWarning, true})
	}
	for _, e := range watch {
		tests = append(tests, row{e, e, TierWatch, true})
	}
	for _, e := range advisory {
		tests = append(tests, row{e, e, TierAdvisory, true})
	}
	for _, e := range statement {
		tests = append(tests, row{e, e, TierStatement, true})
	}

	if got := len(tests); got != 111 {
		t.Fatalf("built %d canonical rows, want 111", got)
	}

	// The canonical set must exactly equal KnownEvents() so adding a name to
	// one side without the other fails.
	known := KnownEvents()
	if len(known) != len(tests) {
		t.Fatalf("KnownEvents() has %d entries, want %d", len(known), len(tests))
	}
	wantSet := map[string]bool{}
	for _, tt := range tests {
		wantSet[tt.event] = true
	}
	for _, e := range known {
		if !wantSet[e] {
			t.Errorf("KnownEvents() contains %q which is not in the test table", e)
		}
	}
	for e := range wantSet {
		found := false
		for _, k := range known {
			if k == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("test table contains %q which KnownEvents() does not", e)
		}
	}

	// Trap rows (§2.2).
	traps := []row{
		{"trap: Special Weather Statement", "Special Weather Statement", TierStatement, true},
		{"trap: Severe Thunderstorm Watch", "Severe Thunderstorm Watch", TierWatch, true},
		{"trap: Severe Thunderstorm Warning", "Severe Thunderstorm Warning", TierWarning, true},
		{"trap: Severe Weather Statement", "Severe Weather Statement", TierStatement, true},
		{"trap: Flash Flood Statement", "Flash Flood Statement", TierStatement, true},
		{"trap: Red Flag Warning", "Red Flag Warning", TierWarning, true},
		{"trap: Air Quality Alert", "Air Quality Alert", TierAdvisory, true},
		{"trap: Small Craft Advisory", "Small Craft Advisory", TierAdvisory, true},
		{"trap: Child Abduction Emergency", "Child Abduction Emergency", TierWarning, true},
		{"trap: 911 Telephone Outage", "911 Telephone Outage", TierStatement, true},
		{"trap: Extreme Fire Danger", "Extreme Fire Danger", TierWarning, true},
		{"trap: Blue Alert", "Blue Alert", TierWarning, true},
		{"trap: Test", "Test", TierStatement, true},
		{"trap: trailing space", "Tornado Warning ", TierWarning, true},
		{"trap: lowercase", "tornado warning", TierWarning, true},
		{"trap: unknown with Warning suffix", "Frog Rain Warning", TierWarning, false},
		{"trap: unknown no suffix", "Solar Flare Notice", TierStatement, false},
		{"trap: empty", "", TierStatement, false},
	}
	tests = append(tests, traps...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTier, gotKnown := TierForEvent(tt.event)
			if gotTier != tt.want {
				t.Errorf("TierForEvent(%q) tier = %v, want %v", tt.event, gotTier, tt.want)
			}
			if gotKnown != tt.known {
				t.Errorf("TierForEvent(%q) known = %v, want %v", tt.event, gotKnown, tt.known)
			}
		})
	}

	// Suffix decides, not a shared prefix: the two Severe Thunderstorm rows
	// must differ.
	watchTier, _ := TierForEvent("Severe Thunderstorm Watch")
	warnTier, _ := TierForEvent("Severe Thunderstorm Warning")
	if watchTier == warnTier {
		t.Errorf("Severe Thunderstorm Watch and Warning resolved to the same tier %v", watchTier)
	}
}

func TestEffectiveEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    string
		severity Severity
		params   map[string][]string
		wantEvt  string
		wantSev  Severity
	}{
		{
			name: "FFW catastrophic escalates", event: "Flash Flood Warning", severity: SeveritySevere,
			params:  map[string][]string{"flashFloodDamageThreat": {"CATASTROPHIC"}},
			wantEvt: "Flash Flood Emergency", wantSev: SeverityExtreme,
		},
		{
			name: "FFW considerable unchanged", event: "Flash Flood Warning", severity: SeveritySevere,
			params:  map[string][]string{"flashFloodDamageThreat": {"CONSIDERABLE"}},
			wantEvt: "Flash Flood Warning", wantSev: SeveritySevere,
		},
		{
			name: "FFW no parameter", event: "Flash Flood Warning", severity: SeveritySevere,
			wantEvt: "Flash Flood Warning", wantSev: SeveritySevere,
		},
		{
			name: "TOR catastrophic escalates", event: "Tornado Warning", severity: SeverityExtreme,
			params:  map[string][]string{"tornadoDamageThreat": {"CATASTROPHIC"}},
			wantEvt: "Tornado Emergency", wantSev: SeverityExtreme,
		},
		{
			name: "TOR considerable unchanged", event: "Tornado Warning", severity: SeverityExtreme,
			params:  map[string][]string{"tornadoDamageThreat": {"CONSIDERABLE"}},
			wantEvt: "Tornado Warning", wantSev: SeverityExtreme,
		},
		{
			name: "TOR catastrophic lowercase still matches", event: "Tornado Warning", severity: SeverityExtreme,
			params:  map[string][]string{"tornadoDamageThreat": {"catastrophic"}},
			wantEvt: "Tornado Emergency", wantSev: SeverityExtreme,
		},
		{
			name: "nonsense combo ignored", event: "Heat Advisory", severity: SeverityModerate,
			params:  map[string][]string{"flashFloodDamageThreat": {"CATASTROPHIC"}},
			wantEvt: "Heat Advisory", wantSev: SeverityModerate,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Alert{Event: tt.event, Severity: tt.severity, Parameters: tt.params}
			gotEvt, gotSev := EffectiveEvent(a)
			if gotEvt != tt.wantEvt {
				t.Errorf("EffectiveEvent() event = %q, want %q", gotEvt, tt.wantEvt)
			}
			if gotSev != tt.wantSev {
				t.Errorf("EffectiveEvent() severity = %v, want %v", gotSev, tt.wantSev)
			}
		})
	}

	// Both derived names must be independently known to the tier table (so
	// the allowlist can name them) and resolve to Warning.
	for _, e := range []string{"Flash Flood Emergency", "Tornado Emergency"} {
		tier, known := TierForEvent(e)
		if !known || tier != TierWarning {
			t.Errorf("TierForEvent(%q) = %v, known=%v; want warning, known=true", e, tier, known)
		}
	}
}

func TestShortCode(t *testing.T) {
	tests := []struct {
		event string
		want  string
	}{
		{"Tornado Warning", "TOR WARN"},
		{"Tornado Emergency", "TOR EMERG"},
		{"Severe Thunderstorm Warning", "SVR WARN"},
		{"Severe Thunderstorm Watch", "SVR WATCH"},
		{"Flash Flood Warning", "FF WARN"},
		{"Flash Flood Emergency", "FF EMERG"},
		{"Flood Watch", "FLD WATCH"},
		{"Heat Advisory", "HEAT ADV"},
		{"Special Weather Statement", "SPS"},
		{"Small Craft Advisory", "SCA"},
		{"Air Quality Alert", "AIR QLTY"},
		{"Winter Weather Advisory", "WNTR WX ADV"},
		{"Extreme Wind Warning", "EXT WIND WARN"},
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			got := ShortCode(tt.event)
			if got != tt.want {
				t.Errorf("ShortCode(%q) = %q, want %q", tt.event, got, tt.want)
			}
			if len(got) > shortCodeMaxLen {
				t.Errorf("ShortCode(%q) = %q is %d chars, want <= %d", tt.event, got, len(got), shortCodeMaxLen)
			}
		})
	}

	t.Run("unknown event falls back to uppercase + suffix, truncated", func(t *testing.T) {
		got := ShortCode("Frog Rain Warning")
		if len(got) > shortCodeMaxLen {
			t.Fatalf("ShortCode(unknown) = %q is %d chars, want <= %d", got, len(got), shortCodeMaxLen)
		}
		if !strings.HasPrefix(got, "FROG RAIN") {
			t.Errorf("ShortCode(unknown) = %q, want prefix FROG RAIN", got)
		}
	})
}

func TestFloorTextConstant(t *testing.T) {
	want := "An Extreme-severity, Immediate-urgency alert inside the watch area always shows at least a toast. Nothing on this page can turn that off."
	if got := FloorText(); got != want {
		t.Errorf("FloorText() = %q, want %q", got, want)
	}
}
