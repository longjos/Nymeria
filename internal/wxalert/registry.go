package wxalert

import (
	"sort"
	"sync"
	"time"
)

// ChangeKind is what happened to an alert during one Registry.Apply/Tick call.
type ChangeKind string

const (
	ChangeAdded     ChangeKind = "added"
	ChangeUpdated   ChangeKind = "updated"
	ChangeCancelled ChangeKind = "cancelled"
	ChangeExpired   ChangeKind = "expired"
	ChangeVanished  ChangeKind = "vanished"
)

// Change is one thing that happened to one alert.
type Change struct {
	Kind  ChangeKind
	Alert MatchedAlert
	Prev  *MatchedAlert
}

// record is the Registry's internal bookkeeping for one alert id.
type record struct {
	alert       MatchedAlert
	prevID      string // the id this one's References[0] pointed to, if any
	cancelledAt time.Time
	expiredAt   time.Time
}

// Registry holds the current and recently-ended alert set for one manager
// instance (it is not per-net; proximity/notify class live on the
// MatchedAlert values callers pass in). It is the pure supersession core:
// deciding what is active, what replaced what, and when something silently
// dropped out of the feed — the Manager drives it once per poll.
type Registry struct {
	mu     sync.Mutex
	active map[string]*record
	ended  map[string]*record
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{active: map[string]*record{}, ended: map[string]*record{}}
}

// Len returns the number of currently-active alerts.
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.active)
}

// Get returns the active alert with id, or nil.
func (r *Registry) Get(id string) *MatchedAlert {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.active[id]; ok {
		a := rec.alert
		return &a
	}
	return nil
}

// Prev returns the alert that id's References[0] pointed to at the time it
// was applied, whether that predecessor is now active, ended, or unknown
// (nil in the last case).
func (r *Registry) Prev(id string) *MatchedAlert {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.active[id]
	if !ok {
		rec, ok = r.ended[id]
	}
	if !ok || rec.prevID == "" {
		return nil
	}
	// The predecessor is always moved to "ended" (superseded/cancelled) the
	// moment it is replaced, so it is never found in r.active here — but a
	// dangling reference to an id this registry has genuinely never seen
	// (the app started mid-event) falls through to nil.
	if p, ok := r.ended[rec.prevID]; ok {
		a := p.alert
		return &a
	}
	return nil
}

// History returns id's predecessor chain, oldest last is NOT guaranteed —
// it is newest-first starting from id's immediate predecessor.
func (r *Registry) History(id string) []MatchedAlert {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []MatchedAlert
	cur := id
	seen := map[string]bool{}
	for {
		rec, ok := r.active[cur]
		if !ok {
			rec, ok = r.ended[cur]
		}
		if !ok || rec.prevID == "" || seen[rec.prevID] {
			break
		}
		seen[rec.prevID] = true
		// As in Prev, a predecessor is always in "ended" by the time it has
		// a successor — never r.active.
		prevRec, ok := r.ended[rec.prevID]
		if !ok {
			break
		}
		out = append(out, prevRec.alert)
		cur = rec.prevID
	}
	return out
}

// Active returns every currently-active alert, sorted by ID for a stable
// iteration order.
func (r *Registry) Active() []MatchedAlert {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]MatchedAlert, 0, len(r.active))
	for _, rec := range r.active {
		out = append(out, rec.alert)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// EndedRecord is one entry of Ended(): the alert plus why and when it ended.
type EndedRecord struct {
	Alert  MatchedAlert
	Reason string // "cancelled" | "expired" | "dropped" (vanished)
	At     time.Time
	Source string // human-readable note, e.g. "no longer listed by NWS"
}

// Ended returns every ended alert still retained (not yet purged).
func (r *Registry) Ended() []EndedRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]EndedRecord, 0, len(r.ended))
	for _, rec := range r.ended {
		if rec.alert.State == AlertStateSuperseded {
			continue // history-only; never listed as "ended" for the panel
		}
		at := rec.cancelledAt
		reason := "cancelled"
		if at.IsZero() {
			at = rec.expiredAt
			reason = string(rec.alert.EndedReason)
			if reason == "" {
				reason = "expired"
			}
		}
		out = append(out, EndedRecord{Alert: rec.alert, Reason: reason, At: at, Source: rec.alert.EndedReason})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alert.ID < out[j].Alert.ID })
	return out
}

// Apply ingests one successful decode (the full current active set from the
// provider) and returns what changed. now is used to stamp FirstSeenAt/
// UpdatedAt/FetchedAt on new and changed records.
func (r *Registry) Apply(alerts []Alert, now time.Time) []Change {
	r.mu.Lock()
	defer r.mu.Unlock()

	var changes []Change
	seenThisBatch := map[string]bool{}
	cancelledThisBatch := map[string]bool{}

	// Cancels first, so an Update (or the very alert being cancelled)
	// arriving in the same batch as its Cancel sees the up-to-date state.
	for _, a := range alerts {
		if a.MessageType != MessageTypeCancel {
			continue
		}
		for _, refID := range a.ReplacesIDs() {
			cancelledThisBatch[refID] = true
			if rec, ok := r.active[refID]; ok {
				rec.alert.State = AlertStateCancelled
				rec.alert.EndedReason = "cancelled"
				at := a.Sent
				rec.alert.EndedAt = &at
				rec.cancelledAt = at
				delete(r.active, refID)
				r.ended[refID] = rec
				changes = append(changes, Change{Kind: ChangeCancelled, Alert: rec.alert})
			}
			// A cancel of an id we never saw, or already ended, is ignored.
		}
	}

	for _, a := range alerts {
		if a.MessageType == MessageTypeCancel {
			continue
		}
		seenThisBatch[a.ID] = true

		if cancelledThisBatch[a.ID] {
			// The same batch cancelled this exact id; do not resurrect it
			// just because the feed also still listed it.
			continue
		}

		if _, ok := r.active[a.ID]; ok {
			continue // duplicate id, identical version already active: no-op
		}

		refIDs := a.ReplacesIDs()
		prevID := ""
		if len(refIDs) > 0 {
			prevID = refIDs[0]
		}

		if prevID == "" {
			// Brand new alert, no reference at all.
			ma := newActiveRecord(a, now)
			r.active[a.ID] = &record{alert: ma}
			changes = append(changes, Change{Kind: ChangeAdded, Alert: ma})
			continue
		}

		if prevActive, ok := r.active[prevID]; ok {
			// Supersedes the currently-active predecessor. The predecessor
			// moves to "superseded" — retained only for Prev/History lookups,
			// never surfaced by Ended() (that is the expired/cancelled/
			// dropped panel list; SQLite is the only place "superseded"
			// exists, per the wire contract).
			prev := prevActive.alert
			ma := newActiveRecord(a, now)
			ma.NetAckedAtCopy(prev) // preserve ack unless the Manager clears it on escalation
			delete(r.active, prevID)
			prevActive.alert.State = AlertStateSuperseded
			prevActive.alert.ReplacedBy = a.ID
			r.ended[prevID] = prevActive
			r.active[a.ID] = &record{alert: ma, prevID: prevID}
			changes = append(changes, Change{Kind: ChangeUpdated, Alert: ma, Prev: &prev})
			continue
		}

		if prevEnded, ok := r.ended[prevID]; ok {
			cutoff := prevEnded.cancelledAt
			if cutoff.IsZero() {
				cutoff = prevEnded.expiredAt
			}
			if !cutoff.IsZero() && a.Sent.Before(cutoff) {
				// Out-of-order Update older than the thing that ended its
				// reference chain: discarded, no resurrection.
				continue
			}
			ma := newActiveRecord(a, now)
			rec := &record{alert: ma, prevID: prevID}
			r.active[a.ID] = rec
			prev := prevEnded.alert
			changes = append(changes, Change{Kind: ChangeAdded, Alert: ma, Prev: &prev})
			continue
		}

		// References an id we have never seen at all: treated as new.
		ma := newActiveRecord(a, now)
		r.active[a.ID] = &record{alert: ma, prevID: prevID}
		changes = append(changes, Change{Kind: ChangeAdded, Alert: ma})
	}

	// Anything still active that this batch never mentioned (and did not
	// just get cancelled/superseded above) has vanished from the feed.
	for id, rec := range r.active {
		if seenThisBatch[id] {
			continue
		}
		rec.alert.State = AlertStateDropped
		rec.alert.EndedReason = "dropped"
		at := now
		rec.alert.EndedAt = &at
		delete(r.active, id)
		r.ended[id] = rec
		changes = append(changes, Change{Kind: ChangeVanished, Alert: rec.alert})
	}

	return changes
}

// ApplyFailure records a failed poll attempt. It never vanishes anything —
// only a successful decode can do that.
func (r *Registry) ApplyFailure(err error) []Change { return nil }

// Tick expires active alerts whose EndTime() has passed. now is the wall
// clock; the recorded ExpiredAt is the alert's own end time, not now.
func (r *Registry) Tick(now time.Time) []Change {
	r.mu.Lock()
	defer r.mu.Unlock()

	var changes []Change
	for id, rec := range r.active {
		end := rec.alert.EndsAt
		if end.IsZero() {
			end = rec.alert.EndTime()
		}
		if now.After(end) {
			rec.alert.State = AlertStateExpired
			rec.alert.EndedReason = "clock"
			rec.expiredAt = end
			endedAt := end
			rec.alert.EndedAt = &endedAt
			delete(r.active, id)
			r.ended[id] = rec
			changes = append(changes, Change{Kind: ChangeExpired, Alert: rec.alert})
		}
	}
	return changes
}

// SetNetAck records the NCS "ack for net" on an alert, active or ended.
func (r *Registry) SetNetAck(id string, ack NetAck) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.active[id]; ok {
		a := ack
		rec.alert.AckedForNet = &a
		return true
	}
	if rec, ok := r.ended[id]; ok {
		a := ack
		rec.alert.AckedForNet = &a
		return true
	}
	return false
}

// UpdateFields lets the Manager set derived fields (proximity, notify class,
// affects, ...) on an existing record — active or ended — without disturbing
// the registry's own supersession bookkeeping.
func (r *Registry) UpdateFields(id string, fn func(*MatchedAlert)) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.active[id]; ok {
		fn(&rec.alert)
		return true
	}
	if rec, ok := r.ended[id]; ok {
		fn(&rec.alert)
		return true
	}
	return false
}

// ClearNetAck removes a net ack, active or ended. The Manager calls this when
// an update escalates an already-acked alert (the ack no longer applies to
// the new, louder version).
func (r *Registry) ClearNetAck(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.active[id]; ok {
		rec.alert.AckedForNet = nil
		return true
	}
	if rec, ok := r.ended[id]; ok {
		rec.alert.AckedForNet = nil
		return true
	}
	return false
}

// PurgeEndedBefore removes ended records whose EndedAt is older than cutoff.
func (r *Registry) PurgeEndedBefore(cutoff time.Time) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for id, rec := range r.ended {
		if rec.alert.EndedAt != nil && rec.alert.EndedAt.Before(cutoff) {
			delete(r.ended, id)
			n++
		}
	}
	return n
}

// Snapshot captures the full registry state for persistence.
type RegistrySnapshot struct {
	Active []snapshotRecord
	Ended  []snapshotRecord
}

type snapshotRecord struct {
	Alert  MatchedAlert
	PrevID string
}

// Snapshot returns the current state for the store to persist.
func (r *Registry) Snapshot() RegistrySnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	var snap RegistrySnapshot
	for _, rec := range r.active {
		snap.Active = append(snap.Active, snapshotRecord{Alert: rec.alert, PrevID: rec.prevID})
	}
	for _, rec := range r.ended {
		snap.Ended = append(snap.Ended, snapshotRecord{Alert: rec.alert, PrevID: rec.prevID})
	}
	return snap
}

// Restore replaces the registry's contents with a previously captured
// snapshot (restart hydration).
func (r *Registry) Restore(snap RegistrySnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = map[string]*record{}
	r.ended = map[string]*record{}
	for _, sr := range snap.Active {
		r.active[sr.Alert.ID] = &record{alert: sr.Alert, prevID: sr.PrevID}
	}
	for _, sr := range snap.Ended {
		rec := &record{alert: sr.Alert, prevID: sr.PrevID}
		if sr.Alert.EndedReason == "cancelled" && sr.Alert.EndedAt != nil {
			rec.cancelledAt = *sr.Alert.EndedAt
		}
		if sr.Alert.EndedReason == "clock" && sr.Alert.EndedAt != nil {
			rec.expiredAt = *sr.Alert.EndedAt
		}
		r.ended[sr.Alert.ID] = rec
	}
}

func newActiveRecord(a Alert, now time.Time) MatchedAlert {
	return MatchedAlert{
		Alert:       a,
		State:       AlertStateActive,
		EndsAt:      a.EndTime(),
		FetchedAt:   now,
		FirstSeenAt: now,
		UpdatedAt:   now,
	}
}

// NetAckedAtCopy carries the previous version's net-ack forward onto ma
// unless the caller (Manager, once it knows whether this update was an
// escalation) clears it. It is exported as a method on MatchedAlert so the
// Manager can call it explicitly too.
func (ma *MatchedAlert) NetAckedAtCopy(prev MatchedAlert) {
	if prev.AckedForNet != nil {
		ack := *prev.AckedForNet
		ma.AckedForNet = &ack
	}
}
