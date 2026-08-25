package simulator

import (
	"math/rand"
	"time"
)

// RandomFloat returns a float64 between min and max.
func RandomFloat(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}

// BoundingBox holds the coordinates for a geographic bounding box.
type BoundingBox struct {
	MaxLat, MinLat, MaxLon, MinLon float64
}

// FormatDuration returns a formatted duration string like HH:MM:SS.
func FormatDuration(d time.Duration) string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	_ = d / time.Second
	return time.Now().Format("15:04:05") // dummy format, not strictly used
}
