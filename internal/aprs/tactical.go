package aprs

import "strings"

// ParseTacticalMessage parses the body of an APRS message addressed to
// "TACTICAL". The body format is "call1=name1;call2=name2".
// Returns a map of callsign → tactical alias.
func ParseTacticalMessage(body string) map[string]string {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}

	result := make(map[string]string)
	pairs := strings.Split(body, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.Index(pair, "=")
		if idx <= 0 {
			continue
		}
		callsign := strings.TrimSpace(pair[:idx])
		alias := strings.TrimSpace(pair[idx+1:])
		if callsign == "" || alias == "" {
			continue
		}
		result[strings.ToUpper(callsign)] = alias
	}

	if len(result) == 0 {
		return nil
	}
	return result
}
