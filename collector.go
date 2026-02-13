package main

import (
	"math"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var labels = []string{"station_id", "station_name"}

// Metric descriptors for observation gauges.
var (
	descWindLull = prometheus.NewDesc(
		"tempest_wind_lull_meters_per_second", "Minimum wind speed over report interval (m/s)", labels, nil)
	descWindAvg = prometheus.NewDesc(
		"tempest_wind_speed_meters_per_second", "Average wind speed over report interval (m/s)", labels, nil)
	descWindGust = prometheus.NewDesc(
		"tempest_wind_gust_meters_per_second", "Maximum wind speed over report interval (m/s)", labels, nil)
	descWindDirection = prometheus.NewDesc(
		"tempest_wind_direction_degrees", "Wind direction in degrees", labels, nil)
	descStationPressure = prometheus.NewDesc(
		"tempest_station_pressure_millibars", "Station pressure in millibars", labels, nil)
	descAirTemperature = prometheus.NewDesc(
		"tempest_air_temperature_celsius", "Air temperature in Celsius", labels, nil)
	descRelativeHumidity = prometheus.NewDesc(
		"tempest_relative_humidity_percent", "Relative humidity percentage", labels, nil)
	descIlluminance = prometheus.NewDesc(
		"tempest_illuminance_lux", "Illuminance in lux", labels, nil)
	descUV = prometheus.NewDesc(
		"tempest_uv_index", "UV index", labels, nil)
	descSolarRadiation = prometheus.NewDesc(
		"tempest_solar_radiation_watts", "Solar radiation in W/m²", labels, nil)
	descRainAccumulated = prometheus.NewDesc(
		"tempest_precipitation_millimeters", "Precipitation accumulation in mm", labels, nil)
	descPrecipitationType = prometheus.NewDesc(
		"tempest_precipitation_type", "Precipitation type (0=none, 1=rain, 2=hail, 3=mix)", labels, nil)
	descLightningStrikeAvgDist = prometheus.NewDesc(
		"tempest_lightning_strike_distance_kilometers", "Average lightning strike distance in km", labels, nil)
	descLightningStrikeCount = prometheus.NewDesc(
		"tempest_lightning_strike_count", "Lightning strike count over report interval", labels, nil)
	descBattery = prometheus.NewDesc(
		"tempest_battery_volts", "Battery voltage", labels, nil)

	// Derived metrics
	descDewPoint = prometheus.NewDesc(
		"tempest_dew_point_celsius", "Dew point computed via Magnus formula", labels, nil)
	descFeelsLike = prometheus.NewDesc(
		"tempest_feels_like_temperature_celsius", "Feels-like temperature (wind chill / heat index)", labels, nil)

	// Event metrics
	descRainStartEpoch = prometheus.NewDesc(
		"tempest_rain_start_epoch_seconds", "Unix timestamp of last rain start event", labels, nil)

	// Health metrics
	descUp = prometheus.NewDesc(
		"tempest_up", "Whether the WebSocket connection is active (1=connected, 0=disconnected)", labels, nil)
	descReconnects = prometheus.NewDesc(
		"tempest_websocket_reconnects_total", "Total number of WebSocket reconnection attempts", labels, nil)
	descLastObservation = prometheus.NewDesc(
		"tempest_last_observation_timestamp_seconds", "Unix timestamp of last received observation", labels, nil)
	descScrapeErrors = prometheus.NewDesc(
		"tempest_scrape_errors_total", "Total errors serving /metrics", labels, nil)
)

// allObsDescs lists all observation metric descriptors for Describe().
var allDescs = []*prometheus.Desc{
	descWindLull, descWindAvg, descWindGust, descWindDirection,
	descStationPressure, descAirTemperature,
	descRelativeHumidity, descIlluminance, descUV, descSolarRadiation,
	descRainAccumulated, descPrecipitationType, descLightningStrikeAvgDist,
	descLightningStrikeCount, descBattery,
	descDewPoint, descFeelsLike, descRainStartEpoch,
	descUp, descReconnects, descLastObservation, descScrapeErrors,
}

// Collector is a custom Prometheus collector for Tempest weather data.
type Collector struct {
	mu sync.RWMutex

	obs       Observation
	hasObs    bool
	connected bool
	reconnects   float64
	scrapeErrors float64
	rainStart    float64

	stationID   string
	stationName string
}

// NewCollector creates a new Tempest metrics collector.
func NewCollector(stationID, stationName string) *Collector {
	return &Collector{
		stationID:   stationID,
		stationName: stationName,
	}
}

// Describe sends all metric descriptors to the channel.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range allDescs {
		ch <- d
	}
}

// Collect emits the current metric values.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	obs := c.obs
	hasObs := c.hasObs
	connected := c.connected
	reconnects := c.reconnects
	scrapeErrors := c.scrapeErrors
	rainStart := c.rainStart
	stationID := c.stationID
	stationName := c.stationName
	c.mu.RUnlock()

	lv := []string{stationID, stationName}

	// Health metrics
	connVal := 0.0
	if connected {
		connVal = 1.0
	}
	ch <- prometheus.MustNewConstMetric(descUp, prometheus.GaugeValue, connVal, lv...)
	ch <- prometheus.MustNewConstMetric(descReconnects, prometheus.CounterValue, reconnects, lv...)
	ch <- prometheus.MustNewConstMetric(descScrapeErrors, prometheus.CounterValue, scrapeErrors, lv...)

	if hasObs {
		ch <- prometheus.MustNewConstMetric(descLastObservation, prometheus.GaugeValue, float64(obs.Timestamp), lv...)
	}

	if !hasObs {
		return
	}

	emitGauge := func(desc *prometheus.Desc, val float64) {
		if !math.IsNaN(val) {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, val, lv...)
		}
	}

	emitGauge(descWindLull, obs.WindLull)
	emitGauge(descWindAvg, obs.WindAvg)
	emitGauge(descWindGust, obs.WindGust)
	emitGauge(descWindDirection, obs.WindDirection)
	emitGauge(descStationPressure, obs.StationPressure)
	emitGauge(descAirTemperature, obs.AirTemperature)
	emitGauge(descRelativeHumidity, obs.RelativeHumidity)
	emitGauge(descIlluminance, obs.Illuminance)
	emitGauge(descUV, obs.UV)
	emitGauge(descSolarRadiation, obs.SolarRadiation)
	emitGauge(descRainAccumulated, obs.RainAccumulated)
	emitGauge(descPrecipitationType, obs.PrecipitationType)
	emitGauge(descLightningStrikeAvgDist, obs.LightningStrikeAvgDist)
	emitGauge(descLightningStrikeCount, obs.LightningStrikeCount)
	emitGauge(descBattery, obs.Battery)

	// Derived metrics
	dp := DewPoint(obs.AirTemperature, obs.RelativeHumidity)
	emitGauge(descDewPoint, dp)

	fl := FeelsLike(obs.AirTemperature, obs.RelativeHumidity, obs.WindAvg)
	emitGauge(descFeelsLike, fl)

	if rainStart > 0 {
		emitGauge(descRainStartEpoch, rainStart)
	}
}

// UpdateObservation stores a new observation.
func (c *Collector) UpdateObservation(obs Observation) {
	c.mu.Lock()
	c.obs = obs
	c.hasObs = true
	c.mu.Unlock()
}

// SetConnected updates the connection state.
func (c *Collector) SetConnected(connected bool) {
	c.mu.Lock()
	c.connected = connected
	c.mu.Unlock()
}

// IncrReconnects increments the reconnection counter.
func (c *Collector) IncrReconnects() {
	c.mu.Lock()
	c.reconnects++
	c.mu.Unlock()
}

// SetRainStart records the epoch of a rain start event.
func (c *Collector) SetRainStart(epoch float64) {
	c.mu.Lock()
	c.rainStart = epoch
	c.mu.Unlock()
}

// IncrScrapeErrors increments the scrape error counter.
func (c *Collector) IncrScrapeErrors() {
	c.mu.Lock()
	c.scrapeErrors++
	c.mu.Unlock()
}

// HasObservation returns whether at least one observation has been received.
func (c *Collector) HasObservation() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hasObs
}
