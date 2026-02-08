package aprs

import (
	"fmt"
	"strconv"
	"strings"
)

// parseWeatherPayload parses positionless weather (DTI '_').
// Format: _MMDDHHMMc...s...g...t...r...p...P...h...b...
func parseWeatherPayload(payload string) (*WeatherData, *PositionData, error) {
	if len(payload) < 2 {
		return nil, nil, fmt.Errorf("weather payload too short")
	}

	// Skip DTI '_' and 8-char timestamp
	rest := payload[1:]
	if len(rest) < 8 {
		return nil, nil, fmt.Errorf("weather payload too short for timestamp")
	}
	// Skip MMDDHHMM timestamp
	wxData := rest[8:]

	wx := parseWeatherFields(wxData)
	return wx, nil, nil
}

// parseWeatherFromPosition parses weather data embedded in a position report.
// This is called when a position report's symbol code is '_' (weather station).
// The comment portion contains weather fields like g005t077r000...
func parseWeatherFromPosition(pos *PositionData) *WeatherData {
	if pos == nil {
		return nil
	}

	// For weather position reports, CSE/SPD actually contains wind direction/speed
	wx := &WeatherData{}

	// Wind direction from course
	if pos.Course > 0 || pos.Speed > 0 {
		dir := pos.Course
		wx.WindDir = &dir
		// Speed was already converted from knots to km/h, but for weather it's mph
		// Actually in APRS weather-in-position, the CSE/SPD gives wind dir/speed in degrees/mph
		// The speed is in mph (not knots like regular CSE/SPD)
		spd := pos.Speed / 1.852 * 0.44704 // undo knots->km/h, then mph->m/s
		wx.WindSpeed = &spd
	}

	// Parse weather fields from comment
	comment := pos.Comment
	parseWeatherString(comment, wx)

	// Clear position comment since it's been parsed as weather
	pos.Comment = ""
	// Zero out course/speed since they were wind data
	pos.Course = 0
	pos.Speed = 0

	return wx
}

// parseWeatherFields parses weather data from a positionless weather string.
func parseWeatherFields(data string) *WeatherData {
	wx := &WeatherData{}

	// Extract wind direction and speed at start (cDDDsDDD format)
	if idx := strings.IndexByte(data, 'c'); idx >= 0 {
		rest := data[idx+1:]
		if len(rest) >= 3 {
			if dir, err := strconv.ParseFloat(rest[:3], 64); err == nil {
				wx.WindDir = &dir
			}
		}
	}
	if idx := strings.IndexByte(data, 's'); idx >= 0 {
		rest := data[idx+1:]
		if len(rest) >= 3 {
			if spd, err := strconv.ParseFloat(rest[:3], 64); err == nil {
				ms := spd * 0.44704 // mph to m/s
				wx.WindSpeed = &ms
			}
		}
	}

	parseWeatherString(data, wx)
	return wx
}

// parseWeatherString parses weather field codes from a string.
func parseWeatherString(data string, wx *WeatherData) {
	for i := 0; i < len(data); i++ {
		c := data[i]
		remaining := data[i+1:]

		switch c {
		case 'g':
			if len(remaining) >= 3 {
				if val, err := strconv.ParseFloat(remaining[:3], 64); err == nil {
					ms := val * 0.44704 // mph to m/s
					wx.WindGust = &ms
				}
				i += 3
			}
		case 't':
			if len(remaining) >= 3 {
				tStr := remaining[:3]
				if val, err := strconv.ParseFloat(tStr, 64); err == nil {
					celsius := (val - 32) * 5.0 / 9.0 // F to C
					wx.Temperature = &celsius
				}
				i += 3
			}
		case 'r':
			if len(remaining) >= 3 {
				if val, err := strconv.ParseFloat(remaining[:3], 64); err == nil {
					mm := val * 0.254 // hundredths of inch to mm
					wx.Rain1h = &mm
				}
				i += 3
			}
		case 'p':
			if len(remaining) >= 3 {
				if val, err := strconv.ParseFloat(remaining[:3], 64); err == nil {
					mm := val * 0.254
					wx.Rain24h = &mm
				}
				i += 3
			}
		case 'P':
			if len(remaining) >= 3 {
				if val, err := strconv.ParseFloat(remaining[:3], 64); err == nil {
					mm := val * 0.254
					wx.RainToday = &mm
				}
				i += 3
			}
		case 'h':
			if len(remaining) >= 2 {
				if val, err := strconv.Atoi(remaining[:2]); err == nil {
					if val == 0 {
						val = 100
					}
					wx.Humidity = &val
				}
				i += 2
			}
		case 'b':
			if len(remaining) >= 5 {
				if val, err := strconv.ParseFloat(remaining[:5], 64); err == nil {
					hpa := val / 10.0 // tenths of hPa to hPa
					wx.Pressure = &hpa
				}
				i += 5
			}
		case 'L', 'l':
			if len(remaining) >= 3 {
				if val, err := strconv.Atoi(remaining[:3]); err == nil {
					if c == 'l' {
						val += 1000
					}
					wx.Luminosity = &val
				}
				i += 3
			}
		case 'X':
			if len(remaining) >= 3 {
				if val, err := strconv.ParseFloat(remaining[:3], 64); err == nil {
					wx.Radiation = &val // nanosieverts/hour
				}
				i += 3
			}
		case 'V':
			if len(remaining) >= 3 {
				if val, err := strconv.ParseFloat(remaining[:3], 64); err == nil {
					wx.Voltage = &val // volts
				}
				i += 3
			}
		case 'F':
			if len(remaining) >= 3 {
				if val, err := strconv.ParseFloat(remaining[:3], 64); err == nil {
					wx.FloodLevel = &val // feet
				}
				i += 3
			}
		}
	}
}
