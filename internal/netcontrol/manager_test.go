package netcontrol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/netprofile"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
)

// eventTypesContain drains every pending event and reports whether any of
// them has the given type. logEvent always emits its own
// EventTimelineEntry alongside a caller's more specific event, so tests
// that call it must check membership rather than assume a fixed position.
func eventTypesContain(mgr *Manager, want string) bool {
	found := false
	for {
		select {
		case evt := <-mgr.Events():
			if evt.Type == want {
				found = true
			}
		default:
			return found
		}
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	tracker := station.NewMemoryTracker(config.StationConfig{
		StaleTimeout:   time.Hour,
		TrackMaxPoints: 10,
		DedupWindow:    30 * time.Second,
	})

	return NewManager(s, tracker)
}

func TestRootCallsign(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"KD7BBC", "KD7BBC"},
		{"KD7BBC-9", "KD7BBC"},
		{"KD7BBC-15", "KD7BBC"},
		{"W1AW", "W1AW"},
		{"W1AW-0", "W1AW"},
		{"N0CALL-1", "N0CALL"},
	}

	for _, tt := range tests {
		got := RootCallsign(tt.input)
		if got != tt.want {
			t.Errorf("RootCallsign(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNetLifecycle(t *testing.T) {
	mgr := newTestManager(t)

	// Create
	n, err := mgr.CreateNet(store.Net{
		Name:        "Emergency Net",
		Frequency:   "146.520 MHz",
		NCSCallsign: "KD7BBC",
		NCSUserID:   "user-1",
	})
	if err != nil {
		t.Fatalf("CreateNet failed: %v", err)
	}
	if n.ID == "" {
		t.Error("expected non-empty ID")
	}
	if n.Status != StatusDraft {
		t.Errorf("status: got %q, want %q", n.Status, StatusDraft)
	}

	// Open
	if err := mgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet failed: %v", err)
	}
	opened, ok := mgr.GetNet(n.ID)
	if !ok {
		t.Fatal("net not found after open")
	}
	if opened.Status != StatusOpen {
		t.Errorf("status: got %q, want %q", opened.Status, StatusOpen)
	}
	if opened.OpenedAt == nil {
		t.Error("openedAt should be set")
	}

	// ActiveNet
	active := mgr.ActiveNet()
	if active == nil {
		t.Fatal("expected active net")
	}
	if active.ID != n.ID {
		t.Errorf("active net ID: got %q, want %q", active.ID, n.ID)
	}

	// Close
	closedNet, summary, err := mgr.CloseNet(n.ID)
	if err != nil {
		t.Fatalf("CloseNet failed: %v", err)
	}
	if closedNet.Status != StatusClosed {
		t.Errorf("status: got %q, want %q", closedNet.Status, StatusClosed)
	}
	if closedNet.ClosedAt == nil {
		t.Error("closedAt should be set")
	}
	if summary == nil {
		t.Fatal("expected summary")
	}
	if summary.Name != "Emergency Net" {
		t.Errorf("summary name: got %q, want %q", summary.Name, "Emergency Net")
	}

	// After close, no active net.
	if mgr.ActiveNet() != nil {
		t.Error("expected no active net after close")
	}
}

func TestCreateNetValidation(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.CreateNet(store.Net{Name: ""})
	if err == nil {
		t.Error("expected error for empty name")
	}

	_, err = mgr.CreateNet(store.Net{Name: "   "})
	if err == nil {
		t.Error("expected error for whitespace-only name")
	}
}

// TestCreateNetWxSlicesNeverNil: a create request that (as every real one
// does — the "new net" form has no weather-watch fields) leaves
// WxExtraZones/WxInterruptEvents unset must not hand the caller `null` for
// either. Unlike SetWxWatch (which already normalizes with
// append([]string{}, ...)), CreateNet used to return and cache the raw
// zero-value nil slices verbatim — GetNet/GetNets read straight from that
// same in-memory cache, so every GET /nets and GET /nets/{id} would echo
// `null` for the life of the process, until a restart reloaded from SQLite
// (whose LoadNet/LoadNets already normalize). TS declares both fields
// non-optional string[].
func TestCreateNetWxSlicesNeverNil(t *testing.T) {
	mgr := newTestManager(t)

	n, err := mgr.CreateNet(store.Net{Name: "Test Net"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if n.WxExtraZones == nil {
		t.Error("CreateNet result: WxExtraZones = nil, want non-nil empty slice")
	}
	if n.WxInterruptEvents == nil {
		t.Error("CreateNet result: WxInterruptEvents = nil, want non-nil empty slice")
	}

	// The bug was specifically that the in-memory cache (read by GetNet/
	// GetNets, not just the create response) kept the nil slices.
	cached, ok := mgr.GetNet(n.ID)
	if !ok {
		t.Fatal("GetNet: not found")
	}
	if cached.WxExtraZones == nil {
		t.Error("GetNet: WxExtraZones = nil, want non-nil empty slice")
	}
	if cached.WxInterruptEvents == nil {
		t.Error("GetNet: WxInterruptEvents = nil, want non-nil empty slice")
	}
}

func TestCheckInBasic(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, err := mgr.CheckIn(n.ID, "KD7BBC", "routine", "")
	if err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}
	if ci.Callsign != "KD7BBC" {
		t.Errorf("callsign: got %q, want %q", ci.Callsign, "KD7BBC")
	}
	if ci.Traffic != TrafficRoutine {
		t.Errorf("traffic: got %q, want %q", ci.Traffic, TrafficRoutine)
	}
	if ci.Status != OpAvailable {
		t.Errorf("status: got %q, want %q", ci.Status, OpAvailable)
	}

	cis := mgr.GetCheckIns(n.ID)
	if len(cis) != 1 {
		t.Fatalf("expected 1 check-in, got %d", len(cis))
	}
}

func TestCheckInCallsignNormalization(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "  kd7bbc  ", "", "")
	if ci.Callsign != "KD7BBC" {
		t.Errorf("callsign should be uppercased and trimmed: got %q", ci.Callsign)
	}
	if ci.Traffic != TrafficNone {
		t.Errorf("traffic should default to none: got %q", ci.Traffic)
	}
}

func TestCheckInValidation(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	_, err := mgr.CheckIn(n.ID, "", "", "")
	if err == nil {
		t.Error("expected error for empty callsign")
	}

	_, err = mgr.CheckIn("nonexistent", "KD7BBC", "", "")
	if err == nil {
		t.Error("expected error for nonexistent net")
	}
}

func TestCheckInAutoPopulateFromTracker(t *testing.T) {
	mgr := newTestManager(t)

	// Add a station to the tracker.
	mgr.tracker.Update(station.Station{
		Callsign:  "KD7BBC",
		SSID:      0,
		LastHeard: time.Now(),
		Position: &station.Position{
			Lat: 34.0522,
			Lon: -118.2437,
		},
		Comment: "Mobile",
	})

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	if ci.Lat == nil || *ci.Lat != 34.0522 {
		t.Errorf("lat should be auto-populated from tracker: got %v", ci.Lat)
	}
	if ci.Lon == nil || *ci.Lon != -118.2437 {
		t.Errorf("lon should be auto-populated from tracker: got %v", ci.Lon)
	}
}

func TestUpdateCheckIn(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")

	// Update status.
	ci.Status = OpAssigned
	updated, err := mgr.UpdateCheckIn(*ci)
	if err != nil {
		t.Fatalf("UpdateCheckIn failed: %v", err)
	}
	if updated.Status != OpAssigned {
		t.Errorf("status: got %q, want %q", updated.Status, OpAssigned)
	}
}

func TestCheckOut(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")

	if err := mgr.CheckOut(n.ID, ci.ID); err != nil {
		t.Fatalf("CheckOut failed: %v", err)
	}

	cis := mgr.GetCheckIns(n.ID)
	if len(cis) != 1 {
		t.Fatalf("expected 1 check-in, got %d", len(cis))
	}
	if cis[0].Status != OpReleased {
		t.Errorf("status: got %q, want %q", cis[0].Status, OpReleased)
	}
	if cis[0].CheckedOutAt == nil {
		t.Error("checkedOutAt should be set")
	}
}

func TestMissionCRUD(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	// Create
	m, err := mgr.CreateMission(store.NetMission{
		NetID:    n.ID,
		Title:    "Deploy to shelter",
		Priority: "priority",
	})
	if err != nil {
		t.Fatalf("CreateMission failed: %v", err)
	}
	if m.ID == "" {
		t.Error("expected non-empty mission ID")
	}
	if m.Status != "open" {
		t.Errorf("status: got %q, want %q", m.Status, "open")
	}

	// Update
	m.Status = "complete"
	updated, err := mgr.UpdateMission(*m)
	if err != nil {
		t.Fatalf("UpdateMission failed: %v", err)
	}
	if updated.Status != "complete" {
		t.Errorf("status: got %q, want %q", updated.Status, "complete")
	}
	if updated.CompletedAt == nil {
		t.Error("completedAt should be set when status is complete")
	}

	missions := mgr.GetMissions(n.ID)
	if len(missions) != 1 {
		t.Fatalf("expected 1 mission, got %d", len(missions))
	}
}

func TestMissionValidation(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	_, err := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: ""})
	if err == nil {
		t.Error("expected error for empty mission title")
	}
}

func TestAddNote(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	note, err := mgr.AddNote(store.NetNote{
		NetID:      n.ID,
		AuthorID:   "user-1",
		AuthorName: "Alice",
		Content:    "Good signal from shelter",
	})
	if err != nil {
		t.Fatalf("AddNote failed: %v", err)
	}
	if note.ID == "" {
		t.Error("expected non-empty note ID")
	}

	notes, err := mgr.GetNotes(n.ID)
	if err != nil {
		t.Fatalf("GetNotes failed: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
}

func TestNoteValidation(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	_, err := mgr.AddNote(store.NetNote{NetID: n.ID, Content: ""})
	if err == nil {
		t.Error("expected error for empty note content")
	}
}

func TestRollCall(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci1, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	ci2, _ := mgr.CheckIn(n.ID, "W1AW", "", "")

	// Initiate roll call — should increment missed count for all active.
	if err := mgr.InitiateRollCall(n.ID); err != nil {
		t.Fatalf("InitiateRollCall failed: %v", err)
	}

	cis := mgr.GetCheckIns(n.ID)
	for _, ci := range cis {
		if ci.MissedRollCalls != 1 {
			t.Errorf("%s missed roll calls: got %d, want 1", ci.Callsign, ci.MissedRollCalls)
		}
	}

	// Record response from KD7BBC only.
	if err := mgr.RecordRollCallResponse(n.ID, ci1.ID); err != nil {
		t.Fatalf("RecordRollCallResponse failed: %v", err)
	}

	cis = mgr.GetCheckIns(n.ID)
	for _, ci := range cis {
		if ci.ID == ci1.ID && ci.MissedRollCalls != 0 {
			t.Errorf("KD7BBC missed should be 0 after response, got %d", ci.MissedRollCalls)
		}
		if ci.ID == ci2.ID && ci.MissedRollCalls != 1 {
			t.Errorf("W1AW missed should still be 1, got %d", ci.MissedRollCalls)
		}
	}
}

func TestTransferNCS(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{
		Name:        "Test Net",
		NCSCallsign: "KD7BBC",
		NCSUserID:   "user-1",
	})
	mgr.OpenNet(n.ID)

	if err := mgr.TransferNCS(n.ID, "W1AW", "user-2"); err != nil {
		t.Fatalf("TransferNCS failed: %v", err)
	}

	updated, _ := mgr.GetNet(n.ID)
	if updated.NCSCallsign != "W1AW" {
		t.Errorf("NCS callsign: got %q, want %q", updated.NCSCallsign, "W1AW")
	}
	if updated.NCSUserID != "user-2" {
		t.Errorf("NCS user ID: got %q, want %q", updated.NCSUserID, "user-2")
	}
}

func TestSearchOperators(t *testing.T) {
	mgr := newTestManager(t)

	// Add stations.
	mgr.tracker.Update(station.Station{Callsign: "KG4YFA", SSID: 0, LastHeard: time.Now()})
	mgr.tracker.Update(station.Station{Callsign: "KD7BBC", SSID: 9, LastHeard: time.Now()})
	mgr.tracker.Update(station.Station{Callsign: "W1AW", SSID: 0, LastHeard: time.Now()})

	tests := []struct {
		query string
		want  int
	}{
		{"KD7", 1},
		{"YFA", 1}, // Substring match, not just prefix.
		{"W1AW", 1},
		{"", 0},
		{"ZZZZZ", 0},
	}

	for _, tt := range tests {
		results := mgr.SearchOperators(tt.query)
		if len(results) != tt.want {
			t.Errorf("SearchOperators(%q): got %d results, want %d", tt.query, len(results), tt.want)
		}
	}
}

func TestTimelineEvents(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	// Drain events channel.
	drainEvents(mgr)

	mgr.CheckIn(n.ID, "KD7BBC", "routine", "")
	drainEvents(mgr)

	events, err := mgr.GetEvents(n.ID)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	// Should have: net_opened, checkin
	if len(events) < 2 {
		t.Errorf("expected at least 2 timeline events, got %d", len(events))
	}

	// Verify event types.
	types := make(map[string]bool)
	for _, e := range events {
		types[e.Type] = true
	}
	if !types["net_opened"] {
		t.Error("expected net_opened event")
	}
	if !types["checkin"] {
		t.Error("expected checkin event")
	}
}

func TestLoadPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	tracker := station.NewMemoryTracker(config.StationConfig{
		StaleTimeout:   time.Hour,
		TrackMaxPoints: 10,
		DedupWindow:    30 * time.Second,
	})

	// Create and populate.
	mgr1 := NewManager(s, tracker)
	n, _ := mgr1.CreateNet(store.Net{Name: "Persist Test"})
	mgr1.OpenNet(n.ID)
	mgr1.CheckIn(n.ID, "KD7BBC", "routine", "")
	mgr1.CreateMission(store.NetMission{NetID: n.ID, Title: "Test Mission"})

	// Create new manager and load from store.
	mgr2 := NewManager(s, tracker)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	net, ok := mgr2.GetNet(n.ID)
	if !ok {
		t.Fatal("net not found after reload")
	}
	if net.Name != "Persist Test" {
		t.Errorf("name: got %q, want %q", net.Name, "Persist Test")
	}

	cis := mgr2.GetCheckIns(n.ID)
	if len(cis) != 1 {
		t.Errorf("expected 1 check-in after reload, got %d", len(cis))
	}

	missions := mgr2.GetMissions(n.ID)
	if len(missions) != 1 {
		t.Errorf("expected 1 mission after reload, got %d", len(missions))
	}

	s.Close()
}

func TestAutoMarkMissingAfterTwoRollCalls(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	drainEvents(mgr)

	// Two roll calls without response.
	mgr.InitiateRollCall(n.ID)
	mgr.InitiateRollCall(n.ID)
	drainEvents(mgr)

	cis := mgr.GetCheckIns(n.ID)
	for _, c := range cis {
		if c.ID == ci.ID {
			if c.Status != OpMissing {
				t.Errorf("expected status %q after 2 missed roll calls, got %q", OpMissing, c.Status)
			}
			if c.MissedRollCalls != 2 {
				t.Errorf("expected 2 missed roll calls, got %d", c.MissedRollCalls)
			}
		}
	}
}

func TestAutoMarkMissingSkipsReleased(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	mgr.CheckOut(n.ID, ci.ID)
	drainEvents(mgr)

	// Two roll calls — released operator should not be affected.
	mgr.InitiateRollCall(n.ID)
	mgr.InitiateRollCall(n.ID)

	cis := mgr.GetCheckIns(n.ID)
	for _, c := range cis {
		if c.ID == ci.ID {
			if c.Status != OpReleased {
				t.Errorf("released operator status changed: got %q", c.Status)
			}
			if c.MissedRollCalls != 0 {
				t.Errorf("released operator missed roll calls should be 0, got %d", c.MissedRollCalls)
			}
		}
	}
}

func TestCheckInSourceAprs(t *testing.T) {
	mgr := newTestManager(t)

	// Add station with position to tracker.
	mgr.tracker.Update(station.Station{
		Callsign:  "KD7BBC",
		SSID:      0,
		LastHeard: time.Now(),
		Position: &station.Position{
			Lat: 34.0522,
			Lon: -118.2437,
		},
	})

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	if ci.Source != "aprs" {
		t.Errorf("source: got %q, want %q", ci.Source, "aprs")
	}
}

func TestCheckInSourceVoice(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	// Unknown station — no tracker entry.
	ci, _ := mgr.CheckIn(n.ID, "UNKNOWN", "", "")
	if ci.Source != "voice" {
		t.Errorf("source: got %q, want %q", ci.Source, "voice")
	}
}

func TestAssignMission(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Deploy"})
	drainEvents(mgr)

	updated, err := mgr.AssignMission(n.ID, ci.ID, m.ID)
	if err != nil {
		t.Fatalf("AssignMission failed: %v", err)
	}
	if len(updated.MissionIDs) != 1 || updated.MissionIDs[0] != m.ID {
		t.Errorf("missionIds: got %v, want [%s]", updated.MissionIDs, m.ID)
	}
	if updated.Status != OpAssigned {
		t.Errorf("status: got %q, want %q", updated.Status, OpAssigned)
	}
}

func TestAssignMultipleMissions(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	m1, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Mission A"})
	m2, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Mission B"})
	drainEvents(mgr)

	mgr.AssignMission(n.ID, ci.ID, m1.ID)
	updated, err := mgr.AssignMission(n.ID, ci.ID, m2.ID)
	if err != nil {
		t.Fatalf("AssignMission second failed: %v", err)
	}
	if len(updated.MissionIDs) != 2 {
		t.Fatalf("expected 2 mission IDs, got %d", len(updated.MissionIDs))
	}
	if updated.MissionIDs[0] != m1.ID || updated.MissionIDs[1] != m2.ID {
		t.Errorf("missionIds: got %v, want [%s %s]", updated.MissionIDs, m1.ID, m2.ID)
	}
}

func TestAssignMissionRejectDuplicate(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Deploy"})
	drainEvents(mgr)

	mgr.AssignMission(n.ID, ci.ID, m.ID)
	_, err := mgr.AssignMission(n.ID, ci.ID, m.ID)
	if err == nil {
		t.Error("expected error for duplicate assignment")
	}
}

func TestUnassignSpecificMission(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	m1, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Mission A"})
	m2, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Mission B"})
	mgr.AssignMission(n.ID, ci.ID, m1.ID)
	mgr.AssignMission(n.ID, ci.ID, m2.ID)
	drainEvents(mgr)

	updated, err := mgr.UnassignMission(n.ID, ci.ID, m1.ID)
	if err != nil {
		t.Fatalf("UnassignMission failed: %v", err)
	}
	if len(updated.MissionIDs) != 1 || updated.MissionIDs[0] != m2.ID {
		t.Errorf("missionIds: got %v, want [%s]", updated.MissionIDs, m2.ID)
	}
	// Should still be assigned (has one mission left).
	if updated.Status != OpAssigned {
		t.Errorf("status: got %q, want %q", updated.Status, OpAssigned)
	}
}

func TestUnassignLastMissionRestoresAvailable(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Deploy"})
	mgr.AssignMission(n.ID, ci.ID, m.ID)
	drainEvents(mgr)

	updated, err := mgr.UnassignMission(n.ID, ci.ID, m.ID)
	if err != nil {
		t.Fatalf("UnassignMission failed: %v", err)
	}
	if len(updated.MissionIDs) != 0 {
		t.Errorf("missionIds should be empty, got %v", updated.MissionIDs)
	}
	if updated.Status != OpAvailable {
		t.Errorf("status: got %q, want %q", updated.Status, OpAvailable)
	}
}

func TestUnassignAllMissions(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	m1, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Mission A"})
	m2, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Mission B"})
	mgr.AssignMission(n.ID, ci.ID, m1.ID)
	mgr.AssignMission(n.ID, ci.ID, m2.ID)
	drainEvents(mgr)

	updated, err := mgr.UnassignAllMissions(n.ID, ci.ID)
	if err != nil {
		t.Fatalf("UnassignAllMissions failed: %v", err)
	}
	if len(updated.MissionIDs) != 0 {
		t.Errorf("missionIds should be empty, got %v", updated.MissionIDs)
	}
	if updated.Status != OpAvailable {
		t.Errorf("status: got %q, want %q", updated.Status, OpAvailable)
	}
}

func TestExportRosterCSV(t *testing.T) {
	checkIns := []store.NetCheckIn{
		{
			Callsign:     "KD7BBC",
			TacticalCall: "Shelter-1",
			OperatorName: "Bob",
			Status:       "available",
			Traffic:      "routine",
			Source:       "aprs",
			Location:     "Downtown",
			CheckedInAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			LastHeard:    time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	if err := ExportRosterCSV(&buf, checkIns, nil); err != nil {
		t.Fatalf("ExportRosterCSV failed: %v", err)
	}

	csv := buf.String()
	if !strings.Contains(csv, "callsign,tacticalCall,operatorName") {
		t.Error("CSV missing header")
	}
	if !strings.Contains(csv, "KD7BBC") {
		t.Error("CSV missing operator data")
	}
	if !strings.Contains(csv, "Shelter-1") {
		t.Error("CSV missing tacticalCall")
	}
	if !strings.Contains(csv, "aprs") {
		t.Error("CSV missing source field")
	}
}

// --- Tracked Devices Tests ---

func TestDiscoverDevicesSSID(t *testing.T) {
	mgr := newTestManager(t)

	// Add multiple SSID variants.
	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 0, LastHeard: time.Now(),
		Position: &station.Position{Lat: 34.0, Lon: -118.0},
	})
	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 4, LastHeard: time.Now(),
		Position: &station.Position{Lat: 34.1, Lon: -118.1},
	})
	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 7, LastHeard: time.Now().Add(time.Minute),
		Position: &station.Position{Lat: 34.2, Lon: -118.2},
	})

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, err := mgr.CheckIn(n.ID, "KG4YFA", "", "")
	if err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}

	if len(ci.TrackedStations) != 3 {
		t.Fatalf("expected 3 auto-linked devices, got %d", len(ci.TrackedStations))
	}
	for _, ts := range ci.TrackedStations {
		if !ts.AutoLinked {
			t.Errorf("expected all devices to be auto-linked, got %+v", ts)
		}
	}
}

func TestDiscoverDevicesNone(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, err := mgr.CheckIn(n.ID, "UNKNOWN", "", "")
	if err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}

	if len(ci.TrackedStations) != 0 {
		t.Errorf("expected 0 tracked stations for unknown callsign, got %d", len(ci.TrackedStations))
	}
}

func TestAddTrackedStationManual(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	drainEvents(mgr)

	updated, err := mgr.AddTrackedStation(n.ID, ci.ID, "A2SV-4")
	if err != nil {
		t.Fatalf("AddTrackedStation failed: %v", err)
	}

	if len(updated.TrackedStations) != 1 {
		t.Fatalf("expected 1 tracked station, got %d", len(updated.TrackedStations))
	}
	ts := updated.TrackedStations[0]
	if ts.Callsign != "A2SV-4" {
		t.Errorf("callsign: got %q, want %q", ts.Callsign, "A2SV-4")
	}
	if ts.AutoLinked {
		t.Error("manually added station should not be auto-linked")
	}
}

func TestAddTrackedStationDuplicate(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	drainEvents(mgr)

	mgr.AddTrackedStation(n.ID, ci.ID, "A2SV-4")
	_, err := mgr.AddTrackedStation(n.ID, ci.ID, "A2SV-4")
	if err == nil {
		t.Error("expected error for duplicate station")
	}
}

func TestRemoveTrackedStation(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	drainEvents(mgr)

	mgr.AddTrackedStation(n.ID, ci.ID, "A2SV-4")
	mgr.AddTrackedStation(n.ID, ci.ID, "KD7BBC-7")
	drainEvents(mgr)

	updated, err := mgr.RemoveTrackedStation(n.ID, ci.ID, "A2SV-4")
	if err != nil {
		t.Fatalf("RemoveTrackedStation failed: %v", err)
	}

	if len(updated.TrackedStations) != 1 {
		t.Fatalf("expected 1 tracked station after removal, got %d", len(updated.TrackedStations))
	}
	if updated.TrackedStations[0].Callsign != "KD7BBC-7" {
		t.Errorf("remaining station: got %q, want %q", updated.TrackedStations[0].Callsign, "KD7BBC-7")
	}

	// Verify index cleanup.
	mgr.mu.RLock()
	_, inIndex := mgr.trackedIndex["A2SV-4"]
	mgr.mu.RUnlock()
	if inIndex {
		t.Error("removed station should not be in trackedIndex")
	}
}

func TestOnStationUpdatePosition(t *testing.T) {
	mgr := newTestManager(t)

	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 4, LastHeard: time.Now(),
		Position: &station.Position{Lat: 34.0, Lon: -118.0},
	})

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KG4YFA", "", "")
	drainEvents(mgr)

	// Update the tracked station in the tracker.
	newTime := time.Now().Add(5 * time.Minute)
	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 4, LastHeard: newTime,
		Position: &station.Position{Lat: 35.0, Lon: -119.0},
	})

	mgr.OnStationUpdate("KG4YFA-4", &station.Position{Lat: 35.0, Lon: -119.0}, newTime)
	drainEvents(mgr)

	cis := mgr.GetCheckIns(n.ID)
	found := false
	for _, c := range cis {
		if c.ID == ci.ID {
			found = true
			if c.Lat == nil || *c.Lat != 35.0 {
				t.Errorf("lat should be updated to 35.0, got %v", c.Lat)
			}
			if c.Lon == nil || *c.Lon != -119.0 {
				t.Errorf("lon should be updated to -119.0, got %v", c.Lon)
			}
		}
	}
	if !found {
		t.Fatal("check-in not found after station update")
	}
}

func TestOnStationUpdateBestPosition(t *testing.T) {
	mgr := newTestManager(t)

	older := time.Now()
	newer := time.Now().Add(time.Minute)

	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 0, LastHeard: older,
		Position: &station.Position{Lat: 34.0, Lon: -118.0},
	})
	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 7, LastHeard: newer,
		Position: &station.Position{Lat: 35.0, Lon: -119.0},
	})

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KG4YFA", "", "")
	drainEvents(mgr)

	// The best position should be from the most recently heard device.
	if ci.Lat == nil || *ci.Lat != 35.0 {
		t.Errorf("lat should be 35.0 (most recent), got %v", ci.Lat)
	}
}

func TestOnStationUpdateIgnoresUntracked(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)
	mgr.CheckIn(n.ID, "KD7BBC", "", "")
	drainEvents(mgr)

	// Call with an untracked station — should be a no-op (no panic).
	mgr.OnStationUpdate("UNKNOWN-5", &station.Position{Lat: 40.0, Lon: -80.0}, time.Now())
}

func TestOnStationUpdateIgnoresReleased(t *testing.T) {
	mgr := newTestManager(t)

	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 0, LastHeard: time.Now(),
		Position: &station.Position{Lat: 34.0, Lon: -118.0},
	})

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KG4YFA", "", "")
	drainEvents(mgr)

	mgr.CheckOut(n.ID, ci.ID)
	drainEvents(mgr)

	// Update tracked station after checkout — should be no-op.
	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 0, LastHeard: time.Now().Add(10 * time.Minute),
		Position: &station.Position{Lat: 99.0, Lon: -99.0},
	})
	mgr.OnStationUpdate("KG4YFA", &station.Position{Lat: 99.0, Lon: -99.0}, time.Now().Add(10*time.Minute))

	cis := mgr.GetCheckIns(n.ID)
	for _, c := range cis {
		if c.ID == ci.ID && c.Lat != nil && *c.Lat == 99.0 {
			t.Error("position should NOT be updated for released operator")
		}
	}
}

func TestCheckOutCleansIndex(t *testing.T) {
	mgr := newTestManager(t)

	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 0, LastHeard: time.Now(),
		Position: &station.Position{Lat: 34.0, Lon: -118.0},
	})
	mgr.tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 4, LastHeard: time.Now(),
		Position: &station.Position{Lat: 34.1, Lon: -118.1},
	})

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KG4YFA", "", "")
	drainEvents(mgr)

	// Verify index has entries.
	mgr.mu.RLock()
	preCount := len(mgr.trackedIndex)
	mgr.mu.RUnlock()
	if preCount == 0 {
		t.Fatal("trackedIndex should have entries before checkout")
	}

	mgr.CheckOut(n.ID, ci.ID)
	drainEvents(mgr)

	// Verify index is cleaned.
	mgr.mu.RLock()
	postCount := len(mgr.trackedIndex)
	mgr.mu.RUnlock()
	if postCount != 0 {
		t.Errorf("expected 0 tracked index entries after checkout, got %d", postCount)
	}
}

func TestTrackedStationsRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	tracker := station.NewMemoryTracker(config.StationConfig{
		StaleTimeout:   time.Hour,
		TrackMaxPoints: 10,
		DedupWindow:    30 * time.Second,
	})

	// Add tracked station.
	tracker.Update(station.Station{
		Callsign: "KG4YFA", SSID: 0, LastHeard: time.Now(),
		Position: &station.Position{Lat: 34.0, Lon: -118.0},
	})

	// Create and populate.
	mgr1 := NewManager(s, tracker)
	n, _ := mgr1.CreateNet(store.Net{Name: "Roundtrip"})
	mgr1.OpenNet(n.ID)
	ci, _ := mgr1.CheckIn(n.ID, "KG4YFA", "", "")
	mgr1.AddTrackedStation(n.ID, ci.ID, "A2SV-4")

	// Create new manager and load.
	mgr2 := NewManager(s, tracker)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cis := mgr2.GetCheckIns(n.ID)
	if len(cis) != 1 {
		t.Fatalf("expected 1 check-in, got %d", len(cis))
	}
	if len(cis[0].TrackedStations) < 2 {
		t.Fatalf("expected at least 2 tracked stations after reload, got %d", len(cis[0].TrackedStations))
	}

	// Verify index was rebuilt.
	mgr2.mu.RLock()
	_, inIndex := mgr2.trackedIndex["A2SV-4"]
	mgr2.mu.RUnlock()
	if !inIndex {
		t.Error("A2SV-4 should be in trackedIndex after reload")
	}
}

// --- Auto-Unassign on Mission Completion Tests ---

func TestCompleteMissionAutoUnassigns(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci1, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	ci2, _ := mgr.CheckIn(n.ID, "W1AW", "", "")
	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Deploy"})
	mgr.AssignMission(n.ID, ci1.ID, m.ID)
	mgr.AssignMission(n.ID, ci2.ID, m.ID)
	drainEvents(mgr)

	// Complete the mission.
	m.Status = "complete"
	_, err := mgr.UpdateMission(*m)
	if err != nil {
		t.Fatalf("UpdateMission failed: %v", err)
	}
	drainEvents(mgr)

	// Both operators should be unassigned and available.
	cis := mgr.GetCheckIns(n.ID)
	for _, ci := range cis {
		if ci.ID == ci1.ID || ci.ID == ci2.ID {
			if len(ci.MissionIDs) != 0 {
				t.Errorf("%s should have no mission IDs, got %v", ci.Callsign, ci.MissionIDs)
			}
			if ci.Status != OpAvailable {
				t.Errorf("%s status should be %q, got %q", ci.Callsign, OpAvailable, ci.Status)
			}
		}
	}
}

func TestCompleteMissionKeepsOtherMissions(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	m1, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Mission A"})
	m2, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Mission B"})
	mgr.AssignMission(n.ID, ci.ID, m1.ID)
	mgr.AssignMission(n.ID, ci.ID, m2.ID)
	drainEvents(mgr)

	// Complete mission A only.
	m1.Status = "complete"
	mgr.UpdateMission(*m1)
	drainEvents(mgr)

	// Operator should still have mission B and remain assigned.
	cis := mgr.GetCheckIns(n.ID)
	for _, c := range cis {
		if c.ID == ci.ID {
			if len(c.MissionIDs) != 1 || c.MissionIDs[0] != m2.ID {
				t.Errorf("expected only mission B (%s), got %v", m2.ID, c.MissionIDs)
			}
			if c.Status != OpAssigned {
				t.Errorf("status should remain %q, got %q", OpAssigned, c.Status)
			}
		}
	}
}

func TestCompleteMissionLogsEvents(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci1, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	ci2, _ := mgr.CheckIn(n.ID, "W1AW", "", "")
	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Deploy"})
	mgr.AssignMission(n.ID, ci1.ID, m.ID)
	mgr.AssignMission(n.ID, ci2.ID, m.ID)
	drainEvents(mgr)

	m.Status = "complete"
	mgr.UpdateMission(*m)
	drainEvents(mgr)

	events, err := mgr.GetEvents(n.ID)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	// Look for auto-unassign events in the timeline.
	autoUnassignCount := 0
	for _, e := range events {
		if e.Type == "assignment" && strings.Contains(e.Summary, "auto-unassigned") {
			autoUnassignCount++
		}
	}
	if autoUnassignCount != 2 {
		t.Errorf("expected 2 auto-unassign events, got %d", autoUnassignCount)
	}
}

func TestCompleteMissionNoOpWhenNoOperators(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Unassigned Mission"})
	drainEvents(mgr)

	// Complete a mission with no operators assigned — should not error.
	m.Status = "complete"
	_, err := mgr.UpdateMission(*m)
	if err != nil {
		t.Fatalf("UpdateMission failed: %v", err)
	}
}

func TestCompleteMissionSkipsReleased(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Deploy"})
	mgr.AssignMission(n.ID, ci.ID, m.ID)
	drainEvents(mgr)

	// Check out the operator (releases them).
	mgr.CheckOut(n.ID, ci.ID)
	drainEvents(mgr)

	// Complete the mission — released operator should be untouched.
	m.Status = "complete"
	mgr.UpdateMission(*m)
	drainEvents(mgr)

	cis := mgr.GetCheckIns(n.ID)
	for _, c := range cis {
		if c.ID == ci.ID {
			if c.Status != OpReleased {
				t.Errorf("released operator status should remain %q, got %q", OpReleased, c.Status)
			}
		}
	}
}

// --- Enhanced Notes Tests ---

func TestAddNoteWithCategory(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)
	drainEvents(mgr)

	note, err := mgr.AddNote(store.NetNote{
		NetID:      n.ID,
		AuthorID:   "user-1",
		AuthorName: "Alice",
		Content:    "Rider down at mile 32",
		Category:   "medical",
		Severity:   "urgent",
	})
	if err != nil {
		t.Fatalf("AddNote failed: %v", err)
	}
	if note.Category != "medical" {
		t.Errorf("category: got %q, want %q", note.Category, "medical")
	}
	if note.Severity != "urgent" {
		t.Errorf("severity: got %q, want %q", note.Severity, "urgent")
	}

	// Verify it appears in timeline with category prefix.
	events, _ := mgr.GetEvents(n.ID)
	found := false
	for _, e := range events {
		if e.Type == "note" && strings.Contains(e.Summary, "[MEDICAL]") {
			found = true
		}
	}
	if !found {
		t.Error("expected timeline event with [MEDICAL] prefix")
	}
}

func TestAddNoteAutoPin(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)
	drainEvents(mgr)

	// Non-urgent should NOT be pinned.
	note1, _ := mgr.AddNote(store.NetNote{
		NetID:      n.ID,
		AuthorID:   "user-1",
		AuthorName: "Bob",
		Content:    "Routine update",
		Severity:   "routine",
	})
	if note1.Pinned {
		t.Error("non-urgent note should not be auto-pinned")
	}

	// Urgent SHOULD be auto-pinned.
	note2, _ := mgr.AddNote(store.NetNote{
		NetID:      n.ID,
		AuthorID:   "user-1",
		AuthorName: "Bob",
		Content:    "Subject located, requesting helicopter",
		Severity:   "urgent",
	})
	if !note2.Pinned {
		t.Error("urgent note should be auto-pinned")
	}

	// Verify pinned flag persists through store.
	notes, _ := mgr.GetNotes(n.ID)
	for _, nn := range notes {
		if nn.ID == note2.ID && !nn.Pinned {
			t.Error("pinned flag should persist in store")
		}
	}
}

func TestAddNoteDefaultCategory(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)
	drainEvents(mgr)

	// Empty category should default to "general".
	note, _ := mgr.AddNote(store.NetNote{
		NetID:      n.ID,
		AuthorID:   "user-1",
		AuthorName: "Bob",
		Content:    "General observation",
	})
	if note.Category != "general" {
		t.Errorf("category: got %q, want %q", note.Category, "general")
	}

	// Empty severity should default to "info".
	if note.Severity != "info" {
		t.Errorf("severity: got %q, want %q", note.Severity, "info")
	}
}

func TestAddNoteMissionAttachment(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)
	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Deploy"})
	drainEvents(mgr)

	note, err := mgr.AddNote(store.NetNote{
		NetID:      n.ID,
		MissionID:  m.ID,
		AuthorID:   "user-1",
		AuthorName: "Alice",
		Content:    "Mission update: supplies delivered",
		Category:   "logistical",
	})
	if err != nil {
		t.Fatalf("AddNote with mission failed: %v", err)
	}
	if note.MissionID != m.ID {
		t.Errorf("missionId: got %q, want %q", note.MissionID, m.ID)
	}

	// Verify roundtrip from store.
	notes, _ := mgr.GetNotes(n.ID)
	found := false
	for _, nn := range notes {
		if nn.ID == note.ID && nn.MissionID == m.ID && nn.Category == "logistical" {
			found = true
		}
	}
	if !found {
		t.Error("note with missionId and category should persist")
	}
}

func TestAddNoteInvalidCategory(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)
	drainEvents(mgr)

	_, err := mgr.AddNote(store.NetNote{
		NetID:      n.ID,
		AuthorID:   "user-1",
		AuthorName: "Bob",
		Content:    "Test note",
		Category:   "invalid_category",
	})
	if err == nil {
		t.Error("expected error for invalid category")
	}
}

func TestSetOpsView(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Ops View Net"})
	mgr.OpenNet(n.ID)
	drainEvents(mgr)

	if err := mgr.SetOpsView(n.ID, 34.05, -118.24, 13); err != nil {
		t.Fatalf("SetOpsView failed: %v", err)
	}

	// Verify in-memory.
	updated, ok := mgr.GetNet(n.ID)
	if !ok {
		t.Fatal("net not found after SetOpsView")
	}
	if updated.OpsViewLat == nil || *updated.OpsViewLat != 34.05 {
		t.Errorf("opsViewLat: got %v, want 34.05", updated.OpsViewLat)
	}
	if updated.OpsViewLon == nil || *updated.OpsViewLon != -118.24 {
		t.Errorf("opsViewLon: got %v, want -118.24", updated.OpsViewLon)
	}
	if updated.OpsViewZoom == nil || *updated.OpsViewZoom != 13 {
		t.Errorf("opsViewZoom: got %v, want 13", updated.OpsViewZoom)
	}

	// Verify event emitted.
	select {
	case evt := <-mgr.Events():
		if evt.Type != EventNetUpdated {
			t.Errorf("expected %s event, got %s", EventNetUpdated, evt.Type)
		}
	default:
		t.Error("expected EventNetUpdated to be emitted")
	}
}

func TestSetOpsViewNetNotFound(t *testing.T) {
	mgr := newTestManager(t)

	err := mgr.SetOpsView("nonexistent", 34.05, -118.24, 13)
	if err == nil {
		t.Error("expected error for nonexistent net")
	}
}

func TestSetWxWatch(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Wx Watch Net"})
	drainEvents(mgr)

	updated, err := mgr.SetWxWatch(n.ID, 15, []string{"MIZ056", "MIC081"}, true, true, []string{"Tornado Warning"})
	if err != nil {
		t.Fatalf("SetWxWatch failed: %v", err)
	}
	if updated.WxBufferMiles != 15 {
		t.Errorf("WxBufferMiles = %v, want 15", updated.WxBufferMiles)
	}
	if len(updated.WxExtraZones) != 2 {
		t.Errorf("WxExtraZones = %v, want 2 entries", updated.WxExtraZones)
	}
	if !updated.WxMuteAdvisories || !updated.WxInterruptCustom {
		t.Errorf("wx bools not set: %+v", updated)
	}
	if len(updated.WxInterruptEvents) != 1 || updated.WxInterruptEvents[0] != "Tornado Warning" {
		t.Errorf("WxInterruptEvents = %v", updated.WxInterruptEvents)
	}

	// Reflected via GetNet (the in-memory cache SetWxWatch mutated).
	cached, ok := mgr.GetNet(n.ID)
	if !ok {
		t.Fatal("net not found after SetWxWatch")
	}
	if cached.WxBufferMiles != 15 {
		t.Errorf("GetNet WxBufferMiles = %v, want 15", cached.WxBufferMiles)
	}

	select {
	case evt := <-mgr.Events():
		if evt.Type != EventNetUpdated {
			t.Errorf("expected %s event, got %s", EventNetUpdated, evt.Type)
		}
	default:
		t.Error("expected EventNetUpdated to be emitted")
	}
}

func TestSetWxWatchNetNotFound(t *testing.T) {
	mgr := newTestManager(t)

	if _, err := mgr.SetWxWatch("nonexistent", 10, nil, false, false, nil); err == nil {
		t.Error("expected error for nonexistent net")
	}
}

func TestAddTimelineEvent(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Timeline Net"})
	drainEvents(mgr)

	if err := mgr.AddTimelineEvent(n.ID, EventWxAlert, "W8ABC", "Tornado Warning received IN"); err != nil {
		t.Fatalf("AddTimelineEvent failed: %v", err)
	}

	events, err := mgr.GetEvents(n.ID)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type == EventWxAlert && e.Callsign == "W8ABC" {
			found = true
		}
	}
	if !found {
		t.Errorf("wx_alert timeline event not persisted: %+v", events)
	}

	select {
	case evt := <-mgr.Events():
		if evt.Type != EventTimelineEntry {
			t.Errorf("expected %s event, got %s", EventTimelineEntry, evt.Type)
		}
	default:
		t.Error("expected EventTimelineEntry to be emitted")
	}
}

func TestAddTimelineEventNetNotFound(t *testing.T) {
	mgr := newTestManager(t)

	if err := mgr.AddTimelineEvent("nonexistent", EventWxAlert, "W8ABC", "should not persist"); err == nil {
		t.Error("expected error for nonexistent net")
	}
}

// TestAddTimelineEventWithDetails covers WP4's addition: same shape as
// AddTimelineEvent, plus a Details payload that must survive to the saved
// NetEvent and the emitted EventTimelineEntry.
func TestAddTimelineEventWithDetails(t *testing.T) {
	mgr := newTestManager(t)

	if err := mgr.AddTimelineEventWithDetails("nonexistent", EventTimelineEntry, "W8ABC", "should not persist", `{"bib":"412"}`); err == nil {
		t.Error("expected error for nonexistent net")
	}

	n, _ := mgr.CreateNet(store.Net{Name: "Timeline Details Net"})
	drainEvents(mgr)

	details := `{"bib":"412","withheld":true}`
	if err := mgr.AddTimelineEventWithDetails(n.ID, "medical_reported", "SAG 2", "MEDICAL from SAG 2", details); err != nil {
		t.Fatalf("AddTimelineEventWithDetails failed: %v", err)
	}

	events, err := mgr.GetEvents(n.ID)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type == "medical_reported" && e.Callsign == "SAG 2" {
			if e.Details != details {
				t.Errorf("saved NetEvent.Details = %q, want %q", e.Details, details)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("medical_reported timeline event not persisted: %+v", events)
	}

	select {
	case evt := <-mgr.Events():
		if evt.Type != EventTimelineEntry {
			t.Errorf("expected %s event, got %s", EventTimelineEntry, evt.Type)
		}
		ne, ok := evt.Data.(store.NetEvent)
		if !ok {
			t.Fatalf("event Data is %T, want store.NetEvent", evt.Data)
		}
		if ne.Details != details {
			t.Errorf("emitted NetEvent.Details = %q, want %q", ne.Details, details)
		}
	default:
		t.Error("expected EventTimelineEntry to be emitted")
	}
}

func TestToggleNotePin(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)
	drainEvents(mgr)

	// Create a non-pinned note.
	note, err := mgr.AddNote(store.NetNote{
		NetID:      n.ID,
		AuthorID:   "user-1",
		AuthorName: "Bob",
		Content:    "Routine observation",
		Severity:   "routine",
	})
	if err != nil {
		t.Fatalf("add note: %v", err)
	}
	if note.Pinned {
		t.Fatal("note should not be pinned initially")
	}
	drainEvents(mgr)

	// Pin it.
	updated, err := mgr.ToggleNotePin(n.ID, note.ID)
	if err != nil {
		t.Fatalf("toggle pin: %v", err)
	}
	if !updated.Pinned {
		t.Error("note should be pinned after toggle")
	}

	// Verify event emitted.
	select {
	case ev := <-mgr.Events():
		if ev.Type != EventTimelineEntry {
			t.Errorf("event type: got %q, want %q", ev.Type, EventTimelineEntry)
		}
	default:
		t.Error("expected event after pin toggle")
	}

	// Unpin it.
	updated, err = mgr.ToggleNotePin(n.ID, note.ID)
	if err != nil {
		t.Fatalf("toggle unpin: %v", err)
	}
	if updated.Pinned {
		t.Error("note should be unpinned after second toggle")
	}

	// Verify persistence.
	notes, _ := mgr.GetNotes(n.ID)
	for _, nn := range notes {
		if nn.ID == note.ID && nn.Pinned {
			t.Error("unpinned state should persist")
		}
	}
}

// drainEvents reads all pending events from the channel.
func drainEvents(mgr *Manager) {
	for {
		select {
		case <-mgr.Events():
		default:
			return
		}
	}
}

// --- Station Category Tests ---

func TestCheckInWithCategory(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, err := mgr.CheckIn(n.ID, "KD7BBC", "routine", "medical")
	if err != nil {
		t.Fatalf("CheckIn with category failed: %v", err)
	}
	if ci.Category != CatMedical {
		t.Errorf("category: got %q, want %q", ci.Category, CatMedical)
	}
}

func TestCheckInCategoryDefault(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, err := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	if err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}
	if ci.Category != CatGeneral {
		t.Errorf("category should default to %q, got %q", CatGeneral, ci.Category)
	}
}

func TestCheckInCategoryInvalid(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	_, err := mgr.CheckIn(n.ID, "KD7BBC", "", "bogus")
	if err == nil {
		t.Error("expected error for invalid category")
	}
}

func TestExportRosterCSVCategory(t *testing.T) {
	checkIns := []store.NetCheckIn{
		{
			Callsign:    "KD7BBC",
			Status:      "available",
			Traffic:     "routine",
			Category:    "medical",
			Source:      "aprs",
			CheckedInAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			LastHeard:   time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC),
		},
		{
			Callsign:    "W1AW",
			Status:      "available",
			Traffic:     "none",
			Category:    "",
			Source:      "voice",
			CheckedInAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			LastHeard:   time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	if err := ExportRosterCSV(&buf, checkIns, nil); err != nil {
		t.Fatalf("ExportRosterCSV failed: %v", err)
	}

	csv := buf.String()
	if !strings.Contains(csv, ",category,") {
		t.Error("CSV header missing category column")
	}
	if !strings.Contains(csv, "medical") {
		t.Error("CSV missing medical category value")
	}
	if !strings.Contains(csv, "general") {
		t.Error("CSV should default empty category to general")
	}
}

func TestCheckInCategoryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	tracker := station.NewMemoryTracker(config.StationConfig{
		StaleTimeout:   time.Hour,
		TrackMaxPoints: 10,
		DedupWindow:    30 * time.Second,
	})

	mgr1 := NewManager(s, tracker)
	n, _ := mgr1.CreateNet(store.Net{Name: "Category Persist"})
	mgr1.OpenNet(n.ID)
	mgr1.CheckIn(n.ID, "KD7BBC", "", "sag")

	// Load in new manager.
	mgr2 := NewManager(s, tracker)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cis := mgr2.GetCheckIns(n.ID)
	if len(cis) != 1 {
		t.Fatalf("expected 1 check-in, got %d", len(cis))
	}
	if cis[0].Category != CatSAG {
		t.Errorf("category after reload: got %q, want %q", cis[0].Category, CatSAG)
	}

	s.Close()
}

func TestPinStation(t *testing.T) {
	mgr := newTestManager(t)
	n, _ := mgr.CreateNet(store.Net{Name: "Pin Test"})
	mgr.OpenNet(n.ID)
	mgr.CheckIn(n.ID, "KD7BBC", "", "")
	mgr.CheckIn(n.ID, "W1AW", "", "")

	updated, err := mgr.PinStation(n.ID, "KD7BBC")
	if err != nil {
		t.Fatalf("PinStation failed: %v", err)
	}
	if len(updated.PinnedStations) != 1 || updated.PinnedStations[0] != "KD7BBC" {
		t.Errorf("expected [KD7BBC], got %v", updated.PinnedStations)
	}

	updated, err = mgr.PinStation(n.ID, "W1AW")
	if err != nil {
		t.Fatalf("PinStation (second) failed: %v", err)
	}
	if len(updated.PinnedStations) != 2 {
		t.Errorf("expected 2 pinned, got %d", len(updated.PinnedStations))
	}
}

func TestPinMax8(t *testing.T) {
	mgr := newTestManager(t)
	n, _ := mgr.CreateNet(store.Net{Name: "Max Pin Test"})
	mgr.OpenNet(n.ID)

	calls := []string{"AA1A", "BB2B", "CC3C", "DD4D", "EE5E", "FF6F", "GG7G", "HH8H", "II9I"}
	for _, cs := range calls {
		mgr.CheckIn(n.ID, cs, "", "")
	}

	// Pin 8 should succeed.
	for i := 0; i < 8; i++ {
		if _, err := mgr.PinStation(n.ID, calls[i]); err != nil {
			t.Fatalf("PinStation(%s) failed: %v", calls[i], err)
		}
	}

	// Pin 9th should fail.
	_, err := mgr.PinStation(n.ID, calls[8])
	if err == nil {
		t.Error("expected error for 9th pin, got nil")
	}
}

func TestPinDuplicate(t *testing.T) {
	mgr := newTestManager(t)
	n, _ := mgr.CreateNet(store.Net{Name: "Dup Pin Test"})
	mgr.OpenNet(n.ID)
	mgr.CheckIn(n.ID, "KD7BBC", "", "")

	mgr.PinStation(n.ID, "KD7BBC")
	_, err := mgr.PinStation(n.ID, "KD7BBC")
	if err == nil {
		t.Error("expected error for duplicate pin, got nil")
	}
}

func TestUnpin(t *testing.T) {
	mgr := newTestManager(t)
	n, _ := mgr.CreateNet(store.Net{Name: "Unpin Test"})
	mgr.OpenNet(n.ID)
	mgr.CheckIn(n.ID, "KD7BBC", "", "")
	mgr.CheckIn(n.ID, "W1AW", "", "")

	mgr.PinStation(n.ID, "KD7BBC")
	mgr.PinStation(n.ID, "W1AW")

	updated, err := mgr.UnpinStation(n.ID, "KD7BBC")
	if err != nil {
		t.Fatalf("UnpinStation failed: %v", err)
	}
	if len(updated.PinnedStations) != 1 || updated.PinnedStations[0] != "W1AW" {
		t.Errorf("expected [W1AW], got %v", updated.PinnedStations)
	}

	// Unpin non-existent.
	_, err = mgr.UnpinStation(n.ID, "N0CALL")
	if err == nil {
		t.Error("expected error for unpinning non-pinned callsign")
	}
}

func TestReorderPins(t *testing.T) {
	mgr := newTestManager(t)
	n, _ := mgr.CreateNet(store.Net{Name: "Reorder Test"})
	mgr.OpenNet(n.ID)
	mgr.CheckIn(n.ID, "KD7BBC", "", "")
	mgr.CheckIn(n.ID, "W1AW", "", "")
	mgr.CheckIn(n.ID, "N0CALL", "", "")

	mgr.PinStation(n.ID, "KD7BBC")
	mgr.PinStation(n.ID, "W1AW")
	mgr.PinStation(n.ID, "N0CALL")

	// Reorder.
	updated, err := mgr.ReorderPins(n.ID, []string{"N0CALL", "KD7BBC", "W1AW"})
	if err != nil {
		t.Fatalf("ReorderPins failed: %v", err)
	}
	if updated.PinnedStations[0] != "N0CALL" || updated.PinnedStations[1] != "KD7BBC" || updated.PinnedStations[2] != "W1AW" {
		t.Errorf("reorder mismatch: got %v", updated.PinnedStations)
	}

	// Wrong count.
	_, err = mgr.ReorderPins(n.ID, []string{"N0CALL", "KD7BBC"})
	if err == nil {
		t.Error("expected error for count mismatch")
	}

	// Unknown callsign.
	_, err = mgr.ReorderPins(n.ID, []string{"N0CALL", "KD7BBC", "FAKE"})
	if err == nil {
		t.Error("expected error for unknown callsign")
	}
}

func TestCheckOutAutoUnpins(t *testing.T) {
	mgr := newTestManager(t)
	n, _ := mgr.CreateNet(store.Net{Name: "AutoUnpin Test"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "", "")
	mgr.CheckIn(n.ID, "W1AW", "", "")

	mgr.PinStation(n.ID, "KD7BBC")
	mgr.PinStation(n.ID, "W1AW")

	// Check out KD7BBC — should auto-unpin.
	if err := mgr.CheckOut(n.ID, ci.ID); err != nil {
		t.Fatalf("CheckOut failed: %v", err)
	}

	got, ok := mgr.GetNet(n.ID)
	if !ok {
		t.Fatal("net not found")
	}
	if len(got.PinnedStations) != 1 || got.PinnedStations[0] != "W1AW" {
		t.Errorf("expected [W1AW] after checkout, got %v", got.PinnedStations)
	}
}

func TestPinnedPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	tracker := station.NewMemoryTracker(config.StationConfig{
		StaleTimeout:   time.Hour,
		TrackMaxPoints: 10,
		DedupWindow:    30 * time.Second,
	})

	mgr1 := NewManager(s, tracker)
	n, _ := mgr1.CreateNet(store.Net{Name: "Persist Pin"})
	mgr1.OpenNet(n.ID)
	mgr1.CheckIn(n.ID, "KD7BBC", "", "")
	mgr1.PinStation(n.ID, "KD7BBC")

	// Load in new manager.
	mgr2 := NewManager(s, tracker)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	got, ok := mgr2.GetNet(n.ID)
	if !ok {
		t.Fatal("net not found after reload")
	}
	if len(got.PinnedStations) != 1 || got.PinnedStations[0] != "KD7BBC" {
		t.Errorf("expected [KD7BBC] after reload, got %v", got.PinnedStations)
	}

	s.Close()
}

// --- Mission create-with-assignees tests (#unify-mission-assignment) ---

// newTwoOperatorNet builds an open net with two available check-ins.
func newTwoOperatorNet(t *testing.T, mgr *Manager) (netID string, a, b *store.NetCheckIn) {
	t.Helper()
	n, err := mgr.CreateNet(store.Net{Name: "Test Net"})
	if err != nil {
		t.Fatalf("CreateNet failed: %v", err)
	}
	if err := mgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet failed: %v", err)
	}
	a, err = mgr.CheckIn(n.ID, "KD7BBC", "", "")
	if err != nil {
		t.Fatalf("CheckIn A failed: %v", err)
	}
	b, err = mgr.CheckIn(n.ID, "W1AW", "", "")
	if err != nil {
		t.Fatalf("CheckIn B failed: %v", err)
	}
	return n.ID, a, b
}

func TestCreateMissionWithAssignees(t *testing.T) {
	mgr := newTestManager(t)
	netID, a, b := newTwoOperatorNet(t, mgr)

	mission, assigned, skipped, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}
	if len(assigned) != 2 {
		t.Fatalf("assigned: got %d, want 2", len(assigned))
	}
	if len(skipped) != 0 {
		t.Errorf("skipped: got %v, want empty", skipped)
	}
	if mission.AssignedTo != "" {
		t.Errorf("assignedTo: got %q, want empty", mission.AssignedTo)
	}
	for _, ci := range assigned {
		if len(ci.MissionIDs) != 1 || ci.MissionIDs[0] != mission.ID {
			t.Errorf("%s missionIds: got %v, want [%s]", ci.Callsign, ci.MissionIDs, mission.ID)
		}
		if ci.Status != OpAssigned {
			t.Errorf("%s status: got %q, want %q", ci.Callsign, ci.Status, OpAssigned)
		}
	}
}

func TestCreateMissionWithAssigneesReflectedInGetCheckIns(t *testing.T) {
	mgr := newTestManager(t)
	netID, a, b := newTwoOperatorNet(t, mgr)

	mission, _, _, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}

	for _, ci := range mgr.GetCheckIns(netID) {
		if len(ci.MissionIDs) != 1 || ci.MissionIDs[0] != mission.ID {
			t.Errorf("%s missionIds in manager state: got %v, want [%s]", ci.Callsign, ci.MissionIDs, mission.ID)
		}
		if ci.Status != OpAssigned {
			t.Errorf("%s status in manager state: got %q, want %q", ci.Callsign, ci.Status, OpAssigned)
		}
	}
}

func TestCreateMissionUnknownAssigneeWarnsAndCreates(t *testing.T) {
	mgr := newTestManager(t)
	netID, a, _ := newTwoOperatorNet(t, mgr)

	mission, assigned, skipped, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{a.ID, "NOT-A-REAL-ID"})
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}
	if len(mgr.GetMissions(netID)) != 1 {
		t.Fatalf("expected the mission to exist, got %d", len(mgr.GetMissions(netID)))
	}
	if len(assigned) != 1 || assigned[0].ID != a.ID {
		t.Errorf("assigned: got %v, want just %s", assigned, a.ID)
	}
	if len(skipped) != 1 || skipped[0] != "NOT-A-REAL-ID" {
		t.Errorf("skipped: got %v, want [NOT-A-REAL-ID]", skipped)
	}
	if mission.ID == "" {
		t.Error("expected mission ID to be set")
	}
}

func TestCreateMissionAllAssigneesUnknownStillCreates(t *testing.T) {
	mgr := newTestManager(t)
	netID, _, _ := newTwoOperatorNet(t, mgr)

	mission, assigned, skipped, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{"nope-1", "nope-2"})
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees must not fail on roster mismatch: %v", err)
	}
	if mission == nil || len(mgr.GetMissions(netID)) != 1 {
		t.Fatal("mission should have been created unassigned")
	}
	if len(assigned) != 0 {
		t.Errorf("assigned: got %v, want empty", assigned)
	}
	if len(skipped) != 2 {
		t.Errorf("skipped: got %v, want 2 entries", skipped)
	}
}

func TestCreateMissionAssigneeByCallsign(t *testing.T) {
	mgr := newTestManager(t)
	netID, a, _ := newTwoOperatorNet(t, mgr)

	_, assigned, skipped, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{"kd7bbc"})
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped: got %v, want empty", skipped)
	}
	if len(assigned) != 1 || assigned[0].ID != a.ID {
		t.Errorf("assigned: got %v, want just %s", assigned, a.ID)
	}
}

func TestCreateMissionAssigneesDeduped(t *testing.T) {
	mgr := newTestManager(t)
	netID, a, _ := newTwoOperatorNet(t, mgr)

	mission, assigned, skipped, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{a.ID, a.ID, "KD7BBC"})
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}
	if len(assigned) != 1 {
		t.Fatalf("assigned: got %d, want 1", len(assigned))
	}
	if len(skipped) != 0 {
		t.Errorf("skipped: got %v, want empty", skipped)
	}
	for _, ci := range mgr.GetCheckIns(netID) {
		if ci.ID == a.ID && len(ci.MissionIDs) != 1 {
			t.Errorf("missionIds: got %v, want exactly [%s]", ci.MissionIDs, mission.ID)
		}
	}
}

func TestCreateMissionEmitsAssignmentsBeforeMissionCreated(t *testing.T) {
	mgr := newTestManager(t)
	netID, a, b := newTwoOperatorNet(t, mgr)
	drainEvents(mgr)

	if _, _, _, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}

	var types []string
	for {
		select {
		case evt := <-mgr.Events():
			types = append(types, evt.Type)
			continue
		default:
		}
		break
	}

	createdIdx := -1
	created := 0
	for i, ty := range types {
		if ty == EventMissionCreated {
			created++
			createdIdx = i
		}
	}
	if created != 1 {
		t.Fatalf("mission_created count: got %d, want 1 (events: %v)", created, types)
	}
	checkinCount := 0
	for i, ty := range types {
		if ty != EventCheckInUpdated {
			continue
		}
		checkinCount++
		if i > createdIdx {
			t.Errorf("checkin_updated at %d comes after mission_created at %d (events: %v)", i, createdIdx, types)
		}
	}
	if checkinCount != 2 {
		t.Errorf("checkin_updated count: got %d, want 2 (events: %v)", checkinCount, types)
	}
}

func TestCreateMissionLogsAssignedOperators(t *testing.T) {
	mgr := newTestManager(t)
	netID, a, b := newTwoOperatorNet(t, mgr)

	if _, _, _, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}

	events, err := mgr.GetEvents(netID)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type != "mission_created" {
			continue
		}
		found = true
		if !strings.Contains(e.Summary, "Sweep Aid 3") ||
			!strings.Contains(e.Summary, "KD7BBC") ||
			!strings.Contains(e.Summary, "W1AW") {
			t.Errorf("mission_created summary %q should name the title and both operators", e.Summary)
		}
		if e.Callsign != "" {
			t.Errorf("mission_created callsign: got %q, want empty", e.Callsign)
		}
	}
	if !found {
		t.Fatal("no mission_created event logged")
	}
}

func TestCreateMissionNoAssigneesUnchanged(t *testing.T) {
	mgr := newTestManager(t)
	netID, _, _ := newTwoOperatorNet(t, mgr)
	drainEvents(mgr)

	_, assigned, skipped, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Deploy"}, nil)
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}
	if assigned == nil || len(assigned) != 0 {
		t.Errorf("assigned: got %v, want empty non-nil", assigned)
	}
	if skipped == nil || len(skipped) != 0 {
		t.Errorf("skipped: got %v, want empty non-nil", skipped)
	}

	for {
		select {
		case evt := <-mgr.Events():
			if evt.Type == EventCheckInUpdated {
				t.Errorf("unexpected checkin_updated event with no assignees")
			}
			continue
		default:
		}
		break
	}

	events, err := mgr.GetEvents(netID)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	for _, e := range events {
		if e.Type == "mission_created" && e.Summary != "Mission: Deploy" {
			t.Errorf("summary: got %q, want %q", e.Summary, "Mission: Deploy")
		}
	}
}

func TestCreateMissionLegacyAssignedToResolves(t *testing.T) {
	mgr := newTestManager(t)
	netID, a, _ := newTwoOperatorNet(t, mgr)

	mission, assigned, skipped, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "X", AssignedTo: "KD7BBC"}, nil)
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}
	if len(assigned) != 1 || assigned[0].ID != a.ID {
		t.Errorf("assigned: got %v, want just %s", assigned, a.ID)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped: got %v, want empty", skipped)
	}
	if mission.AssignedTo != "" {
		t.Errorf("assignedTo: got %q, want empty", mission.AssignedTo)
	}
}

func TestCreateMissionAssignmentsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	tracker := station.NewMemoryTracker(config.StationConfig{
		StaleTimeout:   time.Hour,
		TrackMaxPoints: 10,
		DedupWindow:    30 * time.Second,
	})
	mgr := NewManager(s, tracker)

	netID, a, _ := newTwoOperatorNet(t, mgr)
	mission, _, _, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{a.ID})
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees failed: %v", err)
	}
	s.Close()

	s2 := store.NewSQLiteStore(path)
	if err := s2.Init(); err != nil {
		t.Fatalf("re-Init failed: %v", err)
	}
	defer s2.Close()
	mgr2 := NewManager(s2, tracker)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cis := mgr2.GetCheckIns(netID)
	found := false
	for _, ci := range cis {
		if ci.ID != a.ID {
			continue
		}
		found = true
		if len(ci.MissionIDs) != 1 || ci.MissionIDs[0] != mission.ID {
			t.Errorf("reloaded missionIds: got %v, want [%s]", ci.MissionIDs, mission.ID)
		}
	}
	if !found {
		t.Fatal("check-in did not survive reload")
	}
	ms := mgr2.GetMissions(netID)
	if len(ms) != 1 {
		t.Fatalf("expected 1 reloaded mission, got %d", len(ms))
	}
	if ms[0].AssignedTo != "" {
		t.Errorf("reloaded assignedTo: got %q, want empty", ms[0].AssignedTo)
	}
}

func TestCreateMissionWithAssigneesConcurrent(t *testing.T) {
	mgr := newTestManager(t)
	netID, a, b := newTwoOperatorNet(t, mgr)

	timer := time.AfterFunc(10*time.Second, func() { panic("deadlock in CreateMissionWithAssignees") })
	defer timer.Stop()

	// Drain events in the background: the channel is capped, but emit() is
	// non-blocking, so this only keeps the test realistic.
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-mgr.Events():
			case <-done:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, _, _, err := mgr.CreateMissionWithAssignees(
				store.NetMission{NetID: netID, Title: fmt.Sprintf("Mission %d", i)},
				[]string{a.ID, b.ID},
			); err != nil {
				t.Errorf("concurrent create %d failed: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	close(done)

	if got := len(mgr.GetMissions(netID)); got != 8 {
		t.Errorf("missions: got %d, want 8", got)
	}
	for _, ci := range mgr.GetCheckIns(netID) {
		if len(ci.MissionIDs) != 8 {
			t.Errorf("%s missionIds: got %d, want 8", ci.Callsign, len(ci.MissionIDs))
		}
	}
}

func TestExportRosterCSVMissionsColumn(t *testing.T) {
	missions := []store.NetMission{
		{ID: "m-1", Title: "Deploy"},
		{ID: "m-2", Title: "Sweep"},
	}
	checkIns := []store.NetCheckIn{
		{
			Callsign:    "KD7BBC",
			Status:      "assigned",
			Traffic:     "routine",
			Source:      "voice",
			MissionIDs:  []string{"m-1", "m-2"},
			CheckedInAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			LastHeard:   time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	if err := ExportRosterCSV(&buf, checkIns, missions); err != nil {
		t.Fatalf("ExportRosterCSV failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	header := strings.Split(lines[0], ",")
	if len(header) < 9 || header[8] != "missions" {
		t.Fatalf("header column 8: got %v, want \"missions\"", header)
	}
	if !strings.Contains(buf.String(), "Deploy;Sweep") {
		t.Errorf("CSV should join mission titles: %q", buf.String())
	}
}

// failCheckInStore wraps a real store and can be flipped to fail every
// SaveNetCheckIn, standing in for a disk-full / SQLITE_BUSY write failure.
type failCheckInStore struct {
	store.Store
	fail bool
}

func (f *failCheckInStore) SaveNetCheckIn(ci store.NetCheckIn) error {
	if f.fail {
		return fmt.Errorf("simulated persist failure")
	}
	return f.Store.SaveNetCheckIn(ci)
}

func TestCreateMissionAssigneePersistFailureIsReportedAsSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	fs := &failCheckInStore{Store: s}

	tracker := station.NewMemoryTracker(config.StationConfig{
		StaleTimeout:   time.Hour,
		TrackMaxPoints: 10,
		DedupWindow:    30 * time.Second,
	})
	mgr := NewManager(fs, tracker)

	netID, a, _ := newTwoOperatorNet(t, mgr)
	fs.fail = true

	mission, assigned, skipped, err := mgr.CreateMissionWithAssignees(
		store.NetMission{NetID: netID, Title: "Sweep Aid 3"}, []string{a.ID})
	if err != nil {
		t.Fatalf("CreateMissionWithAssignees must still create the mission: %v", err)
	}
	if mission == nil {
		t.Fatal("mission is nil")
	}
	if len(assigned) != 0 {
		t.Errorf("assigned: got %v, want empty — the assignment was never persisted", assigned)
	}
	if len(skipped) != 1 || skipped[0] != a.ID {
		t.Errorf("skipped: got %v, want [%s]", skipped, a.ID)
	}

	// In-memory state must be rolled back so it matches the store.
	for _, ci := range mgr.GetCheckIns(netID) {
		if ci.ID != a.ID {
			continue
		}
		if len(ci.MissionIDs) != 0 {
			t.Errorf("missionIds: got %v, want empty after rollback", ci.MissionIDs)
		}
		if ci.Status != OpAvailable {
			t.Errorf("status: got %q, want %q after rollback", ci.Status, OpAvailable)
		}
	}
}

// TestOpenNetTimestamps covers the open-time integrity rules: the original
// OpenedAt survives a re-open, and a re-opened net is never left with a
// ClosedAt stamp (which would make it simultaneously open and closed).
func TestOpenNetTimestamps(t *testing.T) {
	tests := []struct {
		name string
		// prepare drives the net into the state under test before the
		// OpenNet call being asserted.
		prepare func(t *testing.T, mgr *Manager, id string)
	}{
		{
			name:    "first open sets openedAt",
			prepare: func(t *testing.T, mgr *Manager, id string) {},
		},
		{
			name: "re-open preserves original openedAt",
			prepare: func(t *testing.T, mgr *Manager, id string) {
				if err := mgr.OpenNet(id); err != nil {
					t.Fatalf("OpenNet failed: %v", err)
				}
			},
		},
		{
			name: "re-open clears closedAt",
			prepare: func(t *testing.T, mgr *Manager, id string) {
				if err := mgr.OpenNet(id); err != nil {
					t.Fatalf("OpenNet failed: %v", err)
				}
				if _, _, err := mgr.CloseNet(id); err != nil {
					t.Fatalf("CloseNet failed: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newTestManager(t)
			n, err := mgr.CreateNet(store.Net{Name: "Net", NCSCallsign: "KD7BBC"})
			if err != nil {
				t.Fatalf("CreateNet failed: %v", err)
			}

			tt.prepare(t, mgr, n.ID)

			before, _ := mgr.GetNet(n.ID)
			var priorOpenedAt *time.Time
			if before.OpenedAt != nil {
				v := *before.OpenedAt
				priorOpenedAt = &v
			}

			// Real clocks are coarse; force a distinguishable second stamp.
			time.Sleep(2 * time.Millisecond)

			if err := mgr.OpenNet(n.ID); err != nil {
				t.Fatalf("OpenNet failed: %v", err)
			}

			got, ok := mgr.GetNet(n.ID)
			if !ok {
				t.Fatal("net not found after open")
			}
			if got.Status != StatusOpen {
				t.Errorf("status = %q, want %q", got.Status, StatusOpen)
			}
			if got.OpenedAt == nil {
				t.Fatal("openedAt should be set after open")
			}
			if priorOpenedAt != nil && !got.OpenedAt.Equal(*priorOpenedAt) {
				t.Errorf("openedAt = %v, want preserved %v", got.OpenedAt, priorOpenedAt)
			}
			if got.ClosedAt != nil {
				t.Errorf("closedAt = %v, want nil on an open net", got.ClosedAt)
			}
		})
	}
}

// --- Net profile / ride config ---

func TestCreateNetDefaultsProfileGeneral(t *testing.T) {
	mgr := newTestManager(t)
	n, err := mgr.CreateNet(store.Net{Name: "Net"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if n.Profile != netprofile.ProfileGeneral {
		t.Errorf("Profile = %q, want %q", n.Profile, netprofile.ProfileGeneral)
	}
	if _, ok := mgr.GetRideConfig(n.ID); ok {
		t.Error("GetRideConfig = ok, want not found for a general net")
	}

	select {
	case evt := <-mgr.Events():
		if evt.Type != EventNetCreated {
			t.Errorf("event type = %s, want %s", evt.Type, EventNetCreated)
		}
	default:
		t.Fatal("expected net_created event")
	}
	select {
	case evt := <-mgr.Events():
		t.Errorf("unexpected extra event %s; a general net must not get a ride-config event", evt.Type)
	default:
	}
}

func TestCreateNetNormalizesProfile(t *testing.T) {
	mgr := newTestManager(t)
	n, err := mgr.CreateNet(store.Net{Name: "Net", Profile: " Bike-Ride "})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if n.Profile != netprofile.ProfileBikeRide {
		t.Errorf("Profile = %q, want %q", n.Profile, netprofile.ProfileBikeRide)
	}
}

func TestCreateNetRejectsUnknownProfile(t *testing.T) {
	mgr := newTestManager(t)
	before := len(mgr.GetNets())

	_, err := mgr.CreateNet(store.Net{Name: "Net", Profile: "sar"})
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	if !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("error = %v, want errors.Is ErrInvalidProfile", err)
	}
	if got := len(mgr.GetNets()); got != before {
		t.Errorf("net count = %d, want unchanged %d", got, before)
	}
}

func TestCreateNetBikeRideSeedsDefaultRideConfig(t *testing.T) {
	mgr := newTestManager(t)
	n, err := mgr.CreateNet(store.Net{Name: "Ride", Profile: netprofile.ProfileBikeRide})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}

	cfg, ok := mgr.GetRideConfig(n.ID)
	if !ok {
		t.Fatal("GetRideConfig = not found, want ok")
	}
	if cfg.NetID != n.ID {
		t.Errorf("NetID = %q, want %q", cfg.NetID, n.ID)
	}
	if len(cfg.Routes) != 0 {
		t.Errorf("Routes = %v, want empty", cfg.Routes)
	}

	stored, ok, err := mgr.store.LoadNetRideConfig(n.ID)
	if err != nil || !ok {
		t.Fatalf("store LoadNetRideConfig: ok=%v err=%v", ok, err)
	}
	if stored.NetID != n.ID {
		t.Errorf("stored NetID = %q, want %q", stored.NetID, n.ID)
	}

	select {
	case evt := <-mgr.Events():
		if evt.Type != EventNetCreated {
			t.Fatalf("first event = %s, want %s", evt.Type, EventNetCreated)
		}
	default:
		t.Fatal("expected net_created event")
	}
	select {
	case evt := <-mgr.Events():
		if evt.Type != EventNetRideConfigUpdated {
			t.Fatalf("second event = %s, want %s", evt.Type, EventNetRideConfigUpdated)
		}
	default:
		t.Fatal("expected net_ride_config_updated event")
	}
}

func TestSetProfileDraftOnly(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, mgr *Manager, id string)
		wantErr error // nil means expect success
	}{
		{"draft", func(t *testing.T, mgr *Manager, id string) {}, nil},
		{"open", func(t *testing.T, mgr *Manager, id string) {
			if err := mgr.OpenNet(id); err != nil {
				t.Fatalf("OpenNet: %v", err)
			}
		}, ErrNotDraft},
		{"closed", func(t *testing.T, mgr *Manager, id string) {
			if err := mgr.OpenNet(id); err != nil {
				t.Fatalf("OpenNet: %v", err)
			}
			if _, _, err := mgr.CloseNet(id); err != nil {
				t.Fatalf("CloseNet: %v", err)
			}
		}, ErrNotDraft},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newTestManager(t)
			n, err := mgr.CreateNet(store.Net{Name: "Net", NCSCallsign: "KD7BBC"})
			if err != nil {
				t.Fatalf("CreateNet: %v", err)
			}
			tt.prepare(t, mgr, n.ID)
			drainEvents(mgr)

			updated, err := mgr.SetProfile(n.ID, netprofile.ProfileBikeRide)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("SetProfile: %v", err)
				}
				if updated.Profile != netprofile.ProfileBikeRide {
					t.Errorf("Profile = %q, want %q", updated.Profile, netprofile.ProfileBikeRide)
				}
				events, err := mgr.store.LoadNetEvents(n.ID)
				if err != nil {
					t.Fatalf("LoadNetEvents: %v", err)
				}
				found := false
				for _, e := range events {
					if e.Type == "profile_changed" {
						found = true
					}
				}
				if !found {
					t.Error("expected a profile_changed timeline entry")
				}
				if !eventTypesContain(mgr, EventNetUpdated) {
					t.Error("expected a net_updated event")
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want errors.Is %v", err, tt.wantErr)
				}
				current, _ := mgr.GetNet(n.ID)
				if current.Profile != netprofile.ProfileGeneral {
					t.Errorf("Profile changed to %q despite error", current.Profile)
				}
				select {
				case evt := <-mgr.Events():
					t.Errorf("unexpected event %s after failed SetProfile", evt.Type)
				default:
				}
			}
		})
	}
}

func TestSetProfileToBikeRideSeedsConfigOnce(t *testing.T) {
	mgr := newTestManager(t)
	n, err := mgr.CreateNet(store.Net{Name: "Net", NCSCallsign: "KD7BBC"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}

	if _, err := mgr.SetProfile(n.ID, netprofile.ProfileBikeRide); err != nil {
		t.Fatalf("SetProfile(bike-ride): %v", err)
	}
	cfg, ok := mgr.GetRideConfig(n.ID)
	if !ok {
		t.Fatal("GetRideConfig after SetProfile: not found")
	}
	cfg.AgencyName = "Marin Cyclists"
	if _, err := mgr.SetRideConfig(n.ID, *cfg); err != nil {
		t.Fatalf("SetRideConfig: %v", err)
	}

	if _, err := mgr.SetProfile(n.ID, netprofile.ProfileGeneral); err != nil {
		t.Fatalf("SetProfile(general): %v", err)
	}
	if _, err := mgr.SetProfile(n.ID, netprofile.ProfileBikeRide); err != nil {
		t.Fatalf("SetProfile(bike-ride again): %v", err)
	}

	got, ok := mgr.GetRideConfig(n.ID)
	if !ok {
		t.Fatal("GetRideConfig after flip-flop: not found")
	}
	if got.AgencyName != "Marin Cyclists" {
		t.Errorf("AgencyName = %q, want preserved %q", got.AgencyName, "Marin Cyclists")
	}
}

func TestSetRideConfig(t *testing.T) {
	mgr := newTestManager(t)
	n, err := mgr.CreateNet(store.Net{Name: "Ride", NCSCallsign: "KD7BBC", Profile: netprofile.ProfileBikeRide})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	drainEvents(mgr)

	input := store.NetRideConfig{
		NetID:      "spoofed-net-id",
		AgencyName: "Marin Cyclists",
		Routes: []store.RideRoute{
			{ID: " 100 ", Name: "100 Mile", DistanceMiles: 100},
		},
	}
	got, err := mgr.SetRideConfig(n.ID, input)
	if err != nil {
		t.Fatalf("SetRideConfig: %v", err)
	}
	if got.NetID != n.ID {
		t.Errorf("NetID = %q, want %q (forced from arg)", got.NetID, n.ID)
	}
	if len(got.Routes) != 1 || got.Routes[0].ID != "100" {
		t.Errorf("Routes = %+v, want normalized id 100", got.Routes)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want set")
	}

	cached, ok := mgr.GetRideConfig(n.ID)
	if !ok {
		t.Fatal("GetRideConfig: not found")
	}
	if cached.AgencyName != "Marin Cyclists" {
		t.Errorf("cached AgencyName = %q, want %q", cached.AgencyName, "Marin Cyclists")
	}

	stored, ok, err := mgr.store.LoadNetRideConfig(n.ID)
	if err != nil || !ok {
		t.Fatalf("store LoadNetRideConfig: ok=%v err=%v", ok, err)
	}
	if stored.AgencyName != "Marin Cyclists" {
		t.Errorf("stored AgencyName = %q, want %q", stored.AgencyName, "Marin Cyclists")
	}

	if !eventTypesContain(mgr, EventNetRideConfigUpdated) {
		t.Error("expected a net_ride_config_updated event")
	}

	events, err := mgr.store.LoadNetEvents(n.ID)
	if err != nil {
		t.Fatalf("LoadNetEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type == "ride_config_updated" {
			found = true
		}
	}
	if !found {
		t.Error("expected a ride_config_updated timeline entry")
	}
}

func TestSetRideConfigRejectsInvalid(t *testing.T) {
	mgr := newTestManager(t)
	n, err := mgr.CreateNet(store.Net{Name: "Ride", NCSCallsign: "KD7BBC", Profile: netprofile.ProfileBikeRide})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	before, ok := mgr.GetRideConfig(n.ID)
	if !ok {
		t.Fatal("GetRideConfig before: not found")
	}
	drainEvents(mgr)

	invalid := store.NetRideConfig{
		Routes: []store.RideRoute{{ID: "a", Name: "A", DistanceMiles: 0}},
	}
	_, err = mgr.SetRideConfig(n.ID, invalid)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidRideConfig) {
		t.Errorf("error = %v, want errors.Is ErrInvalidRideConfig", err)
	}

	after, ok := mgr.GetRideConfig(n.ID)
	if !ok {
		t.Fatal("GetRideConfig after: not found")
	}
	if len(after.Routes) != len(before.Routes) {
		t.Errorf("cached config changed despite invalid input: %+v vs %+v", after, before)
	}

	select {
	case evt := <-mgr.Events():
		t.Errorf("unexpected event %s after rejected SetRideConfig", evt.Type)
	default:
	}
}

func TestSetRideConfigProfileMismatch(t *testing.T) {
	mgr := newTestManager(t)
	n, err := mgr.CreateNet(store.Net{Name: "Net", NCSCallsign: "KD7BBC"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, err = mgr.SetRideConfig(n.ID, store.NetRideConfig{})
	if !errors.Is(err, ErrProfileMismatch) {
		t.Errorf("error = %v, want errors.Is ErrProfileMismatch", err)
	}
}

func TestSetRideConfigClosedNet(t *testing.T) {
	mgr := newTestManager(t)
	n, err := mgr.CreateNet(store.Net{Name: "Ride", NCSCallsign: "KD7BBC", Profile: netprofile.ProfileBikeRide})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := mgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	if _, err := mgr.SetRideConfig(n.ID, store.NetRideConfig{AgencyName: "Open Agency"}); err != nil {
		t.Fatalf("SetRideConfig on open net: %v", err)
	}

	if _, _, err := mgr.CloseNet(n.ID); err != nil {
		t.Fatalf("CloseNet: %v", err)
	}
	_, err = mgr.SetRideConfig(n.ID, store.NetRideConfig{AgencyName: "Closed Agency"})
	if !errors.Is(err, ErrNetClosed) {
		t.Errorf("error = %v, want errors.Is ErrNetClosed", err)
	}
}

func TestProfileView(t *testing.T) {
	t.Run("general", func(t *testing.T) {
		mgr := newTestManager(t)
		n, err := mgr.CreateNet(store.Net{Name: "Net", NCSCallsign: "KD7BBC"})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		view, err := mgr.ProfileView(n.ID)
		if err != nil {
			t.Fatalf("ProfileView: %v", err)
		}
		if view.RideConfig != nil {
			t.Error("RideConfig != nil for general net")
		}
		if len(view.EffectivePriorityTiers) != 4 {
			t.Errorf("EffectivePriorityTiers len = %d, want 4", len(view.EffectivePriorityTiers))
		}
	})

	t.Run("bike-ride default", func(t *testing.T) {
		mgr := newTestManager(t)
		n, err := mgr.CreateNet(store.Net{Name: "Ride", NCSCallsign: "KD7BBC", Profile: netprofile.ProfileBikeRide})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		view, err := mgr.ProfileView(n.ID)
		if err != nil {
			t.Fatalf("ProfileView: %v", err)
		}
		if view.RideConfig == nil {
			t.Fatal("RideConfig = nil, want non-nil for bike-ride net")
		}
		if len(view.EffectivePriorityTiers) != 5 {
			t.Errorf("EffectivePriorityTiers len = %d, want 5", len(view.EffectivePriorityTiers))
		}
	})

	t.Run("bike-ride custom tiers", func(t *testing.T) {
		mgr := newTestManager(t)
		n, err := mgr.CreateNet(store.Net{Name: "Ride", NCSCallsign: "KD7BBC", Profile: netprofile.ProfileBikeRide})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		custom := []store.PriorityTier{
			{ID: "a", Label: "A", Rank: 1},
			{ID: "b", Label: "B", Rank: 2},
			{ID: "c", Label: "C", Rank: 3},
		}
		if _, err := mgr.SetRideConfig(n.ID, store.NetRideConfig{PriorityTiers: custom}); err != nil {
			t.Fatalf("SetRideConfig: %v", err)
		}
		view, err := mgr.ProfileView(n.ID)
		if err != nil {
			t.Fatalf("ProfileView: %v", err)
		}
		if len(view.EffectivePriorityTiers) != 3 {
			t.Errorf("EffectivePriorityTiers len = %d, want 3", len(view.EffectivePriorityTiers))
		}
	})

	t.Run("unknown net", func(t *testing.T) {
		mgr := newTestManager(t)
		if _, err := mgr.ProfileView("nonexistent"); err == nil {
			t.Error("expected error for unknown net")
		}
	})
}

func TestLoadRestoresRideConfigs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	tracker := station.NewMemoryTracker(config.StationConfig{
		StaleTimeout:   time.Hour,
		TrackMaxPoints: 10,
		DedupWindow:    30 * time.Second,
	})

	mgr := NewManager(s, tracker)
	n, err := mgr.CreateNet(store.Net{Name: "Ride Net", NCSCallsign: "KD7BBC", Profile: netprofile.ProfileBikeRide})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	edited := netprofile.DefaultRideConfig(n.ID)
	edited.AgencyName = "Marin Cyclists"
	if _, err := mgr.SetRideConfig(n.ID, edited); err != nil {
		t.Fatalf("SetRideConfig: %v", err)
	}

	// An archived net's ride config exists in the store but must not be
	// loaded into the cache — archived nets are skipped entirely, same as
	// their check-ins and missions.
	archivedNet := store.Net{ID: "archived-net", Name: "Old Ride", Status: StatusArchived, Profile: netprofile.ProfileBikeRide, PinnedStations: []string{}}
	if err := s.SaveNet(archivedNet); err != nil {
		t.Fatalf("SaveNet(archived): %v", err)
	}
	if err := s.SaveNetRideConfig(netprofile.DefaultRideConfig(archivedNet.ID)); err != nil {
		t.Fatalf("SaveNetRideConfig(archived): %v", err)
	}

	mgr2 := NewManager(s, tracker)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	got, ok := mgr2.GetRideConfig(n.ID)
	if !ok {
		t.Fatal("GetRideConfig after Load: not found")
	}
	if got.AgencyName != "Marin Cyclists" {
		t.Errorf("AgencyName = %q, want %q", got.AgencyName, "Marin Cyclists")
	}

	if _, ok := mgr2.GetRideConfig(archivedNet.ID); ok {
		t.Error("GetRideConfig for archived net = ok, want not found")
	}
}

func TestGeneralNetJSONUnchangedExceptProfile(t *testing.T) {
	mgr := newTestManager(t)
	n, err := mgr.CreateNet(store.Net{Name: "Net", NCSCallsign: "KD7BBC"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}

	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	want := []string{
		"id", "name", "type", "frequency", "ncsCallsign", "ncsUserId", "status",
		"notes", "missionBrief", "pinnedStations",
		"wxBufferMiles", "wxExtraZones", "wxMuteAdvisories", "wxInterruptCustom", "wxInterruptEvents",
		"profile",
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("missing key %q in %s", key, b)
		}
	}
	for _, key := range []string{"openedAt", "closedAt", "opsViewLat", "opsViewLon", "opsViewZoom"} {
		if _, ok := got[key]; ok {
			t.Errorf("unexpected key %q present on a fresh draft net: %s", key, b)
		}
	}
	if len(got) != len(want) {
		keys := make([]string, 0, len(got))
		for k := range got {
			keys = append(keys, k)
		}
		t.Errorf("key count = %d, want %d (got keys: %v)", len(got), len(want), keys)
	}
}

