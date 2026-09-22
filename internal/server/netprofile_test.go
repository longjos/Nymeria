package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/netprofile"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/store"
)

func TestGetNetProfilesObserver(t *testing.T) {
	srv, _, sessMgr := newTestNetServer(t)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	r := httptest.NewRequest("GET", "/api/net-profiles", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var profiles []netprofile.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &profiles); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(profiles) != 2 || profiles[0].ID != netprofile.ProfileGeneral {
		t.Errorf("profiles = %+v, want [general, bike-ride]", profiles)
	}

	r2 := httptest.NewRequest("GET", "/api/net-profiles", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, r2)
	if w2.Code != 401 {
		t.Errorf("no-auth status = %d, want 401", w2.Code)
	}
}

func TestGetNetProfileByID(t *testing.T) {
	srv, _, sessMgr := newTestNetServer(t)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	r := httptest.NewRequest("GET", "/api/net-profiles/bike-ride", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var p netprofile.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !p.HasRideConfig {
		t.Error("HasRideConfig = false, want true")
	}

	r2 := httptest.NewRequest("GET", "/api/net-profiles/nope", nil)
	r2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, r2)
	if w2.Code != 404 {
		t.Errorf("status = %d, want 404 (body %s)", w2.Code, w2.Body.String())
	}
}

func TestGetNetProfileView(t *testing.T) {
	srv, netMgr, sessMgr := newTestNetServer(t)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	generalNet, err := netMgr.CreateNet(store.Net{Name: "General Net"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	r := httptest.NewRequest("GET", "/api/nets/"+generalNet.ID+"/profile", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"rideConfig"`) {
		t.Errorf("general net view should omit rideConfig key, got %s", w.Body.String())
	}
	var view netprofile.NetProfileView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(view.EffectivePriorityTiers) != 4 {
		t.Errorf("EffectivePriorityTiers len = %d, want 4", len(view.EffectivePriorityTiers))
	}

	rideNet, err := netMgr.CreateNet(store.Net{Name: "Ride Net", Profile: netprofile.ProfileBikeRide})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	r2 := httptest.NewRequest("GET", "/api/nets/"+rideNet.ID+"/profile", nil)
	r2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, r2)
	if w2.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), `"rideConfig"`) {
		t.Errorf("bike-ride net view should include rideConfig key, got %s", w2.Body.String())
	}
}

func TestGetRideConfigGeneralNet409(t *testing.T) {
	srv, netMgr, sessMgr := newTestNetServer(t)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	n, err := netMgr.CreateNet(store.Net{Name: "General Net"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}

	r := httptest.NewRequest("GET", "/api/nets/"+n.ID+"/ride-config", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatalf("status = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
}

func TestPutNetProfile(t *testing.T) {
	t.Run("operator draft succeeds", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		user, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Net", NCSUserID: user.ID})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}

		body, _ := json.Marshal(map[string]string{"profile": "bike-ride"})
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/profile", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
		}
		var view netprofile.NetProfileView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if view.Profile.ID != netprofile.ProfileBikeRide {
			t.Errorf("view.profile.id = %q, want bike-ride", view.Profile.ID)
		}

		r2 := httptest.NewRequest("GET", "/api/nets/"+n.ID, nil)
		r2.Header.Set("Authorization", "Bearer "+token)
		w2 := httptest.NewRecorder()
		srv.ServeHTTP(w2, r2)
		var got struct {
			Net store.Net `json:"net"`
		}
		if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode net: %v", err)
		}
		if got.Net.Profile != netprofile.ProfileBikeRide {
			t.Errorf("GET net profile = %q, want bike-ride", got.Net.Profile)
		}
	})

	t.Run("invalid profile 400", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		user, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Net", NCSUserID: user.ID})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}

		body, _ := json.Marshal(map[string]string{"profile": "sar"})
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/profile", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("status = %d, want 400 (body %s)", w.Code, w.Body.String())
		}
	})

	t.Run("open net 409", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		user, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Net", NCSUserID: user.ID})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		if err := netMgr.OpenNet(n.ID); err != nil {
			t.Fatalf("OpenNet: %v", err)
		}

		body, _ := json.Marshal(map[string]string{"profile": "bike-ride"})
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/profile", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 409 {
			t.Fatalf("status = %d, want 409 (body %s)", w.Code, w.Body.String())
		}
	})

	t.Run("observer 403", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		n, err := netMgr.CreateNet(store.Net{Name: "Net"})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

		body, _ := json.Marshal(map[string]string{"profile": "bike-ride"})
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/profile", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("status = %d, want 403 (body %s)", w.Code, w.Body.String())
		}
	})

	t.Run("non-NCS operator while NCS session live 403", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		ncs, _ := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		_, token := userWithRole(t, sessMgr, "OTHER", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Net", NCSUserID: ncs.ID})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}

		body, _ := json.Marshal(map[string]string{"profile": "bike-ride"})
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/profile", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("status = %d, want 403 (body %s)", w.Code, w.Body.String())
		}
	})
}

func TestPutRideConfig(t *testing.T) {
	t.Run("valid body, netId spoof ignored", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		user, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Ride", NCSUserID: user.ID, Profile: netprofile.ProfileBikeRide})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}

		payload := map[string]any{
			"netId":      "spoofed-id",
			"agencyName": "Marin Cyclists",
		}
		body, _ := json.Marshal(payload)
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/ride-config", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
		}
		var cfg store.NetRideConfig
		if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if cfg.NetID != n.ID {
			t.Errorf("body.netId = %q, want %q (forced from URL)", cfg.NetID, n.ID)
		}
	})

	t.Run("invalid distance 400", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		user, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Ride", NCSUserID: user.ID, Profile: netprofile.ProfileBikeRide})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}

		payload := map[string]any{
			"routes": []map[string]any{{"id": "a", "name": "A", "distanceMiles": 0}},
		}
		body, _ := json.Marshal(payload)
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/ride-config", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("status = %d, want 400 (body %s)", w.Code, w.Body.String())
		}
		var resp map[string]string
		json.Unmarshal(w.Body.Bytes(), &resp)
		if !strings.Contains(resp["error"], "distance") {
			t.Errorf("error = %q, want substring %q", resp["error"], "distance")
		}
	})

	t.Run("general net 409", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		user, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Net", NCSUserID: user.ID})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}

		body, _ := json.Marshal(map[string]any{"agencyName": "Agency"})
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/ride-config", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 409 {
			t.Fatalf("status = %d, want 409 (body %s)", w.Code, w.Body.String())
		}
	})

	t.Run("closed net 409", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		user, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Ride", NCSUserID: user.ID, Profile: netprofile.ProfileBikeRide})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		if err := netMgr.OpenNet(n.ID); err != nil {
			t.Fatalf("OpenNet: %v", err)
		}
		if _, _, err := netMgr.CloseNet(n.ID); err != nil {
			t.Fatalf("CloseNet: %v", err)
		}

		body, _ := json.Marshal(map[string]any{"agencyName": "Agency"})
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/ride-config", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 409 {
			t.Fatalf("status = %d, want 409 (body %s)", w.Code, w.Body.String())
		}
	})

	t.Run("observer 403", func(t *testing.T) {
		srv, netMgr, sessMgr := newTestNetServer(t)
		n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: netprofile.ProfileBikeRide})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

		body, _ := json.Marshal(map[string]any{"agencyName": "Agency"})
		r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/ride-config", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("status = %d, want 403 (body %s)", w.Code, w.Body.String())
		}
	})
}

func TestCreateNetAcceptsProfile(t *testing.T) {
	srv, _, sessMgr := newTestNetServer(t)
	_, token := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	body, _ := json.Marshal(map[string]string{"name": "Ride", "profile": "bike-ride"})
	r := httptest.NewRequest("POST", "/api/nets", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 201 && w.Code != 200 {
		t.Fatalf("status = %d, want 200/201 (body %s)", w.Code, w.Body.String())
	}
	var n store.Net
	if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if n.Profile != netprofile.ProfileBikeRide {
		t.Errorf("profile = %q, want bike-ride", n.Profile)
	}

	body2, _ := json.Marshal(map[string]string{"name": "Bad", "profile": "sar"})
	r2 := httptest.NewRequest("POST", "/api/nets", bytes.NewReader(body2))
	r2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, r2)
	if w2.Code != 400 {
		t.Fatalf("status = %d, want 400 (body %s)", w2.Code, w2.Body.String())
	}
}

func TestNetsWithoutProfileFieldStillCreate(t *testing.T) {
	srv, _, sessMgr := newTestNetServer(t)
	_, token := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	body, _ := json.Marshal(map[string]string{"name": "Legacy"})
	r := httptest.NewRequest("POST", "/api/nets", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 201 && w.Code != 200 {
		t.Fatalf("status = %d, want 200/201 (body %s)", w.Code, w.Body.String())
	}
	var n store.Net
	if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if n.Profile != netprofile.ProfileGeneral {
		t.Errorf("profile = %q, want general", n.Profile)
	}
}

func TestRideConfigWSEvent(t *testing.T) {
	srv, netMgr, sessMgr := newTestNetServer(t)
	user, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", NCSUserID: user.ID, Profile: netprofile.ProfileBikeRide})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}

	client := &ws.Client{ID: "test-client", Send: make(chan []byte, 16)}
	srv.hub.Register(client)
	defer srv.hub.Unregister(client)
	// Give the hub's Run loop time to process the registration.
	time.Sleep(20 * time.Millisecond)

	body, _ := json.Marshal(map[string]any{"agencyName": "Marin Cyclists"})
	r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/ride-config", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-client.Send:
			var envelope struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msg, &envelope); err != nil {
				t.Fatalf("decode WS message: %v", err)
			}
			if envelope.Type == netcontrol.EventNetRideConfigUpdated {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for net_ride_config_updated broadcast")
		}
	}
}
