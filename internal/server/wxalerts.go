package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/ics309"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/wxalert"
)

// writeWxUnavailable answers the /wx/* routes that genuinely cannot work
// without a manager — acking, relaying, resolving a zone — 503 with a plain
// reason, never 404 (the route exists, the feature just isn't on).
func writeWxUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nws alerts not available"})
}

// offLinkStatus describes a feature that is switched off, and says WHY so the
// UI can name the fix. Being off is not an error: the read endpoints answer
// 200 with this rather than 503, because a client that gets an HTTP failure
// cannot tell "turned off" from "server unreachable" and ends up telling the
// operator it can't connect when nothing is actually wrong.
func (s *Server) offLinkStatus() wxalert.LinkStatus {
	st := wxalert.LinkStatus{State: wxalert.LinkOff, Reason: wxalert.ReasonDisabled}
	if s.configMgr == nil {
		return st
	}
	cfg := s.configMgr.Get().WxAlerts
	st.Enabled = cfg.Enabled
	st.ContactConfigured = strings.TrimSpace(cfg.Contact) != ""
	st.Sounds = cfg.Sounds
	switch {
	case !cfg.Enabled:
		st.Reason = wxalert.ReasonDisabled
	case !st.ContactConfigured:
		st.Reason = wxalert.ReasonContactMissing
	default:
		// Enabled and configured, yet there is no manager: it failed to start.
		st.Reason = wxalert.ReasonInitFailed
	}
	return st
}

// writeWxOffSnapshot answers a read endpoint with an empty, well-formed
// snapshot in the "off" state. Slices are never nil — the frontend iterates
// them against non-optional types.
func (s *Server) writeWxOffSnapshot(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"alerts":    []any{},
		"status":    s.offLinkStatus(),
		"footprint": nil,
		"policy":    nil,
	})
}

// wxAlertIDParam reads the {id} path param for an alert-scoped /wx/alerts/*
// route and URL-decodes it. NWS alert ids are `urn:oid:...` — the client
// (lib/api.ts) encodeURIComponent's the id before building the URL, which
// percent-encodes the colons. chi v5 captures URL params from r.URL.RawPath
// when it differs from r.URL.Path (exactly the case here, since a colon
// doesn't need escaping in a path segment), so chi.URLParam returns the
// still-encoded "urn%3Aoid%3A..." rather than the decoded id every alert is
// actually stored under. Decoding here is what makes GET/ack/ack-net/relay
// resolve a real alert id at all; net/zone {id}/{ugc} params (UUIDs, UGC
// codes) never contain reserved characters so they are unaffected.
func wxAlertIDParam(r *http.Request) string {
	raw := chi.URLParam(r, "id")
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}

// --- Read endpoints -------------------------------------------------------

func (s *Server) handleGetWxAlerts(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		s.writeWxOffSnapshot(w)
		return
	}
	writeJSON(w, http.StatusOK, mgr.Snapshot())
}

func (s *Server) handleGetWxAlert(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	id := wxAlertIDParam(r)
	alert := mgr.Get(id)
	if alert == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alert": alert, "history": mgr.History(id)})
}

func (s *Server) handleGetWxStatus(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeJSON(w, http.StatusOK, s.offLinkStatus())
		return
	}
	writeJSON(w, http.StatusOK, mgr.Link())
}

func (s *Server) handleWxRefresh(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	// Rebuild the footprint first. PollNow alone re-enters the poller's
	// empty-query skip whenever no watch area has resolved yet (cold start),
	// so "check now" could not break the link out of "not monitoring" —
	// which is the one thing an operator expects this control to do.
	s.refreshWxFootprint("manual refresh")
	mgr.PollNow()
	writeJSON(w, http.StatusAccepted, mgr.Link())
}

// handleGetWxFootprint's outline is always null: the Manager's Footprint
// bounding box lives on its unexported *Footprint value and is not surfaced
// by FootprintSummary or any Manager accessor in this work package's API
// surface, so there is nothing to build a GeoJSON outline from yet. The
// wire contract allows null explicitly for this reason.
func (s *Server) handleGetWxFootprint(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": mgr.Footprint(), "outline": nil})
}

func (s *Server) handleGetWxEventTypes(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, mgr.EventTypes())
}

// handleSearchWxZones requires state (the per-state index is how the zone
// cache can answer offline at all); with no cached index for that state and
// no network, every ListState call fails and the whole request is 503.
func (s *Server) handleSearchWxZones(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	zc := mgr.Zones()
	if zc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "zone cache not configured"})
		return
	}

	q := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("q")))
	state := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("state")))
	if state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "state is required"})
		return
	}

	var all []wxalert.ZoneRef
	anySucceeded := false
	for _, zt := range []string{"county", "forecast", "fire"} {
		refs, err := zc.ListState(r.Context(), zt, state, true)
		if err != nil {
			continue
		}
		anySucceeded = true
		all = append(all, refs...)
	}
	if !anySucceeded {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "zone index not available offline"})
		return
	}

	out := make([]wxalert.ZoneRef, 0, len(all))
	for _, z := range all {
		if q != "" && !strings.Contains(strings.ToUpper(z.Name), q) && !strings.Contains(z.UGC, q) {
			continue
		}
		out = append(out, z)
		if len(out) >= 25 {
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetWxZone(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	ugc := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "ugc")))
	rec, err := mgr.Zone(r.Context(), ugc, true)
	if err != nil {
		offline := !errors.Is(err, wxalert.ErrBadUGC)
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "zone not cached", "offline": offline})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// --- Local (per-device) acknowledgement -----------------------------------

func (s *Server) handleAckWxAlert(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	id := wxAlertIDParam(r)
	if err := mgr.AcknowledgeLocal(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
		return
	}
	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now().UTC(), UserID: wxUserID(user), UserName: wxUserName(user),
			Action: activity.ActionWxAlertAcknowledged, Target: id,
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- NCS "ack for net" (decision 5: clears the banner for everyone) ------

func (s *Server) handleAckWxAlertForNet(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	id := wxAlertIDParam(r)

	var netID string
	var net *store.Net
	if s.netMgr != nil {
		if n := s.netMgr.ActiveNet(); n != nil {
			netID, net = n.ID, n
		}
	}
	if netID == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no active net to acknowledge for"})
		return
	}
	if !s.allowNetControlAction(w, r, netID, "acknowledge alerts for the net") {
		return
	}

	user, _ := UserFromContext(r.Context())
	ack := wxalert.NetAck{At: time.Now().UTC()}
	if user != nil {
		ack.UserID, ack.UserName, ack.Callsign = user.ID, user.Name, user.Callsign
	}
	if ack.Callsign == "" && net != nil {
		ack.Callsign = net.NCSCallsign
	}

	updated, err := mgr.AckForNet(id, ack)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
		return
	}
	// Broadcast, persistence and activity/timeline logging happen on the
	// manager's event bridge (EventAlertAckNet) — AckForNet already queued
	// that event, so the handler does not duplicate it here.
	writeJSON(w, http.StatusOK, updated)
}

// --- Relay to the net (note / bulletin / per-roster messages) ------------

// wxRelayRequest is the PUT body for POST /wx/alerts/{id}/relay. Text is
// used for both the bulletin and the per-roster messages (APRS message
// bodies are limited; NWS-note content is server-built from the alert
// instead, so it never needs to fit in Text).
type wxRelayRequest struct {
	Note     bool   `json:"note"`
	Bulletin bool   `json:"bulletin"`
	Messages bool   `json:"messages"`
	Text     string `json:"text"`
}

type wxRelayResult struct {
	NoteID         string   `json:"noteId,omitempty"`
	BulletinSent   bool     `json:"bulletinSent"`
	MessagesSent   int      `json:"messagesSent"`
	MessagesFailed []string `json:"messagesFailed"`
}

const wxRelayTextMaxBytes = 67

func (s *Server) handleRelayWxAlert(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	id := wxAlertIDParam(r)
	alert := mgr.Get(id)
	if alert == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
		return
	}

	var req wxRelayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if (req.Bulletin || req.Messages) && len([]byte(req.Text)) > wxRelayTextMaxBytes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("text must be %d bytes or fewer", wxRelayTextMaxBytes)})
		return
	}
	if (req.Bulletin || req.Messages) && s.msgEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "messaging not available"})
		return
	}

	var netID string
	var net *store.Net
	if s.netMgr != nil {
		if n := s.netMgr.ActiveNet(); n != nil {
			netID, net = n.ID, n
		}
	}
	if netID == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no active net to relay to"})
		return
	}
	if !s.allowNetControlAction(w, r, netID, "relay a weather alert to the net") {
		return
	}

	user, _ := UserFromContext(r.Context())
	callsign := wxUserCallsign(user)
	if callsign == "" && net != nil {
		callsign = net.NCSCallsign
	}
	if callsign == "" {
		callsign = s.stationCfg.Callsign
	}

	result := wxRelayResult{MessagesFailed: []string{}}
	var toParts []string

	if req.Note && s.netMgr != nil {
		content := alert.Headline
		if strings.TrimSpace(alert.Instruction) != "" {
			content = strings.TrimSpace(content + "\n\n" + alert.Instruction)
		}
		note, err := s.netMgr.AddNote(store.NetNote{
			NetID: netID, Content: content, Category: "weather", Pinned: true,
			AuthorID: wxUserID(user), AuthorName: wxUserName(user),
		})
		if err != nil {
			log.Printf("[server] wx relay note: %v", err)
		} else {
			result.NoteID = note.ID
			toParts = append(toParts, "Situation Board")
		}
	}

	if req.Bulletin {
		if _, err := s.msgEngine.Send("BLN0", req.Text); err != nil {
			log.Printf("[server] wx relay bulletin: %v", err)
		} else {
			result.BulletinSent = true
			toParts = append(toParts, "BLN0")
		}
	}

	if req.Messages && s.netMgr != nil {
		for _, ci := range s.netMgr.GetCheckIns(netID) {
			if ci.Status == "released" {
				continue
			}
			if callsign != "" && strings.EqualFold(ci.Callsign, callsign) {
				continue // never message the relaying NCS themselves
			}
			if _, err := s.msgEngine.Send(ci.Callsign, req.Text); err != nil {
				result.MessagesFailed = append(result.MessagesFailed, ci.Callsign)
				continue
			}
			result.MessagesSent++
			toParts = append(toParts, ci.Callsign)
		}
	}

	if s.actLogger != nil {
		details, _ := json.Marshal(ics309.WxRelayDetails{
			Office: alert.SenderID, To: strings.Join(toParts, ", "), Callsign: callsign, Subject: alert.EffectiveEvent,
		})
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now().UTC(), UserID: wxUserID(user), UserName: wxUserName(user),
			Action: activity.ActionWxAlertRelayed, Target: id, Details: string(details),
		})
	}
	if s.netMgr != nil && len(toParts) > 0 {
		if err := s.netMgr.AddTimelineEvent(netID, netcontrol.EventWxAlert, callsign,
			fmt.Sprintf("Relayed %s to %s", alert.EffectiveEvent, strings.Join(toParts, ", "))); err != nil {
			log.Printf("[server] wx relay timeline: %v", err)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func wxUserID(u *session.User) string {
	if u == nil {
		return ""
	}
	return u.ID
}

func wxUserName(u *session.User) string {
	if u == nil {
		return ""
	}
	return u.Name
}

func wxUserCallsign(u *session.User) string {
	if u == nil {
		return ""
	}
	return u.Callsign
}

// --- Per-net weather watch area -------------------------------------------

func (s *Server) handleGetNetWxWatch(w http.ResponseWriter, r *http.Request) {
	if s.WxAlertManager() == nil {
		writeWxUnavailable(w)
		return
	}
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}
	id := chi.URLParam(r, "id")
	n, ok := s.netMgr.GetNet(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		return
	}
	writeJSON(w, http.StatusOK, netWatchFromNet(n, s.wxAlertsConfig()))
}

func (s *Server) handleUpdateNetWxWatch(w http.ResponseWriter, r *http.Request) {
	if s.WxAlertManager() == nil {
		writeWxUnavailable(w)
		return
	}
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if !s.allowNetControlAction(w, r, id, "change the weather watch area") {
		return
	}

	var body wxalert.NetWatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.BufferMiles != 0 && (body.BufferMiles < 2 || body.BufferMiles > 50) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bufferMiles must be 0 (inherit) or 2-50"})
		return
	}
	events := body.InterruptEvents
	if body.InterruptCustom {
		canon, err := wxalert.ValidateInterruptEvents(events)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		events = canon
	}

	updated, err := s.netMgr.SetWxWatch(id, body.BufferMiles, body.ExtraZones, body.MuteAdvisories, body.InterruptCustom, events)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	s.refreshWxFootprint("netwatch")

	writeJSON(w, http.StatusOK, netWatchFromNet(updated, s.wxAlertsConfig()))
}

func (s *Server) handleGetNetWxWatchZones(w http.ResponseWriter, r *http.Request) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		writeWxUnavailable(w)
		return
	}
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.netMgr.GetNet(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		return
	}

	fp := mgr.Footprint()
	// The manager is single-footprint: only the currently active net's
	// watch area is actually being matched against right now.
	if fp.NetID != id {
		writeJSON(w, http.StatusOK, wxalert.NetWatchZones{Resolved: []wxalert.ZoneRef{}, Neighbors: []wxalert.ZoneRef{}})
		return
	}
	neighbors := append([]wxalert.ZoneRef{}, fp.NearZones...)
	writeJSON(w, http.StatusOK, wxalert.NetWatchZones{Resolved: fp.Zones, Neighbors: neighbors})
}

func netWatchFromNet(n *store.Net, base config.WxAlertsConfig) wxalert.NetWatch {
	return wxalert.NetWatch{
		NetID:           n.ID,
		BufferMiles:     n.WxBufferMiles,
		ExtraZones:      append([]string{}, n.WxExtraZones...),
		MuteAdvisories:  n.WxMuteAdvisories,
		InterruptCustom: n.WxInterruptCustom,
		InterruptEvents: append([]string{}, n.WxInterruptEvents...),
		Effective:       computeWxEffectivePolicy(base, n),
	}
}

// computeWxEffectivePolicy merges global config defaults with a net's
// overrides. Pure — it never touches the live Manager — so it answers
// correctly for any net, not only the currently active one.
func computeWxEffectivePolicy(base config.WxAlertsConfig, net *store.Net) wxalert.EffectivePolicy {
	pol := wxalert.EffectivePolicy{
		BufferMiles:     base.DefaultBufferMiles,
		InterruptEvents: append([]string{}, base.InterruptEvents...),
		WatchNotify:     base.WatchNotify,
		AdvisoryNotify:  base.AdvisoryNotify,
		StatementNotify: base.StatementNotify,
		FloorText:       wxalert.FloorText(),
	}
	if net != nil {
		pol.NetID = net.ID
		if net.WxBufferMiles > 0 {
			pol.BufferMiles = net.WxBufferMiles
		}
		if net.WxInterruptCustom {
			pol.InterruptEvents = append([]string{}, net.WxInterruptEvents...)
			pol.InterruptCustom = true
		}
		pol.MuteAdvisories = net.WxMuteAdvisories
	}
	return pol
}

// wxAlertsConfig reads the current wx_alerts settings, or the zero value
// when there is no config manager.
func (s *Server) wxAlertsConfig() config.WxAlertsConfig {
	if s.configMgr == nil {
		return config.WxAlertsConfig{}
	}
	return s.configMgr.Get().WxAlerts
}

// wxManagerConfigFor rebuilds the live Manager's Config from its own base
// (connection details fixed at construction — see WxBaseConfig) plus the
// current global policy and, when net is non-nil, that net's overrides. It
// never changes Enabled/UserAgent/BaseURL/PollInterval/ZoneDataDir: those
// require a fresh Manager (SetWxAlertManager), not UpdateConfig.
func (s *Server) wxManagerConfigFor(net *store.Net) wxalert.Config {
	cfg := s.WxBaseConfig()
	base := s.wxAlertsConfig()
	pol := computeWxEffectivePolicy(base, net)

	cfg.DefaultBufferMiles = pol.BufferMiles
	cfg.InterruptEvents = pol.InterruptEvents
	cfg.InterruptCustom = pol.InterruptCustom
	cfg.WatchNotify = wxalert.NotifyClass(pol.WatchNotify)
	cfg.AdvisoryNotify = wxalert.NotifyClass(pol.AdvisoryNotify)
	cfg.StatementNotify = wxalert.NotifyClass(pol.StatementNotify)
	cfg.MuteAdvisories = pol.MuteAdvisories
	cfg.Sounds = base.Sounds
	return cfg
}

// --- Footprint recompute ---------------------------------------------------

// ownWxPosition is the operator's own position for the watch footprint: a
// live GPS fix when present, else the configured station coordinates
// (0,0 — the zero value nobody actually operates from — counts as "none").
func (s *Server) ownWxPosition() (lat, lon float64, source string, ageSec int) {
	if s.gpsMgr != nil {
		if fix, ok := s.gpsMgr.Current(); ok && fix.HasPosition() {
			return fix.Lat, fix.Lon, "gps", int(fix.Age(time.Now()).Seconds())
		}
	}
	if s.configMgr != nil {
		st := s.configMgr.Get().Station
		if st.Lat != 0 || st.Lon != 0 {
			return st.Lat, st.Lon, "config", 0
		}
	}
	return 0, 0, "none", 0
}

func parseWxPointGeometry(raw string) (wxalert.LatLon, bool) {
	var g struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(raw), &g); err != nil || g.Type != "Point" {
		return wxalert.LatLon{}, false
	}
	var c [2]float64
	if err := json.Unmarshal(g.Coordinates, &c); err != nil {
		return wxalert.LatLon{}, false
	}
	return wxalert.LatLon{Lat: c[1], Lon: c[0]}, true
}

// refreshWxFootprint recomputes the watch footprint (own position, plus —
// when a net is open — its roster, tracked stations, checkpoints and
// course/area annotations), regionally scopes the poller's query to it, and
// pushes the merged (global + net-override) policy into the live manager.
// reason labels the resulting wx_alerts broadcast ('footprint'|'netwatch').
func (s *Server) refreshWxFootprint(reason string) {
	mgr := s.WxAlertManager()
	if mgr == nil {
		return
	}
	base := s.wxAlertsConfig()

	var net *store.Net
	netID := ""
	if s.netMgr != nil {
		if n := s.netMgr.ActiveNet(); n != nil {
			net, netID = n, n.ID
		}
	}

	in := wxalert.Inputs{NetID: netID, Now: time.Now().UTC(), BufferMiles: base.DefaultBufferMiles}
	// Home zones are watched whether or not a net is open — that is what makes
	// them "home". A net's own extra zones are added on top, never instead.
	in.ExtraZones = append([]string{}, base.DefaultZones...)
	if net != nil {
		if net.WxBufferMiles > 0 {
			in.BufferMiles = net.WxBufferMiles
		}
		for _, z := range net.WxExtraZones {
			if !slices.Contains(in.ExtraZones, z) {
				in.ExtraZones = append(in.ExtraZones, z)
			}
		}
	}

	ownLat, ownLon, ownSource, ownAge := s.ownWxPosition()
	zonePoints := map[string]wxalert.LatLon{}
	if ownSource != "none" {
		own := wxalert.LatLon{Lat: ownLat, Lon: ownLon}
		in.Own, in.OwnSource, in.OwnAgeSec = &own, ownSource, ownAge
		zonePoints["own"] = own
	}

	if net != nil {
		if s.annMgr != nil {
			in.Annotations = s.annMgr.All()
		}
		if s.cpMgr != nil {
			if cps, err := s.cpMgr.GetCheckpointsForNet(netID); err == nil {
				seq := make(map[string]int, len(cps))
				for _, cp := range cps {
					seq[cp.Annotation.ID] = cp.Meta.SequenceNumber
				}
				in.CheckpointSeq = seq
			}
		}
		for _, ann := range in.Annotations {
			if ann.Type != "point" {
				continue
			}
			if ann.NetID != "" && ann.NetID != netID {
				continue
			}
			if pt, ok := parseWxPointGeometry(ann.Geometry); ok {
				zonePoints[ann.ID] = pt
			}
		}
		if s.netMgr != nil {
			for _, ci := range s.netMgr.GetCheckIns(netID) {
				if ci.Status == "released" {
					continue
				}
				if ci.Lat != nil && ci.Lon != nil {
					p := wxalert.LatLon{Lat: *ci.Lat, Lon: *ci.Lon}
					in.Positions = append(in.Positions, wxalert.PositionInput{
						Kind: "roster", ID: ci.Callsign, CheckInID: ci.ID, Label: ci.Callsign,
						Lat: p.Lat, Lon: p.Lon, HeardAt: ci.LastHeard,
					})
					zonePoints["station:"+ci.Callsign] = p
				}
				if s.tracker == nil {
					continue
				}
				for _, ts := range ci.TrackedStations {
					st, ok := s.tracker.Get(ts.Callsign)
					if !ok || st.Position == nil {
						continue
					}
					p := wxalert.LatLon{Lat: st.Position.Lat, Lon: st.Position.Lon}
					in.Positions = append(in.Positions, wxalert.PositionInput{
						Kind: "tracked", ID: ts.Callsign, CheckInID: ci.ID, Label: ts.Callsign,
						Lat: p.Lat, Lon: p.Lon, HeardAt: st.LastHeard,
					})
					zonePoints["station:"+ts.Callsign] = p
				}
			}
		}
	}

	in.ZoneLookup = s.resolveZonesFor(zonePoints)
	mgr.SetFootprintInputs(in)

	fp := mgr.Footprint()
	query := wxalert.ActiveQuery{IncludeTest: base.IncludeTest}
	states := map[string]bool{}
	for _, z := range fp.Zones {
		if len(z.UGC) >= 2 {
			states[z.UGC[:2]] = true
		}
	}
	for _, z := range fp.NearZones {
		if len(z.UGC) >= 2 {
			states[z.UGC[:2]] = true
		}
	}
	for st := range states {
		query.Areas = append(query.Areas, st)
	}
	if len(query.Areas) == 0 && in.Own != nil {
		query.Point = in.Own
	}
	mgr.SetQuery(query, false)

	// Warm the zone cache for the footprint's own zones. Without this nothing
	// ever populates it, so every zone reports cached=false, the client never
	// asks for its polygon (Map.svelte only requests cached ones), and the
	// ~85% of alerts that ship no polygon of their own can never draw — nor
	// survive going offline, which is the point of caching them at all.
	// Runs in the background: Prefetch is rate-limited and refreshWxFootprint
	// can be called synchronously from a settings PUT handler.
	if zc := mgr.Zones(); zc != nil {
		ugcs := make([]string, 0, len(fp.Zones))
		for _, z := range fp.Zones {
			ugcs = append(ugcs, z.UGC)
		}
		if len(ugcs) > 0 {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				zc.Prefetch(ctx, ugcs)
			}()
		}
	}

	mgr.UpdateConfig(s.wxManagerConfigFor(net))

	s.broadcastWxSnapshot(mgr, reason)
}

// wxPointZoneCell is the disk-cache grid key (BUILD-PLAN §1 row 48): two
// integer columns, never a formatted float string.
type wxPointZoneCell struct{ lat, lon int }

func wxCellOf(p wxalert.LatLon) wxPointZoneCell {
	return wxPointZoneCell{int(math.Floor(p.Lat * 100)), int(math.Floor(p.Lon * 100))}
}

// wxMaxFreshZoneLookups bounds how many *new* NWS /points requests one
// refreshWxFootprint call may make — refreshWxFootprint can run
// synchronously inside a settings PUT handler, so an unbounded footprint
// (a long course with many never-before-seen checkpoints) fills in over
// several refreshes rather than blocking one request for a long time.
const wxMaxFreshZoneLookups = 8

// resolveZonesFor resolves each named point's UGC codes via the disk-backed
// grid cache (store.WxPointZone), falling back to one NWS /points lookup
// per distinct, not-yet-cached cell — never per point, so a cluster of
// checkpoints in the same neighborhood costs at most one request.
func (s *Server) resolveZonesFor(pts map[string]wxalert.LatLon) map[string][]string {
	result := make(map[string][]string, len(pts))
	if len(pts) == 0 {
		return result
	}

	pointCell := make(map[string]wxPointZoneCell, len(pts))
	cellPoint := map[wxPointZoneCell]wxalert.LatLon{}
	for key, p := range pts {
		c := wxCellOf(p)
		pointCell[key] = c
		cellPoint[c] = p
	}

	now := time.Now().UTC()
	cellUGC := map[wxPointZoneCell][]string{}
	if s.store != nil {
		if rows, err := s.store.LoadWxPointZones(); err == nil {
			for _, row := range rows {
				if row.ExpiresAt.After(now) {
					cellUGC[wxPointZoneCell{row.CellLat, row.CellLon}] = row.UGC
				}
			}
		}
	}

	base := s.wxAlertsConfig()
	var provider wxalert.Provider
	if base.Enabled && strings.TrimSpace(base.Contact) != "" {
		ua := wxUserAgent(base.Contact)
		if p, err := wxalert.NewNWSProvider(wxalert.NWSConfig{BaseURL: base.BaseURL, UserAgent: ua}); err == nil {
			provider = p
		}
	}

	fresh := 0
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for c, p := range cellPoint {
		if _, ok := cellUGC[c]; ok {
			continue
		}
		if provider == nil || fresh >= wxMaxFreshZoneLookups {
			continue
		}
		fresh++
		pz, err := provider.ResolvePoint(ctx, p)
		if err != nil {
			continue
		}
		ugc := dedupNonEmpty(pz.County, pz.Forecast, pz.Fire)
		cellUGC[c] = ugc
		if s.store != nil {
			if err := s.store.SaveWxPointZone(store.WxPointZone{
				CellLat: c.lat, CellLon: c.lon, UGC: ugc, ExpiresAt: now.Add(30 * 24 * time.Hour),
			}); err != nil {
				log.Printf("[server] cache wx point zone: %v", err)
			}
		}
	}

	for key, c := range pointCell {
		result[key] = cellUGC[c]
	}
	return result
}

func dedupNonEmpty(vals ...string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range vals {
		v = strings.ToUpper(strings.TrimSpace(v))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// wxUserAgent mirrors app.go's manager construction so an ad-hoc provider
// built here for point resolution sends the same, NWS-policy-compliant
// User-Agent as the poller.
func wxUserAgent(contact string) string {
	return fmt.Sprintf("Nymeria (%s)", contact)
}

// --- Manager event bridge --------------------------------------------------

// wxAlertsWSPayload flattens Snapshot and reason into one JSON object (a TS
// intersection type on the wire), matching what stores/wxAlerts.ts expects.
type wxAlertsWSPayload struct {
	wxalert.Snapshot
	Reason string `json:"reason"`
}

func (s *Server) sendWxAlertsSnapshot(snap wxalert.Snapshot, reason string) {
	data, err := json.Marshal(map[string]any{"type": "wx_alerts", "data": wxAlertsWSPayload{Snapshot: snap, Reason: reason}})
	if err != nil {
		log.Printf("[server] marshal wx_alerts: %v", err)
		return
	}
	s.hub.Broadcast(data)
}

func (s *Server) broadcastWxLinkStatus(st wxalert.LinkStatus) {
	data, err := json.Marshal(map[string]any{"type": "wx_link_status", "data": st})
	if err != nil {
		log.Printf("[server] marshal wx_link_status: %v", err)
		return
	}
	s.hub.Broadcast(data)
}

func (s *Server) broadcastWxAckNet(id, netID string, ack wxalert.NetAck) {
	data, err := json.Marshal(map[string]any{"type": "wx_alert_ack_net", "data": map[string]any{
		"alertId": id, "netId": netID, "ackedForNet": ack,
	}})
	if err != nil {
		log.Printf("[server] marshal wx_alert_ack_net: %v", err)
		return
	}
	s.hub.Broadcast(data)
}

// broadcastWxSnapshot is refreshWxFootprint/settings-change's path: those
// mutate the manager without going through OnAlerts/TickExpiry, so no
// EventAlerts fires on its own — this reshapes and sends a snapshot the
// same way the event-driven path does.
func (s *Server) broadcastWxSnapshot(mgr *wxalert.Manager, reason string) {
	snap := mgr.Snapshot()
	s.sendWxAlertsSnapshot(snap, reason)
	s.persistWxSnapshot(mgr, snap)
	s.logWxSnapshotActivity(snap)
}

// handleWxAlertEvent is the manager event bridge's dispatcher (see
// bridgeWxAlertEvents in server.go).
func (s *Server) handleWxAlertEvent(mgr *wxalert.Manager, evt wxalert.Event) {
	switch evt.Type {
	case wxalert.EventAlerts:
		m, _ := evt.Data.(map[string]any)
		snap, _ := m["snapshot"].(wxalert.Snapshot)
		reason, _ := m["reason"].(string)
		s.sendWxAlertsSnapshot(snap, reason)
		s.persistWxSnapshot(mgr, snap)
		s.logWxSnapshotActivity(snap)

	case wxalert.EventLinkStatus:
		st, _ := evt.Data.(wxalert.LinkStatus)
		s.broadcastWxLinkStatus(st)
		s.logWxLinkTransition(st)

	case wxalert.EventAlertAckNet:
		m, _ := evt.Data.(map[string]any)
		id, _ := m["id"].(string)
		netID, _ := m["netId"].(string)
		ack, _ := m["ackedForNet"].(wxalert.NetAck)
		s.broadcastWxAckNet(id, netID, ack)
		s.persistWxAckNet(mgr, id, ack)
	}
}

// persistWxSnapshot writes the audit-trail row for every alert currently in
// the snapshot and the full registry snapshot (restart hydration — see
// internal/app's boot restore, which reads this same "registry_snapshot"
// key back through encoding/json without ever needing to name the
// registry's unexported element type).
func (s *Server) persistWxSnapshot(mgr *wxalert.Manager, snap wxalert.Snapshot) {
	if s.store == nil {
		return
	}
	for _, a := range snap.Alerts {
		data, err := json.Marshal(a)
		if err != nil {
			continue
		}
		row := store.WxAlertRow{
			ID: a.ID, NetID: a.NetID, Event: a.Event, Tier: string(a.Tier), State: string(a.State),
			Proximity: string(a.Proximity), NotifyClass: string(a.NotifyClass),
			Sent: a.Sent, Expires: a.Expires, EndsAt: a.EndsAt, ReplacedBy: a.ReplacedBy,
			FetchedAt: a.FetchedAt, FirstSeenAt: a.FirstSeenAt, UpdatedAt: a.UpdatedAt, Data: string(data),
		}
		if a.AckedForNet != nil {
			row.NetAckCallsign = a.AckedForNet.Callsign
			at := a.AckedForNet.At
			row.NetAckAt = &at
		}
		if err := s.store.SaveWxAlert(row); err != nil {
			log.Printf("[server] save wx alert %s: %v", a.ID, err)
		}
	}
	if blob, err := json.Marshal(mgr.RegistrySnapshotForPersistence()); err == nil {
		if err := s.store.SetWxMeta("registry_snapshot", string(blob)); err != nil {
			log.Printf("[server] persist wx registry snapshot: %v", err)
		}
	}
}

func (s *Server) persistWxAckNet(mgr *wxalert.Manager, id string, ack wxalert.NetAck) {
	a := mgr.Get(id)
	if a == nil {
		return
	}
	data, err := json.Marshal(a)
	if err == nil && s.store != nil {
		if err := s.store.UpdateWxAlertNetAck(id, ack.Callsign, ack.At, string(data)); err != nil {
			// The audit row may not exist yet (this ack raced the alert's
			// first snapshot persistence) — fall back to a full upsert.
			row := store.WxAlertRow{
				ID: a.ID, NetID: a.NetID, Event: a.Event, Tier: string(a.Tier), State: string(a.State),
				Proximity: string(a.Proximity), NotifyClass: string(a.NotifyClass),
				Sent: a.Sent, Expires: a.Expires, EndsAt: a.EndsAt,
				NetAckCallsign: ack.Callsign, NetAckAt: &ack.At,
				FetchedAt: a.FetchedAt, FirstSeenAt: a.FirstSeenAt, UpdatedAt: a.UpdatedAt, Data: string(data),
			}
			if err := s.store.SaveWxAlert(row); err != nil {
				log.Printf("[server] save wx alert (ack-net fallback) %s: %v", id, err)
			}
		}
	}
	if s.actLogger != nil {
		s.actLogger.Log(activity.Entry{
			Timestamp: ack.At, UserID: ack.UserID, UserName: ack.UserName,
			Action: activity.ActionWxAlertNetAcked, Target: id, Details: ack.Callsign,
		})
	}
	if s.netMgr != nil && a.NetID != "" {
		if err := s.netMgr.AddTimelineEvent(a.NetID, netcontrol.EventWxAlert, ack.Callsign,
			fmt.Sprintf("%s acknowledged for net", a.EffectiveEvent)); err != nil {
			log.Printf("[server] wx ack-net timeline: %v", err)
		}
	}
}

// logWxSnapshotActivity logs each alert's activity-log entry exactly once
// per real transition. A MatchedAlert's UpdatedAt changes precisely when
// the manager just recomputed it in reaction to a change (see
// Manager.applyNotifyForChanges), so tracking "have we logged this
// UpdatedAt for this id yet" reliably dedupes using only the public
// Snapshot API — the underlying Change list itself is never exposed on
// Events().
func (s *Server) logWxSnapshotActivity(snap wxalert.Snapshot) {
	if s.actLogger == nil && s.netMgr == nil {
		return
	}

	for _, a := range snap.Alerts {
		s.wxSeenMu.Lock()
		if s.wxSeen == nil {
			s.wxSeen = map[string]time.Time{}
		}
		last, ok := s.wxSeen[a.ID]
		alreadyLogged := ok && last.Equal(a.UpdatedAt)
		s.wxSeen[a.ID] = a.UpdatedAt
		s.wxSeenMu.Unlock()
		if alreadyLogged {
			continue
		}

		var action activity.Action
		switch {
		case a.State != wxalert.AlertStateActive:
			if a.EndedReason == "cancelled" {
				action = activity.ActionWxAlertCancelled
			} else {
				action = activity.ActionWxAlertExpired
			}
		case a.NotifyReason == "new":
			action = activity.ActionWxAlertReceived
		case a.NotifyReason == "update" || a.NotifyReason == "escalated":
			action = activity.ActionWxAlertUpdated
		default:
			continue
		}

		if s.actLogger != nil {
			s.actLogger.Log(activity.Entry{
				Timestamp: a.UpdatedAt, Action: action, Target: a.ID,
				Details: fmt.Sprintf("%s (%s)", a.EffectiveEvent, a.Proximity),
			})
		}

		watchOrAbove := wxalert.TierRank(a.Tier) >= wxalert.TierRank(wxalert.TierWatch)
		if s.netMgr == nil || a.NetID == "" || a.Proximity != wxalert.ProximityIn || !watchOrAbove {
			continue
		}
		switch {
		case a.State != wxalert.AlertStateActive:
			s.netMgr.AddTimelineEvent(a.NetID, netcontrol.EventWxAlert, "", fmt.Sprintf("%s ended (%s)", a.EffectiveEvent, a.EndedReason))
		case a.NotifyReason == "new" || a.NotifyReason == "escalated":
			s.netMgr.AddTimelineEvent(a.NetID, netcontrol.EventWxAlert, "", fmt.Sprintf("%s received", a.EffectiveEvent))
		}
	}
}

func (s *Server) logWxLinkTransition(st wxalert.LinkStatus) {
	if s.actLogger == nil {
		return
	}
	s.wxLinkMu.Lock()
	wasDown := s.wxLinkWasDown
	isDown := st.State == wxalert.LinkDown
	s.wxLinkWasDown = isDown
	s.wxLinkMu.Unlock()

	switch {
	case isDown && !wasDown:
		s.actLogger.Log(activity.Entry{Timestamp: time.Now().UTC(), Action: activity.ActionWxLinkDown, Details: st.LastError})
	case !isDown && wasDown:
		s.actLogger.Log(activity.Entry{Timestamp: time.Now().UTC(), Action: activity.ActionWxLinkRestored})
	}
}
