package annotation

import "github.com/narvel/nymeria/internal/aprs"

// categoryToSymbol maps annotation categories to APRS symbol defaults.
var categoryToSymbol = map[string]aprs.Symbol{
	CategoryIncident:   {Table: '/', Code: 'e'}, // Eyeball (incident marker)
	CategoryResource:   {Table: '/', Code: '+'}, // Red Cross (medical/resource)
	CategoryCheckpoint: {Table: '/', Code: '#'}, // Number sign (checkpoint)
	CategoryHazard:     {Table: '\\', Code: 'H'}, // Hazmat
	CategoryRoute:      {Table: '/', Code: '-'}, // House (generic)
	CategoryBoundary:   {Table: '/', Code: '-'}, // House (generic)
	CategoryAssignment: {Table: '/', Code: 'A'}, // Aid Station
	CategoryGeneral:    {Table: '/', Code: '-'}, // House (generic)
}

// symbolToCategory maps APRS symbols back to annotation categories.
var symbolToCategory = map[aprs.Symbol]string{
	{Table: '/', Code: 'e'}:  CategoryIncident,
	{Table: '/', Code: '+'}:  CategoryResource,
	{Table: '/', Code: '#'}:  CategoryCheckpoint,
	{Table: '\\', Code: 'H'}: CategoryHazard,
	{Table: '/', Code: 'A'}:  CategoryAssignment,
}

// SymbolForCategory returns the APRS symbol for a given annotation category.
func SymbolForCategory(cat string) aprs.Symbol {
	if sym, ok := categoryToSymbol[cat]; ok {
		return sym
	}
	return aprs.Symbol{Table: '/', Code: '-'}
}

// CategoryForSymbol returns the annotation category for an APRS symbol.
func CategoryForSymbol(sym aprs.Symbol) string {
	if cat, ok := symbolToCategory[sym]; ok {
		return cat
	}
	return CategoryGeneral
}
