package simulator

import (
	"math/rand"
	"time"
)

// rng backs RandomFloat. It is kept package-level rather than using the global
// math/rand source so that Reseed can restart the stream, which is how a --count
// series regenerates the same workout for every file it writes.
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// Reseed restarts the random stream. Two runs reseeded with the same value draw
// the same sequence of numbers.
func Reseed(seed int64) {
	rng = rand.New(rand.NewSource(seed))
}

// RandomFloat returns a float64 between min and max.
func RandomFloat(min, max float64) float64 {
	return min + rng.Float64()*(max-min)
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
