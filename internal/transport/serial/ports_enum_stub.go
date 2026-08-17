//go:build !linux && !windows

package serial

import bugstserial "go.bug.st/serial"

func listRawDefault() ([]rawPort, error) {
	names, err := bugstserial.GetPortsList()
	if err != nil {
		return nil, err
	}
	out := make([]rawPort, 0, len(names))
	for _, name := range names {
		out = append(out, rawPort{Name: name})
	}
	return out, nil
}
