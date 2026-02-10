package aprs

import (
	"fmt"
	"strconv"
	"strings"
)

// parseTelemetryPayload parses a telemetry payload (DTI 'T').
// Format: T#SSS,AAA,AAA,AAA,AAA,AAA,BBBBBBBB[,comment]
// SSS = sequence number, AAA = analog values, BBBBBBBB = 8-bit digital value
func parseTelemetryPayload(payload string) (*TelemetryData, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("telemetry payload too short")
	}

	// Skip "T#"
	rest := payload
	if !strings.HasPrefix(rest, "T#") {
		return nil, fmt.Errorf("telemetry must start with T#")
	}
	rest = rest[2:]

	parts := strings.SplitN(rest, ",", 8) // seq + 5 analog + digital + optional comment
	if len(parts) < 7 {
		return nil, fmt.Errorf("telemetry needs at least 7 fields, got %d", len(parts))
	}

	tel := &TelemetryData{}

	// Sequence number
	seq, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid telemetry sequence: %w", err)
	}
	tel.Seq = seq

	// 5 analog values
	for i := 0; i < 5; i++ {
		val, err := strconv.ParseFloat(parts[i+1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid analog value %d: %w", i, err)
		}
		tel.Analog[i] = val
	}

	// 8-bit digital value (binary string)
	digitalStr := parts[6]
	if len(digitalStr) >= 8 {
		var digital byte
		for i := 0; i < 8; i++ {
			if digitalStr[i] == '1' {
				digital |= 1 << (7 - i)
			}
		}
		tel.Digital = digital
	}

	// Optional comment
	if len(parts) > 7 {
		tel.Comment = parts[7]
	}

	return tel, nil
}

// IsTelemetryMeta returns true if the message text starts with a telemetry
// metadata prefix (PARM, UNIT, EQNS, or BITS).
func IsTelemetryMeta(text string) bool {
	for _, prefix := range []string{"PARM.", "UNIT.", "EQNS.", "BITS."} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// ParseTelemetryPARM parses a PARM. message into parameter names.
// Format: PARM.name1,name2,name3,name4,name5[,bit1,...,bit8]
func ParseTelemetryPARM(text string) (*TelemetryMetaMessage, error) {
	if !strings.HasPrefix(text, "PARM.") {
		return nil, fmt.Errorf("not a PARM message")
	}
	rest := text[5:]
	parts := strings.Split(rest, ",")

	meta := &TelemetryMetaMessage{MetaType: TelemetryMetaPARM}
	for i := 0; i < 5 && i < len(parts); i++ {
		meta.ParamNames[i] = strings.TrimSpace(parts[i])
	}
	// Bits 6-13 are digital channel labels
	for i := 5; i < 13 && i < len(parts); i++ {
		meta.BitLabels[i-5] = strings.TrimSpace(parts[i])
	}
	return meta, nil
}

// ParseTelemetryUNIT parses a UNIT. message into unit labels.
// Format: UNIT.unit1,unit2,unit3,unit4,unit5[,bunit1,...,bunit8]
func ParseTelemetryUNIT(text string) (*TelemetryMetaMessage, error) {
	if !strings.HasPrefix(text, "UNIT.") {
		return nil, fmt.Errorf("not a UNIT message")
	}
	rest := text[5:]
	parts := strings.Split(rest, ",")

	meta := &TelemetryMetaMessage{MetaType: TelemetryMetaUNIT}
	for i := 0; i < 5 && i < len(parts); i++ {
		meta.UnitLabels[i] = strings.TrimSpace(parts[i])
	}
	// Bits 6-13 are digital channel unit labels (stored in BitLabels)
	for i := 5; i < 13 && i < len(parts); i++ {
		meta.BitLabels[i-5] = strings.TrimSpace(parts[i])
	}
	return meta, nil
}

// ParseTelemetryEQNS parses an EQNS. message into equation coefficients.
// Format: EQNS.a1,b1,c1,a2,b2,c2,...,a5,b5,c5 (15 values total)
func ParseTelemetryEQNS(text string) (*TelemetryMetaMessage, error) {
	if !strings.HasPrefix(text, "EQNS.") {
		return nil, fmt.Errorf("not an EQNS message")
	}
	rest := text[5:]
	parts := strings.Split(rest, ",")
	if len(parts) < 15 {
		return nil, fmt.Errorf("EQNS needs 15 coefficients, got %d", len(parts))
	}

	meta := &TelemetryMetaMessage{MetaType: TelemetryMetaEQNS}
	for ch := 0; ch < 5; ch++ {
		for j := 0; j < 3; j++ {
			v, err := strconv.ParseFloat(strings.TrimSpace(parts[ch*3+j]), 64)
			if err != nil {
				return nil, fmt.Errorf("invalid EQNS coefficient [%d][%d]: %w", ch, j, err)
			}
			meta.Equations[ch][j] = v
		}
	}
	return meta, nil
}

// ParseTelemetryBITS parses a BITS. message into bit sense and project title.
// Format: BITS.XXXXXXXX[,project title]
// X = 0 or 1, indicating the active sense for each digital bit.
func ParseTelemetryBITS(text string) (*TelemetryMetaMessage, error) {
	if !strings.HasPrefix(text, "BITS.") {
		return nil, fmt.Errorf("not a BITS message")
	}
	rest := text[5:]

	meta := &TelemetryMetaMessage{MetaType: TelemetryMetaBITS}

	// Split on comma — first part is bit sense, rest is project title
	if idx := strings.IndexByte(rest, ','); idx >= 0 {
		meta.ProjectTitle = strings.TrimSpace(rest[idx+1:])
		rest = rest[:idx]
	}

	if len(rest) >= 8 {
		var bits byte
		for i := 0; i < 8; i++ {
			if rest[i] == '1' {
				bits |= 1 << (7 - i)
			}
		}
		meta.BitSense = bits
	}
	return meta, nil
}

// ParseTelemetryMeta dispatches to the appropriate metadata parser based on prefix.
func ParseTelemetryMeta(text string) (*TelemetryMetaMessage, error) {
	switch {
	case strings.HasPrefix(text, "PARM."):
		return ParseTelemetryPARM(text)
	case strings.HasPrefix(text, "UNIT."):
		return ParseTelemetryUNIT(text)
	case strings.HasPrefix(text, "EQNS."):
		return ParseTelemetryEQNS(text)
	case strings.HasPrefix(text, "BITS."):
		return ParseTelemetryBITS(text)
	default:
		return nil, fmt.Errorf("unknown telemetry metadata prefix")
	}
}
