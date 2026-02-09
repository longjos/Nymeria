package aprs

// Symbol represents an APRS symbol identified by table and code characters.
type Symbol struct {
	Table byte `json:"table"` // '/' for primary, '\\' for alternate, or overlay char
	Code  byte `json:"code"`  // Symbol code character
}

// IsPrimary returns true if the symbol is from the primary table.
func (s Symbol) IsPrimary() bool {
	return s.Table == '/'
}

// IsAlternate returns true if the symbol is from the alternate table.
func (s Symbol) IsAlternate() bool {
	return s.Table == '\\'
}

// HasOverlay returns true if the symbol has an overlay character.
func (s Symbol) HasOverlay() bool {
	return s.Table != '/' && s.Table != '\\'
}
