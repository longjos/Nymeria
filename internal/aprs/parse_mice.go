package aprs

import (
	"fmt"
)

// Mic-E message types based on the 3-bit message code
var micEMessages = [8]string{
	"Emergency",
	"Priority",
	"Special",
	"Committed",
	"Returning",
	"In Service",
	"En Route",
	"Off Duty",
}

// parseMicEPayload parses a Mic-E position payload (DTI '`' or '\'').
// Mic-E encodes latitude in the destination address field.
// The info field encodes longitude, speed, and course.
func parseMicEPayload(frame APRSFrame) (*MicEData, error) {
	payload := frame.Payload
	if len(payload) < 9 {
		return nil, fmt.Errorf("mic-e payload too short: need at least 9 bytes, got %d", len(payload))
	}

	dest := frame.Destination.Call
	if len(dest) < 6 {
		// Pad with spaces if needed
		for len(dest) < 6 {
			dest += " "
		}
	}

	// Decode latitude from destination address
	// Each byte of the destination encodes one digit of latitude
	// plus message bits and N/S/E/W indicators
	lat, msgBits, north, lonOffset, west, err := decodeMicEDest(dest)
	if err != nil {
		return nil, fmt.Errorf("mic-e destination decode: %w", err)
	}

	// Skip DTI byte
	info := payload[1:]

	// Decode longitude from info field bytes 0-2
	lon, err := decodeMicELon(info, lonOffset, west)
	if err != nil {
		return nil, fmt.Errorf("mic-e longitude decode: %w", err)
	}

	// Decode speed and course from info field bytes 3-5
	speed, course := decodeMicESpeedCourse(info)

	if !north {
		lat = -lat
	}

	// Symbol: info[6] = symbol code, info[7] = symbol table
	var sym Symbol
	if len(info) >= 8 {
		sym.Code = info[6]
		sym.Table = info[7]
	}

	// Determine Mic-E message type from message bits
	msgType := ""
	if msgBits < 8 {
		msgType = micEMessages[msgBits]
	}

	// Detect radio model from suffix bytes
	radioModel := detectMicERadio(info)

	micE := &MicEData{
		Position: PositionData{
			Lat:    lat,
			Lon:    lon,
			Speed:  speed * 1.852, // knots to km/h
			Course: course,
			Symbol: sym,
		},
		MicEMsg:    msgType,
		RadioModel: radioModel,
	}

	return micE, nil
}

// decodeMicEDest extracts lat, message bits, and lon offset from destination.
// Returns: lat (degrees), msgBits, isNorth, lonOffset, isWest, error
func decodeMicEDest(dest string) (float64, int, bool, int, bool, error) {
	// Each destination byte encodes a latitude digit
	// Characters 0-9 = digits 0-9 (standard)
	// Characters A-K = digits 0-9 + custom message
	// Characters P-Y = digits 0-9 + standard message
	// Characters L,Z = space (not used for digit)

	digits := make([]int, 6)
	msgBits := 0
	north := false
	lonOffset := 0
	west := false

	for i := 0; i < 6; i++ {
		c := dest[i]
		var digit int
		var msg bool

		switch {
		case c >= '0' && c <= '9':
			digit = int(c - '0')
			msg = false
		case c >= 'A' && c <= 'J':
			digit = int(c - 'A')
			msg = true // custom
		case c == 'K':
			digit = 0 // space
			msg = true
		case c == 'L':
			digit = 0 // space
			msg = false
		case c >= 'P' && c <= 'Y':
			digit = int(c - 'P')
			msg = true // standard
		case c == 'Z':
			digit = 0 // space
			msg = true
		default:
			return 0, 0, false, 0, false, fmt.Errorf("invalid mic-e destination char %d: %c", i, c)
		}

		digits[i] = digit

		// Message bits from bytes 0-2 (A-K or P-Z = 1, 0-9 or L = 0)
		if i < 3 && msg {
			msgBits |= 1 << (2 - i)
		}

		// Byte 3: N/S indicator (A-K,P-Z = North, 0-9,L = South)
		if i == 3 {
			north = msg
		}
		// Byte 4: longitude offset (A-K,P-Z = +100, 0-9,L = +0)
		if i == 4 {
			if msg {
				lonOffset = 100
			}
		}
		// Byte 5: E/W indicator (A-K,P-Z = West, 0-9,L = East)
		if i == 5 {
			west = msg
		}
	}

	// Latitude: digits[0:2] = degrees, digits[2:4] = minutes, digits[4:6] = hundredths of minutes
	latDeg := float64(digits[0]*10 + digits[1])
	latMin := float64(digits[2]*10+digits[3]) + float64(digits[4]*10+digits[5])/100.0
	lat := latDeg + latMin/60.0

	return lat, msgBits, north, lonOffset, west, nil
}

// decodeMicELon decodes longitude from Mic-E info field.
func decodeMicELon(info string, lonOffset int, west bool) (float64, error) {
	if len(info) < 3 {
		return 0, fmt.Errorf("info field too short for longitude")
	}

	// Byte 0: longitude degrees
	d := int(info[0]) - 28 + lonOffset
	if d >= 180 && d <= 189 {
		d -= 80
	} else if d >= 190 && d <= 199 {
		d -= 190
	}

	// Byte 1: longitude minutes
	m := int(info[1]) - 28
	if m >= 60 {
		m -= 60
	}

	// Byte 2: longitude hundredths of minutes
	h := int(info[2]) - 28

	lon := float64(d) + (float64(m)+float64(h)/100.0)/60.0
	if west {
		lon = -lon
	}

	return lon, nil
}

// decodeMicESpeedCourse decodes speed and course from Mic-E info field bytes 3-5.
func decodeMicESpeedCourse(info string) (float64, float64) {
	if len(info) < 6 {
		return 0, 0
	}

	// Speed: encoded in bytes 3-4
	// Byte 3: speed hundreds/tens digit
	sp := int(info[3]) - 28
	speed := sp / 10 * 100 // hundreds digit * 100
	speedUnits := sp % 10  // tens digit

	// Byte 4: speed units and course hundreds
	dc := int(info[4]) - 28
	speedUnits = speedUnits*10 + dc/10
	speed += speedUnits
	courseHundreds := (dc % 10) * 100

	// Byte 5: course tens/units
	course := int(info[5]) - 28
	course += courseHundreds

	// Speed >= 800 means subtract 800
	if speed >= 800 {
		speed -= 800
	}

	// Course >= 400 means subtract 400
	if course >= 400 {
		course -= 400
	}

	return float64(speed), float64(course)
}

// detectMicERadio attempts to identify the radio model from Mic-E suffix bytes.
// Supports APRS 1.0 Kenwood types and APRS 1.2 extended MFR TYPE codes.
func detectMicERadio(info string) string {
	if len(info) < 9 {
		return ""
	}
	suffix := info[8:]
	if len(suffix) == 0 {
		return ""
	}

	// APRS 1.2: "OTHER Mic-E" format uses 3-byte suffix: `MT or 'MT
	// where ` or ' is the format marker, M is manufacturer, T is type
	if len(suffix) >= 3 && (suffix[0] == '`' || suffix[0] == '\'') {
		mfr := suffix[1]
		typ := suffix[2]

		// Yaesu manufacturer prefix '_'
		if mfr == '_' {
			switch typ {
			case 'b':
				return "Yaesu VX-8"
			case '"':
				return "Yaesu FTM-350"
			case '#':
				return "Yaesu VX-8G"
			case '$':
				return "Yaesu FT1D"
			case '%':
				return "Yaesu FTM-400DR"
			case ')':
				return "Yaesu FTM-100D"
			case '(':
				return "Yaesu FT2D"
			case '0':
				return "Yaesu FT3D"
			case '3':
				return "Yaesu FT5D"
			case '1':
				return "Yaesu FTM-300D"
			}
		}

		// Anytone manufacturer prefix '('
		if mfr == '(' {
			switch typ {
			case '5':
				return "Anytone D578UV"
			case '8':
				return "Anytone D878UV"
			}
		}

		// Byonics manufacturer prefix '|'
		if mfr == '|' {
			switch typ {
			case '3':
				return "Byonics TinyTrack3"
			case '4':
				return "Byonics TinyTrack4"
			}
		}

		// SCS manufacturer prefix ':'
		if mfr == ':' {
			switch typ {
			case '4':
				return "SCS P4dragon DR-7400"
			case '8':
				return "SCS P4dragon DR-7800"
			}
		}
	}

	// Kenwood TM-D710: ends with "]=", must check before TM-D700
	if len(suffix) >= 2 && suffix[len(suffix)-2:] == "]=" {
		return "Kenwood TM-D710"
	}

	// Kenwood TH-D74: ends with ">^"
	if len(suffix) >= 2 && suffix[len(suffix)-2:] == ">^" {
		return "Kenwood TH-D74"
	}

	// Kenwood TH-D7: ends with ">"
	if suffix[len(suffix)-1] == '>' {
		return "Kenwood TH-D7"
	}

	// Kenwood TM-D700: ends with "]"
	if suffix[len(suffix)-1] == ']' {
		return "Kenwood TM-D700"
	}

	return ""
}
