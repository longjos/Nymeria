package server

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
)

// newTestNetServer builds a server with a real net control manager backed by a
// real SQLite store.
func newTestNetServer(t *testing.T) (*Server, *netcontrol.Manager, *session.MemoryManager) {
	t.Helper()

	tracker := station.NewMemoryTracker(config.StationConfig{
		Callsign:       "N0CALL",
		TrackMaxPoints: 10,
		StaleTimeout:   time.Hour,
	})
	tm := transport.NewManager()

	db := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err := db.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	netMgr := netcontrol.NewManager(db, tracker)
	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{InactivityTimeout: 30 * time.Minute})
	annMgr := annotation.NewManager(db)

	srv := New(tracker, tm, message.Engine(nil), db,
		WithSessionManager(sessMgr),
		WithNetControlManager(netMgr),
		WithAnnotationManager(annMgr),
	)
	testAnnMgr = annMgr
	return srv, netMgr, sessMgr
}

// Set by newTestNetServer so annotation-facing tests can reach the same manager
// the server is using without changing the helper's signature.
var testAnnMgr *annotation.Manager

// userWithRole creates an approved session user at the given role and returns
// the user plus its bearer token.
func userWithRole(t *testing.T, sessMgr *session.MemoryManager, name string, role session.Role) (*session.User, string) {
	t.Helper()
	user, err := sessMgr.Create(name, session.CreateOpts{})
	if err != nil {
		t.Fatalf("session create: %v", err)
	}
	// The first user is auto-approved as admin, so Approve is best-effort.
	sessMgr.Approve(user.ID, role)
	if err := sessMgr.UpdateRole(user.ID, role); err != nil {
		t.Fatalf("session role: %v", err)
	}
	return user, user.Token
}

func TestHandleCloseNet_Authorization(t *testing.T) {
	tests := []struct {
		name     string
		role     session.Role
		isNCS    bool
		wantCode int
	}{
		{"ncs can close", session.RoleOperator, true, 200},
		{"admin can close", session.RoleAdmin, false, 200},
		{"other operator forbidden", session.RoleOperator, false, 403},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, netMgr, sessMgr := newTestNetServer(t)
			ncs, _ := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
			actor, token := userWithRole(t, sessMgr, "ACTOR", tt.role)

			ncsUserID := ncs.ID
			if tt.isNCS {
				ncsUserID = actor.ID
			}
			n, err := netMgr.CreateNet(store.Net{
				Name:        "Emergency Net",
				NCSCallsign: "KD7BBC",
				NCSUserID:   ncsUserID,
			})
			if err != nil {
				t.Fatalf("CreateNet failed: %v", err)
			}
			if err := netMgr.OpenNet(n.ID); err != nil {
				t.Fatalf("OpenNet failed: %v", err)
			}

			r := httptest.NewRequest("POST", "/api/nets/"+n.ID+"/close", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)

			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantCode, w.Body.String())
			}

			got, _ := netMgr.GetNet(n.ID)
			if tt.wantCode == 200 {
				if got.Status != netcontrol.StatusClosed {
					t.Errorf("net status = %q, want closed", got.Status)
				}
			} else {
				if got.Status != netcontrol.StatusOpen {
					t.Errorf("net status = %q, want still open", got.Status)
				}
				var resp map[string]string
				json.NewDecoder(w.Body).Decode(&resp)
				if resp["error"] == "" {
					t.Error("expected an error message explaining the refusal")
				}
			}
		})
	}
}

// The recorded NCS is a session id, and sessions do not survive a restart, a
// timeout sweep or a re-login — but the net does. If the gate refused everyone
// whose id does not match a persisted NCSUserID, the genuine net control
// station would be locked out of ending their own net the moment their session
// was reissued, which is worse than the problem the gate solves. An NCS who is
// no longer connected leaves the net orphaned, and any operator may act.
func TestHandleCloseNet_OrphanedNCSDoesNotLockOut(t *testing.T) {
	srv, netMgr, sessMgr := newTestNetServer(t)
	departed, _ := userWithRole(t, sessMgr, "GONE", session.RoleOperator)
	_, token := userWithRole(t, sessMgr, "ACTOR", session.RoleOperator)

	n, err := netMgr.CreateNet(store.Net{
		Name:        "Orphaned Net",
		NCSCallsign: "KD7BBC",
		NCSUserID:   departed.ID,
	})
	if err != nil {
		t.Fatalf("CreateNet failed: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet failed: %v", err)
	}

	// The NCS session goes away, exactly as it would on restart or timeout.
	if err := sessMgr.Remove(departed.ID); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	r := httptest.NewRequest("POST", "/api/nets/"+n.ID+"/close", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 — an orphaned net must stay closable (body %s)", w.Code, w.Body.String())
	}
	got, _ := netMgr.GetNet(n.ID)
	if got.Status != netcontrol.StatusClosed {
		t.Errorf("status = %q, want closed", got.Status)
	}
}

// Without a gate on transfer the end-net gate is advisory: a refused operator
// could hand themselves net control and close the net on the next call.
func TestHandleTransferNCS_RequiresNCSOrAdmin(t *testing.T) {
	srv, netMgr, sessMgr := newTestNetServer(t)
	ncs, _ := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	actor, token := userWithRole(t, sessMgr, "ACTOR", session.RoleOperator)

	n, err := netMgr.CreateNet(store.Net{
		Name:        "Emergency Net",
		NCSCallsign: "KD7BBC",
		NCSUserID:   ncs.ID,
	})
	if err != nil {
		t.Fatalf("CreateNet failed: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet failed: %v", err)
	}

	body := strings.NewReader(`{"callsign":"W4ABC","userId":"` + actor.ID + `"}`)
	r := httptest.NewRequest("POST", "/api/nets/"+n.ID+"/transfer", body)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 403 {
		t.Fatalf("status = %d, want 403 (body %s)", w.Code, w.Body.String())
	}
	got, _ := netMgr.GetNet(n.ID)
	if got.NCSUserID != ncs.ID {
		t.Errorf("NCSUserID = %q, want it unchanged at %q", got.NCSUserID, ncs.ID)
	}
}

// OpenNet clears ClosedAt, so re-opening a closed net is a real undo of the
// NCS's decision and must obey the same gate as closing it.
func TestHandleOpenNet_ReopenRequiresNCSOrAdmin(t *testing.T) {
	srv, netMgr, sessMgr := newTestNetServer(t)
	ncs, _ := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	_, token := userWithRole(t, sessMgr, "ACTOR", session.RoleOperator)

	n, err := netMgr.CreateNet(store.Net{
		Name:        "Emergency Net",
		NCSCallsign: "KD7BBC",
		NCSUserID:   ncs.ID,
	})
	if err != nil {
		t.Fatalf("CreateNet failed: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet failed: %v", err)
	}
	if _, _, err := netMgr.CloseNet(n.ID); err != nil {
		t.Fatalf("CloseNet failed: %v", err)
	}

	r := httptest.NewRequest("POST", "/api/nets/"+n.ID+"/open", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 403 {
		t.Fatalf("status = %d, want 403 (body %s)", w.Code, w.Body.String())
	}
	got, _ := netMgr.GetNet(n.ID)
	if got.Status != netcontrol.StatusClosed {
		t.Errorf("status = %q, want it to stay closed", got.Status)
	}
}


// Ending a net must leave its annotations exactly as the operators left them.
// The old behaviour walked every non-terminal annotation on the net and pushed
// it to a terminal status with a ResolvedAt fabricated at close time — an
// incident still being worked would read "resolved" in the log, and the record
// of what was outstanding when the net ended was gone for good (#117).
func TestCloseNet_DoesNotResolveAnnotations(t *testing.T) {
	srv, netMgr, sessMgr := newTestNetServer(t)
	annMgr := testAnnMgr
	ncs, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)

	n, err := netMgr.CreateNet(store.Net{Name: "Emergency Net", NCSCallsign: "KD7BBC", NCSUserID: ncs.ID})
	if err != nil {
		t.Fatalf("CreateNet failed: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet failed: %v", err)
	}

	live, err := annMgr.Create(annotation.Annotation{
		Type: "point", Label: "Tree down on Main", Category: annotation.CategoryIncident,
		Status: "responding", NetID: n.ID,
		Geometry: `{"type":"Point","coordinates":[-86.6,35.7]}`,
	})
	if err != nil {
		t.Fatalf("Create annotation failed: %v", err)
	}

	r := httptest.NewRequest("POST", "/api/nets/"+n.ID+"/close", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("close status = %d (body %s)", w.Code, w.Body.String())
	}

	// The event bridge is asynchronous; give it room to do the wrong thing.
	time.Sleep(250 * time.Millisecond)

	got, ok := annMgr.Get(live.ID)
	if !ok {
		t.Fatal("annotation disappeared when the net closed")
	}
	if got.Status != "responding" {
		t.Errorf("status = %q, want %q — closing a net must not resolve an open incident", got.Status, "responding")
	}
	if got.ResolvedAt != nil {
		t.Errorf("ResolvedAt = %v, want nil — nobody resolved this", got.ResolvedAt)
	}
}
