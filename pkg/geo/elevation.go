package geo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type elevationRequest struct {
	Locations []location `json:"locations"`
}

type location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type elevationResponse struct {
	Results []result `json:"results"`
}

type result struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation float64 `json:"elevation"`
}

// FetchElevation calls the Open-Elevation API to fill in missing elevation data.
// It processes the points in batches to avoid overwhelming the API.
func FetchElevation(points []Point, batchSize int) []Point {
	url := "https://api.open-elevation.com/api/v1/lookup"
	enriched := make([]Point, len(points))
	copy(enriched, points)

	total := len(points)
	client := &http.Client{Timeout: 30 * time.Second}

	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		batch := points[start:end]

		reqData := elevationRequest{}
		for _, p := range batch {
			reqData.Locations = append(reqData.Locations, location{
				Latitude:  p.Lat,
				Longitude: p.Lon,
			})
		}

		payload, err := json.Marshal(reqData)
		if err != nil {
			fmt.Printf("  Warning: Failed to marshal batch %d-%d: %v\n", start, end, err)
			continue
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
		if err != nil {
			fmt.Printf("  Warning: Failed to create request for batch %d-%d: %v\n", start, end, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("  Warning: Failed to fetch elevation for batch %d-%d: %v\n", start, end, err)
			continue
		}

		var respData elevationResponse
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			resp.Body.Close()
			fmt.Printf("  Warning: Failed to decode response for batch %d-%d: %v\n", start, end, err)
			continue
		}
		resp.Body.Close()

		for i, res := range respData.Results {
			idx := start + i
			enriched[idx].Ele = res.Elevation
		}

		pct := (end * 100) / total
		fmt.Printf("  Fetched elevation for %d/%d points (%d%%)...\n", end, total, pct)
	}

	return enriched
}
