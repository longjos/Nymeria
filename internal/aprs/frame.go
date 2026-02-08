package aprs

// Address represents an AX.25 address (callsign + SSID).
type Address struct {
	Call string `json:"call"`
	SSID int    `json:"ssid,omitempty"`
	HBit bool   `json:"hBit,omitempty"` // Has-been-digipeated flag (AX.25)
}

// String returns the address as "CALL-SSID" or just "CALL" if SSID is 0.
// If HBit is set, appends "*" (TNC2 convention for has-been-digipeated).
func (a Address) String() string {
	s := a.Call
	if a.SSID != 0 {
		s += "-" + itoa(a.SSID)
	}
	if a.HBit {
		s += "*"
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 2)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// APRSFrame represents a raw APRS frame with source, destination, path, and payload.
type APRSFrame struct {
	Source      Address
	Destination Address
	Path        []Address
	Payload     string
}

// String returns the frame in TNC2 format: SOURCE>DESTINATION,PATH:payload
func (f APRSFrame) String() string {
	s := f.Source.String() + ">" + f.Destination.String()
	for _, p := range f.Path {
		s += "," + p.String()
	}
	s += ":" + f.Payload
	return s
}
