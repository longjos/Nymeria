package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
)

// testServerWithMessages builds a server backed by a real message engine and a
// real SQLite store, which the plain testServer() helper cannot provide (it
// passes a nil engine and can only exercise the 503 path).
func testServerWithMessages(t *testing.T, opts ...Option) (*Server, *message.MemoryEngine, *store.SQLiteStore, *session.MemoryManager) {
	t.Helper()

	tracker := station.NewMemoryTracker(config.StationConfig{
		Callsign:       "N0CALL",
		TrackMaxPoints: 10,
		StaleTimeout:   time.Hour,
	})
	tm := transport.NewManager()

	eng := message.NewMemoryEngine("N0CALL", func(aprs.APRSFrame) error { return nil }, message.DefaultRetryConfig())
	t.Cleanup(eng.Close)

	db := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err := db.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{InactivityTimeout: 30 * time.Minute})
	opts = append([]Option{WithSessionManager(sessMgr)}, opts...)

	srv := New(tracker, tm, eng, db, opts...)
	return srv, eng, db, sessMgr
}

func observerToken(sessMgr *session.MemoryManager) string {
	// Burn the auto-admin slot first so this user really is an observer.
	first, _ := sessMgr.Create("seed-admin", session.CreateOpts{})
	sessMgr.Approve(first.ID, session.RoleAdmin)

	user, _ := sessMgr.Create("observer", session.CreateOpts{})
	sessMgr.Approve(user.ID, session.RoleObserver)
	return user.Token
}

func seedInbound(t *testing.T, eng *message.MemoryEngine, dbs ...*store.SQLiteStore) {
	t.Helper()
	m := message.Message{
		ID:        "m1",
		From:      "W1AW-9",
		To:        "N0CALL",
		Body:      "hi",
		Inbound:   true,
		State:     message.StateAcked,
		Timestamp: time.Now(),
	}
	eng.Import([]message.Message{m})
	for _, db := range dbs {
		if err := db.SaveMessage(m); err != nil {
			t.Fatalf("seed SaveMessage: %v", err)
		}
	}
}

func unreadFor(t *testing.T, srv *Server, callsign, token string) int {
	t.Helper()
	w := doRequest(srv, "GET", "/api/messages", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/messages: %d", w.Code)
	}
	var convos []message.Conversation
	if err := json.NewDecoder(w.Body).Decode(&convos); err != nil {
		t.Fatalf("decode conversations: %v", err)
	}
	for _, c := range convos {
		if c.Callsign == callsign {
			return c.UnreadCount
		}
	}
	t.Fatalf("conversation %q not found", callsign)
	return -1
}

func TestMarkConversationReadClearsUnread(t *testing.T) {
	srv, eng, _, sessMgr := testServerWithMessages(t)
	seedInbound(t, eng)
	tok := adminToken(sessMgr)

	if got := unreadFor(t, srv, "W1AW-9", tok); got != 1 {
		t.Fatalf("initial unreadCount = %d, want 1", got)
	}

	w := doRequest(srv, "POST", "/api/messages/W1AW-9/read", map[string]any{}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("POST read: %d (%s)", w.Code, w.Body.String())
	}

	var resp struct {
		Callsign    string `json:"callsign"`
		UnreadCount int    `json:"unreadCount"`
		LastReadAt  string `json:"lastReadAt"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Callsign != "W1AW-9" {
		t.Errorf("callsign = %q, want W1AW-9", resp.Callsign)
	}
	if resp.UnreadCount != 0 {
		t.Errorf("unreadCount = %d, want 0", resp.UnreadCount)
	}
	if resp.LastReadAt == "" {
		t.Error("lastReadAt is empty")
	}

	// CRITERION 1 — verify via a follow-up GET, not just the status code.
	// A handler that normalized the callsign would 200 while clearing nothing.
	if got := unreadFor(t, srv, "W1AW-9", tok); got != 0 {
		t.Errorf("unreadCount after mark-read = %d, want 0", got)
	}
}

func TestMarkConversationReadAllowsObserver(t *testing.T) {
	srv, eng, _, sessMgr := testServerWithMessages(t)
	seedInbound(t, eng)

	tok := observerToken(sessMgr)
	w := doRequest(srv, "POST", "/api/messages/W1AW-9/read", map[string]any{}, tok)
	if w.Code == http.StatusForbidden {
		t.Fatal("observer got 403; mark-read must be observer-accessible")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("observer POST read: %d (%s)", w.Code, w.Body.String())
	}
}

func TestMarkConversationReadAcceptsEmptyJSONBody(t *testing.T) {
	srv, eng, _, sessMgr := testServerWithMessages(t)
	seedInbound(t, eng)
	tok := adminToken(sessMgr)

	// Body `{}` — what the frontend post() helper always sends.
	if w := doRequest(srv, "POST", "/api/messages/W1AW-9/read", map[string]any{}, tok); w.Code != http.StatusOK {
		t.Errorf("empty JSON object body: %d (%s)", w.Code, w.Body.String())
	}
	// No body at all.
	if w := doRequest(srv, "POST", "/api/messages/W1AW-9/read", nil, tok); w.Code != http.StatusOK {
		t.Errorf("nil body: %d (%s)", w.Code, w.Body.String())
	}
}

func TestMarkConversationReadRejectsBulletin(t *testing.T) {
	srv, _, _, sessMgr := testServerWithMessages(t)

	w := doRequest(srv, "POST", "/api/messages/BLN1/read", map[string]any{}, adminToken(sessMgr))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("expected an error key in the 400 body")
	}
}

func TestMarkConversationReadNoEngine(t *testing.T) {
	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{InactivityTimeout: 30 * time.Minute})
	srv := testServer(WithSessionManager(sessMgr))

	w := doRequest(srv, "POST", "/api/messages/W1AW-9/read", map[string]any{}, adminToken(sessMgr))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "message engine not available" {
		t.Errorf("error = %q, want %q", resp["error"], "message engine not available")
	}
}

func TestMarkConversationReadPersists(t *testing.T) {
	srv, eng, db, sessMgr := testServerWithMessages(t)
	seedInbound(t, eng, db)

	if w := doRequest(srv, "POST", "/api/messages/W1AW-9/read", map[string]any{}, adminToken(sessMgr)); w.Code != http.StatusOK {
		t.Fatalf("POST read: %d (%s)", w.Code, w.Body.String())
	}

	reads, err := db.LoadConversationReads()
	if err != nil {
		t.Fatalf("LoadConversationReads: %v", err)
	}
	got, ok := reads["W1AW-9"]
	if !ok {
		t.Fatal("W1AW-9 read marker not persisted")
	}
	if got.IsZero() {
		t.Fatal("persisted lastReadAt is zero")
	}

	// CRITERION 2 — simulate a real restart: BOTH the messages and the markers
	// come back out of SQLite. Importing the in-memory messages instead would
	// skip the timestamp parse path that the restart actually depends on.
	dbMsgs, err := db.LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(dbMsgs) == 0 {
		t.Fatal("no messages round-tripped through the store")
	}

	restarted := message.NewMemoryEngine("N0CALL", func(aprs.APRSFrame) error { return nil }, message.DefaultRetryConfig())
	defer restarted.Close()
	restarted.Import(dbMsgs)
	restarted.ImportReadState(reads)

	found := false
	for _, c := range restarted.Conversations() {
		if c.Callsign != "W1AW-9" {
			continue
		}
		found = true
		if c.UnreadCount != 0 {
			t.Errorf("unreadCount after restart = %d, want 0", c.UnreadCount)
		}
		if c.LastReadAt == nil {
			t.Error("lastReadAt is nil after restart")
		}
	}
	if !found {
		t.Fatal("W1AW-9 conversation missing after restart")
	}
}

func TestMarkConversationReadUnknownCallsignPersistsNothing(t *testing.T) {
	srv, _, db, sessMgr := testServerWithMessages(t)

	// A callsign with no messages: accepted idempotently, but it must not
	// write a row — otherwise any client can grow the table without bound.
	w := doRequest(srv, "POST", "/api/messages/NOSUCH-7/read", map[string]any{}, adminToken(sessMgr))
	if w.Code != http.StatusOK {
		t.Fatalf("POST read: %d (%s)", w.Code, w.Body.String())
	}

	reads, err := db.LoadConversationReads()
	if err != nil {
		t.Fatalf("LoadConversationReads: %v", err)
	}
	if _, ok := reads["NOSUCH-7"]; ok {
		t.Errorf("persisted a read marker for a callsign with no conversation: %v", reads)
	}
}

// Read receipts fire again every time a message lands on an open thread, for
// every viewing client. Writing them to the operational activity log (which is
// exported as an ICS-309-adjacent record) would bury real traffic.
func TestMarkConversationReadDoesNotLogActivity(t *testing.T) {
	logStore := store.NewSQLiteStore(filepath.Join(t.TempDir(), "activity.db"))
	if err := logStore.Init(); err != nil {
		t.Fatalf("activity store init: %v", err)
	}
	t.Cleanup(func() { logStore.Close() })

	srv, eng, _, sessMgr := testServerWithMessages(t, WithActivityLogger(activity.NewStoreLogger(logStore)))
	seedInbound(t, eng)
	tok := adminToken(sessMgr)

	for i := 0; i < 3; i++ {
		if w := doRequest(srv, "POST", "/api/messages/W1AW-9/read", map[string]any{}, tok); w.Code != http.StatusOK {
			t.Fatalf("POST read %d: %d (%s)", i, w.Code, w.Body.String())
		}
	}

	entries, _, err := logStore.QueryActivity(store.ActivityFilter{Limit: 100})
	if err != nil {
		t.Fatalf("QueryActivity: %v", err)
	}
	for _, e := range entries {
		if e.Action == "message_read" {
			t.Fatalf("mark-read wrote an activity log entry: %+v", e)
		}
	}
}

func TestMarkConversationReadBroadcastsWS(t *testing.T) {
	srv, eng, _, sessMgr := testServerWithMessages(t)
	seedInbound(t, eng)
	tok := adminToken(sessMgr)

	c := &ws.Client{ID: "t1", Send: make(chan []byte, 8)}
	srv.Hub().Register(c)

	deadline := time.Now().Add(time.Second)
	for srv.Hub().ClientCount() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("client never registered with hub")
		}
		time.Sleep(time.Millisecond)
	}

	if w := doRequest(srv, "POST", "/api/messages/W1AW-9/read", map[string]any{}, tok); w.Code != http.StatusOK {
		t.Fatalf("POST read: %d (%s)", w.Code, w.Body.String())
	}

	select {
	case data := <-c.Send:
		var env struct {
			Type         string                `json:"type"`
			Conversation *message.Conversation `json:"conversation"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("unmarshal broadcast: %v (%s)", err, string(data))
		}
		if env.Type != "conversation_read" {
			t.Fatalf("broadcast type = %q, want conversation_read", env.Type)
		}
		if env.Conversation == nil {
			t.Fatalf("broadcast dropped the conversation field: %s", string(data))
		}
		if env.Conversation.Callsign != "W1AW-9" {
			t.Errorf("broadcast callsign = %q, want W1AW-9", env.Conversation.Callsign)
		}
		if env.Conversation.UnreadCount != 0 {
			t.Errorf("broadcast unreadCount = %d, want 0", env.Conversation.UnreadCount)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no conversation_read broadcast received")
	}
}

func TestBridgeSkipsZeroMessagePersist(t *testing.T) {
	_, eng, db, _ := testServerWithMessages(t)
	seedInbound(t, eng)

	if err := eng.ClaimConversation("W1AW-9", "u1", "Op"); err != nil {
		t.Fatalf("ClaimConversation: %v", err)
	}
	if _, err := eng.MarkRead("W1AW-9", time.Now()); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	// Let the bridge goroutine drain.
	time.Sleep(200 * time.Millisecond)

	msgs, err := db.LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	for _, m := range msgs {
		if m.ID == "" {
			t.Fatalf("bridge persisted a phantom message row with an empty ID: %+v", m)
		}
	}
}
