//go:build linux || windows

package serial

import "go.bug.st/serial/enumerator"

func listRawDefault() ([]rawPort, error) {
	details, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	out := make([]rawPort, 0, len(details))
	for _, d := range details {
		if d == nil {
			continue
		}
		out = append(out, rawPort{
			Name:         d.Name,
			VID:          d.VID,
			PID:          d.PID,
			SerialNumber: d.SerialNumber,
			Product:      d.Product,
			IsUSB:        d.IsUSB,
		})
	}
	return out, nil
}
