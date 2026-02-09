package annotation

// Template represents a pre-built annotation template for quick creation.
type Template struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Pack            string `json:"pack"`
	Category        string `json:"category"`
	Type            string `json:"type"`
	DefaultPriority string `json:"defaultPriority"`
	Description     string `json:"description"`
}

// BuiltinTemplates contains all pre-defined annotation templates.
var BuiltinTemplates = []Template{
	// --- Event Pack ---
	{ID: "event-aid-station", Name: "Aid Station", Pack: "event", Category: CategoryResource, Type: TypePoint, DefaultPriority: PriorityRoutine, Description: "Medical aid or water station along a course"},
	{ID: "event-sag-vehicle", Name: "SAG Vehicle", Pack: "event", Category: CategoryResource, Type: TypePoint, DefaultPriority: PriorityRoutine, Description: "Support and gear vehicle position"},
	{ID: "event-course-route", Name: "Course Route", Pack: "event", Category: CategoryRoute, Type: TypeLine, DefaultPriority: PriorityRoutine, Description: "Event course or parade route"},
	{ID: "event-incident", Name: "Incident Report", Pack: "event", Category: CategoryIncident, Type: TypePoint, DefaultPriority: PriorityPriority, Description: "On-course incident requiring attention"},
	{ID: "event-start-finish", Name: "Start / Finish", Pack: "event", Category: CategoryCheckpoint, Type: TypePoint, DefaultPriority: PriorityRoutine, Description: "Start or finish line location"},

	// --- SAR Pack ---
	{ID: "sar-lkp", Name: "Last Known Point", Pack: "sar", Category: CategoryIncident, Type: TypePoint, DefaultPriority: PriorityUrgent, Description: "Last known position of subject"},
	{ID: "sar-search-sector", Name: "Search Sector", Pack: "sar", Category: CategoryAssignment, Type: TypeArea, DefaultPriority: PriorityPriority, Description: "Defined search area assignment"},
	{ID: "sar-clue", Name: "Clue", Pack: "sar", Category: CategoryIncident, Type: TypePoint, DefaultPriority: PriorityPriority, Description: "Physical clue or evidence location"},
	{ID: "sar-base-camp", Name: "Base Camp", Pack: "sar", Category: CategoryResource, Type: TypePoint, DefaultPriority: PriorityRoutine, Description: "SAR base of operations"},
	{ID: "sar-hasty-route", Name: "Hasty Search Route", Pack: "sar", Category: CategoryRoute, Type: TypeLine, DefaultPriority: PriorityUrgent, Description: "Initial rapid search route"},

	// --- Disaster Pack ---
	{ID: "disaster-shelter", Name: "Shelter", Pack: "disaster", Category: CategoryResource, Type: TypePoint, DefaultPriority: PriorityRoutine, Description: "Emergency shelter or refuge"},
	{ID: "disaster-damage", Name: "Damage Assessment", Pack: "disaster", Category: CategoryIncident, Type: TypePoint, DefaultPriority: PriorityPriority, Description: "Structural damage assessment point"},
	{ID: "disaster-road-closure", Name: "Road Closure", Pack: "disaster", Category: CategoryHazard, Type: TypePoint, DefaultPriority: PriorityUrgent, Description: "Blocked or impassable road"},
	{ID: "disaster-distribution", Name: "Distribution Point", Pack: "disaster", Category: CategoryResource, Type: TypePoint, DefaultPriority: PriorityRoutine, Description: "Supply distribution or pickup point"},
	{ID: "disaster-evac-route", Name: "Evacuation Route", Pack: "disaster", Category: CategoryRoute, Type: TypeLine, DefaultPriority: PriorityUrgent, Description: "Designated evacuation path"},
}

// AllTemplates returns all builtin templates.
func AllTemplates() []Template {
	return BuiltinTemplates
}

// TemplatesByPack returns templates filtered by pack name.
func TemplatesByPack(pack string) []Template {
	var result []Template
	for _, t := range BuiltinTemplates {
		if t.Pack == pack {
			result = append(result, t)
		}
	}
	return result
}
