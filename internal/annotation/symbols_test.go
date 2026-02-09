package annotation

import (
	"testing"

	"github.com/narvel/nymeria/internal/aprs"
)

func TestSymbolForCategory(t *testing.T) {
	tests := []struct {
		category string
		wantCode byte
	}{
		{CategoryIncident, 'e'},
		{CategoryResource, '+'},
		{CategoryCheckpoint, '#'},
		{CategoryHazard, 'H'},
		{CategoryAssignment, 'A'},
		{CategoryGeneral, '-'},
		{CategoryRoute, '-'},
		{CategoryBoundary, '-'},
	}

	for _, tt := range tests {
		sym := SymbolForCategory(tt.category)
		if sym.Code != tt.wantCode {
			t.Errorf("SymbolForCategory(%q): code=%c, want=%c", tt.category, sym.Code, tt.wantCode)
		}
	}
}

func TestCategoryForSymbol(t *testing.T) {
	tests := []struct {
		sym      aprs.Symbol
		wantCat  string
	}{
		{aprs.Symbol{Table: '/', Code: 'e'}, CategoryIncident},
		{aprs.Symbol{Table: '/', Code: '+'}, CategoryResource},
		{aprs.Symbol{Table: '/', Code: '#'}, CategoryCheckpoint},
		{aprs.Symbol{Table: '\\', Code: 'H'}, CategoryHazard},
		{aprs.Symbol{Table: '/', Code: 'A'}, CategoryAssignment},
	}

	for _, tt := range tests {
		got := CategoryForSymbol(tt.sym)
		if got != tt.wantCat {
			t.Errorf("CategoryForSymbol(%c/%c): got %q, want %q", tt.sym.Table, tt.sym.Code, got, tt.wantCat)
		}
	}
}

func TestUnknownSymbolDefaultsGeneral(t *testing.T) {
	sym := aprs.Symbol{Table: '/', Code: 'Z'}
	cat := CategoryForSymbol(sym)
	if cat != CategoryGeneral {
		t.Errorf("unknown symbol should default to general, got %q", cat)
	}
}
