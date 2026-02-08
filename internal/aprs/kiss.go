package aprs

import (
	"fmt"
	"io"
)

// KISS protocol constants.
const (
	FEND  = 0xC0 // Frame delimiter
	FESC  = 0xDB // Escape character
	TFEND = 0xDC // Transposed FEND
	TFESC = 0xDD // Transposed FESC

	CmdData = 0x00 // Data frame command (port 0)
)

// KISSEscape escapes raw data bytes for KISS transport.
// FEND (0xC0) → FESC TFEND, FESC (0xDB) → FESC TFESC.
func KISSEscape(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		switch b {
		case FEND:
			out = append(out, FESC, TFEND)
		case FESC:
			out = append(out, FESC, TFESC)
		default:
			out = append(out, b)
		}
	}
	return out
}

// KISSUnescape reverses KISS escaping.
// FESC TFEND → FEND, FESC TFESC → FESC.
func KISSUnescape(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == FESC && i+1 < len(data) {
			switch data[i+1] {
			case TFEND:
				out = append(out, FEND)
				i++
			case TFESC:
				out = append(out, FESC)
				i++
			default:
				out = append(out, data[i])
			}
		} else {
			out = append(out, data[i])
		}
	}
	return out
}

// KISSEncode wraps AX.25 data in a KISS frame: FEND CMD DATA FEND.
// The data bytes are escaped: FEND→FESC,TFEND and FESC→FESC,TFESC.
func KISSEncode(data []byte) []byte {
	escaped := KISSEscape(data)
	frame := make([]byte, 0, len(escaped)+3)
	frame = append(frame, FEND, CmdData)
	frame = append(frame, escaped...)
	frame = append(frame, FEND)
	return frame
}

// KISSDecode extracts the AX.25 payload from a KISS frame.
// Strips FEND delimiters, verifies command byte, and unescapes data.
func KISSDecode(frame []byte) ([]byte, error) {
	if len(frame) == 0 {
		return nil, fmt.Errorf("empty KISS frame")
	}

	// Strip leading FENDs
	start := 0
	for start < len(frame) && frame[start] == FEND {
		start++
	}
	if start >= len(frame) {
		return nil, fmt.Errorf("KISS frame contains only FEND bytes")
	}

	// Strip trailing FENDs
	end := len(frame)
	for end > start && frame[end-1] == FEND {
		end--
	}

	// First byte after FEND(s) is the command byte
	cmd := frame[start]
	if cmd != CmdData {
		return nil, fmt.Errorf("unsupported KISS command byte: 0x%02X", cmd)
	}

	data := frame[start+1 : end]
	return KISSUnescape(data), nil
}

// KISSFrameReader reads KISS frames from a byte stream.
// It accumulates bytes between FEND delimiters.
type KISSFrameReader struct {
	r   io.Reader
	buf [1]byte
}

// NewKISSFrameReader creates a reader that reads KISS frames from an io.Reader.
func NewKISSFrameReader(r io.Reader) *KISSFrameReader {
	return &KISSFrameReader{r: r}
}

// ReadFrame reads the next complete KISS frame from the stream.
// It returns the decoded AX.25 data (unescaped, command byte stripped).
// Returns io.EOF when the underlying reader is exhausted.
func (kr *KISSFrameReader) ReadFrame() ([]byte, error) {
	// Skip any leading FENDs and find the start of data
	var frame []byte
	inFrame := false

	for {
		_, err := kr.r.Read(kr.buf[:])
		if err != nil {
			if err == io.EOF && len(frame) > 0 {
				// Partial frame at EOF — decode what we have
				return KISSDecode(append([]byte{FEND}, append(frame, FEND)...))
			}
			return nil, err
		}

		b := kr.buf[0]

		if b == FEND {
			if inFrame && len(frame) > 0 {
				// End of frame — decode it
				return KISSDecode(append([]byte{FEND}, append(frame, FEND)...))
			}
			// Leading FEND or empty frame, skip
			inFrame = true
			continue
		}

		if inFrame {
			frame = append(frame, b)
		}
	}
}
