package aprs

import (
	"fmt"
	"strings"
)

const (
	ax25AddrLen     = 7  // Each AX.25 address is 7 bytes
	ax25MaxDigis    = 8  // Maximum digipeaters in path
	ax25MaxCallLen  = 6  // Max callsign length
	ax25Control     = 0x03 // UI frame
	ax25PID         = 0xF0 // No layer 3 protocol
)

// encodeAX25Address encodes a single Address into 7 AX.25 bytes.
// The last parameter sets the last-address bit (bit 0 of SSID byte).
func encodeAX25Address(addr Address, last bool) []byte {
	out := make([]byte, ax25AddrLen)
	call := strings.ToUpper(addr.Call)

	// Encode callsign: left-shift each ASCII byte by 1, pad with shifted spaces
	for i := 0; i < ax25MaxCallLen; i++ {
		if i < len(call) {
			out[i] = call[i] << 1
		} else {
			out[i] = ' ' << 1 // 0x40
		}
	}

	// SSID byte: bits 6-7 reserved (set to 1), bits 1-4 SSID, bit 5 H-bit, bit 0 last flag
	ssidByte := byte(0x60) // reserved bits 6,7 = 0b01100000
	ssidByte |= byte(addr.SSID&0x0F) << 1
	if addr.HBit {
		ssidByte |= 0x80 // bit 7... actually H-bit is bit 7 in some refs
		// In AX.25: SSID byte bit layout: C R R SSID SSID SSID SSID EXT
		// C = command/response (bit 7), R R = reserved (bits 6,5), SSID = bits 4-1, EXT = bit 0
		// For digipeater addresses, bit 7 is the H-bit (has-been-repeated)
		// So H-bit is bit 7 = 0x80
	}
	if last {
		ssidByte |= 0x01 // bit 0 = last address extension bit
	}
	out[6] = ssidByte

	return out
}

// decodeAX25Address decodes 7 AX.25 bytes into an Address and last flag.
func decodeAX25Address(data []byte) (Address, bool) {
	// Decode callsign: right-shift each byte by 1, trim trailing spaces
	var call strings.Builder
	for i := 0; i < ax25MaxCallLen; i++ {
		ch := data[i] >> 1
		if ch != ' ' {
			call.WriteByte(ch)
		}
	}

	ssidByte := data[6]
	ssid := int((ssidByte >> 1) & 0x0F)
	hbit := ssidByte&0x80 != 0
	last := ssidByte&0x01 != 0

	return Address{
		Call: call.String(),
		SSID: ssid,
		HBit: hbit,
	}, last
}

// validateAddress checks that an address is valid for AX.25 encoding.
func validateAddress(addr Address) error {
	if len(addr.Call) > ax25MaxCallLen {
		return fmt.Errorf("callsign %q exceeds %d characters", addr.Call, ax25MaxCallLen)
	}
	if len(addr.Call) == 0 {
		return fmt.Errorf("empty callsign")
	}
	if addr.SSID < 0 || addr.SSID > 15 {
		return fmt.Errorf("SSID %d out of range 0-15", addr.SSID)
	}
	return nil
}

// EncodeAX25 encodes an APRSFrame into raw AX.25 bytes (without FCS).
// The FCS is handled by the TNC in KISS mode.
// Frame format: [Dest 7] [Src 7] [Digi0 7]...[DigiN 7] [Control] [PID] [Info...]
func EncodeAX25(frame APRSFrame) ([]byte, error) {
	if len(frame.Path) > ax25MaxDigis {
		return nil, fmt.Errorf("too many digipeaters: %d (max %d)", len(frame.Path), ax25MaxDigis)
	}

	// Validate all addresses
	if err := validateAddress(frame.Destination); err != nil {
		return nil, fmt.Errorf("destination: %w", err)
	}
	if err := validateAddress(frame.Source); err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	for i, p := range frame.Path {
		if err := validateAddress(p); err != nil {
			return nil, fmt.Errorf("path[%d]: %w", i, err)
		}
	}

	// Determine which address is last
	noPath := len(frame.Path) == 0

	// Total size: (2 + len(path)) * 7 + 2 (control+PID) + len(payload)
	addrCount := 2 + len(frame.Path)
	size := addrCount*ax25AddrLen + 2 + len(frame.Payload)
	out := make([]byte, 0, size)

	// Destination (never last unless no source, but source always follows)
	out = append(out, encodeAX25Address(frame.Destination, false)...)

	// Source (last only if no path)
	out = append(out, encodeAX25Address(frame.Source, noPath)...)

	// Digipeater path
	for i, p := range frame.Path {
		isLast := i == len(frame.Path)-1
		out = append(out, encodeAX25Address(p, isLast)...)
	}

	// Control and PID
	out = append(out, ax25Control, ax25PID)

	// Info field
	out = append(out, []byte(frame.Payload)...)

	return out, nil
}

// DecodeAX25 decodes raw AX.25 bytes into an APRSFrame.
// The bytes should NOT include FCS (stripped by TNC in KISS mode).
func DecodeAX25(data []byte) (APRSFrame, error) {
	// Minimum: destination(7) + source(7) + control(1) + PID(1) = 16
	if len(data) < 16 {
		return APRSFrame{}, fmt.Errorf("AX.25 frame too short: %d bytes (minimum 16)", len(data))
	}

	// Decode destination
	dst, _ := decodeAX25Address(data[0:7])
	dst.HBit = false // H-bit not meaningful for destination

	// Decode source
	src, lastIsSrc := decodeAX25Address(data[7:14])
	src.HBit = false // H-bit not meaningful for source

	offset := 14
	var path []Address

	// Decode digipeater path if source wasn't the last address
	if !lastIsSrc {
		for offset+ax25AddrLen <= len(data) {
			addr, last := decodeAX25Address(data[offset : offset+ax25AddrLen])
			path = append(path, addr)
			offset += ax25AddrLen
			if last {
				break
			}
		}
	}

	// Need at least 2 more bytes for control + PID
	if offset+2 > len(data) {
		return APRSFrame{}, fmt.Errorf("AX.25 frame truncated: missing control/PID at offset %d", offset)
	}

	control := data[offset]
	pid := data[offset+1]
	offset += 2

	if control != ax25Control {
		return APRSFrame{}, fmt.Errorf("unexpected AX.25 control byte: 0x%02X (expected 0x03 UI frame)", control)
	}
	if pid != ax25PID {
		return APRSFrame{}, fmt.Errorf("unexpected AX.25 PID byte: 0x%02X (expected 0xF0)", pid)
	}

	payload := string(data[offset:])

	return APRSFrame{
		Source:      src,
		Destination: dst,
		Path:        path,
		Payload:     payload,
	}, nil
}
