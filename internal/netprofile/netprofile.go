// Package netprofile is the per-net vocabulary registry: which panels the
// frontend mounts, which annotation/check-in categories a profile suggests,
// and the traffic-priority ladder a net uses. It is pure data plus
// validation — it never decides UI layout itself (the "panels" ids are a
// contract with the frontend) and it imports store only, never netcontrol or
// annotation (both of which may import netprofile), to avoid an import
// cycle.
package netprofile

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/narvel/nymeria/internal/store"
)

// Profile ids.
const (
	ProfileGeneral  = "general"
	ProfileBikeRide = "bike-ride"
)

// Priority ladder ids.
const (
	LadderARRL     = "arrl"      // existing netcontrol.Traffic* words, as tiers, for a uniform frontend shape
	LadderMarinARS = "marin-ars" // five-tier ride ladder
)

// Limits.
const (
	MaxTiers  = 8
	MaxRoutes = 12
)

// tierIDPattern is a lowercase slug: starts with a letter, up to 24 chars
// total, letters/digits/hyphens only. Route ids use routeIDPattern instead
// because ride agencies name routes "100", "75", "55" — a leading digit is
// the normal case, not the exception.
var tierIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)
var routeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,23}$`)

// Profile is a registry entry: pure data the frontend reads. Go never
// interprets Panels; the ids are a contract with the frontend.
type Profile struct {
	ID                     string               `json:"id"`
	Label                  string               `json:"label"`
	Description            string               `json:"description"`
	Panels                 []string             `json:"panels"`
	AnnotationCategories   []string             `json:"annotationCategories"`
	CheckInCategories      []string             `json:"checkInCategories"`
	DefaultCheckInCategory string               `json:"defaultCheckInCategory"`
	PriorityLadderID       string               `json:"priorityLadderId"`
	PriorityTiers          []store.PriorityTier `json:"priorityTiers"`
	HasRideConfig          bool                 `json:"hasRideConfig"`
}

// NetProfileView is what GET /api/nets/{id}/profile returns: everything the
// frontend needs to mount a net in one round trip.
type NetProfileView struct {
	NetID                  string               `json:"netId"`
	Profile                Profile              `json:"profile"`
	RideConfig             *store.NetRideConfig `json:"rideConfig,omitempty"`
	EffectivePriorityTiers []store.PriorityTier `json:"effectivePriorityTiers"`
}

func copyStrings(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	return out
}

func copyTiers(t []store.PriorityTier) []store.PriorityTier {
	out := make([]store.PriorityTier, len(t))
	for i, tier := range t {
		out[i] = tier
		out[i].Examples = copyStrings(tier.Examples)
	}
	return out
}

func copyProfile(p Profile) Profile {
	cp := p
	cp.Panels = copyStrings(p.Panels)
	cp.AnnotationCategories = copyStrings(p.AnnotationCategories)
	cp.CheckInCategories = copyStrings(p.CheckInCategories)
	cp.PriorityTiers = copyTiers(p.PriorityTiers)
	return cp
}

// All returns every registered profile, in stable order (general,
// bike-ride). Every element is a deep copy — mutating the result never
// affects the registry.
func All() []Profile {
	out := make([]Profile, len(registry))
	for i, p := range registry {
		out[i] = copyProfile(p)
	}
	return out
}

// Get returns a deep copy of the profile with the given id (already
// normalized — callers pass the exact registry id).
func Get(id string) (Profile, bool) {
	for _, p := range registry {
		if p.ID == id {
			return copyProfile(p), true
		}
	}
	return Profile{}, false
}

// Valid reports whether id is exactly a registered profile id. Callers
// normalize first (Normalize does not itself reject anything).
func Valid(id string) bool {
	_, ok := Get(id)
	return ok
}

// Normalize lowercases and trims id, and maps "" to ProfileGeneral. It never
// rejects an id — Valid does that.
func Normalize(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return ProfileGeneral
	}
	return id
}

// LadderTiers returns a deep copy of the shipped tier set for a ladder id,
// or nil if the ladder id is unknown.
func LadderTiers(ladderID string) []store.PriorityTier {
	switch ladderID {
	case LadderARRL:
		return copyTiers(arrlTiers)
	case LadderMarinARS:
		return copyTiers(marinARSTiers)
	default:
		return nil
	}
}

// EffectiveTiers resolves the priority ladder actually in force for a net:
// a non-empty override wins outright, otherwise the profile's shipped
// ladder. An unknown profile id falls back to the general ladder. Never
// returns nil.
func EffectiveTiers(profileID string, override []store.PriorityTier) []store.PriorityTier {
	if len(override) > 0 {
		return copyTiers(override)
	}
	p, ok := Get(Normalize(profileID))
	if !ok {
		p, _ = Get(ProfileGeneral)
	}
	tiers := copyTiers(p.PriorityTiers)
	if tiers == nil {
		tiers = []store.PriorityTier{}
	}
	return tiers
}

// ValidateTiers validates a custom priority-tier override. An empty slice is
// valid (it means "use the profile's shipped ladder").
func ValidateTiers(t []store.PriorityTier) error {
	if len(t) == 0 {
		return nil
	}
	if len(t) > MaxTiers {
		return fmt.Errorf("at most %d priority tiers", MaxTiers)
	}
	seenIDs := make(map[string]bool, len(t))
	seenRanks := make(map[int]bool, len(t))
	for _, tier := range t {
		if !tierIDPattern.MatchString(tier.ID) {
			return fmt.Errorf("tier id %q must be a lowercase slug", tier.ID)
		}
		if seenIDs[tier.ID] {
			return fmt.Errorf("duplicate tier id %q", tier.ID)
		}
		seenIDs[tier.ID] = true
		if strings.TrimSpace(tier.Label) == "" {
			return fmt.Errorf("tier %q needs a label", tier.ID)
		}
		seenRanks[tier.Rank] = true
	}
	for rank := 1; rank <= len(t); rank++ {
		if !seenRanks[rank] {
			return fmt.Errorf("tier ranks must be 1..%d with no gaps or repeats", len(t))
		}
	}
	return nil
}

// DefaultRideConfig returns the ride config seeded for a net the moment its
// profile becomes "bike-ride", so GET ride-config never 404s.
func DefaultRideConfig(netID string) store.NetRideConfig {
	return store.NetRideConfig{
		NetID:  netID,
		Routes: []store.RideRoute{},
		Cutoff: store.RideCutoffPolicy{
			DeclinedSAGIsUnsupported: true,
		},
		PriorityTiers: []store.PriorityTier{},
	}
}

// NormalizeRideConfig trims strings and turns nil slices into empty ones in
// place. It never rejects anything — ValidateRideConfig does that, after
// calling this first.
func NormalizeRideConfig(c *store.NetRideConfig) {
	if c == nil {
		return
	}
	c.AgencyName = strings.TrimSpace(c.AgencyName)
	c.EventName = strings.TrimSpace(c.EventName)
	c.EventDate = strings.TrimSpace(c.EventDate)
	c.Cutoff.Notes = strings.TrimSpace(c.Cutoff.Notes)

	if c.Routes == nil {
		c.Routes = []store.RideRoute{}
	}
	for i := range c.Routes {
		c.Routes[i].ID = strings.ToLower(strings.TrimSpace(c.Routes[i].ID))
		c.Routes[i].Name = strings.TrimSpace(c.Routes[i].Name)
	}

	if c.PriorityTiers == nil {
		c.PriorityTiers = []store.PriorityTier{}
	}
	for i := range c.PriorityTiers {
		c.PriorityTiers[i].ID = strings.ToLower(strings.TrimSpace(c.PriorityTiers[i].ID))
		c.PriorityTiers[i].Label = strings.TrimSpace(c.PriorityTiers[i].Label)
		if c.PriorityTiers[i].Examples == nil {
			c.PriorityTiers[i].Examples = []string{}
		}
	}
}

// ValidateRideConfig normalizes c in place (trims strings, nil slices ->
// empty, lowercases route/tier ids) and then validates it.
func ValidateRideConfig(c *store.NetRideConfig) error {
	if c == nil {
		return fmt.Errorf("ride config is required")
	}
	NormalizeRideConfig(c)

	if utf8.RuneCountInString(c.AgencyName) > 120 {
		return fmt.Errorf("agencyName must be at most 120 characters")
	}
	if utf8.RuneCountInString(c.EventName) > 120 {
		return fmt.Errorf("eventName must be at most 120 characters")
	}
	if utf8.RuneCountInString(c.Cutoff.Notes) > 2000 {
		return fmt.Errorf("notes must be at most 2000 characters")
	}
	if c.EventDate != "" {
		if _, err := time.Parse("2006-01-02", c.EventDate); err != nil {
			return fmt.Errorf("eventDate must be in YYYY-MM-DD format")
		}
	}

	if len(c.Routes) > MaxRoutes {
		return fmt.Errorf("at most %d routes", MaxRoutes)
	}
	seenRouteIDs := make(map[string]bool, len(c.Routes))
	for _, route := range c.Routes {
		if !routeIDPattern.MatchString(route.ID) {
			return fmt.Errorf("route id %q must be a lowercase slug", route.ID)
		}
		if seenRouteIDs[route.ID] {
			return fmt.Errorf("duplicate route id %q", route.ID)
		}
		seenRouteIDs[route.ID] = true
		if route.Name == "" {
			return fmt.Errorf("route %q needs a name", route.ID)
		}
		if route.DistanceMiles <= 0 || route.DistanceMiles > 1000 {
			return fmt.Errorf("route %q distance must be greater than 0 and at most 1000 miles", route.ID)
		}
		if route.CutoffAt != nil && c.Cutoff.CourseClosesAt != nil && route.CutoffAt.After(*c.Cutoff.CourseClosesAt) {
			return fmt.Errorf("route %q cutoffAt must not be after courseClosesAt", route.ID)
		}
	}

	if c.Cutoff.CourseOpensAt != nil && c.Cutoff.CourseClosesAt != nil {
		if !c.Cutoff.CourseOpensAt.Before(*c.Cutoff.CourseClosesAt) {
			return fmt.Errorf("courseOpensAt must be before courseClosesAt")
		}
	}

	if err := ValidateTiers(c.PriorityTiers); err != nil {
		return err
	}

	return nil
}
