package wxalert

import "testing"

func TestClassifyMatrix(t *testing.T) {
	pol := DefaultPolicy()

	type row struct {
		name  string
		event string
		sev   Severity
		urg   Urgency
		prox  Proximity
		want  NotifyClass
	}
	tests := []row{
		{"1: TOR Extreme Immediate In allowlisted", "Tornado Warning", SeverityExtreme, UrgencyImmediate, ProximityIn, NotifyInterrupt},
		{"2: TOR Extreme Immediate Near never interrupt", "Tornado Warning", SeverityExtreme, UrgencyImmediate, ProximityNear, NotifyToast},
		{"4: TOR Extreme Expected In allowlist ignores urgency floor", "Tornado Warning", SeverityExtreme, UrgencyExpected, ProximityIn, NotifyInterrupt},
		{"6: SVR Severe Immediate In allowlisted by default", "Severe Thunderstorm Warning", SeveritySevere, UrgencyImmediate, ProximityIn, NotifyInterrupt},
		{"7: SVR Severe Immediate Near", "Severe Thunderstorm Warning", SeveritySevere, UrgencyImmediate, ProximityNear, NotifyToast},
		{"8: SVR Moderate Expected In allowlist ignores severity", "Severe Thunderstorm Warning", SeverityModerate, UrgencyExpected, ProximityIn, NotifyInterrupt},
		{"9: FFW Severe Immediate In", "Flash Flood Warning", SeveritySevere, UrgencyImmediate, ProximityIn, NotifyInterrupt},
		{"11: Gale Warning Severe Expected In not allowlisted -> toast", "Gale Warning", SeveritySevere, UrgencyExpected, ProximityIn, NotifyToast},
		{"12: Gale Warning Severe Expected Near -> badge", "Gale Warning", SeveritySevere, UrgencyExpected, ProximityNear, NotifyBadge},
		{"13: Red Flag Warning Severe Expected In -> toast", "Red Flag Warning", SeveritySevere, UrgencyExpected, ProximityIn, NotifyToast},
		{"14: High Wind Warning Moderate Expected In -> toast", "High Wind Warning", SeverityModerate, UrgencyExpected, ProximityIn, NotifyToast},
		{"15: High Wind Warning Minor Immediate In -> toast", "High Wind Warning", SeverityMinor, UrgencyImmediate, ProximityIn, NotifyToast},
		{"16: High Wind Warning Minor Immediate Near -> badge", "High Wind Warning", SeverityMinor, UrgencyImmediate, ProximityNear, NotifyBadge},
		{"17: Hurricane Warning Extreme Immediate In not allowlisted -> toast (floor)", "Hurricane Warning", SeverityExtreme, UrgencyImmediate, ProximityIn, NotifyToast},
		{"18: Hurricane Warning Extreme Immediate Near -> toast", "Hurricane Warning", SeverityExtreme, UrgencyImmediate, ProximityNear, NotifyToast},
		{"19: Tornado Watch Severe Expected In -> toast", "Tornado Watch", SeveritySevere, UrgencyExpected, ProximityIn, NotifyToast},
		{"20: Tornado Watch Severe Expected Near -> badge", "Tornado Watch", SeveritySevere, UrgencyExpected, ProximityNear, NotifyBadge},
		{"21: Flood Watch Minor Future In -> toast (tier regardless of severity)", "Flood Watch", SeverityMinor, UrgencyFuture, ProximityIn, NotifyToast},
		{"22: Heat Advisory Moderate Expected In -> badge", "Heat Advisory", SeverityModerate, UrgencyExpected, ProximityIn, NotifyBadge},
		{"23: Heat Advisory Moderate Expected Near -> panel", "Heat Advisory", SeverityModerate, UrgencyExpected, ProximityNear, NotifyPanel},
		{"24: Small Craft Advisory Minor Expected In -> badge", "Small Craft Advisory", SeverityMinor, UrgencyExpected, ProximityIn, NotifyBadge},
		{"25: Air Quality Alert Unknown Unknown In -> badge", "Air Quality Alert", SeverityUnknown, UrgencyUnknown, ProximityIn, NotifyBadge},
		{"26: SPS Moderate Expected In -> panel", "Special Weather Statement", SeverityModerate, UrgencyExpected, ProximityIn, NotifyPanel},
		{"27: SPS Moderate Expected Near -> panel", "Special Weather Statement", SeverityModerate, UrgencyExpected, ProximityNear, NotifyPanel},
		{"28: SPS Severe Immediate In severity cannot lift a statement", "Special Weather Statement", SeverityExtreme, UrgencyImmediate, ProximityIn, NotifyToast}, // floored: Extreme+Immediate+In
		{"29: Hazardous Weather Outlook Unknown Future In -> panel", "Hazardous Weather Outlook", SeverityUnknown, UrgencyFuture, ProximityIn, NotifyPanel},
		{"32: Child Abduction Emergency Unknown Immediate In -> toast", "Child Abduction Emergency", SeverityUnknown, UrgencyImmediate, ProximityIn, NotifyToast},
		{"33: Civil Danger Warning Extreme Immediate In not allowlisted, floor holds", "Civil Danger Warning", SeverityExtreme, UrgencyImmediate, ProximityIn, NotifyToast},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, _ := TierForEvent(tt.event)
			in := Input{EffectiveEvent: tt.event, Tier: tier, Severity: tt.sev, Urgency: tt.urg, Proximity: tt.prox, Active: true}
			got := Classify(in, pol)
			if got.Class != tt.want {
				t.Errorf("Classify(%s) = %v (reason=%s floored=%v), want %v", tt.event, got.Class, got.Reason, got.Floored, tt.want)
			}
		})
	}
}

func TestClassifyRow28IsFloored(t *testing.T) {
	tier, _ := TierForEvent("Special Weather Statement")
	in := Input{EffectiveEvent: "Special Weather Statement", Tier: tier, Severity: SeverityExtreme, Urgency: UrgencyImmediate, Proximity: ProximityIn, Active: true}
	got := Classify(in, DefaultPolicy())
	if !got.Floored {
		t.Errorf("Floored = false, want true (Extreme+Immediate+In always floors to toast)")
	}
}

func TestClassifyFarIsNeverSurfaced(t *testing.T) {
	tiers := []Tier{TierWarning, TierWatch, TierAdvisory, TierStatement}
	for _, tier := range tiers {
		in := Input{EffectiveEvent: "Tornado Warning", Tier: tier, Severity: SeverityExtreme, Urgency: UrgencyImmediate, Proximity: ProximityFar, Active: true}
		got := Classify(in, DefaultPolicy())
		if got.Class != NotifyPanel {
			t.Errorf("Far alert (tier=%v) = %v, want panel", tier, got.Class)
		}
	}
}

func TestClassifyPastAndUnknownUrgencyDisqualifyInterrupt(t *testing.T) {
	tests := []struct {
		name string
		urg  Urgency
		want NotifyClass
	}{
		{"Past -> panel (expired-like, any tier)", UrgencyPast, NotifyPanel},
		{"Unknown -> falls to the tier row (toast), not interrupt", UrgencyUnknown, NotifyToast},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := Input{EffectiveEvent: "Tornado Warning", Tier: TierWarning, Severity: SeverityExtreme, Urgency: tt.urg, Proximity: ProximityIn, Active: true}
			got := Classify(in, DefaultPolicy())
			if got.Class != tt.want {
				t.Errorf("Classify() = %v, want %v", got.Class, tt.want)
			}
			if got.Class == NotifyInterrupt {
				t.Errorf("urgency %v produced Interrupt, want it disqualified from the allowlist", tt.urg)
			}
		})
	}
}

func TestClassifyTestStatusIsNeverSurfaced(t *testing.T) {
	in := Input{EffectiveEvent: "Tornado Warning", Tier: TierWarning, Severity: SeverityExtreme, Urgency: UrgencyImmediate, Proximity: ProximityIn, Active: true, IsTest: true}
	got := Classify(in, DefaultPolicy())
	if got.Class != NotifyPanel {
		t.Errorf("Classify(IsTest) = %v, want panel", got.Class)
	}
}

func TestClassifyAllowlistEdits(t *testing.T) {
	tests := []struct {
		name   string
		policy func() Policy
		event  string
		sev    Severity
		urg    Urgency
		want   NotifyClass
	}{
		{
			name:   "empty allowlist, TOR Extreme Immediate In -> toast (floor)",
			policy: func() Policy { p := DefaultPolicy(); p.InterruptEvents = nil; return p },
			event:  "Tornado Warning", sev: SeverityExtreme, urg: UrgencyImmediate, want: NotifyToast,
		},
		{
			name:   "empty allowlist, SVR Severe Immediate In -> toast (warning row)",
			policy: func() Policy { p := DefaultPolicy(); p.InterruptEvents = nil; return p },
			event:  "Severe Thunderstorm Warning", sev: SeveritySevere, urg: UrgencyImmediate, want: NotifyToast,
		},
		{
			name:   "sub-warning-tier entry leaked into the list is ignored by Classify",
			policy: func() Policy { p := DefaultPolicy(); p.InterruptEvents = []string{"Winter Weather Advisory"}; return p },
			event:  "Winter Weather Advisory", sev: SeverityMinor, urg: UrgencyExpected, want: NotifyBadge,
		},
		{
			name:   "case-insensitive allowlist match",
			policy: func() Policy { p := DefaultPolicy(); p.InterruptEvents = []string{"tornado warning"}; return p },
			event:  "Tornado Warning", sev: SeverityExtreme, urg: UrgencyImmediate, want: NotifyInterrupt,
		},
		{
			name:   "Hurricane Warning NCS-added -> interrupt",
			policy: func() Policy { p := DefaultPolicy(); p.InterruptEvents = []string{"Hurricane Warning"}; return p },
			event:  "Hurricane Warning", sev: SeverityExtreme, urg: UrgencyImmediate, want: NotifyInterrupt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, _ := TierForEvent(tt.event)
			in := Input{EffectiveEvent: tt.event, Tier: tier, Severity: tt.sev, Urgency: tt.urg, Proximity: ProximityIn, Active: true}
			got := Classify(in, tt.policy())
			if got.Class != tt.want {
				t.Errorf("Classify() = %v, want %v", got.Class, tt.want)
			}
		})
	}
}

func TestClassifyAllowlistEffectiveEventMatching(t *testing.T) {
	ffwCatastrophic := Input{Event: "Flash Flood Warning", EffectiveEvent: "Flash Flood Emergency", Tier: TierWarning, Severity: SeverityExtreme, Urgency: UrgencyImmediate, Proximity: ProximityIn, Active: true}
	ffwPlain := Input{Event: "Flash Flood Warning", EffectiveEvent: "Flash Flood Warning", Tier: TierWarning, Severity: SeveritySevere, Urgency: UrgencyImmediate, Proximity: ProximityIn, Active: true}

	onlyEmergency := DefaultPolicy()
	onlyEmergency.InterruptEvents = []string{"Flash Flood Emergency"}
	if got := Classify(ffwCatastrophic, onlyEmergency).Class; got != NotifyInterrupt {
		t.Errorf("FFW+CATASTROPHIC against [Flash Flood Emergency] = %v, want interrupt", got)
	}
	if got := Classify(ffwPlain, onlyEmergency).Class; got != NotifyToast {
		t.Errorf("plain FFW against [Flash Flood Emergency] = %v, want toast", got)
	}

	onlyWarning := DefaultPolicy()
	onlyWarning.InterruptEvents = []string{"Flash Flood Warning"}
	if got := Classify(ffwCatastrophic, onlyWarning).Class; got != NotifyInterrupt {
		t.Errorf("FFW+CATASTROPHIC against [Flash Flood Warning] = %v, want interrupt (an emergency is still a FFW)", got)
	}
}

func TestClassifyHardFloor(t *testing.T) {
	watchLevels := []NotifyClass{NotifyToast, NotifyBadge, NotifyPanel}
	advisoryLevels := []NotifyClass{NotifyBadge, NotifyPanel}
	allowlists := [][]string{DefaultInterruptEvents, nil}
	mutes := []bool{false, true}

	floorEvents := []string{"Hurricane Warning", "Tornado Warning", "Extreme Wind Warning", "Civil Danger Warning"}

	for _, allow := range allowlists {
		for _, mute := range mutes {
			for _, wl := range watchLevels {
				for _, al := range advisoryLevels {
					pol := Policy{InterruptEvents: allow, WatchNotify: wl, AdvisoryNotify: al, StatementNotify: NotifyPanel, MuteAdvisories: mute}
					for _, event := range floorEvents {
						in := Input{EffectiveEvent: event, Tier: TierWarning, Severity: SeverityExtreme, Urgency: UrgencyImmediate, Proximity: ProximityIn, Active: true}
						got := Classify(in, pol)
						if NotifyRank(got.Class) < NotifyRank(NotifyToast) {
							t.Errorf("policy %+v: %s = %v, want >= toast (hard floor)", pol, event, got.Class)
						}
					}
					// Any IN warning-tier alert, any severity, must not go below toast.
					anyWarning := Input{EffectiveEvent: "Gale Warning", Tier: TierWarning, Severity: SeverityMinor, Urgency: UrgencyExpected, Proximity: ProximityIn, Active: true}
					if got := Classify(anyWarning, pol).Class; NotifyRank(got) < NotifyRank(NotifyToast) {
						t.Errorf("policy %+v: warning-tier alert = %v, want >= toast", pol, got)
					}
				}
			}
		}
	}

	// Sanity: muting/advisory-panel settings DO let an advisory reach panel,
	// and an In alert is never ClassNone-equivalent (panel is the floor).
	quiet := Policy{WatchNotify: NotifyBadge, AdvisoryNotify: NotifyPanel, StatementNotify: NotifyPanel, MuteAdvisories: true}
	heat := Input{EffectiveEvent: "Heat Advisory", Tier: TierAdvisory, Severity: SeverityModerate, Urgency: UrgencyExpected, Proximity: ProximityIn, Active: true}
	if got := Classify(heat, quiet).Class; got != NotifyPanel {
		t.Errorf("muted advisory = %v, want panel", got)
	}
}

func TestClassifyEndedTransitions(t *testing.T) {
	tests := []struct {
		name      string
		prevClass NotifyClass
		tier      Tier
		prox      Proximity
		want      NotifyClass
	}{
		{"warning In, was interrupt -> toast (all clear)", NotifyInterrupt, TierWarning, ProximityIn, NotifyToast},
		{"advisory In, was badge -> panel (advisories never toast on the way out)", NotifyBadge, TierAdvisory, ProximityIn, NotifyPanel},
		{"watch Near, was toast -> panel (Near never announces all-clear)", NotifyToast, TierWatch, ProximityNear, NotifyPanel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := Input{Tier: tt.tier, Proximity: tt.prox, Active: false, PrevClass: tt.prevClass}
			got := Classify(in, DefaultPolicy())
			if got.Class != tt.want {
				t.Errorf("Classify(ended) = %v, want %v", got.Class, tt.want)
			}
			if got.Reason != "ended" {
				t.Errorf("Reason = %q, want ended", got.Reason)
			}
			if got.Floored {
				t.Errorf("Floored = true, want false (no floor for ended alerts)")
			}
		})
	}
}

func TestClassifyUpdateIsQuieter(t *testing.T) {
	tests := []struct {
		name      string
		event     string
		tier      Tier
		sev       Severity
		urg       Urgency
		escalated bool
		netAcked  bool
		want      NotifyClass
	}{
		{"Interrupt update, unchanged -> toast (one quieter)", "Tornado Warning", TierWarning, SeverityExtreme, UrgencyImmediate, false, false, NotifyToast},
		{"Interrupt update, escalated -> stays interrupt", "Tornado Warning", TierWarning, SeverityExtreme, UrgencyImmediate, true, false, NotifyInterrupt},
		{"Flood Watch update -> badge", "Flood Watch", TierWatch, SeverityModerate, UrgencyExpected, false, false, NotifyBadge},
		{"Heat Advisory update (was badge) -> panel", "Heat Advisory", TierAdvisory, SeverityModerate, UrgencyExpected, false, false, NotifyPanel},
		{"SPS update -> panel (floor of the update rule is panel, never none)", "Special Weather Statement", TierStatement, SeverityModerate, UrgencyExpected, false, false, NotifyPanel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := Input{EffectiveEvent: tt.event, Tier: tt.tier, Severity: tt.sev, Urgency: tt.urg, Proximity: ProximityIn, Active: true, IsUpdate: true, Escalated: tt.escalated, NetAcked: tt.netAcked}
			got := Classify(in, DefaultPolicy())
			if got.Class != tt.want {
				t.Errorf("Classify() = %v (reason=%s), want %v", got.Class, got.Reason, tt.want)
			}
		})
	}
}

func TestClassifyNetAckedUpdate(t *testing.T) {
	in := Input{EffectiveEvent: "Severe Thunderstorm Warning", Tier: TierWarning, Severity: SeveritySevere, Urgency: UrgencyImmediate, Proximity: ProximityIn, Active: true, IsUpdate: true, NetAcked: true}
	got := Classify(in, DefaultPolicy())
	if got.Class != NotifyBadge {
		t.Errorf("net-acked non-escalated update = %v, want badge", got.Class)
	}

	inEsc := in
	inEsc.Escalated = true
	gotEsc := Classify(inEsc, DefaultPolicy())
	if gotEsc.Class != NotifyInterrupt {
		t.Errorf("net-acked escalated update = %v, want interrupt (escalation un-acks)", gotEsc.Class)
	}
}

func TestClassifyPreviouslyUnseenReferenceIsTreatedAsNew(t *testing.T) {
	// prev nil (app started mid-event): IsUpdate must be false so the alert
	// gets full loudness, not one-quieter.
	in := Input{EffectiveEvent: "Tornado Warning", Tier: TierWarning, Severity: SeverityExtreme, Urgency: UrgencyImmediate, Proximity: ProximityIn, Active: true, IsUpdate: false}
	got := Classify(in, DefaultPolicy())
	if got.Class != NotifyInterrupt {
		t.Errorf("Classify() = %v, want interrupt (full loudness for an unseen alert)", got.Class)
	}
}

func TestInAllowlist(t *testing.T) {
	list := []string{"Tornado Warning", "Flash Flood Warning"}
	if !InAllowlist("tornado warning", list) {
		t.Errorf("InAllowlist case-insensitive match failed")
	}
	if !InAllowlist("  Flash Flood Warning  ", list) {
		t.Errorf("InAllowlist trim failed")
	}
	if InAllowlist("Flood Warning", list) {
		t.Errorf("InAllowlist matched a non-member")
	}
}

func TestValidateInterruptEvents(t *testing.T) {
	t.Run("unknown event rejected", func(t *testing.T) {
		_, err := ValidateInterruptEvents([]string{"Frog Rain Warning"})
		if err == nil || err.Error() != "unknown event: Frog Rain Warning" {
			t.Errorf("err = %v, want %q", err, "unknown event: Frog Rain Warning")
		}
	})
	t.Run("sub-warning tier rejected", func(t *testing.T) {
		_, err := ValidateInterruptEvents([]string{"Heat Advisory"})
		if err == nil || err.Error() != "interrupt list may only contain warning-tier events: Heat Advisory" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("case duplicates collapse to one canonical entry", func(t *testing.T) {
		out, err := ValidateInterruptEvents([]string{"Tornado Warning", "tornado warning"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 || out[0] != "Tornado Warning" {
			t.Errorf("out = %v, want [Tornado Warning]", out)
		}
	})
	t.Run("derived emergency name is valid", func(t *testing.T) {
		out, err := ValidateInterruptEvents([]string{"Flash Flood Emergency"})
		if err != nil || len(out) != 1 || out[0] != "Flash Flood Emergency" {
			t.Errorf("out=%v err=%v", out, err)
		}
	})
	t.Run("nil input returns non-nil empty slice", func(t *testing.T) {
		out, err := ValidateInterruptEvents(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out == nil || len(out) != 0 {
			t.Errorf("out = %v, want non-nil empty", out)
		}
	})
}

func TestDefaultInterruptEventsMatchesFloorSeed(t *testing.T) {
	want := []string{
		"Tornado Warning", "Flash Flood Emergency", "Severe Thunderstorm Warning", "Flash Flood Warning",
		"Extreme Wind Warning", "Ice Storm Warning", "Blizzard Warning",
	}
	if len(DefaultInterruptEvents) != len(want) {
		t.Fatalf("len = %d, want %d", len(DefaultInterruptEvents), len(want))
	}
	for i, e := range want {
		if DefaultInterruptEvents[i] != e {
			t.Errorf("DefaultInterruptEvents[%d] = %q, want %q", i, DefaultInterruptEvents[i], e)
		}
	}
	// Every seed entry must itself validate (warning tier, known name).
	if _, err := ValidateInterruptEvents(DefaultInterruptEvents); err != nil {
		t.Errorf("DefaultInterruptEvents fails its own validator: %v", err)
	}
}
