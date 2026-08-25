package geo

import "math"

// Haversine calculates the great-circle distance between two points on a sphere
// given their longitudes and latitudes. The returned distance is in meters.
func Haversine(lon1, lat1, lon2, lat2 float64) float64 {
	const R = 6371e3 // Radius of Earth in meters
	
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	
	dphi := (lat2 - lat1) * math.Pi / 180
	dlambda := (lon2 - lon1) * math.Pi / 180
	
	a := math.Pow(math.Sin(dphi/2), 2) + math.Cos(phi1)*math.Cos(phi2)*math.Pow(math.Sin(dlambda/2), 2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	
	return R * c
}
