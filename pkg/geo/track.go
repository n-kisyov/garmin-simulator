package geo

import "sort"

// Cumulative returns the distance in metres from the start of the track to each
// of its points, so that dists[i] is how far along point i sits. The first entry
// is always 0 and the last is the length of the whole track.
func Cumulative(points []Point) []float64 {
	dists := make([]float64, len(points))
	for i := 1; i < len(points); i++ {
		dists[i] = dists[i-1] + Haversine(points[i-1].Lon, points[i-1].Lat, points[i].Lon, points[i].Lat)
	}
	return dists
}

// HasElevation reports whether any point carries a non-zero elevation. A KML
// exported without altitudes leaves every third coordinate at 0, which is the
// signal to go and look the terrain up instead.
func HasElevation(points []Point) bool {
	for _, p := range points {
		if p.Ele != 0.0 {
			return true
		}
	}
	return false
}

// Interpolate returns the point d metres along the track, sliding linearly
// between the two vertices that straddle d. A d outside the track clamps to its
// nearest end, so a caller that overshoots by rounding still gets a real
// position rather than a wild one.
//
// dists must be the Cumulative result for points.
func Interpolate(points []Point, dists []float64, d float64) Point {
	if len(points) == 0 {
		return Point{}
	}
	if d <= 0 || len(points) == 1 {
		return points[0]
	}
	last := len(points) - 1
	if d >= dists[last] {
		return points[last]
	}

	// The vertex at or before d. SearchFloat64s returns the first index whose
	// distance is >= d, and d < dists[last] guarantees that index exists.
	i := sort.SearchFloat64s(dists, d)
	if dists[i] > d {
		i--
	}
	if i >= last {
		return points[last]
	}

	span := dists[i+1] - dists[i]
	if span <= 0 {
		return points[i]
	}
	f := (d - dists[i]) / span
	return Point{
		Lon: points[i].Lon + f*(points[i+1].Lon-points[i].Lon),
		Lat: points[i].Lat + f*(points[i+1].Lat-points[i].Lat),
		Ele: points[i].Ele + f*(points[i+1].Ele-points[i].Ele),
	}
}

// Bounds returns the corners of the box enclosing the track: the north-east
// corner first, then the south-west. A FIT session records both so that a viewer
// can frame the map without reading every record.
func Bounds(points []Point) (necLat, necLon, swcLat, swcLon float64) {
	if len(points) == 0 {
		return 0, 0, 0, 0
	}
	necLat, swcLat = points[0].Lat, points[0].Lat
	necLon, swcLon = points[0].Lon, points[0].Lon
	for _, p := range points[1:] {
		if p.Lat > necLat {
			necLat = p.Lat
		}
		if p.Lat < swcLat {
			swcLat = p.Lat
		}
		if p.Lon > necLon {
			necLon = p.Lon
		}
		if p.Lon < swcLon {
			swcLon = p.Lon
		}
	}
	return necLat, necLon, swcLat, swcLon
}
