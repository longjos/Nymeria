package wxalert

import (
	"fmt"
	"testing"
	"time"
)

func urnID(n int) string {
	return fmt.Sprintf("urn:oid:2.49.0.1.840.0.%040d.001.1", n)
}

var edt = time.FixedZone("EDT", -4*3600)

func mkRegAlert(id string, sent time.Time, opts ...func(*Alert)) Alert {
	a := Alert{
		ID: id, Event: "Tornado Warning", Tier: TierWarning, Status: StatusActual,
		MessageType: MessageTypeAlert, Sent: sent, Expires: sent.Add(45 * time.Minute),
		Ends: timePtr(sent.Add(45 * time.Minute)),
	}
	for _, o := range opts {
		o(&a)
	}
	return a
}

func timePtr(t time.Time) *time.Time { return &t }

func withRef(id string, sent time.Time) func(*Alert) {
	return func(a *Alert) { a.References = append(a.References, Reference{ID: id, Sent: sent}) }
}

func withCancel() func(*Alert) {
	return func(a *Alert) { a.MessageType = MessageTypeCancel }
}

func withEnds(t time.Time) func(*Alert) {
	return func(a *Alert) { a.Ends = &t; a.Expires = t }
}

func TestRegistryAddThenUpdateReplaces(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idB := urnID(1), urnID(2)
	r := NewRegistry()

	changes1 := r.Apply([]Alert{mkRegAlert(idA, t0)}, t0)
	if len(changes1) != 1 || changes1[0].Kind != ChangeAdded {
		t.Fatalf("step1 changes = %+v, want one Added", changes1)
	}

	t1 := t0.Add(15 * time.Minute)
	changes2 := r.Apply([]Alert{mkRegAlert(idB, t1, withRef(idA, t0))}, t1)

	if r.Len() != 1 {
		t.Errorf("Len() = %d, want 1", r.Len())
	}
	if r.Get(idB) == nil {
		t.Errorf("Get(B) = nil, want the active alert")
	}
	if r.Get(idA) != nil {
		t.Errorf("Get(A) = %+v, want nil (superseded)", r.Get(idA))
	}
	prev := r.Prev(idB)
	if prev == nil || prev.ID != idA {
		t.Fatalf("Prev(B) = %v, want id %s", prev, idA)
	}
	if len(changes2) != 1 || changes2[0].Kind != ChangeUpdated || changes2[0].Alert.ID != idB || changes2[0].Prev == nil || changes2[0].Prev.ID != idA {
		t.Fatalf("step2 changes = %+v", changes2)
	}
	hist := r.History(idB)
	if len(hist) != 1 || hist[0].ID != idA {
		t.Fatalf("History(B) = %v, want [A]", hist)
	}
}

func TestRegistryCancelRemoves(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idC := urnID(1), urnID(2)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0)}, t0)

	t1 := t0.Add(20 * time.Minute)
	cancel := mkRegAlert(idC, t1, withRef(idA, t0), withCancel())
	changes := r.Apply([]Alert{mkRegAlert(idA, t0), cancel}, t1)

	if len(r.Active()) != 0 {
		t.Errorf("Active() = %v, want empty", r.Active())
	}
	ended := r.Ended()
	if len(ended) != 1 || ended[0].Alert.ID != idA || ended[0].Reason != "cancelled" {
		t.Fatalf("Ended() = %+v, want [A cancelled]", ended)
	}
	if !ended[0].At.Equal(t1) {
		t.Errorf("CancelledAt = %v, want %v", ended[0].At, t1)
	}
	if r.Get(idC) != nil {
		t.Errorf("Get(C) = %v, want nil (the Cancel message itself is not an alert)", r.Get(idC))
	}
	if len(changes) != 1 || changes[0].Kind != ChangeCancelled || changes[0].Alert.ID != idA {
		t.Fatalf("changes = %+v, want [Cancelled{A}]", changes)
	}
}

func TestRegistryCancelOfUnknownIsIgnored(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	r := NewRegistry()
	changes := r.Apply([]Alert{mkRegAlert(urnID(9), t0, withRef(urnID(404), t0), withCancel())}, t0)
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none", changes)
	}
	if r.Len() != 0 {
		t.Errorf("Len() = %d, want 0", r.Len())
	}
}

func TestRegistryOutOfOrderUpdateDoesNotResurrect(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idC, idB := urnID(1), urnID(2), urnID(3)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0)}, t0)

	tCancel := t0.Add(20 * time.Minute)
	r.Apply([]Alert{mkRegAlert(idC, tCancel, withRef(idA, t0), withCancel())}, tCancel)

	tUpdate := t0.Add(10 * time.Minute) // older than the cancel
	changes := r.Apply([]Alert{mkRegAlert(idB, tUpdate, withRef(idA, t0))}, tUpdate)

	if len(r.Active()) != 0 {
		t.Errorf("Active() = %v, want empty", r.Active())
	}
	if r.Get(idB) != nil {
		t.Errorf("Get(B) = %v, want nil (discarded, older than the cancel)", r.Get(idB))
	}
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none", changes)
	}
	ended := r.Ended()
	if len(ended) != 1 || ended[0].Alert.ID != idA {
		t.Fatalf("Ended() = %v, want just A", ended)
	}
}

func TestRegistryUpdateNewerThanCancelReplaces(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idC, idB := urnID(1), urnID(2), urnID(3)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0)}, t0)

	tCancel := t0.Add(20 * time.Minute)
	r.Apply([]Alert{mkRegAlert(idC, tCancel, withRef(idA, t0), withCancel())}, tCancel)

	tUpdate := t0.Add(25 * time.Minute) // newer than the cancel
	changes := r.Apply([]Alert{mkRegAlert(idB, tUpdate, withRef(idA, t0))}, tUpdate)

	if r.Get(idB) == nil {
		t.Fatalf("Get(B) = nil, want active (NWS re-issued after cancelling)")
	}
	if len(changes) != 1 || changes[0].Kind != ChangeAdded || changes[0].Prev == nil || changes[0].Prev.ID != idA {
		t.Fatalf("changes = %+v, want [Added{B, Prev:A}]", changes)
	}
}

func TestRegistryExpiredByClock(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA := urnID(1)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0, withEnds(t0.Add(45*time.Minute)))}, t0)

	if changes := r.Tick(t0.Add(44*time.Minute + 59*time.Second)); len(changes) != 0 {
		t.Errorf("Tick before expiry produced %v, want none", changes)
	}
	changes := r.Tick(t0.Add(45*time.Minute + time.Second))
	if len(r.Active()) != 0 {
		t.Errorf("Active() = %v, want empty after expiry", r.Active())
	}
	ended := r.Ended()
	if len(ended) != 1 || ended[0].Alert.State != AlertStateExpired || ended[0].Reason != "clock" {
		t.Fatalf("Ended() = %+v, want state expired / reason clock", ended)
	}
	if !ended[0].At.Equal(t0.Add(45 * time.Minute)) {
		t.Errorf("ExpiredAt = %v, want the alert's own end time", ended[0].At)
	}
	if len(changes) != 1 || changes[0].Kind != ChangeExpired {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestRegistryExpiresFallbackWhenEndsNil(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA := urnID(1)
	r := NewRegistry()
	a := mkRegAlert(idA, t0)
	a.Ends = nil
	a.Expires = t0.Add(30 * time.Minute)
	r.Apply([]Alert{a}, t0)

	if changes := r.Tick(t0.Add(30 * time.Minute)); len(changes) != 0 {
		t.Errorf("Tick at exactly Expires produced %v, want none yet", changes)
	}
	changes := r.Tick(t0.Add(30*time.Minute + time.Second))
	if len(changes) != 1 {
		t.Fatalf("changes = %v, want one expiry", changes)
	}
}

func TestRegistryCancelledByNWSBeatsClock(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idC := urnID(1), urnID(2)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0, withEnds(t0.Add(45*time.Minute)))}, t0)

	tCancel := t0.Add(20 * time.Minute)
	r.Apply([]Alert{mkRegAlert(idC, tCancel, withRef(idA, t0), withCancel())}, tCancel)

	changes := r.Tick(t0.Add(50 * time.Minute))
	if len(changes) != 0 {
		t.Errorf("Tick after an already-cancelled alert's would-be expiry produced %v, want none", changes)
	}
	ended := r.Ended()
	if len(ended) != 1 || ended[0].Reason != "cancelled" || !ended[0].At.Equal(tCancel) {
		t.Fatalf("Ended() = %+v, want reason cancelled at %v unchanged", ended, tCancel)
	}
}

func TestRegistryVanished(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idB := urnID(1), urnID(2)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0), mkRegAlert(idB, t0)}, t0)

	changes := r.Apply([]Alert{mkRegAlert(idA, t0)}, t0.Add(time.Minute))
	if len(changes) != 1 || changes[0].Kind != ChangeVanished || changes[0].Alert.ID != idB {
		t.Fatalf("changes = %+v, want [Vanished{B}]", changes)
	}
	ended := r.Ended()
	if len(ended) != 1 || ended[0].Alert.ID != idB || ended[0].Reason != "dropped" {
		t.Fatalf("Ended() = %+v, want B dropped", ended)
	}
}

func TestRegistryVanishedNotTriggeredByFailedPoll(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idB := urnID(1), urnID(2)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0), mkRegAlert(idB, t0)}, t0)

	changes := r.ApplyFailure(fmt.Errorf("boom"))
	if len(changes) != 0 {
		t.Errorf("ApplyFailure produced %v, want none", changes)
	}
	if len(r.Active()) != 2 {
		t.Errorf("Active() = %v, want both still active", r.Active())
	}
}

func TestRegistryExpiredPurgeAfterHour(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA := urnID(1)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0, withEnds(t0.Add(45*time.Minute)))}, t0)
	r.Tick(t0.Add(45*time.Minute + time.Second))
	endedAt := t0.Add(45 * time.Minute)

	// The Manager's retention policy is "purge anything ended more than an
	// hour ago", i.e. it calls PurgeEndedBefore(now.Add(-time.Hour)); a
	// cutoff still short of the alert's own EndedAt must not remove it yet.
	if n := r.PurgeEndedBefore(endedAt.Add(-time.Minute)); n != 0 {
		t.Errorf("PurgeEndedBefore too early removed %d, want 0", n)
	}
	if len(r.Ended()) != 1 {
		t.Fatalf("Ended() before purge = %v, want 1", r.Ended())
	}
	if n := r.PurgeEndedBefore(endedAt.Add(time.Minute)); n != 1 {
		t.Errorf("PurgeEndedBefore removed %d, want 1", n)
	}
	if len(r.Ended()) != 0 {
		t.Errorf("Ended() after purge = %v, want empty", r.Ended())
	}
}

func TestRegistryUpdatePreservesAck(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idB := urnID(1), urnID(2)

	t.Run("preserved by default", func(t *testing.T) {
		r := NewRegistry()
		r.Apply([]Alert{mkRegAlert(idA, t0)}, t0)
		ackAt := t0.Add(5 * time.Minute)
		r.SetNetAck(idA, NetAck{Callsign: "W8ABC", At: ackAt})

		r.Apply([]Alert{mkRegAlert(idB, t0.Add(15*time.Minute), withRef(idA, t0))}, t0.Add(15*time.Minute))
		b := r.Get(idB)
		if b == nil || b.AckedForNet == nil || !b.AckedForNet.At.Equal(ackAt) {
			t.Fatalf("Get(B).AckedForNet = %v, want preserved from A", b)
		}
	})

	t.Run("cleared on escalation", func(t *testing.T) {
		r := NewRegistry()
		r.Apply([]Alert{mkRegAlert(idA, t0)}, t0)
		r.SetNetAck(idA, NetAck{Callsign: "W8ABC", At: t0.Add(5 * time.Minute)})

		r.Apply([]Alert{mkRegAlert(idB, t0.Add(15*time.Minute), withRef(idA, t0))}, t0.Add(15*time.Minute))
		// The Manager decides "this was an escalation" (via notify.Classify)
		// and clears the ack explicitly.
		r.ClearNetAck(idB)
		b := r.Get(idB)
		if b == nil || b.AckedForNet != nil {
			t.Fatalf("Get(B).AckedForNet = %v, want nil after ClearNetAck", b)
		}
	})
}

func TestRegistryReferencesChainDepth(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idB, idC, idD := urnID(1), urnID(2), urnID(3), urnID(4)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0)}, t0)
	r.Apply([]Alert{mkRegAlert(idB, t0.Add(1*time.Minute), withRef(idA, t0))}, t0.Add(1*time.Minute))
	r.Apply([]Alert{mkRegAlert(idC, t0.Add(2*time.Minute), withRef(idB, t0.Add(1*time.Minute)))}, t0.Add(2*time.Minute))
	r.Apply([]Alert{mkRegAlert(idD, t0.Add(3*time.Minute), withRef(idC, t0.Add(2*time.Minute)))}, t0.Add(3*time.Minute))

	hist := r.History(idD)
	if len(hist) != 3 || hist[0].ID != idC || hist[1].ID != idB || hist[2].ID != idA {
		t.Fatalf("History(D) = %v, want [C B A]", hist)
	}
	if r.Len() != 1 {
		t.Errorf("Len() = %d, want 1", r.Len())
	}
}

func TestRegistryDuplicateIDIsNoop(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA := urnID(1)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0)}, t0)
	changes := r.Apply([]Alert{mkRegAlert(idA, t0)}, t0.Add(time.Minute))
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none for a re-applied duplicate id", changes)
	}
	if r.Len() != 1 {
		t.Errorf("Len() = %d, want 1", r.Len())
	}
}

func TestRegistryPrevOnEndedAndUnknown(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idB := urnID(1), urnID(2)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0)}, t0)
	r.Apply([]Alert{mkRegAlert(idB, t0.Add(time.Minute), withRef(idA, t0))}, t0.Add(time.Minute))
	r.Tick(t0.Add(46 * time.Minute)) // expire B

	if p := r.Prev(idB); p == nil || p.ID != idA {
		t.Errorf("Prev(B) after B itself ended = %v, want A", p)
	}
	if p := r.Prev(idA); p != nil {
		t.Errorf("Prev(A) = %v, want nil (A has no predecessor)", p)
	}
	if p := r.Prev("unknown-id"); p != nil {
		t.Errorf("Prev(unknown) = %v, want nil", p)
	}
}

func TestRegistryPrevDanglingReferenceIsNil(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idB := urnID(2)
	r := NewRegistry()
	// B references an id this registry has never seen at all.
	r.Apply([]Alert{mkRegAlert(idB, t0, withRef(urnID(404), t0))}, t0)
	if p := r.Prev(idB); p != nil {
		t.Errorf("Prev(B) with a dangling reference = %v, want nil", p)
	}
	if h := r.History(idB); len(h) != 0 {
		t.Errorf("History(B) with a dangling reference = %v, want empty", h)
	}
}

func TestRegistrySetAndClearNetAckOnEndedAndUnknown(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA := urnID(1)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0, withEnds(t0.Add(10*time.Minute)))}, t0)
	r.Tick(t0.Add(11 * time.Minute))

	if !r.SetNetAck(idA, NetAck{Callsign: "W8ABC", At: t0}) {
		t.Fatalf("SetNetAck on an ended alert should succeed")
	}
	if !r.ClearNetAck(idA) {
		t.Fatalf("ClearNetAck on an ended alert should succeed")
	}
	if r.SetNetAck("unknown", NetAck{}) {
		t.Errorf("SetNetAck(unknown) = true, want false")
	}
	if r.ClearNetAck("unknown") {
		t.Errorf("ClearNetAck(unknown) = true, want false")
	}
	if r.UpdateFields("unknown", func(*MatchedAlert) {}) {
		t.Errorf("UpdateFields(unknown) = true, want false")
	}
}

func TestRegistryPersistRoundTrip(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 16, 0, 0, 0, edt)
	idA, idB := urnID(1), urnID(2)
	r := NewRegistry()
	r.Apply([]Alert{mkRegAlert(idA, t0), mkRegAlert(idB, t0, withEnds(t0.Add(10*time.Minute)))}, t0)
	r.Tick(t0.Add(11 * time.Minute))
	r.SetNetAck(idA, NetAck{Callsign: "W8ABC", At: t0.Add(2 * time.Minute)})

	snap := r.Snapshot()
	r2 := NewRegistry()
	r2.Restore(snap)

	if r2.Len() != r.Len() {
		t.Fatalf("restored Len() = %d, want %d", r2.Len(), r.Len())
	}
	if len(r2.Ended()) != len(r.Ended()) {
		t.Fatalf("restored Ended() = %v, want same length as %v", r2.Ended(), r.Ended())
	}
	orig := r.Get(idA)
	restored := r2.Get(idA)
	if orig == nil || restored == nil || restored.AckedForNet == nil || !restored.AckedForNet.At.Equal(orig.AckedForNet.At) {
		t.Fatalf("restored ack = %v, want %v", restored, orig)
	}
}

// Alerts persisted before Classify normalised Affects come back off disk with
// nil slices. Active ones are re-classified on the next poll, but ended ones
// never are — they would keep serving JSON nulls forever and freeze the alert
// detail when opened from the expired group.
func TestRestoreNormalisesAffects(t *testing.T) {
	r := NewRegistry()
	nilAffects := Affects{Summary: "somewhere"}
	r.Restore(RegistrySnapshot{
		Active: []snapshotRecord{{Alert: MatchedAlert{Alert: Alert{ID: "active-1"}, State: AlertStateActive, Affects: nilAffects}}},
		Ended:  []snapshotRecord{{Alert: MatchedAlert{Alert: Alert{ID: "ended-1"}, State: AlertStateExpired, Affects: nilAffects}}},
	})

	got := r.Active()
	for _, e := range r.Ended() {
		got = append(got, e.Alert)
	}
	for _, got := range got {
		if got.Affects.Checkpoints == nil || got.Affects.Locations == nil || got.Affects.Stations == nil ||
			got.Affects.CheckpointSeqRange == nil || got.Affects.RouteSpans == nil {
			t.Errorf("Restore(%s): Affects still has nil slices: %+v", got.ID, got.Affects)
		}
		if got.Affects.Summary != "somewhere" {
			t.Errorf("Restore(%s): Summary = %q, want it preserved", got.ID, got.Affects.Summary)
		}
	}
}
