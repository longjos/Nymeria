package netcontrol

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
)

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

func TestCheckInBasic(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, err := mgr.CheckIn(n.ID, "KD7BBC", "routine")
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

	ci, _ := mgr.CheckIn(n.ID, "  kd7bbc  ", "")
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

	_, err := mgr.CheckIn(n.ID, "", "")
	if err == nil {
		t.Error("expected error for empty callsign")
	}

	_, err = mgr.CheckIn("nonexistent", "KD7BBC", "")
	if err == nil {
		t.Error("expected error for nonexistent net")
	}
}

func TestCheckInAutoPopulateFromTracker(t *testing.T) {
	mgr := newTestManager(t)

	// Add a station to the tracker.
	mgr.tracker.Update(station.Station{
		Callsign: "KD7BBC",
		SSID:     0,
		LastHeard: time.Now(),
		Position: &station.Position{
			Lat: 34.0522,
			Lon: -118.2437,
		},
		Comment: "Mobile",
	})

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "")
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

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "")

	// Update status.
	ci.Status = OpAssigned
	ci.Assignment = "Red Cross Shelter"
	updated, err := mgr.UpdateCheckIn(*ci)
	if err != nil {
		t.Fatalf("UpdateCheckIn failed: %v", err)
	}
	if updated.Status != OpAssigned {
		t.Errorf("status: got %q, want %q", updated.Status, OpAssigned)
	}
	if updated.Assignment != "Red Cross Shelter" {
		t.Errorf("assignment: got %q, want %q", updated.Assignment, "Red Cross Shelter")
	}
}

func TestCheckOut(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "")

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
		NetID:      n.ID,
		Title:      "Deploy to shelter",
		AssignedTo: "KD7BBC",
		Priority:   "priority",
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

	ci1, _ := mgr.CheckIn(n.ID, "KD7BBC", "")
	ci2, _ := mgr.CheckIn(n.ID, "W1AW", "")

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
		{"YFA", 1},  // Substring match, not just prefix.
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

	mgr.CheckIn(n.ID, "KD7BBC", "routine")
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
	mgr1.CheckIn(n.ID, "KD7BBC", "routine")
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

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "")
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

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "")
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

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "")
	if ci.Source != "aprs" {
		t.Errorf("source: got %q, want %q", ci.Source, "aprs")
	}
}

func TestCheckInSourceVoice(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	// Unknown station — no tracker entry.
	ci, _ := mgr.CheckIn(n.ID, "UNKNOWN", "")
	if ci.Source != "voice" {
		t.Errorf("source: got %q, want %q", ci.Source, "voice")
	}
}

func TestAssignMission(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "")
	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Deploy"})
	drainEvents(mgr)

	updated, err := mgr.AssignMission(n.ID, ci.ID, m.ID)
	if err != nil {
		t.Fatalf("AssignMission failed: %v", err)
	}
	if updated.MissionID != m.ID {
		t.Errorf("missionId: got %q, want %q", updated.MissionID, m.ID)
	}
	if updated.Status != OpAssigned {
		t.Errorf("status: got %q, want %q", updated.Status, OpAssigned)
	}
}

func TestAssignMissionCopiesCoords(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "")
	lat, lon := 34.05, -118.24
	m, _ := mgr.CreateMission(store.NetMission{
		NetID: n.ID,
		Title: "Deploy to shelter",
		Lat:   &lat,
		Lon:   &lon,
	})
	drainEvents(mgr)

	updated, err := mgr.AssignMission(n.ID, ci.ID, m.ID)
	if err != nil {
		t.Fatalf("AssignMission failed: %v", err)
	}
	if updated.AssignmentLat == nil || *updated.AssignmentLat != lat {
		t.Errorf("assignmentLat: got %v, want %f", updated.AssignmentLat, lat)
	}
	if updated.AssignmentLon == nil || *updated.AssignmentLon != lon {
		t.Errorf("assignmentLon: got %v, want %f", updated.AssignmentLon, lon)
	}
}

func TestUnassignMission(t *testing.T) {
	mgr := newTestManager(t)

	n, _ := mgr.CreateNet(store.Net{Name: "Test Net"})
	mgr.OpenNet(n.ID)

	ci, _ := mgr.CheckIn(n.ID, "KD7BBC", "")
	m, _ := mgr.CreateMission(store.NetMission{NetID: n.ID, Title: "Deploy"})
	mgr.AssignMission(n.ID, ci.ID, m.ID)
	drainEvents(mgr)

	updated, err := mgr.UnassignMission(n.ID, ci.ID)
	if err != nil {
		t.Fatalf("UnassignMission failed: %v", err)
	}
	if updated.MissionID != "" {
		t.Errorf("missionId should be empty, got %q", updated.MissionID)
	}
	if updated.Status != OpAvailable {
		t.Errorf("status: got %q, want %q", updated.Status, OpAvailable)
	}
	if updated.AssignmentLat != nil {
		t.Errorf("assignmentLat should be nil, got %v", updated.AssignmentLat)
	}
}

func TestExportRosterCSV(t *testing.T) {
	checkIns := []store.NetCheckIn{
		{
			Callsign:    "KD7BBC",
			TacticalCall: "Shelter-1",
			OperatorName: "Bob",
			Status:      "available",
			Traffic:     "routine",
			Source:      "aprs",
			Location:    "Downtown",
			CheckedInAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			LastHeard:   time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	if err := ExportRosterCSV(&buf, checkIns); err != nil {
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
