package aprs

import (
	"regexp"
	"strconv"
	"strings"
)

// APRS 1.2 frequency-in-comment patterns.
// Matches: "146.520 MHz", "147.105MHz", "446.000 mhz" (case-insensitive on receipt per spec)
var freqRe = regexp.MustCompile(`(\d{2,3}\.\d{2,3})\s*[Mm][Hh][Zz]`)

// parseFrequency extracts APRS 1.2 frequency data from a position comment.
// Returns nil if no frequency is found.
// Supports microwave band prefixes A-E before the frequency:
//
//	A = +1200 MHz (23cm band)
//	B = +2300 MHz (13cm band)
//	C = +3400 MHz (9cm band)
//	D = +5600 MHz (6cm band)
//	E = +10000 MHz (3cm band)
func parseFrequency(comment string) *FrequencyData {
	// Check for microwave band prefix (A-E) before the frequency
	var microwaveOffset float64
	freqComment := comment
	if len(freqComment) > 0 {
		switch freqComment[0] {
		case 'A':
			microwaveOffset = 1200.0
			freqComment = freqComment[1:]
		case 'B':
			microwaveOffset = 2300.0
			freqComment = freqComment[1:]
		case 'C':
			microwaveOffset = 3400.0
			freqComment = freqComment[1:]
		case 'D':
			microwaveOffset = 5600.0
			freqComment = freqComment[1:]
		case 'E':
			microwaveOffset = 10000.0
			freqComment = freqComment[1:]
		}
	}

	m := freqRe.FindStringSubmatchIndex(freqComment)
	if m == nil {
		return nil
	}

	freqStr := freqComment[m[2]:m[3]]
	freq, err := strconv.ParseFloat(freqStr, 64)
	if err != nil {
		return nil
	}

	fd := &FrequencyData{Freq: freq + microwaveOffset}

	// Parse optional fields after the frequency match
	rest := freqComment[m[1]:]
	rest = strings.TrimLeft(rest, " ")

	// Parse space-separated fields: Tnnn, Cnnn, Dnnn, +/-nnn, Rnnm/Rnnk
	for len(rest) > 0 {
		rest = strings.TrimLeft(rest, " ")
		if len(rest) == 0 {
			break
		}

		consumed := false

		// Tone: Tnnn (PL tone, no decimal)
		if len(rest) >= 4 && rest[0] == 'T' && rest[1] >= '0' && rest[1] <= '9' {
			end := 1
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			if val, err := strconv.ParseFloat(rest[1:end], 64); err == nil {
				fd.Tone = val
				rest = rest[end:]
				consumed = true
			}
		}

		// CTCSS: Cnnn
		if !consumed && len(rest) >= 4 && rest[0] == 'C' && rest[1] >= '0' && rest[1] <= '9' {
			end := 1
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			if val, err := strconv.ParseFloat(rest[1:end], 64); err == nil {
				fd.Tone = val
				rest = rest[end:]
				consumed = true
			}
		}

		// DCS: Dnnn
		if !consumed && len(rest) >= 4 && rest[0] == 'D' && rest[1] >= '0' && rest[1] <= '9' {
			end := 1
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			if val, err := strconv.Atoi(rest[1:end]); err == nil {
				fd.DCS = val
				rest = rest[end:]
				consumed = true
			}
		}

		// Offset: +nnn or -nnn (tens of kHz)
		if !consumed && len(rest) >= 4 && (rest[0] == '+' || rest[0] == '-') && rest[1] >= '0' && rest[1] <= '9' {
			sign := 1.0
			if rest[0] == '-' {
				sign = -1.0
			}
			end := 1
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			if val, err := strconv.ParseFloat(rest[1:end], 64); err == nil {
				fd.Offset = sign * val * 10.0 / 1000.0 // tens of kHz to MHz
				rest = rest[end:]
				consumed = true
			}
		}

		// Range: Rnnm (miles) or Rnnk (km)
		if !consumed && len(rest) >= 3 && rest[0] == 'R' && rest[1] >= '0' && rest[1] <= '9' {
			end := 1
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			if end < len(rest) {
				if val, err := strconv.ParseFloat(rest[1:end], 64); err == nil {
					unit := rest[end]
					if unit == 'm' {
						fd.Range = val * 1.60934 // miles to km
						rest = rest[end+1:]
						consumed = true
					} else if unit == 'k' {
						fd.Range = val
						rest = rest[end+1:]
						consumed = true
					}
				}
			}
		}

		if !consumed {
			// Skip to next space-separated token
			idx := strings.IndexByte(rest, ' ')
			if idx < 0 {
				break
			}
			rest = rest[idx+1:]
		}
	}

	return fd
}
