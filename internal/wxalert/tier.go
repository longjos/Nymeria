package wxalert

import "strings"

// FloorText is the sentence the settings UI and the per-net watch-area sheet
// must display verbatim: the hard floor in notify.go cannot be turned off by
// any allowlist edit, mute or per-net setting, and the UI has to say so in
// plain words. Keep identical to the frontend's FLOOR_TEXT constant.
func FloorText() string {
	return "An Extreme-severity, Immediate-urgency alert inside the watch area always shows at least a toast. Nothing on this page can turn that off."
}

// tierOverrides holds event names that don't follow the " Warning"/" Watch"/
// " Advisory"/" Statement" suffix convention, plus the two damage-threat-
// derived "emergency" names so the seed allowlist can name them. Keys are
// lowercase.
var tierOverrides = map[string]Tier{
	"flash flood emergency":     TierWarning,
	"tornado emergency":         TierWarning,
	"child abduction emergency": TierWarning,
	"civil emergency message":   TierWarning,
	"evacuation immediate":      TierWarning,
	"extreme fire danger":       TierWarning,
	"local area emergency":      TierWarning,
	"blue alert":                TierWarning,
	"air quality alert":         TierAdvisory,
	"911 telephone outage":      TierStatement,
	"administrative message":    TierStatement,
	"hazardous weather outlook": TierStatement,
	"hydrologic outlook":        TierStatement,
	"short term forecast":       TierStatement,
	"test":                      TierStatement,
}

// knownEvents is the canonical 111-name NWS event list (the union of every
// warning/watch/advisory/statement name, EXCLUDING the two derived emergency
// names — GET /wx/event-types adds those two on top of KnownEvents()).
var knownEvents = []string{
	// Warning (48): 42 suffix-based + 6 overrides.
	"Ashfall Warning", "Avalanche Warning", "Blizzard Warning", "Blowing Dust Warning",
	"Civil Danger Warning", "Coastal Flood Warning", "Dust Storm Warning", "Earthquake Warning",
	"Extreme Heat Warning", "Extreme Cold Warning", "Extreme Wind Warning", "Fire Warning",
	"Flash Flood Warning", "Flood Warning", "Freeze Warning", "Gale Warning",
	"Hazardous Materials Warning", "Hazardous Seas Warning", "Heavy Freezing Spray Warning",
	"High Surf Warning", "High Wind Warning", "Hurricane Force Wind Warning", "Hurricane Warning",
	"Ice Storm Warning", "Lake Effect Snow Warning", "Lakeshore Flood Warning",
	"Law Enforcement Warning", "Nuclear Power Plant Warning", "Radiological Hazard Warning",
	"Red Flag Warning", "Severe Thunderstorm Warning", "Shelter In Place Warning",
	"Snow Squall Warning", "Special Marine Warning", "Storm Surge Warning", "Storm Warning",
	"Tornado Warning", "Tropical Storm Warning", "Tsunami Warning", "Typhoon Warning",
	"Volcano Warning", "Winter Storm Warning",
	"Child Abduction Emergency", "Civil Emergency Message", "Evacuation Immediate",
	"Extreme Fire Danger", "Local Area Emergency", "Blue Alert",

	// Watch (23)
	"Avalanche Watch", "Coastal Flood Watch", "Extreme Heat Watch", "Extreme Cold Watch",
	"Fire Weather Watch", "Flash Flood Watch", "Flood Watch", "Freeze Watch", "Gale Watch",
	"Hazardous Seas Watch", "Heavy Freezing Spray Watch", "High Wind Watch",
	"Hurricane Force Wind Watch", "Hurricane Watch", "Lakeshore Flood Watch",
	"Severe Thunderstorm Watch", "Storm Surge Watch", "Storm Watch", "Tornado Watch",
	"Tropical Storm Watch", "Tsunami Watch", "Typhoon Watch", "Winter Storm Watch",

	// Advisory (24): 23 suffix-based + 1 override.
	"Air Stagnation Advisory", "Ashfall Advisory", "Avalanche Advisory", "Blowing Dust Advisory",
	"Brisk Wind Advisory", "Coastal Flood Advisory", "Cold Weather Advisory", "Dense Fog Advisory",
	"Dense Smoke Advisory", "Dust Advisory", "Flood Advisory", "Freezing Fog Advisory",
	"Freezing Spray Advisory", "Frost Advisory", "Heat Advisory", "High Surf Advisory",
	"Lake Wind Advisory", "Lakeshore Flood Advisory", "Low Water Advisory",
	"Small Craft Advisory", "Tsunami Advisory", "Wind Advisory", "Winter Weather Advisory",
	"Air Quality Alert",

	// Statement (16): 10 suffix-based + 6 overrides.
	"Beach Hazards Statement", "Coastal Flood Statement", "Flash Flood Statement",
	"Flood Statement", "Lakeshore Flood Statement", "Marine Weather Statement",
	"Rip Current Statement", "Severe Weather Statement", "Special Weather Statement",
	"Tropical Cyclone Local Statement",
	"911 Telephone Outage", "Administrative Message", "Hazardous Weather Outlook",
	"Hydrologic Outlook", "Short Term Forecast", "Test",
}

// KnownEvents returns the canonical 111 NWS event names, in a stable order.
func KnownEvents() []string {
	out := make([]string, len(knownEvents))
	copy(out, knownEvents)
	return out
}

// eventCanonical maps every recognized event name, lowercased, to its
// properly-cased canonical spelling — the 111 canonical names plus the two
// damage-threat-derived "emergency" names.
var eventCanonical = func() map[string]string {
	m := make(map[string]string, len(knownEvents)+2)
	for _, e := range knownEvents {
		m[strings.ToLower(e)] = e
	}
	m["flash flood emergency"] = "Flash Flood Emergency"
	m["tornado emergency"] = "Tornado Emergency"
	return m
}()

// CanonicalEventName looks up event case-insensitively and returns its
// canonical spelling, or ok=false if it is not a recognized NWS event name
// (or one of the two derived emergency names).
func CanonicalEventName(event string) (string, bool) {
	name, ok := eventCanonical[strings.ToLower(strings.TrimSpace(event))]
	return name, ok
}

// knownEventSet is the lowercase-keyed lookup for "is this exact name one of
// the 111 canonical events" (built once from knownEvents).
var knownEventSet = func() map[string]bool {
	m := make(map[string]bool, len(knownEvents))
	for _, e := range knownEvents {
		m[strings.ToLower(e)] = true
	}
	return m
}()

var suffixTier = []struct {
	suffix string
	tier   Tier
}{
	{" warning", TierWarning},
	{" watch", TierWatch},
	{" advisory", TierAdvisory},
	{" statement", TierStatement},
}

// TierForEvent maps an NWS event name to a Tier, and reports whether the name
// is recognized. Recognition means the name is an exact-match override (which
// includes the two derived emergency names) or is literally one of the 111
// canonical event names — an unrecognized name still gets a best-effort tier
// from its suffix (or TierStatement) so the UI never crashes on a name NWS
// adds later, but known is false so callers (e.g. the allowlist validator)
// can reject it.
func TierForEvent(event string) (Tier, bool) {
	norm := strings.ToLower(strings.TrimSpace(event))
	if norm == "" {
		return TierStatement, false
	}
	if t, ok := tierOverrides[norm]; ok {
		return t, true
	}
	known := knownEventSet[norm]
	for _, s := range suffixTier {
		if strings.HasSuffix(norm, s.suffix) {
			return s.tier, known
		}
	}
	return TierStatement, known
}

// EffectiveEvent derives the name and severity the notification/allowlist
// logic should use: a Flash Flood Warning with flashFloodDamageThreat
// CATASTROPHIC is treated as "Flash Flood Emergency" at Extreme severity; a
// Tornado Warning with tornadoDamageThreat CATASTROPHIC becomes "Tornado
// Emergency". Every other alert is unchanged.
func EffectiveEvent(a Alert) (string, Severity) {
	norm := strings.ToLower(strings.TrimSpace(a.Event))
	switch norm {
	case "flash flood warning":
		if hasCatastrophic(a.Parameters["flashFloodDamageThreat"]) {
			return "Flash Flood Emergency", SeverityExtreme
		}
	case "tornado warning":
		if hasCatastrophic(a.Parameters["tornadoDamageThreat"]) {
			return "Tornado Emergency", SeverityExtreme
		}
	}
	return a.Event, a.Severity
}

func hasCatastrophic(v []string) bool {
	for _, s := range v {
		if strings.EqualFold(strings.TrimSpace(s), "CATASTROPHIC") {
			return true
		}
	}
	return false
}

// shortCodeWhole is checked before the prefix+suffix rule.
var shortCodeWhole = map[string]string{
	"special weather statement": "SPS",
	"small craft advisory":      "SCA",
	"air quality alert":         "AIR QLTY",
}

// shortCodePrefix maps the event name with its tier suffix removed to an
// on-air style prefix code. Checked as a full-string match (not substring)
// so "Wind" never shadows "Extreme Wind" or "High Wind".
var shortCodePrefix = map[string]string{
	"tornado":             "TOR",
	"severe thunderstorm": "SVR",
	"flash flood":         "FF",
	"flood":               "FLD",
	"extreme wind":        "EXT WIND",
	"ice storm":           "ICE STORM",
	"blizzard":            "BLIZZARD",
	"winter storm":        "WNTR STORM",
	"winter weather":      "WNTR WX",
	"high wind":           "HI WIND",
	"wind":                "WIND",
	"heat":                "HEAT",
	"excessive heat":      "EXT HEAT",
	"extreme heat":        "EXT HEAT",
	"dense fog":           "FOG",
	"red flag":            "RED FLAG",
	"hurricane":           "HURCN",
	"tropical storm":      "TS",
	"gale":                "GALE",
	"dust storm":          "DUST",
	"freeze":              "FREEZE",
	"frost":               "FROST",
}

var shortCodeSuffix = []struct {
	suffix string
	code   string
}{
	{" warning", "WARN"},
	{" watch", "WATCH"},
	{" advisory", "ADV"},
	{" statement", "STMT"},
	{" emergency", "EMERG"},
}

const shortCodeMaxLen = 14

// ShortCode returns the on-air style code for map chips and APRS bulletins,
// at most 14 characters. Call it with EffectiveEvent, not the raw Event, so
// "Flash Flood Emergency" gets its own code rather than the warning's.
func ShortCode(event string) string {
	norm := strings.ToLower(strings.TrimSpace(event))
	if code, ok := shortCodeWhole[norm]; ok {
		return code
	}

	base := norm
	suffixCode := ""
	for _, s := range shortCodeSuffix {
		if strings.HasSuffix(norm, s.suffix) {
			base = strings.TrimSuffix(norm, s.suffix)
			suffixCode = s.code
			break
		}
	}

	var prefixCode string
	if code, ok := shortCodePrefix[base]; ok {
		prefixCode = code
	} else {
		prefixCode = strings.ToUpper(base)
	}

	var out string
	if suffixCode != "" {
		out = prefixCode + " " + suffixCode
	} else {
		out = prefixCode
	}
	out = strings.ToUpper(strings.TrimSpace(out))
	if len(out) > shortCodeMaxLen {
		out = strings.TrimSpace(out[:shortCodeMaxLen])
	}
	return out
}
