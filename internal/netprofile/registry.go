package netprofile

import "github.com/narvel/nymeria/internal/store"

// arrlTiers is the generic ARRL-style traffic ladder used by the "general"
// profile. The ids are the literal netcontrol.Traffic* constant values —
// netprofile cannot import netcontrol (netcontrol imports netprofile), so
// internal/netcontrol/profile_test.go pins the two together from the other
// side.
var arrlTiers = []store.PriorityTier{
	{ID: "emergency", Label: "Emergency", Rank: 1, Description: "Immediate threat to life or property.", Examples: []string{"Injury requiring ambulance", "Structure fire"}},
	{ID: "priority", Label: "Priority", Rank: 2, Description: "Time-sensitive traffic that must move ahead of routine.", Examples: []string{"Resource request for an active incident"}},
	{ID: "welfare", Label: "Welfare", Rank: 3, Description: "Health-and-welfare inquiries about persons in the affected area.", Examples: []string{"Is my family at the shelter"}},
	{ID: "routine", Label: "Routine", Rank: 4, Description: "Everything else.", Examples: []string{"Position report", "Status check"}},
}

// marinARSTiers is the five-tier Marin ARS ladder used by the "bike-ride"
// profile. The Examples encode the load-bearing distinctions the research
// flagged (out-of-water vs. running-low, from-the-course vs.
// from-a-checkpoint) — do not "tidy" them into one tier.
var marinARSTiers = []store.PriorityTier{
	{ID: "emergency", Label: "Emergency", Rank: 1,
		Description: "Life-threatening injury or illness; ambulance needed now. Break any traffic.",
		Examples:    []string{"Rider unresponsive or not breathing", "Suspected heart attack or stroke", "Vehicle vs. rider collision with serious injury", "Fire or crime in progress"}},
	{ID: "priority", Label: "Priority", Rank: 2,
		Description: "Injury or hazard needing medical or law-enforcement attention that is not immediately life-threatening.",
		Examples:    []string{"Rider down, conscious, needs medical evaluation", "Possible fracture or head strike", "Missing or overdue rider", "Road hazard blocking the route"}},
	{ID: "high", Label: "High", Rank: 3,
		Description: "Rider or rest stop cannot continue without help; move ahead of medium.",
		Examples:    []string{"Rest stop OUT of water or food", "Transport requested FROM THE COURSE (rider stranded between stops)", "Mechanical that SAG cannot fix on the road", "Course marking wrong at a turn"}},
	{ID: "medium", Label: "Medium", Rank: 4,
		Description: "Needs action this hour; nobody is stranded.",
		Examples:    []string{"Rest stop supplies RUNNING LOW", "Transport requested FROM A CHECKPOINT or rest stop", "SAG vehicle at capacity", "Sweep position report"}},
	{ID: "low", Label: "Low", Rank: 5,
		Description: "Informational; wait for a lull.",
		Examples:    []string{"Routine check-in", "Lead / trailing rider report", "Rest stop opened on schedule", "Weather observation"}},
}

// registry is the ordered set of net profiles the frontend can mount. Order
// is part of the contract (All() returns it as-is): general first, bike-ride
// second.
var registry = []Profile{
	{
		ID:          ProfileGeneral,
		Label:       "General",
		Description: "Standard tactical / emergency net.",
		Panels: []string{
			"stations", "messages", "netcontrol", "annotations", "checkpoints",
			"weather", "telemetry", "wxalerts", "activity",
		},
		AnnotationCategories: []string{
			"incident", "resource", "checkpoint", "hazard", "route", "boundary",
			"assignment", "general", "aid", "staging", "shelter", "parking",
			"start", "finish",
		},
		CheckInCategories:      []string{"general", "command", "medical", "sag", "marshal", "fixed", "mobile", "tactical"},
		DefaultCheckInCategory: "general",
		PriorityLadderID:       LadderARRL,
		PriorityTiers:          arrlTiers,
		HasRideConfig:          false,
	},
	{
		ID:          ProfileBikeRide,
		Label:       "Bike Ride",
		Description: "Charity ride support: SAG, rest stops, sweep, course closure.",
		Panels: []string{
			"netcontrol", "ride-status", "sag-board", "rest-stops", "shutoffs",
			"supply", "medical", "annotations", "checkpoints", "wxalerts",
			"activity", "messages",
		},
		AnnotationCategories: []string{
			"route", "start", "finish", "aid", "checkpoint", "hazard", "incident",
			"staging", "parking", "general",
		},
		CheckInCategories:      []string{"sag", "fixed", "medical", "marshal", "mobile", "command"},
		DefaultCheckInCategory: "sag",
		PriorityLadderID:       LadderMarinARS,
		PriorityTiers:          marinARSTiers,
		HasRideConfig:          true,
	},
}
