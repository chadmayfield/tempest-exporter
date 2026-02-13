package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Observation holds the parsed fields from an obs_st WebSocket message.
// All numeric fields are float64 for Prometheus compatibility.
// Fields that were null in the source data are stored as math.NaN.
type Observation struct {
	Timestamp                int64
	WindLull                 float64
	WindAvg                  float64
	WindGust                 float64
	WindDirection            float64
	WindSampleInterval       float64
	StationPressure          float64
	AirTemperature           float64
	RelativeHumidity         float64
	Illuminance              float64
	UV                       float64
	SolarRadiation           float64
	RainAccumulated          float64
	PrecipitationType        float64
	LightningStrikeAvgDist   float64
	LightningStrikeCount     float64
	Battery                  float64
	ReportInterval           float64
}

const obsSTFieldCount = 18

// ParseObservation safely extracts an Observation from the raw obs_st array.
// The obs_st array contains 18 elements in a fixed order.
func ParseObservation(raw []any) (Observation, error) {
	if len(raw) < obsSTFieldCount {
		return Observation{}, fmt.Errorf("obs_st array too short: got %d, want %d", len(raw), obsSTFieldCount)
	}

	ts, err := toInt64(raw[0])
	if err != nil {
		return Observation{}, fmt.Errorf("parsing timestamp: %w", err)
	}

	return Observation{
		Timestamp:                ts,
		WindLull:                 toFloat(raw[1]),
		WindAvg:                  toFloat(raw[2]),
		WindGust:                 toFloat(raw[3]),
		WindDirection:            toFloat(raw[4]),
		WindSampleInterval:       toFloat(raw[5]),
		StationPressure:          toFloat(raw[6]),
		AirTemperature:           toFloat(raw[7]),
		RelativeHumidity:         toFloat(raw[8]),
		Illuminance:              toFloat(raw[9]),
		UV:                       toFloat(raw[10]),
		SolarRadiation:           toFloat(raw[11]),
		RainAccumulated:          toFloat(raw[12]),
		PrecipitationType:        toFloat(raw[13]),
		LightningStrikeAvgDist:   toFloat(raw[14]),
		LightningStrikeCount:     toFloat(raw[15]),
		Battery:                  toFloat(raw[16]),
		ReportInterval:           toFloat(raw[17]),
	}, nil
}

// toFloat converts a JSON number (float64) or nil to float64.
// Returns math.NaN for nil values.
func toFloat(v any) float64 {
	if v == nil {
		return math.NaN()
	}
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return math.NaN()
		}
		return f
	default:
		return math.NaN()
	}
}

// toInt64 converts a JSON number to int64. Returns error for nil, out-of-range, or invalid types.
func toInt64(v any) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("nil timestamp")
	}
	switch n := v.(type) {
	case float64:
		if n < math.MinInt64 || n > math.MaxInt64 || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, fmt.Errorf("timestamp out of int64 range: %v", n)
		}
		return int64(n), nil
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp number: %w", err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("invalid timestamp type")
	}
}

// DewPoint computes dew point in °C using the Magnus formula.
// tempC is air temperature in Celsius, humidityPct is relative humidity (0-100).
func DewPoint(tempC, humidityPct float64) float64 {
	if math.IsNaN(tempC) || math.IsNaN(humidityPct) || humidityPct <= 0 {
		return math.NaN()
	}
	const a = 17.27
	const b = 237.7
	gamma := (a*tempC)/(b+tempC) + math.Log(humidityPct/100.0)
	return (b * gamma) / (a - gamma)
}

// FeelsLike computes the "feels like" temperature in °C.
// Uses wind chill when temp < 10°C and wind > 4.8 km/h,
// heat index when temp >= 27°C and humidity >= 40%,
// otherwise returns the air temperature.
func FeelsLike(tempC, humidityPct, windMps float64) float64 {
	if math.IsNaN(tempC) {
		return math.NaN()
	}

	windKmh := windMps * 3.6

	// Wind chill (Environment Canada formula)
	if tempC < 10.0 && windKmh > 4.8 {
		v16 := math.Pow(windKmh, 0.16)
		return 13.12 + 0.6215*tempC - 11.37*v16 + 0.3965*tempC*v16
	}

	// Heat index (simplified Rothfusz/NOAA regression)
	if tempC >= 27.0 && humidityPct >= 40.0 {
		// Convert to Fahrenheit for the standard NOAA formula
		tf := tempC*1.8 + 32.0
		rh := humidityPct
		hi := -42.379 +
			2.04901523*tf +
			10.14333127*rh -
			0.22475541*tf*rh -
			0.00683783*tf*tf -
			0.05481717*rh*rh +
			0.00122874*tf*tf*rh +
			0.00085282*tf*rh*rh -
			0.00000199*tf*tf*rh*rh
		// Convert back to Celsius
		return (hi - 32.0) / 1.8
	}

	return tempC
}

// WSMessage is the envelope for all WebSocket messages, used to determine the type.
type WSMessage struct {
	Type string `json:"type"`
}

// ObsSTMessage is an obs_st observation message from the WebSocket.
type ObsSTMessage struct {
	Type     string  `json:"type"`
	DeviceID int     `json:"device_id"`
	Obs      [][]any `json:"obs"`
}

// StrikeEvent is an evt_strike lightning event from the WebSocket.
type StrikeEvent struct {
	Type     string `json:"type"`
	DeviceID int    `json:"device_id"`
	Evt      []any  `json:"evt"`
}

// PrecipEvent is an evt_precip rain start event from the WebSocket.
type PrecipEvent struct {
	Type     string `json:"type"`
	DeviceID int    `json:"device_id"`
	Evt      []any  `json:"evt"`
}

// redactToken replaces occurrences of the token in a string with "[REDACTED]".
func redactToken(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "[REDACTED]")
}
