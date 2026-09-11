package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
)

// newTestAnnotationServer builds a server with a real annotation manager backed
// by a real SQLite store.
func newTestAnnotationServer(t *testing.T) (*Server, *annotation.Manager, *session.MemoryManager) {
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

	annMgr := annotation.NewManager(db)
	if err := annMgr.Load(); err != nil {
		t.Fatalf("annotation load: %v", err)
	}
	objMgr := object.NewManager("TEST", 0, func(aprs.APRSFrame) error { return nil },
		object.ManagerConfig{RetransmitInterval: time.Minute})

	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{InactivityTimeout: 30 * time.Minute})

	srv := New(tracker, tm, message.Engine(nil), db,
		WithSessionManager(sessMgr),
		WithAnnotationManager(annMgr),
		WithObjectManager(objMgr),
	)
	return srv, annMgr, sessMgr
}

func plotterToken(sessMgr *session.MemoryManager) string {
	user, _ := sessMgr.Create("plotter", session.CreateOpts{})
	sessMgr.Approve(user.ID, session.RolePlotter)
	sessMgr.UpdateRole(user.ID, session.RolePlotter)
	return user.Token
}

func gpxWithWaypoints(names ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><gpx version="1.1">`)
	for i, n := range names {
		fmt.Fprintf(&b, `<wpt lat="%f" lon="%f"><name>%s</name></wpt>`,
			34.0+float64(i)/100, -118.0-float64(i)/100, n)
	}
	b.WriteString(`</gpx>`)
	return b.String()
}

func doUpload(srv *Server, path, filename, content, token string, fields map[string]string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write([]byte(content))
	mw.Close()

	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func decodeImport(t *testing.T, w *httptest.ResponseRecorder) annotation.BatchResult {
	t.Helper()
	var res annotation.BatchResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode import result: %v (body %s)", err, w.Body.String())
	}
	return res
}

func decodeDelete(t *testing.T, w *httptest.ResponseRecorder) annotation.DeleteResult {
	t.Helper()
	var res annotation.DeleteResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode delete result: %v (body %s)", err, w.Body.String())
	}
	return res
}

func errorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (body %s)", err, w.Body.String())
	}
	s, _ := body["error"].(string)
	return s
}

func TestImportAnnotationsReturnsBatchEnvelope(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	w := doUpload(srv, "/api/annotations/import", "Day_1.gpx", gpxWithWaypoints("A", "B", "C"), token, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201 (body %s)", w.Code, w.Body.String())
	}

	res := decodeImport(t, w)
	if res.BatchID == "" {
		t.Error("expected batchId")
	}
	if res.BatchLabel != "Day_1.gpx" {
		t.Errorf("batchLabel: got %q, want %q", res.BatchLabel, "Day_1.gpx")
	}
	if res.Count != 3 || len(res.Annotations) != 3 {
		t.Fatalf("count: got %d/%d, want 3", res.Count, len(res.Annotations))
	}
	for _, a := range res.Annotations {
		if a.BatchID != res.BatchID {
			t.Errorf("member batchId: got %q, want %q", a.BatchID, res.BatchID)
		}
	}
}

func TestImportAnnotationsEmptyFileReturns400(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	w := doUpload(srv, "/api/annotations/import", "empty.gpx", gpxWithWaypoints(), token, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (body %s)", w.Code, w.Body.String())
	}
	if msg := errorMessage(t, w); msg != "no waypoints or routes found in file" {
		t.Errorf("error: got %q", msg)
	}
}

func TestImportAnnotationsAppendsToBatchID(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	first := decodeImport(t, doUpload(srv, "/api/annotations/import", "Day_1.gpx", gpxWithWaypoints("A"), token, nil))

	w := doUpload(srv, "/api/annotations/import", "Day_1_more.gpx", gpxWithWaypoints("B"), token,
		map[string]string{"batchId": first.BatchID})
	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201 (body %s)", w.Code, w.Body.String())
	}
	second := decodeImport(t, w)
	if second.BatchID != first.BatchID {
		t.Errorf("batchId: got %q, want %q", second.BatchID, first.BatchID)
	}
	if second.BatchLabel != "Day_1.gpx" {
		t.Errorf("batchLabel: got %q, want the original label", second.BatchLabel)
	}
}

func TestImportAnnotationsUnknownBatchIDReturns404(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	w := doUpload(srv, "/api/annotations/import", "Day_1.gpx", gpxWithWaypoints("A"), token,
		map[string]string{"batchId": "ghost"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404 (body %s)", w.Code, w.Body.String())
	}
}

func TestBulkDeleteByBatchID(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	res := decodeImport(t, doUpload(srv, "/api/annotations/import", "Day_1.gpx", gpxWithWaypoints("A", "B"), token, nil))

	w := doRequest(srv, "POST", "/api/annotations/bulk-delete", map[string]any{"batchId": res.BatchID}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	del := decodeDelete(t, w)
	if del.DeletedCount != 2 {
		t.Errorf("deletedCount: got %d, want 2", del.DeletedCount)
	}
	if del.UndoToken == "" {
		t.Error("expected an undoToken")
	}

	list := doRequest(srv, "GET", "/api/annotations", nil, token)
	var anns []map[string]any
	json.Unmarshal(list.Body.Bytes(), &anns)
	if len(anns) != 0 {
		t.Errorf("annotations remain: %d", len(anns))
	}
}

func TestBulkDeleteByIDs(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	res := decodeImport(t, doUpload(srv, "/api/annotations/import", "Day_1.gpx", gpxWithWaypoints("A", "B"), token, nil))

	w := doRequest(srv, "POST", "/api/annotations/bulk-delete",
		map[string]any{"ids": []string{res.Annotations[0].ID}}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	if del := decodeDelete(t, w); del.DeletedCount != 1 {
		t.Errorf("deletedCount: got %d, want 1", del.DeletedCount)
	}
}

func TestBulkDeleteRejectsBothOrNeither(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	tests := []struct {
		name    string
		payload map[string]any
		wantMsg string
	}{
		{"neither", map[string]any{}, "provide exactly one of batchId or ids"},
		{"both", map[string]any{"batchId": "x", "ids": []string{"a"}}, "provide exactly one of batchId or ids"},
		{"empty ids", map[string]any{"ids": []string{}}, "provide exactly one of batchId or ids"},
		{"too many ids", map[string]any{"ids": makeIDs(501)}, "too many ids (max 500)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(srv, "POST", "/api/annotations/bulk-delete", tt.payload, token)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status: got %d, want 400 (body %s)", w.Code, w.Body.String())
			}
			if msg := errorMessage(t, w); msg != tt.wantMsg {
				t.Errorf("error: got %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

func makeIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("id-%d", i)
	}
	return out
}

func TestBulkDeleteUnknownBatchReturns404(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	w := doRequest(srv, "POST", "/api/annotations/bulk-delete", map[string]any{"batchId": "ghost"}, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404 (body %s)", w.Code, w.Body.String())
	}
	if msg := errorMessage(t, w); msg != `batch "ghost" not found` {
		t.Errorf("error: got %q", msg)
	}
}

func TestBulkDeleteRequiresPlotterRole(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := observerToken(sessMgr)

	w := doRequest(srv, "POST", "/api/annotations/bulk-delete", map[string]any{"batchId": "x"}, token)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403 (body %s)", w.Code, w.Body.String())
	}
}

func TestBulkDeleteStopTransmitRequiresOperator(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	adminToken(sessMgr) // burn the auto-admin slot
	token := plotterToken(sessMgr)

	w := doRequest(srv, "POST", "/api/annotations/bulk-delete",
		map[string]any{"batchId": "x", "stopTransmit": true}, token)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403 (body %s)", w.Code, w.Body.String())
	}
	if msg := errorMessage(t, w); msg != "operator role required to stop APRS transmission" {
		t.Errorf("error: got %q", msg)
	}
}

func TestBulkDeleteTransmittingReturns409(t *testing.T) {
	srv, annMgr, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	res := decodeImport(t, doUpload(srv, "/api/annotations/import", "Day_1.gpx", gpxWithWaypoints("A", "B"), token, nil))
	if _, err := annMgr.PromoteToObject(res.Annotations[0].ID); err != nil {
		t.Fatalf("PromoteToObject: %v", err)
	}

	w := doRequest(srv, "POST", "/api/annotations/bulk-delete", map[string]any{"batchId": res.BatchID}, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409 (body %s)", w.Code, w.Body.String())
	}

	var body struct {
		Error        string `json:"error"`
		Transmitting []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"transmitting"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Error != "annotations are transmitting as APRS objects" {
		t.Errorf("error: got %q", body.Error)
	}
	if len(body.Transmitting) != 1 || body.Transmitting[0].ID != res.Annotations[0].ID {
		t.Errorf("transmitting: got %+v", body.Transmitting)
	}
	if got := len(annMgr.BatchMembers(res.BatchID)); got != 2 {
		t.Errorf("nothing should be deleted, %d members remain", got)
	}
}

func TestUndoDeleteRestoresThen410(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	res := decodeImport(t, doUpload(srv, "/api/annotations/import", "Day_1.gpx", gpxWithWaypoints("A", "B"), token, nil))
	del := decodeDelete(t, doRequest(srv, "POST", "/api/annotations/bulk-delete",
		map[string]any{"batchId": res.BatchID}, token))

	w := doRequest(srv, "POST", "/api/annotations/undo-delete", map[string]any{"undoToken": del.UndoToken}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	restored := decodeImport(t, w)
	if restored.BatchID != res.BatchID || restored.Count != 2 {
		t.Errorf("restored: got batch %q count %d", restored.BatchID, restored.Count)
	}

	w2 := doRequest(srv, "POST", "/api/annotations/undo-delete", map[string]any{"undoToken": del.UndoToken}, token)
	if w2.Code != http.StatusGone {
		t.Fatalf("second undo: got %d, want 410 (body %s)", w2.Code, w2.Body.String())
	}
}

func TestRenameBatchLabel(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	res := decodeImport(t, doUpload(srv, "/api/annotations/import", "Day_1.gpx", gpxWithWaypoints("A", "B"), token, nil))

	tests := []struct {
		name     string
		batchID  string
		label    string
		wantCode int
		wantMsg  string
	}{
		{"ok", res.BatchID, "Day 1 — Jack and Back", http.StatusOK, ""},
		{"empty label", res.BatchID, "  ", http.StatusBadRequest, "batchLabel is required"},
		{"too long", res.BatchID, strings.Repeat("x", 121), http.StatusBadRequest, "batchLabel too long"},
		{"unknown batch", "ghost", "New", http.StatusNotFound, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(srv, "PATCH", "/api/annotations/batch-label",
				map[string]any{"batchId": tt.batchID, "batchLabel": tt.label}, token)
			if w.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body %s)", w.Code, tt.wantCode, w.Body.String())
			}
			if tt.wantMsg != "" {
				if msg := errorMessage(t, w); msg != tt.wantMsg {
					t.Errorf("error: got %q, want %q", msg, tt.wantMsg)
				}
			}
			if tt.wantCode == http.StatusOK {
				var body struct {
					BatchID    string `json:"batchId"`
					BatchLabel string `json:"batchLabel"`
					Updated    int    `json:"updated"`
				}
				json.Unmarshal(w.Body.Bytes(), &body)
				if body.Updated != 2 || body.BatchLabel != tt.label {
					t.Errorf("body: %+v", body)
				}
			}
		})
	}
}

func TestUpdateAnnotationCannotChangeBatchID(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	res := decodeImport(t, doUpload(srv, "/api/annotations/import", "Day_1.gpx", gpxWithWaypoints("A"), token, nil))
	ann := res.Annotations[0]

	w := doRequest(srv, "PUT", "/api/annotations/"+ann.ID, map[string]any{
		"label":      "Renamed",
		"batchId":    "hijacked",
		"batchLabel": "hijacked label",
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	var updated store.Annotation
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.BatchID != res.BatchID {
		t.Errorf("batchId: got %q, want %q", updated.BatchID, res.BatchID)
	}
	if updated.BatchLabel != res.BatchLabel {
		t.Errorf("batchLabel: got %q, want %q", updated.BatchLabel, res.BatchLabel)
	}
}

func TestCreateAnnotationValidatesBatchID(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	tests := []struct {
		name       string
		batchID    string
		batchLabel string
		wantCode   int
		wantLabel  string
	}{
		{"valid", "9f1c-4ab2_XY", "Template: Marathon", http.StatusCreated, "Template: Marathon"},
		{"invalid chars", "bad id!", "x", http.StatusBadRequest, ""},
		{"too long", strings.Repeat("a", 65), "x", http.StatusBadRequest, ""},
		{"label clamped", "batch-2", strings.Repeat("L", 200), http.StatusCreated, strings.Repeat("L", 120)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(srv, "POST", "/api/annotations", map[string]any{
				"type":       "point",
				"label":      "Pt",
				"geometry":   `{"type":"Point","coordinates":[-118,34]}`,
				"batchId":    tt.batchID,
				"batchLabel": tt.batchLabel,
			}, token)
			if w.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body %s)", w.Code, tt.wantCode, w.Body.String())
			}
			if tt.wantCode == http.StatusBadRequest {
				if msg := errorMessage(t, w); msg != "invalid batchId" {
					t.Errorf("error: got %q, want %q", msg, "invalid batchId")
				}
				return
			}
			var created store.Annotation
			json.Unmarshal(w.Body.Bytes(), &created)
			if created.BatchID != tt.batchID {
				t.Errorf("batchId: got %q, want %q", created.BatchID, tt.batchID)
			}
			if created.BatchLabel != tt.wantLabel {
				t.Errorf("batchLabel length: got %d, want %d", len(created.BatchLabel), len(tt.wantLabel))
			}
		})
	}
}

func TestGetAnnotationsFilteredByBatchID(t *testing.T) {
	srv, _, sessMgr := newTestAnnotationServer(t)
	token := adminToken(sessMgr)

	a := decodeImport(t, doUpload(srv, "/api/annotations/import", "a.gpx", gpxWithWaypoints("A1", "A2"), token, nil))
	doUpload(srv, "/api/annotations/import", "b.gpx", gpxWithWaypoints("B1"), token, nil)

	w := doRequest(srv, "GET", "/api/annotations?batchId="+a.BatchID, nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var anns []store.Annotation
	json.Unmarshal(w.Body.Bytes(), &anns)
	if len(anns) != 2 {
		t.Fatalf("filtered: got %d, want 2", len(anns))
	}
	for _, x := range anns {
		if x.BatchID != a.BatchID {
			t.Errorf("unexpected member with batch %q", x.BatchID)
		}
	}
}
