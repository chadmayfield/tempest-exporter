package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"
)

const defaultBaseURL = "https://swd.weatherflow.com/swd/rest"

// maxResponseBytes is the maximum size of a REST API response body (1 MB).
const maxResponseBytes = 1 << 20

// RESTClient polls the Tempest REST API as a fallback when the WebSocket is disconnected.
type RESTClient struct {
	httpClient *http.Client
	token      string
	stationID  string
	baseURL    string
	collector  *Collector
}

// NewRESTClient creates a new REST API client.
func NewRESTClient(token, stationID string, collector *Collector) *RESTClient {
	return &RESTClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		},
		token:     token,
		stationID: stationID,
		baseURL:   defaultBaseURL,
		collector: collector,
	}
}

// restResponse is the top-level REST API response for station observations.
type restResponse struct {
	Obs []restObs `json:"obs"`
}

// restObs is a single observation from the REST API.
type restObs struct {
	Timestamp              *float64 `json:"timestamp"`
	WindLull               *float64 `json:"wind_lull"`
	WindAvg                *float64 `json:"wind_avg"`
	WindGust               *float64 `json:"wind_gust"`
	WindDirection          *float64 `json:"wind_direction"`
	StationPressure        *float64 `json:"station_pressure"`
	AirTemperature         *float64 `json:"air_temperature"`
	RelativeHumidity       *float64 `json:"relative_humidity"`
	Illuminance            *float64 `json:"illuminance"`
	UV                     *float64 `json:"uv"`
	SolarRadiation         *float64 `json:"solar_radiation"`
	RainAccumulated        *float64 `json:"rain_accumulated"`
	PrecipitationType      *float64 `json:"precip_type"`
	LightningStrikeAvgDist *float64 `json:"lightning_strike_avg_distance"`
	LightningStrikeCount   *float64 `json:"lightning_strike_count"`
	Battery                *float64 `json:"battery"`
	ReportInterval         *float64 `json:"report_interval"`
}

// FetchObservation retrieves the latest observation from the REST API.
func (r *RESTClient) FetchObservation(ctx context.Context) (*Observation, error) {
	url := fmt.Sprintf("%s/observations/station/%s?token=%s", r.baseURL, r.stationID, r.token)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		// Redact the token from HTTP client error messages (may contain the URL).
		return nil, fmt.Errorf("fetching observations: %s", redactToken(err.Error(), r.token))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// Limit response body size to prevent memory exhaustion from oversized responses.
	limitedBody := io.LimitReader(resp.Body, maxResponseBytes)
	var result restResponse
	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Obs) == 0 {
		return nil, fmt.Errorf("no observations in response")
	}

	obs := restObsToObservation(result.Obs[0])
	return &obs, nil
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func restObsToObservation(ro restObs) Observation {
	obs := Observation{
		WindLull:               deref(ro.WindLull),
		WindAvg:                deref(ro.WindAvg),
		WindGust:               deref(ro.WindGust),
		WindDirection:          deref(ro.WindDirection),
		StationPressure:        deref(ro.StationPressure),
		AirTemperature:         deref(ro.AirTemperature),
		RelativeHumidity:       deref(ro.RelativeHumidity),
		Illuminance:            deref(ro.Illuminance),
		UV:                     deref(ro.UV),
		SolarRadiation:         deref(ro.SolarRadiation),
		RainAccumulated:        deref(ro.RainAccumulated),
		PrecipitationType:      deref(ro.PrecipitationType),
		LightningStrikeAvgDist: deref(ro.LightningStrikeAvgDist),
		LightningStrikeCount:   deref(ro.LightningStrikeCount),
		Battery:                deref(ro.Battery),
		ReportInterval:         deref(ro.ReportInterval),
	}
	if ro.Timestamp != nil {
		ts := *ro.Timestamp
		if ts >= 0 && ts <= float64(math.MaxInt64) {
			obs.Timestamp = int64(ts)
		}
	}
	return obs
}

// RunFallback polls the REST API when the WebSocket has been disconnected
// for longer than the fallback threshold. It blocks until context cancellation.
func (r *RESTClient) RunFallback(ctx context.Context, disconnectThreshold time.Duration, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var disconnectedSince *time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check connection state via collector
			if r.collector.isConnected() {
				disconnectedSince = nil
				continue
			}

			now := time.Now()
			if disconnectedSince == nil {
				disconnectedSince = &now
				continue
			}

			if time.Since(*disconnectedSince) < disconnectThreshold {
				continue
			}

			slog.Warn("REST fallback activated, polling for observations")

			obs, err := r.FetchObservation(ctx)
			if err != nil {
				slog.Error("REST fallback error", "error", err)
				continue
			}

			r.collector.UpdateObservation(*obs)
			slog.Info("REST fallback: observation updated",
				"air_temp_c", obs.AirTemperature,
			)
		}
	}
}

// isConnected returns the current connection state.
func (c *Collector) isConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}
