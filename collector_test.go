package main

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testObservation() Observation {
	return Observation{
		Timestamp:              1700000000,
		WindLull:               0.5,
		WindAvg:                1.2,
		WindGust:               2.3,
		WindDirection:          180,
		WindSampleInterval:     3,
		StationPressure:        1013.25,
		AirTemperature:         22.5,
		RelativeHumidity:       65.0,
		Illuminance:            50000,
		UV:                     3.5,
		SolarRadiation:         300,
		RainAccumulated:        0.1,
		PrecipitationType:      1,
		LightningStrikeAvgDist: 10,
		LightningStrikeCount:   2,
		Battery:                2.65,
		ReportInterval:         60,
	}
}

func TestCollector_AllMetricsPresent(t *testing.T) {
	c := NewCollector("12345", "backyard")
	c.SetConnected(true)
	c.UpdateObservation(testObservation())

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather error: %v", err)
	}

	expected := map[string]bool{
		"tempest_wind_lull_meters_per_second":          false,
		"tempest_wind_speed_meters_per_second":         false,
		"tempest_wind_gust_meters_per_second":          false,
		"tempest_wind_direction_degrees":               false,
		"tempest_station_pressure_millibars":           false,
		"tempest_air_temperature_celsius":              false,
		"tempest_relative_humidity_percent":            false,
		"tempest_illuminance_lux":                      false,
		"tempest_uv_index":                             false,
		"tempest_solar_radiation_watts":                false,
		"tempest_precipitation_millimeters":            false,
		"tempest_precipitation_type":                   false,
		"tempest_lightning_strike_distance_kilometers": false,
		"tempest_lightning_strike_count":               false,
		"tempest_battery_volts":                        false,
		"tempest_dew_point_celsius":                    false,
		"tempest_feels_like_temperature_celsius":       false,
		"tempest_up":                                   false,
		"tempest_websocket_reconnects_total":           false,
		"tempest_last_observation_timestamp_seconds":   false,
		"tempest_scrape_errors_total":                  false,
	}

	for _, mf := range mfs {
		name := mf.GetName()
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing metric: %s", name)
		}
	}
}

func TestCollector_ObservationValues(t *testing.T) {
	c := NewCollector("12345", "backyard")
	c.UpdateObservation(testObservation())

	expected := `
		# HELP tempest_air_temperature_celsius Air temperature in Celsius
		# TYPE tempest_air_temperature_celsius gauge
		tempest_air_temperature_celsius{station_id="12345",station_name="backyard"} 22.5
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_air_temperature_celsius"); err != nil {
		t.Error(err)
	}

	expected = `
		# HELP tempest_battery_volts Battery voltage
		# TYPE tempest_battery_volts gauge
		tempest_battery_volts{station_id="12345",station_name="backyard"} 2.65
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_battery_volts"); err != nil {
		t.Error(err)
	}
}

func TestCollector_UpMetric(t *testing.T) {
	c := NewCollector("12345", "backyard")

	// Default: disconnected
	expected := `
		# HELP tempest_up Whether the WebSocket connection is active (1=connected, 0=disconnected)
		# TYPE tempest_up gauge
		tempest_up{station_id="12345",station_name="backyard"} 0
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_up"); err != nil {
		t.Errorf("disconnected: %v", err)
	}

	// Connected
	c.SetConnected(true)
	expected = `
		# HELP tempest_up Whether the WebSocket connection is active (1=connected, 0=disconnected)
		# TYPE tempest_up gauge
		tempest_up{station_id="12345",station_name="backyard"} 1
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_up"); err != nil {
		t.Errorf("connected: %v", err)
	}
}

func TestCollector_LastObservationTimestamp(t *testing.T) {
	c := NewCollector("12345", "backyard")
	c.UpdateObservation(testObservation())

	expected := `
		# HELP tempest_last_observation_timestamp_seconds Unix timestamp of last received observation
		# TYPE tempest_last_observation_timestamp_seconds gauge
		tempest_last_observation_timestamp_seconds{station_id="12345",station_name="backyard"} 1.7e+09
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_last_observation_timestamp_seconds"); err != nil {
		t.Error(err)
	}
}

func TestCollector_DerivedMetrics(t *testing.T) {
	c := NewCollector("12345", "backyard")
	c.UpdateObservation(testObservation())

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, _ := reg.Gather()
	var foundDP, foundFL bool
	for _, mf := range mfs {
		switch mf.GetName() {
		case "tempest_dew_point_celsius":
			foundDP = true
			val := mf.GetMetric()[0].GetGauge().GetValue()
			expectedDP := DewPoint(22.5, 65.0)
			if val != expectedDP {
				t.Errorf("dew_point = %v, want %v", val, expectedDP)
			}
		case "tempest_feels_like_temperature_celsius":
			foundFL = true
			val := mf.GetMetric()[0].GetGauge().GetValue()
			// 22.5C/65%/1.2m/s = mild conditions, should equal air temp
			if val != 22.5 {
				t.Errorf("feels_like = %v, want 22.5 (mild conditions)", val)
			}
		}
	}
	if !foundDP {
		t.Error("missing tempest_dew_point_celsius metric")
	}
	if !foundFL {
		t.Error("missing tempest_feels_like_temperature_celsius metric")
	}
}

func TestCollector_NoMetricsBeforeObservation(t *testing.T) {
	c := NewCollector("12345", "backyard")

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather error: %v", err)
	}

	// Should have health metrics but no observation metrics
	for _, mf := range mfs {
		name := mf.GetName()
		switch name {
		case "tempest_up", "tempest_websocket_reconnects_total", "tempest_scrape_errors_total":
			// expected health metrics always emitted
		default:
			t.Errorf("unexpected metric before observation: %s", name)
		}
	}
}

func TestCollector_Labels(t *testing.T) {
	c := NewCollector("12345", "backyard")
	c.UpdateObservation(testObservation())

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		name := mf.GetName()
		// All metrics should now have station labels
		for _, m := range mf.GetMetric() {
			labels := m.GetLabel()
			if len(labels) != 2 {
				t.Errorf("%s: expected 2 labels, got %d", name, len(labels))
				continue
			}
			if labels[0].GetName() != "station_id" || labels[0].GetValue() != "12345" {
				t.Errorf("%s: bad station_id label: %v", name, labels[0])
			}
			if labels[1].GetName() != "station_name" || labels[1].GetValue() != "backyard" {
				t.Errorf("%s: bad station_name label: %v", name, labels[1])
			}
		}
	}
}

func TestCollector_IncrReconnects(t *testing.T) {
	c := NewCollector("12345", "backyard")

	expected := `
		# HELP tempest_websocket_reconnects_total Total number of WebSocket reconnection attempts
		# TYPE tempest_websocket_reconnects_total counter
		tempest_websocket_reconnects_total{station_id="12345",station_name="backyard"} 0
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_websocket_reconnects_total"); err != nil {
		t.Errorf("initial: %v", err)
	}

	c.IncrReconnects()
	c.IncrReconnects()
	expected = `
		# HELP tempest_websocket_reconnects_total Total number of WebSocket reconnection attempts
		# TYPE tempest_websocket_reconnects_total counter
		tempest_websocket_reconnects_total{station_id="12345",station_name="backyard"} 2
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_websocket_reconnects_total"); err != nil {
		t.Errorf("after 2 reconnects: %v", err)
	}
}

func TestCollector_SetRainStart(t *testing.T) {
	c := NewCollector("12345", "backyard")
	c.UpdateObservation(testObservation())

	// Before rain start is set, the metric should not be present
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "tempest_rain_start_epoch_seconds" {
			t.Fatal("rain_start metric should not be present when rainStart is 0")
		}
	}

	// Set rain start
	c.SetRainStart(1700000500)

	// Now it should appear
	expected := `
		# HELP tempest_rain_start_epoch_seconds Unix timestamp of last rain start event
		# TYPE tempest_rain_start_epoch_seconds gauge
		tempest_rain_start_epoch_seconds{station_id="12345",station_name="backyard"} 1.7000005e+09
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_rain_start_epoch_seconds"); err != nil {
		t.Error(err)
	}
}

func TestCollector_HasObservation(t *testing.T) {
	c := NewCollector("12345", "backyard")

	if c.HasObservation() {
		t.Error("HasObservation should be false before any observation")
	}

	c.UpdateObservation(testObservation())

	if !c.HasObservation() {
		t.Error("HasObservation should be true after observation")
	}
}

func TestCollector_ScrapeErrorsMetric(t *testing.T) {
	c := NewCollector("12345", "backyard")

	expected := `
		# HELP tempest_scrape_errors_total Total errors serving /metrics
		# TYPE tempest_scrape_errors_total counter
		tempest_scrape_errors_total{station_id="12345",station_name="backyard"} 0
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_scrape_errors_total"); err != nil {
		t.Error(err)
	}

	c.IncrScrapeErrors()
	expected = `
		# HELP tempest_scrape_errors_total Total errors serving /metrics
		# TYPE tempest_scrape_errors_total counter
		tempest_scrape_errors_total{station_id="12345",station_name="backyard"} 1
	`
	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "tempest_scrape_errors_total"); err != nil {
		t.Error(err)
	}
}
