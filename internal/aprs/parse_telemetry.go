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
