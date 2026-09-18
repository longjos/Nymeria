package wxalert

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		in   string
		want Severity
	}{
		{"Extreme", SeverityExtreme},
		{"Severe", SeveritySevere},
		{"Moderate", SeverityModerate},
		{"Minor", SeverityMinor},
		{"Unknown", SeverityUnknown},
		{"extreme", SeverityExtreme},
		{"", SeverityUnknown},
		{"Catastrophic", SeverityUnknown}, // not a CAP value
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ParseSeverity(tt.in); got != tt.want {
				t.Errorf("ParseSeverity(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseUrgency(t *testing.T) {
	tests := []struct {
		in   string
		want Urgency
	}{
		{"Immediate", UrgencyImmediate},
		{"Expected", UrgencyExpected},
		{"Future", UrgencyFuture},
		{"Past", UrgencyPast},
		{"Unknown", UrgencyUnknown},
		{"immediate", UrgencyImmediate},
		{"", UrgencyUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ParseUrgency(tt.in); got != tt.want {
				t.Errorf("ParseUrgency(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseCertainty(t *testing.T) {
	tests := []struct {
		in   string
		want Certainty
	}{
		{"Observed", CertaintyObserved},
		{"Likely", CertaintyLikely},
		{"Possible", CertaintyPossible},
		{"Unlikely", CertaintyUnlikely},
		{"", CertaintyUnknown},
	}
	for _, tt := range tests {
		if got := ParseCertainty(tt.in); got != tt.want {
			t.Errorf("ParseCertainty(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		in   string
		want Status
	}{
		{"Actual", StatusActual},
		{"actual", StatusActual},
		{"Exercise", StatusExercise},
		{"System", StatusSystem},
		{"Test", StatusTest},
		{"Draft", StatusDraft},
		{"", ""},
		{"bogus", ""},
	}
	for _, tt := range tests {
		if got := ParseStatus(tt.in); got != tt.want {
			t.Errorf("ParseStatus(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseMessageType(t *testing.T) {
	tests := []struct {
		in   string
		want MessageType
	}{
		{"Alert", MessageTypeAlert},
		{"Update", MessageTypeUpdate},
		{"Cancel", MessageTypeCancel},
		{"", ""},
	}
	for _, tt := range tests {
		if got := ParseMessageType(tt.in); got != tt.want {
			t.Errorf("ParseMessageType(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestRanksAreTotallyOrdered(t *testing.T) {
	if !(SeverityRank(SeverityExtreme) > SeverityRank(SeveritySevere) &&
		SeverityRank(SeveritySevere) > SeverityRank(SeverityModerate) &&
		SeverityRank(SeverityModerate) > SeverityRank(SeverityMinor) &&
		SeverityRank(SeverityMinor) > SeverityRank(SeverityUnknown)) {
		t.Errorf("SeverityRank is not strictly ordered Extreme>Severe>Moderate>Minor>Unknown")
	}
	if !(UrgencyRank(UrgencyImmediate) > UrgencyRank(UrgencyExpected) &&
		UrgencyRank(UrgencyExpected) > UrgencyRank(UrgencyFuture) &&
		UrgencyRank(UrgencyFuture) > UrgencyRank(UrgencyPast) &&
		UrgencyRank(UrgencyPast) > UrgencyRank(UrgencyUnknown)) {
		t.Errorf("UrgencyRank is not strictly ordered")
	}
	if !(TierRank(TierWarning) > TierRank(TierWatch) &&
		TierRank(TierWatch) > TierRank(TierAdvisory) &&
		TierRank(TierAdvisory) > TierRank(TierStatement)) {
		t.Errorf("TierRank is not strictly ordered")
	}
	if !(NotifyRank(NotifyInterrupt) > NotifyRank(NotifyToast) &&
		NotifyRank(NotifyToast) > NotifyRank(NotifyBadge) &&
		NotifyRank(NotifyBadge) > NotifyRank(NotifyPanel)) {
		t.Errorf("NotifyRank is not strictly ordered")
	}
}

func TestLouderQuieter(t *testing.T) {
	if got := Louder(NotifyBadge, NotifyToast); got != NotifyToast {
		t.Errorf("Louder(badge,toast) = %v, want toast", got)
	}
	if got := Quieter(NotifyBadge, NotifyToast); got != NotifyBadge {
		t.Errorf("Quieter(badge,toast) = %v, want badge", got)
	}
	if got := StepQuieter(NotifyInterrupt); got != NotifyToast {
		t.Errorf("StepQuieter(interrupt) = %v, want toast", got)
	}
	if got := StepQuieter(NotifyPanel); got != NotifyPanel {
		t.Errorf("StepQuieter(panel) = %v, want panel (floor)", got)
	}
}

func TestAlertEndTime(t *testing.T) {
	base := time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)
	t.Run("ends set", func(t *testing.T) {
		ends := base.Add(45 * time.Minute)
		a := Alert{Ends: &ends, Expires: base.Add(2 * time.Hour)}
		if !a.EndTime().Equal(ends) {
			t.Errorf("EndTime() = %v, want %v", a.EndTime(), ends)
		}
	})
	t.Run("falls back to eventEndingTime parameter", func(t *testing.T) {
		want := base.Add(90 * time.Minute)
		a := Alert{
			Expires:    base.Add(3 * time.Hour),
			Parameters: map[string][]string{"eventEndingTime": {want.Format(time.RFC3339)}},
		}
		if !a.EndTime().Equal(want) {
			t.Errorf("EndTime() = %v, want %v", a.EndTime(), want)
		}
	})
	t.Run("falls back to expires", func(t *testing.T) {
		exp := base.Add(30 * time.Minute)
		a := Alert{Expires: exp}
		if !a.EndTime().Equal(exp) {
			t.Errorf("EndTime() = %v, want %v", a.EndTime(), exp)
		}
	})
}

func TestAlertHelpers(t *testing.T) {
	a := Alert{Status: StatusActual, Geometry: nil, UGC: nil}
	if a.IsTest() {
		t.Errorf("Actual alert reported IsTest()")
	}
	if a.HasPolygon() {
		t.Errorf("nil Geometry reported HasPolygon()")
	}
	if a.HasLocation() {
		t.Errorf("no UGC and no polygon reported HasLocation()")
	}
	a.UGC = []string{"MIC081"}
	if !a.HasLocation() {
		t.Errorf("UGC set but HasLocation() false")
	}
	a2 := Alert{Status: StatusTest}
	if !a2.IsTest() {
		t.Errorf("Test status not reported by IsTest()")
	}
	c := Alert{MessageType: MessageTypeCancel}
	if !c.IsCancel() {
		t.Errorf("Cancel message type not reported by IsCancel()")
	}
}

func TestAlertVTECAction(t *testing.T) {
	tests := []struct {
		name string
		vtec []string
		want string
	}{
		{"new", []string{"/O.NEW.KGRR.TO.W.0007.260917T2000Z-260917T2045Z/"}, "NEW"},
		{"con", []string{"/O.CON.KABQ.FF.W.0150.000000T0000Z-260918T0400Z/"}, "CON"},
		{"ext", []string{"/O.EXT.KOHX.HT.Y.0009.000000T0000Z-260921T0000Z/"}, "EXT"},
		{"absent", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Alert{}
			if tt.vtec != nil {
				a.Parameters = map[string][]string{"VTEC": tt.vtec}
			}
			if got := a.VTECAction(); got != tt.want {
				t.Errorf("VTECAction() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAlertReplacesIDs(t *testing.T) {
	a := Alert{References: []Reference{{ID: "a"}, {ID: "b"}}}
	got := a.ReplacesIDs()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("ReplacesIDs() = %v, want [a b]", got)
	}
}

func TestBBoxCrossesAntimeridian(t *testing.T) {
	if (BBox{West: 10, East: 20}).CrossesAntimeridian() {
		t.Errorf("normal box reported crossing the antimeridian")
	}
	if !(BBox{West: 179, East: -179}).CrossesAntimeridian() {
		t.Errorf("west>east box not reported crossing the antimeridian")
	}
}

// ---------------------------------------------------------------------------
// TestJSONTagsAreCamelCase — the cheap insurance CLAUDE.md asks for: every
// exported field of every struct that reaches the frontend must carry a
// json:"camelCase" tag (or an explicit json:"-").
// ---------------------------------------------------------------------------

func TestJSONTagsAreCamelCase(t *testing.T) {
	types := []any{
		Alert{},
		MatchedAlert{},
		Reference{},
		Geometry{},
		LatLon{},
		BBox{},
		AffectedItem{},
		Affects{},
		RouteSpan{},
		NetAck{},
		ZoneRef{},
		EffectivePolicy{},
		NetWatch{},
		NetWatchZones{},
		FootprintSummary{},
		LinkStatus{},
		Snapshot{},
		ZoneRecord{},
		EventType{},
	}
	for _, v := range types {
		checkJSONTags(t, reflect.TypeOf(v), reflect.TypeOf(v).Name())
	}
}

func checkJSONTags(t *testing.T, typ reflect.Type, path string) {
	t.Helper()
	if typ.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		fieldPath := path + "." + f.Name

		// Unexported fields are never serialised; skip.
		if f.PkgPath != "" {
			continue
		}

		tag, ok := f.Tag.Lookup("json")
		if f.Anonymous && !ok {
			// Embedded struct with no explicit tag: its fields are promoted;
			// recurse into it instead of demanding a tag on the field itself.
			ft := f.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			checkJSONTags(t, ft, path)
			continue
		}
		if !ok {
			t.Errorf("%s: exported field has no json tag", fieldPath)
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "-" {
			continue // explicitly excluded from the wire
		}
		if name == "" {
			t.Errorf("%s: json tag %q has no name", fieldPath, tag)
			continue
		}
		r := []rune(name)[0]
		if r < 'a' || r > 'z' {
			t.Errorf("%s: json tag %q is not camelCase (starts with %q)", fieldPath, tag, r)
		}

		// Recurse into nested structs (and slices/pointers of structs) so a
		// struct embedded by value without "json:\"-\"" is also covered.
		ft := f.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft != reflect.TypeOf(time.Time{}) {
			checkJSONTags(t, ft, fieldPath)
		}
	}
}
