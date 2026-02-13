package main

import (
	"encoding/json"
	"math"
	"testing"
)

func validObsArray() []any {
	return []any{
		float64(1700000000), // 0: timestamp
		float64(0.5),        // 1: wind lull
		float64(1.2),        // 2: wind avg
		float64(2.3),        // 3: wind gust
		float64(180),        // 4: wind direction
		float64(3),          // 5: wind sample interval
		float64(1013.25),    // 6: station pressure
		float64(22.5),       // 7: air temperature
		float64(65.0),       // 8: relative humidity
		float64(50000),      // 9: illuminance
		float64(3.5),        // 10: UV
		float64(300),        // 11: solar radiation
		float64(0.1),        // 12: rain accumulated
		float64(1),          // 13: precipitation type
		float64(10),         // 14: lightning strike avg distance
		float64(2),          // 15: lightning strike count
		float64(2.65),       // 16: battery
		float64(60),         // 17: report interval
	}
}

func TestParseObservation_Valid(t *testing.T) {
	raw := validObsArray()
	obs, err := ParseObservation(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"WindLull", obs.WindLull, 0.5},
		{"WindAvg", obs.WindAvg, 1.2},
		{"WindGust", obs.WindGust, 2.3},
		{"WindDirection", obs.WindDirection, 180},
		{"WindSampleInterval", obs.WindSampleInterval, 3},
		{"StationPressure", obs.StationPressure, 1013.25},
		{"AirTemperature", obs.AirTemperature, 22.5},
		{"RelativeHumidity", obs.RelativeHumidity, 65.0},
		{"Illuminance", obs.Illuminance, 50000},
		{"UV", obs.UV, 3.5},
		{"SolarRadiation", obs.SolarRadiation, 300},
		{"RainAccumulated", obs.RainAccumulated, 0.1},
		{"PrecipitationType", obs.PrecipitationType, 1},
		{"LightningStrikeAvgDist", obs.LightningStrikeAvgDist, 10},
		{"LightningStrikeCount", obs.LightningStrikeCount, 2},
		{"Battery", obs.Battery, 2.65},
		{"ReportInterval", obs.ReportInterval, 60},
	}

	if obs.Timestamp != 1700000000 {
		t.Errorf("Timestamp = %d, want 1700000000", obs.Timestamp)
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestParseObservation_ShortArray(t *testing.T) {
	raw := []any{float64(1700000000), float64(1.0)}
	_, err := ParseObservation(raw)
	if err == nil {
		t.Fatal("expected error for short array")
	}
}

func TestParseObservation_NullValues(t *testing.T) {
	raw := validObsArray()
	raw[1] = nil  // wind lull
	raw[14] = nil // lightning distance
	raw[15] = nil // lightning count

	obs, err := ParseObservation(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !math.IsNaN(obs.WindLull) {
		t.Errorf("WindLull = %v, want NaN", obs.WindLull)
	}
	if !math.IsNaN(obs.LightningStrikeAvgDist) {
		t.Errorf("LightningStrikeAvgDist = %v, want NaN", obs.LightningStrikeAvgDist)
	}
	if !math.IsNaN(obs.LightningStrikeCount) {
		t.Errorf("LightningStrikeCount = %v, want NaN", obs.LightningStrikeCount)
	}
	// Non-null fields should still be correct
	if obs.AirTemperature != 22.5 {
		t.Errorf("AirTemperature = %v, want 22.5", obs.AirTemperature)
	}
}

func TestParseObservation_WrongType(t *testing.T) {
	raw := validObsArray()
	raw[2] = "not a number" // wind avg

	obs, err := ParseObservation(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !math.IsNaN(obs.WindAvg) {
		t.Errorf("WindAvg = %v, want NaN for wrong type", obs.WindAvg)
	}
}

func TestParseObservation_NilTimestamp(t *testing.T) {
	raw := validObsArray()
	raw[0] = nil

	_, err := ParseObservation(raw)
	if err == nil {
		t.Fatal("expected error for nil timestamp")
	}
}

func TestDewPoint(t *testing.T) {
	// At 22.5°C and 65% RH, dew point should be ~15.6°C
	dp := DewPoint(22.5, 65.0)
	if math.Abs(dp-15.63) > 0.1 {
		t.Errorf("DewPoint(22.5, 65) = %v, want ~15.6", dp)
	}

	// At 30°C and 80% RH, dew point should be ~26.2°C
	dp = DewPoint(30.0, 80.0)
	if math.Abs(dp-26.2) > 0.2 {
		t.Errorf("DewPoint(30, 80) = %v, want ~26.2", dp)
	}

	// NaN inputs
	if !math.IsNaN(DewPoint(math.NaN(), 50)) {
		t.Error("DewPoint(NaN, 50) should be NaN")
	}
	if !math.IsNaN(DewPoint(20, math.NaN())) {
		t.Error("DewPoint(20, NaN) should be NaN")
	}
	if !math.IsNaN(DewPoint(20, 0)) {
		t.Error("DewPoint(20, 0) should be NaN")
	}
}

func TestFeelsLike_WindChill(t *testing.T) {
	// 5°C with 20 km/h wind (~5.56 m/s) should give wind chill
	fl := FeelsLike(5.0, 50.0, 5.56)
	if fl >= 5.0 {
		t.Errorf("FeelsLike(5, 50, 5.56) = %v, want < 5.0 (wind chill)", fl)
	}
	// Wind chill at 5°C, 20km/h should be roughly 1.1°C
	if math.Abs(fl-1.1) > 0.5 {
		t.Errorf("FeelsLike(5, 50, 5.56) = %v, want ~1.1", fl)
	}
}

func TestFeelsLike_HeatIndex(t *testing.T) {
	// 35°C with 60% humidity should give heat index above air temp
	fl := FeelsLike(35.0, 60.0, 1.0)
	if fl <= 35.0 {
		t.Errorf("FeelsLike(35, 60, 1.0) = %v, want > 35.0 (heat index)", fl)
	}
}

func TestFeelsLike_Normal(t *testing.T) {
	// 20°C mild conditions should return air temp
	fl := FeelsLike(20.0, 50.0, 2.0)
	if fl != 20.0 {
		t.Errorf("FeelsLike(20, 50, 2.0) = %v, want 20.0", fl)
	}
}

func TestFeelsLike_NaN(t *testing.T) {
	if !math.IsNaN(FeelsLike(math.NaN(), 50, 2)) {
		t.Error("FeelsLike(NaN, ...) should be NaN")
	}
}

func TestToInt64_Overflow(t *testing.T) {
	// Infinity should error
	_, err := toInt64(math.Inf(1))
	if err == nil {
		t.Error("toInt64(+Inf) should return error")
	}

	// NaN should error
	_, err = toInt64(math.NaN())
	if err == nil {
		t.Error("toInt64(NaN) should return error")
	}

	// Negative infinity should error
	_, err = toInt64(math.Inf(-1))
	if err == nil {
		t.Error("toInt64(-Inf) should return error")
	}

	// Valid value should succeed
	val, err := toInt64(float64(1700000000))
	if err != nil {
		t.Fatalf("toInt64(1700000000) unexpected error: %v", err)
	}
	if val != 1700000000 {
		t.Errorf("toInt64(1700000000) = %d, want 1700000000", val)
	}
}

func TestToFloat_JsonNumber(t *testing.T) {
	// Valid json.Number
	val := toFloat(json.Number("3.14"))
	if val != 3.14 {
		t.Errorf("toFloat(json.Number(3.14)) = %v, want 3.14", val)
	}

	// Invalid json.Number
	val = toFloat(json.Number("not-a-number"))
	if !math.IsNaN(val) {
		t.Errorf("toFloat(json.Number(invalid)) = %v, want NaN", val)
	}
}

func TestToInt64_JsonNumber(t *testing.T) {
	// Valid json.Number
	val, err := toInt64(json.Number("1700000000"))
	if err != nil {
		t.Fatalf("toInt64(json.Number(valid)) unexpected error: %v", err)
	}
	if val != 1700000000 {
		t.Errorf("toInt64(json.Number(1700000000)) = %d, want 1700000000", val)
	}

	// Invalid json.Number
	_, err = toInt64(json.Number("not-a-number"))
	if err == nil {
		t.Error("toInt64(json.Number(invalid)) should return error")
	}
}

func TestToInt64_InvalidType(t *testing.T) {
	_, err := toInt64("a string")
	if err == nil {
		t.Error("toInt64(string) should return error")
	}
}

func TestToFloat_Nil(t *testing.T) {
	val := toFloat(nil)
	if !math.IsNaN(val) {
		t.Errorf("toFloat(nil) = %v, want NaN", val)
	}
}

func TestToFloat_UnsupportedType(t *testing.T) {
	val := toFloat(true) // bool is not a supported type
	if !math.IsNaN(val) {
		t.Errorf("toFloat(bool) = %v, want NaN", val)
	}
}

func TestRedactToken(t *testing.T) {
	result := redactToken("dial wss://ws.weatherflow.com/swd/data?token=abc123: connection refused", "abc123")
	if result != "dial wss://ws.weatherflow.com/swd/data?token=[REDACTED]: connection refused" {
		t.Errorf("redactToken did not redact token: %s", result)
	}

	// Empty token should be a no-op
	result = redactToken("some error message", "")
	if result != "some error message" {
		t.Errorf("redactToken with empty token should be no-op: %s", result)
	}
}
