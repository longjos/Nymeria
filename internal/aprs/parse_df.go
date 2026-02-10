package aprs

import (
	"math"
	"strconv"
)

// isDFSymbol checks if the symbol indicates a Direction Finding station.
// DF symbol is Table='/' Code='\'.
func isDFSymbol(sym Symbol) bool {
	return sym.Table == '/' && sym.Code == '\\'
}

// parseDFComment extracts DF bearing/NRQ data from a position comment.
// After CSE/SPD has been parsed from the comment, the remaining comment
// for a DF report starts with "/BRG/NRQ" where BRG is 3-digit bearing
// and NRQ is 3-digit Number/Range/Quality.
// Returns the parsed DFData and remaining comment text, or nil if not a DF comment.
func parseDFComment(comment string) (*DFData, string) {
	// Need at least "/BRG/NRQ" = 8 characters
	if len(comment) < 8 {
		return nil, comment
	}

	if comment[0] != '/' {
		return nil, comment
	}

	// Extract BRG (3 digits after first /)
	brgStr := comment[1:4]
	brg, err := strconv.ParseFloat(brgStr, 64)
	if err != nil {
		return nil, comment
	}

	// Must have separator
	if comment[4] != '/' {
		return nil, comment
	}

	// Extract NRQ (3 digits)
	nrqStr := comment[5:8]
	nrq, err := strconv.Atoi(nrqStr)
	if err != nil {
		return nil, comment
	}

	n := nrq / 100       // first digit: number of hits
	r := (nrq / 10) % 10 // second digit: range exponent
	q := nrq % 10         // third digit: quality

	// Decode range: 2^R miles for R>0, 0 for R=0
	var rangeMiles float64
	if r > 0 {
		rangeMiles = math.Pow(2, float64(r))
	}

	df := &DFData{
		Bearing: brg,
		Number:  n,
		Range:   rangeMiles,
		Quality: q,
	}

	remain := comment[8:]
	return df, remain
}
