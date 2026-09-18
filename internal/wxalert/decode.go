package wxalert

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// ErrMalformed wraps every decode failure that means "the body could not be
// parsed as an NWS active-alerts FeatureCollection at all" — as opposed to a
// single bad feature, which is silently dropped (see DropReason) so a partial
// payload degrades instead of blanking the whole panel.
var ErrMalformed = errors.New("wxalert: malformed NWS response")

// DropReason records why one feature in a FeatureCollection was not kept.
type DropReason struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Decoded is the result of DecodeCollection.
type Decoded struct {
	Alerts  []Alert
	Dropped []DropReason
	Updated time.Time // parsed from the collection's top-level "updated"; zero if absent
}

// rawCollection is the top-level GeoJSON FeatureCollection shape, plus the
// RFC7807 problem+json fields NWS returns on a bad request so a malformed
// body can name itself in the error ("Bad Request").
type rawCollection struct {
	Type     string            `json:"type"`
	Updated  string            `json:"updated"`
	Title    string            `json:"title"`
	Features []json.RawMessage `json:"features"`
}

type rawFeature struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Geometry   json.RawMessage `json:"geometry"`
	Properties *rawProperties  `json:"properties"`
}

type rawGeocode struct {
	SAME []string `json:"SAME"`
	UGC  []string `json:"UGC"`
}

type rawReference struct {
	ID         string `json:"@id"`
	Identifier string `json:"identifier"`
	Sender     string `json:"sender"`
	Sent       string `json:"sent"`
}

type rawProperties struct {
	ID            string              `json:"id"`
	AreaDesc      string              `json:"areaDesc"`
	Geocode       rawGeocode          `json:"geocode"`
	AffectedZones []string            `json:"affectedZones"`
	References    []rawReference      `json:"references"`
	Sent          string              `json:"sent"`
	Effective     string              `json:"effective"`
	Onset         *string             `json:"onset"`
	Expires       string              `json:"expires"`
	Ends          *string             `json:"ends"`
	Status        string              `json:"status"`
	MessageType   string              `json:"messageType"`
	Category      string              `json:"category"`
	Severity      string              `json:"severity"`
	Certainty     string              `json:"certainty"`
	Urgency       string              `json:"urgency"`
	Event         string              `json:"event"`
	Sender        string              `json:"sender"`
	SenderName    string              `json:"senderName"`
	Headline      string              `json:"headline"`
	Description   string              `json:"description"`
	Instruction   *string             `json:"instruction"`
	Response      string              `json:"response"`
	Parameters    map[string][]string `json:"parameters"`
}

// DecodeCollection parses an NWS active-alerts GeoJSON FeatureCollection.
// now is reserved for callers that want to timestamp the decode; it is not
// currently required to derive any Alert field.
func DecodeCollection(r io.Reader, now time.Time) (Decoded, error) {
	_ = now
	body, err := io.ReadAll(r)
	if err != nil {
		return Decoded{}, fmt.Errorf("%w: reading body: %v", ErrMalformed, err)
	}

	var coll rawCollection
	if err := json.Unmarshal(body, &coll); err != nil {
		return Decoded{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if coll.Type != "FeatureCollection" {
		msg := coll.Title
		if msg == "" {
			msg = fmt.Sprintf("unexpected top-level type %q", coll.Type)
		}
		return Decoded{}, fmt.Errorf("%w: %s", ErrMalformed, msg)
	}

	out := Decoded{}
	if coll.Updated != "" {
		if t, err := time.Parse(time.RFC3339, coll.Updated); err == nil {
			out.Updated = t.UTC()
		}
	}

	for _, raw := range coll.Features {
		a, drop, err := parseFeature(raw)
		if err != nil {
			return Decoded{}, err
		}
		if drop != "" {
			out.Dropped = append(out.Dropped, DropReason{ID: featureIDForDrop(raw), Reason: drop})
			continue
		}
		out.Alerts = append(out.Alerts, a)
	}
	return out, nil
}

// featureIDForDrop makes a best effort to recover an id for the drop log even
// when properties (which normally carries it) is missing or malformed.
func featureIDForDrop(raw json.RawMessage) string {
	var f struct {
		ID         string `json:"id"`
		Properties struct {
			ID string `json:"id"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return ""
	}
	if f.Properties.ID != "" {
		return f.Properties.ID
	}
	return f.ID
}

// parseFeature decodes one GeoJSON Feature into an Alert. A non-empty drop
// return means the feature is skipped without being an error for the whole
// payload; a non-nil err means the payload itself is unparsable JSON (should
// not normally happen since the caller already validated the array shape,
// but a feature that is not a JSON object would hit it).
func parseFeature(raw json.RawMessage) (Alert, string, error) {
	var f rawFeature
	if err := json.Unmarshal(raw, &f); err != nil {
		return Alert{}, "", fmt.Errorf("%w: feature: %v", ErrMalformed, err)
	}
	if f.Properties == nil {
		return Alert{}, "missing properties", nil
	}
	p := f.Properties

	sent, err := parseRFC3339(p.Sent)
	if err != nil {
		return Alert{}, "bad sent", nil
	}
	expires, err := parseRFC3339(p.Expires)
	if err != nil {
		return Alert{}, "bad expires", nil
	}

	a := Alert{
		ID:          p.ID,
		Provider:    "nws",
		ProviderURL: f.ID,
		Event:       p.Event,
		Headline:    p.Headline,
		Description: p.Description,
		Response:    p.Response,
		Category:    p.Category,
		Severity:    ParseSeverity(p.Severity),
		Certainty:   ParseCertainty(p.Certainty),
		Urgency:     ParseUrgency(p.Urgency),
		Status:      ParseStatus(p.Status),
		MessageType: ParseMessageType(p.MessageType),
		Sent:        sent,
		Expires:     expires,
		SenderName:  p.SenderName,
		Sender:      p.Sender,
		AreaDesc:    p.AreaDesc,
		Parameters:  p.Parameters,
	}
	if p.Instruction != nil {
		a.Instruction = *p.Instruction
	}
	if a.Parameters == nil {
		a.Parameters = map[string][]string{}
	}

	// Status/event guard: nothing that isn't a real, active-status alert
	// about real weather is ever broadcast to an operator. An alert with no
	// status at all is not trusted either.
	rawStatus := strings.ToLower(strings.TrimSpace(p.Status))
	switch {
	case rawStatus == "":
		return Alert{}, "status empty", nil
	case rawStatus != "actual":
		return Alert{}, "status " + rawStatus, nil
	case strings.EqualFold(strings.TrimSpace(p.Event), "test"):
		return Alert{}, "event test", nil
	}

	if p.Effective != "" {
		if t, err := parseRFC3339(p.Effective); err == nil {
			a.Effective = t
		}
	} else {
		a.Effective = sent
	}
	if p.Onset != nil {
		if t, err := parseRFC3339(*p.Onset); err == nil {
			a.Onset = &t
		}
	}
	if p.Ends != nil {
		if t, err := parseRFC3339(*p.Ends); err == nil {
			a.Ends = &t
		}
		// A malformed "ends" is optional data — kept, but left nil rather
		// than failing the whole alert.
	}

	a.UGC = unionUGC(p.Geocode.UGC, p.AffectedZones)
	a.SAME = p.Geocode.SAME
	if a.SAME == nil {
		a.SAME = []string{}
	}

	a.References = make([]Reference, 0, len(p.References))
	for _, r := range p.References {
		ref := Reference{ID: r.Identifier}
		if ref.ID == "" {
			ref.ID = idFromAlertURL(r.ID)
		}
		if t, err := parseRFC3339(r.Sent); err == nil {
			ref.Sent = t
		}
		a.References = append(a.References, ref)
	}

	a.SenderID = OfficeFromVTEC(a.Parameters["VTEC"])
	a.EffectiveEvent, _ = EffectiveEvent(a)
	a.Tier, _ = TierForEvent(a.Event)
	a.ShortCode = ShortCode(a.EffectiveEvent)

	if len(f.Geometry) > 0 && string(f.Geometry) != "null" {
		typ, rings, _, err := ringsFromGeoJSON(f.Geometry)
		if err != nil {
			return Alert{}, err.Error(), nil
		}
		bbox := BBox{}
		if len(rings) > 0 {
			var all []LatLon
			for _, ring := range rings {
				all = append(all, ring...)
			}
			bbox = bboxOf(all)
		}
		a.Geometry = &Geometry{Type: typ, Coordinates: geometryCoordinates(f.Geometry), Rings: rings, BBox: bbox}
	}

	return a, "", nil
}

func geometryCoordinates(raw json.RawMessage) json.RawMessage {
	var g struct {
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil
	}
	return g.Coordinates
}

func parseRFC3339(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// unionUGC returns the deduplicated union of geocode UGC codes and UGC codes
// derived from affected-zone URLs, preserving the order UGC codes appear in
// first, never nil.
func unionUGC(ugc []string, zoneURLs []string) []string {
	seen := make(map[string]bool, len(ugc)+len(zoneURLs))
	out := make([]string, 0, len(ugc)+len(zoneURLs))
	add := func(code string) {
		if code == "" || seen[code] {
			return
		}
		seen[code] = true
		out = append(out, code)
	}
	for _, u := range ugc {
		add(u)
	}
	for _, u := range zoneURLs {
		add(UGCFromZoneURL(u))
	}
	return out
}

// UGCFromZoneURL extracts the UGC code from a zone URL, e.g.
// "https://api.weather.gov/zones/county/MIC081" -> "MIC081". Returns "" for
// anything that doesn't end in a plausible UGC-shaped segment.
func UGCFromZoneURL(u string) string {
	idx := strings.LastIndex(u, "/")
	if idx < 0 || idx == len(u)-1 {
		return u
	}
	return u[idx+1:]
}

// OfficeFromVTEC returns the 4-letter WFO id from the first VTEC string,
// e.g. "/O.NEW.KGRR.TO.W.0012.260917T2000Z-260917T2045Z/" -> "KGRR". Returns
// "" when vtec is empty or unparsable — callers must not guess an office
// from SenderName.
func OfficeFromVTEC(vtec []string) string {
	if len(vtec) == 0 {
		return ""
	}
	parts := strings.Split(strings.Trim(vtec[0], "/"), ".")
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

// idFromAlertURL extracts the "urn:oid:..." id from an alert URL such as
// "https://api.weather.gov/alerts/urn:oid:...".
func idFromAlertURL(u string) string {
	const marker = "/alerts/"
	if idx := strings.Index(u, marker); idx >= 0 {
		return u[idx+len(marker):]
	}
	return u
}
