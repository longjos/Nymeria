package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/narvel/nymeria/internal/message"
)

func TestSendMessageUsesStationPathWhenOmitted(t *testing.T) {
	srv, _, _, sessMgr := testServerWithMessages(t)
	token := adminToken(sessMgr)

	w := doRequest(srv, "POST", "/api/messages", map[string]any{
		"to":   "W3ADO",
		"body": "hello",
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/messages: %d %s", w.Code, w.Body.String())
	}
	var msg message.Message
	json.Unmarshal(w.Body.Bytes(), &msg)
	if msg.Path != "WIDE1-1,WIDE2-1" {
		t.Errorf("default path = %q, want WIDE1-1,WIDE2-1", msg.Path)
	}
}

func TestSendMessagePathOverride(t *testing.T) {
	srv, _, _, sessMgr := testServerWithMessages(t)
	token := adminToken(sessMgr)

	w := doRequest(srv, "POST", "/api/messages", map[string]any{
		"to":   "W3ADO",
		"body": "hello",
		"path": "WIDE1-1",
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/messages: %d %s", w.Code, w.Body.String())
	}
	var msg message.Message
	json.Unmarshal(w.Body.Bytes(), &msg)
	if msg.Path != "WIDE1-1" {
		t.Errorf("path = %q, want WIDE1-1", msg.Path)
	}
}

func TestSendMessagePathDirect(t *testing.T) {
	srv, _, _, sessMgr := testServerWithMessages(t)
	token := adminToken(sessMgr)

	w := doRequest(srv, "POST", "/api/messages", map[string]any{
		"to":   "W3ADO",
		"body": "hello",
		"path": "",
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/messages: %d %s", w.Code, w.Body.String())
	}
	var msg message.Message
	json.Unmarshal(w.Body.Bytes(), &msg)
	if msg.Path != "" {
		t.Errorf("direct path = %q, want empty", msg.Path)
	}
}

func TestSendMessageInvalidPath(t *testing.T) {
	srv, _, _, sessMgr := testServerWithMessages(t)
	token := adminToken(sessMgr)

	w := doRequest(srv, "POST", "/api/messages", map[string]any{
		"to":   "W3ADO",
		"body": "hello",
		"path": "TOOLONG-1",
	}, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid path: got %d, want 400 (%s)", w.Code, w.Body.String())
	}
}
