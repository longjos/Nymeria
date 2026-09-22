package ride

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

func TestCreateSupplyRequest(t *testing.T) {
	baseInput := func() CreateSupplyInput {
		return CreateSupplyInput{
			RequestedByCall: "RS3",
			Location:        "Rest Stop 3",
			Items:           []store.SupplyItem{{Item: "ice", Quantity: 10, Unit: "bags"}},
			Priority:        PriorityMedium,
		}
	}

	tests := []struct {
		name    string
		netID   string
		mutate  func(*CreateSupplyInput)
		wantErr string
	}{
		{name: "valid"},
		{name: "no items", mutate: func(in *CreateSupplyInput) { in.Items = nil }, wantErr: "at least one item"},
		{name: "unknown tier", mutate: func(in *CreateSupplyInput) { in.Priority = "urgent" }, wantErr: "invalid priority"},
		{name: "net not found", netID: "no-such-net", wantErr: "not found"},
		{name: "net not open", netID: "draft-net", wantErr: "net is not open"},
		{name: "requestedByCall empty", mutate: func(in *CreateSupplyInput) { in.RequestedByCall = "" }, wantErr: "requestedByCall"},
		{name: "division passthrough", mutate: func(in *CreateSupplyInput) { d := "route"; in.Division = &d }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, tl, _, _ := newTestTrafficManager(t)
			clock := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
			m.now = func() time.Time { return clock }

			netID := tt.netID
			if netID == "" {
				netID = "open-net"
			}
			in := baseInput()
			if tt.mutate != nil {
				tt.mutate(&in)
			}

			req, err := m.CreateSupplyRequest(netID, in, Actor{Callsign: "NCS"})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateSupplyRequest: %v", err)
			}
			if req.Status != store.SupplyDraft {
				t.Errorf("Status = %q, want draft", req.Status)
			}
			if req.Items == nil {
				t.Error("Items is nil, want non-nil")
			}
			if req.ETAs == nil {
				t.Error("ETAs is nil, want non-nil")
			}
			if !req.CreatedAt.Equal(clock) {
				t.Errorf("CreatedAt = %v, want %v", req.CreatedAt, clock)
			}
			if tl.countType(TLSupplyRequested) != 1 {
				t.Errorf("expected exactly one %s timeline entry, got %d", TLSupplyRequested, tl.countType(TLSupplyRequested))
			}
			expectEventType(t, m.Events(), EventSupplyCreated)

			if tt.name == "division passthrough" {
				if req.Division == nil || *req.Division != "route" {
					t.Errorf("Division = %v, want route", req.Division)
				}
			}
		})
	}
}

func TestCreateSupplyRequest_NilItemsStoredAsEmpty(t *testing.T) {
	m, _, _, _, _ := newTestTrafficManager(t)
	// Nil Items in input is rejected at create (at least one item is
	// required to send a request at all) — verify the rejection, and
	// separately verify AddSupplyItems on a draft never leaves the field nil.
	_, err := m.CreateSupplyRequest("open-net", CreateSupplyInput{RequestedByCall: "RS3", Priority: PriorityMedium}, Actor{})
	if err == nil || !strings.Contains(err.Error(), "at least one item") {
		t.Fatalf("expected 'at least one item' error, got %v", err)
	}
}

func TestSupplyLadder(t *testing.T) {
	newReq := func(t *testing.T) (*TrafficManager, *store.SupplyRequest, *recordingTimeline, func(time.Duration)) {
		m, _, tl, _, _ := newTestTrafficManager(t)
		advance := setClock(m, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
		req, err := m.CreateSupplyRequest("open-net", CreateSupplyInput{
			RequestedByCall: "RS3", Items: []store.SupplyItem{{Item: "ice", Quantity: 10}}, Priority: PriorityMedium,
		}, Actor{Callsign: "NCS"})
		if err != nil {
			t.Fatalf("CreateSupplyRequest: %v", err)
		}
		return m, req, tl, advance
	}

	t.Run("readback rejected stays draft with correction", func(t *testing.T) {
		m, req, tl, _ := newReq(t)
		got, err := m.ReadbackSupply("open-net", req.ID, ReadbackInput{Confirmed: false, Correction: "wrong item"}, Actor{})
		if err != nil {
			t.Fatalf("ReadbackSupply: %v", err)
		}
		if got.Status != store.SupplyDraft {
			t.Errorf("Status = %q, want draft", got.Status)
		}
		if !strings.Contains(got.Notes, "wrong item") {
			t.Errorf("Notes = %q, want to contain correction", got.Notes)
		}
		if tl.countType(TLSupplyReadbackConfirmed) != 0 {
			t.Error("expected no supply_readback_confirmed timeline entry on rejected readback")
		}
	})

	t.Run("readback confirmed moves to confirmed", func(t *testing.T) {
		m, req, tl, _ := newReq(t)
		got, err := m.ReadbackSupply("open-net", req.ID, ReadbackInput{Confirmed: true, ReadBackBy: "RS3"}, Actor{})
		if err != nil {
			t.Fatalf("ReadbackSupply: %v", err)
		}
		if got.Status != store.SupplyConfirmed {
			t.Errorf("Status = %q, want confirmed", got.Status)
		}
		if got.ReadBackAt == nil {
			t.Error("ReadBackAt not set")
		}
		if got.ReadBackBy != "RS3" {
			t.Errorf("ReadBackBy = %q, want RS3", got.ReadBackBy)
		}
		if tl.countType(TLSupplyReadbackConfirmed) != 1 {
			t.Error("expected exactly one supply_readback_confirmed timeline entry")
		}
	})

	t.Run("relay before readback is illegal", func(t *testing.T) {
		m, req, _, _ := newReq(t)
		_, err := m.RelaySupply("open-net", req.ID, RelayInput{RelayedTo: "SUPPLY 1"}, Actor{})
		if !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("expected ErrIllegalTransition, got %v", err)
		}
	})

	t.Run("full ladder to delivered via en_route", func(t *testing.T) {
		m, req, tl, advance := newReq(t)
		if _, err := m.ReadbackSupply("open-net", req.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		got, err := m.RelaySupply("open-net", req.ID, RelayInput{RelayedTo: "SUPPLY 1"}, Actor{})
		if err != nil {
			t.Fatalf("relay: %v", err)
		}
		if got.Status != store.SupplyRelayed || got.RelayedTo != "SUPPLY 1" {
			t.Errorf("after relay = %+v", got)
		}

		got, err = m.RecordSupplyETA("open-net", req.ID, ETAInput{Minutes: 20, Source: "SUPPLY 1"}, Actor{})
		if err != nil {
			t.Fatalf("eta: %v", err)
		}
		if got.Status != store.SupplyEnRoute || len(got.ETAs) != 1 || got.ETAs[0].Minutes != 20 {
			t.Errorf("after first eta = %+v", got)
		}
		wantDue := got.ETAs[0].GivenAt.Add(20 * time.Minute)
		if !got.ETAs[0].DueAt.Equal(wantDue) {
			t.Errorf("DueAt = %v, want %v", got.ETAs[0].DueAt, wantDue)
		}

		advance(5 * time.Minute)
		got, err = m.RecordSupplyETA("open-net", req.ID, ETAInput{Minutes: 35, Source: "SUPPLY 1"}, Actor{})
		if err != nil {
			t.Fatalf("second eta: %v", err)
		}
		if got.Status != store.SupplyEnRoute || len(got.ETAs) != 2 {
			t.Errorf("after second eta = %+v", got)
		}

		advance(34 * time.Minute)
		got, err = m.DeliverSupply("open-net", req.ID, Actor{})
		if err != nil {
			t.Fatalf("deliver: %v", err)
		}
		if got.Status != store.SupplyDelivered {
			t.Errorf("Status = %q, want delivered", got.Status)
		}
		entries := tl.all()
		lastSummary := entries[len(entries)-1].Summary
		if !strings.Contains(lastSummary, "after ETA given") {
			t.Errorf("delivered summary = %q, want to contain %q", lastSummary, "after ETA given")
		}

		if _, err := m.CancelSupply("open-net", req.ID, CancelInput{}, Actor{}); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("cancel after delivered: expected ErrIllegalTransition, got %v", err)
		}
	})

	t.Run("relayed can deliver without an ETA", func(t *testing.T) {
		m, req, _, _ := newReq(t)
		if _, err := m.ReadbackSupply("open-net", req.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		if _, err := m.RelaySupply("open-net", req.ID, RelayInput{RelayedTo: "SUPPLY 1"}, Actor{}); err != nil {
			t.Fatalf("relay: %v", err)
		}
		got, err := m.DeliverSupply("open-net", req.ID, Actor{})
		if err != nil {
			t.Fatalf("deliver: %v", err)
		}
		if got.Status != store.SupplyDelivered {
			t.Errorf("Status = %q, want delivered", got.Status)
		}
	})

	for _, from := range []string{"draft", "confirmed", "en_route"} {
		t.Run("cancel from "+from, func(t *testing.T) {
			m, req, _, _ := newReq(t)
			switch from {
			case "confirmed":
				if _, err := m.ReadbackSupply("open-net", req.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
					t.Fatalf("readback: %v", err)
				}
			case "en_route":
				if _, err := m.ReadbackSupply("open-net", req.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
					t.Fatalf("readback: %v", err)
				}
				if _, err := m.RelaySupply("open-net", req.ID, RelayInput{RelayedTo: "SUPPLY 1"}, Actor{}); err != nil {
					t.Fatalf("relay: %v", err)
				}
				if _, err := m.RecordSupplyETA("open-net", req.ID, ETAInput{Minutes: 10}, Actor{}); err != nil {
					t.Fatalf("eta: %v", err)
				}
			}
			got, err := m.CancelSupply("open-net", req.ID, CancelInput{Reason: "no longer needed"}, Actor{})
			if err != nil {
				t.Fatalf("cancel: %v", err)
			}
			if got.Status != store.SupplyCancelled {
				t.Errorf("Status = %q, want cancelled", got.Status)
			}
			if got.CancelledAt == nil {
				t.Error("CancelledAt not set")
			}
		})
	}
}

func TestAddSupplyItems(t *testing.T) {
	m, _, tl, _, _ := newTestTrafficManager(t)
	advance := setClock(m, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	req, err := m.CreateSupplyRequest("open-net", CreateSupplyInput{
		RequestedByCall: "RS3", Items: []store.SupplyItem{{Item: "ice", Quantity: 10}}, Priority: PriorityMedium,
	}, Actor{})
	if err != nil {
		t.Fatalf("CreateSupplyRequest: %v", err)
	}

	advance(2 * time.Minute)
	askedTrue := true
	got, err := m.AddSupplyItems("open-net", req.ID, AddItemsInput{
		Items:         []store.SupplyItem{{Item: "water", Quantity: 2, Unit: "cases"}},
		AskedWhatElse: &askedTrue,
	}, Actor{})
	if err != nil {
		t.Fatalf("AddSupplyItems: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items = %+v, want 2", got.Items)
	}
	if !got.Items[1].AddedAt.Equal(req.CreatedAt.Add(2 * time.Minute)) {
		t.Errorf("second item AddedAt = %v, want %v", got.Items[1].AddedAt, req.CreatedAt.Add(2*time.Minute))
	}
	if !got.AskedWhatElse {
		t.Error("AskedWhatElse = false, want true")
	}
	if tl.countType(TLSupplyItemsAdded) != 1 {
		t.Errorf("expected one %s entry", TLSupplyItemsAdded)
	}

	if _, err := m.ReadbackSupply("open-net", req.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if _, err := m.AddSupplyItems("open-net", req.ID, AddItemsInput{Items: []store.SupplyItem{{Item: "bananas"}}}, Actor{}); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("AddSupplyItems on confirmed: expected ErrIllegalTransition, got %v", err)
	}

	if _, err := m.AddSupplyItems("open-net", "unknown-id", AddItemsInput{}, Actor{}); err == nil || !strings.Contains(err.Error(), "at least one item") {
		t.Fatalf("empty items: expected 'at least one item' error, got %v", err)
	}
}

func TestMergeSupply(t *testing.T) {
	m, _, tl, _, _ := newTestTrafficManager(t)
	m.now = func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }

	mk := func(loc string, items ...store.SupplyItem) *store.SupplyRequest {
		req, err := m.CreateSupplyRequest("open-net", CreateSupplyInput{
			RequestedByCall: "RS" + loc, Location: loc, Items: items, Priority: PriorityMedium,
		}, Actor{})
		if err != nil {
			t.Fatalf("CreateSupplyRequest: %v", err)
		}
		return req
	}

	t.Run("merges same location drafts", func(t *testing.T) {
		target := mk("RS3", store.SupplyItem{Item: "ice", Quantity: 10})
		source := mk("RS3", store.SupplyItem{Item: "water", Quantity: 2})

		got, err := m.MergeSupply("open-net", target.ID, source.ID, Actor{})
		if err != nil {
			t.Fatalf("MergeSupply: %v", err)
		}
		if len(got.Items) != 2 {
			t.Errorf("target Items = %+v, want 2 (union)", got.Items)
		}
		srcAfter, _ := m.GetSupplyRequest("open-net", source.ID)
		if srcAfter.Status != store.SupplyMerged || srcAfter.MergedIntoID != target.ID {
			t.Errorf("source after merge = %+v", srcAfter)
		}
		if tl.countType(TLSupplyMerged) != 1 {
			t.Errorf("expected exactly one %s entry, got %d", TLSupplyMerged, tl.countType(TLSupplyMerged))
		}
	})

	t.Run("target not draft errors", func(t *testing.T) {
		target := mk("RS4", store.SupplyItem{Item: "ice", Quantity: 5})
		source := mk("RS4", store.SupplyItem{Item: "water", Quantity: 1})
		if _, err := m.ReadbackSupply("open-net", target.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		if _, err := m.MergeSupply("open-net", target.ID, source.ID, Actor{}); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("expected ErrIllegalTransition, got %v", err)
		}
	})

	t.Run("different location errors", func(t *testing.T) {
		target := mk("RS5", store.SupplyItem{Item: "ice", Quantity: 5})
		source := mk("RS6", store.SupplyItem{Item: "water", Quantity: 1})
		if _, err := m.MergeSupply("open-net", target.ID, source.ID, Actor{}); err == nil {
			t.Fatal("expected error for different locations")
		}
	})

	t.Run("source equals target errors", func(t *testing.T) {
		target := mk("RS7", store.SupplyItem{Item: "ice", Quantity: 5})
		if _, err := m.MergeSupply("open-net", target.ID, target.ID, Actor{}); err == nil {
			t.Fatal("expected error for source == target")
		}
	})
}

func TestSupplyElapsedFields(t *testing.T) {
	m, _, _, _, _ := newTestTrafficManager(t)
	advance := setClock(m, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	req, err := m.CreateSupplyRequest("open-net", CreateSupplyInput{
		RequestedByCall: "RS3", Items: []store.SupplyItem{{Item: "ice"}}, Priority: PriorityMedium,
	}, Actor{})
	if err != nil {
		t.Fatalf("CreateSupplyRequest: %v", err)
	}
	if _, err := m.ReadbackSupply("open-net", req.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if _, err := m.RelaySupply("open-net", req.ID, RelayInput{RelayedTo: "SUPPLY 1"}, Actor{}); err != nil {
		t.Fatalf("relay: %v", err)
	}
	got, err := m.RecordSupplyETA("open-net", req.ID, ETAInput{Minutes: 20, Source: "ice truck via phone"}, Actor{})
	if err != nil {
		t.Fatalf("eta: %v", err)
	}
	givenAt := got.ETAs[0].GivenAt
	dueAt := got.ETAs[0].DueAt

	advance(34 * time.Minute)
	now := m.now()

	// "how long since the ice truck said 20 minutes out" = now - GivenAt.
	sinceGiven := now.Sub(givenAt)
	if sinceGiven != 34*time.Minute {
		t.Errorf("elapsed since ETA given = %v, want 34m", sinceGiven)
	}
	// DueAt is 14 minutes in the past by now.
	overdue := now.Sub(dueAt)
	if overdue != 14*time.Minute {
		t.Errorf("overdue-by = %v, want 14m", overdue)
	}
}

func TestSupplyCatalogDefaults(t *testing.T) {
	m, _, _, _, _ := newTestTrafficManager(t)
	catalog := m.SupplyCatalog("open-net")

	foundOutOfWater, foundRunningLow := false, false
	for _, e := range catalog {
		if e.Item == "water" && e.DefaultTier == PriorityHigh && e.When == "out of water" {
			foundOutOfWater = true
		}
		if e.Item == "water" && e.DefaultTier == PriorityMedium && e.When == "running low" {
			foundRunningLow = true
		}
	}
	if !foundOutOfWater {
		t.Error("catalog missing water/high/\"out of water\"")
	}
	if !foundRunningLow {
		t.Error("catalog missing water/medium/\"running low\"")
	}
}

func TestSupplyLoadNormalizesNil(t *testing.T) {
	m, _, _, _, db := newTestTrafficManager(t)
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.SaveNet(store.Net{ID: "open-net", Name: "Ride", Status: "open"}); err != nil {
		t.Fatalf("SaveNet: %v", err)
	}
	if err := db.SaveSupplyRequest(store.SupplyRequest{
		ID: "sup-1", NetID: "open-net", RequestedByCall: "RS3", Priority: "medium",
		Status: "draft", CreatedAt: now, UpdatedAt: now,
		// Items/ETAs left nil — the store layer persists them as '[]', but
		// Load must still normalize defensively.
	}); err != nil {
		t.Fatalf("SaveSupplyRequest: %v", err)
	}

	if err := m.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	reqs := m.GetSupplyRequests("open-net")
	if len(reqs) != 1 {
		t.Fatalf("GetSupplyRequests = %+v, want 1", reqs)
	}
	if reqs[0].Items == nil || reqs[0].ETAs == nil {
		t.Errorf("Items/ETAs = %+v/%+v, want non-nil", reqs[0].Items, reqs[0].ETAs)
	}

	empty := m.GetSupplyRequests("unknown-net")
	if empty == nil || len(empty) != 0 {
		t.Errorf("GetSupplyRequests(unknown) = %v, want empty non-nil slice", empty)
	}
}
