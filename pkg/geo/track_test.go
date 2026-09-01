package geo

import (
	"math"
	"testing"
)

// leg is roughly 100 m of longitude at this latitude; the exact figure comes out
// of Haversine and is only ever compared against itself.
var straight = []Point{
	{Lon: 23.0000, Lat: 42.0000, Ele: 500},
	{Lon: 23.0012, Lat: 42.0000, Ele: 520},
	{Lon: 23.0024, Lat: 42.0000, Ele: 510},
}

func TestCumulativeGrowsFromZero(t *testing.T) {
	dists := Cumulative(straight)
	if len(dists) != len(straight) {
		t.Fatalf("got %d distances for %d points", len(dists), len(straight))
	}
	if dists[0] != 0 {
		t.Errorf("dists[0] = %v, want 0", dists[0])
	}
	for i := 1; i < len(dists); i++ {
		if dists[i] <= dists[i-1] {
			t.Errorf("dists[%d] = %v is not past dists[%d] = %v", i, dists[i], i-1, dists[i-1])
		}
	}
	// The two legs are the same length, so the midpoint is half the total.
	if got, want := dists[1], dists[2]/2; math.Abs(got-want) > 0.01 {
		t.Errorf("dists[1] = %v, want half of %v", got, dists[2])
	}
}

func TestCumulativeHandlesShortTracks(t *testing.T) {
	if got := Cumulative(nil); len(got) != 0 {
		t.Errorf("Cumulative(nil) = %v, want empty", got)
	}
	if got := Cumulative(straight[:1]); len(got) != 1 || got[0] != 0 {
		t.Errorf("Cumulative(one point) = %v, want [0]", got)
	}
}

func TestInterpolateSlidesBetweenVertices(t *testing.T) {
	dists := Cumulative(straight)
	half := dists[1] / 2

	p := Interpolate(straight, dists, half)
	if want := 23.0006; math.Abs(p.Lon-want) > 1e-6 {
		t.Errorf("halfway along the first leg is at lon %v, want %v", p.Lon, want)
	}
	if want := 510.0; math.Abs(p.Ele-want) > 0.01 {
		t.Errorf("halfway along the first leg is at %v m, want %v", p.Ele, want)
	}

	// Landing exactly on a vertex returns that vertex.
	if p := Interpolate(straight, dists, dists[1]); p.Lon != straight[1].Lon {
		t.Errorf("at the second vertex got lon %v, want %v", p.Lon, straight[1].Lon)
	}
}

func TestInterpolateClampsToTheEnds(t *testing.T) {
	dists := Cumulative(straight)
	last := straight[len(straight)-1]

	for _, d := range []float64{-500, 0} {
		if p := Interpolate(straight, dists, d); p != straight[0] {
			t.Errorf("Interpolate(%v) = %+v, want the start %+v", d, p, straight[0])
		}
	}
	for _, d := range []float64{dists[len(dists)-1], 1e9} {
		if p := Interpolate(straight, dists, d); p != last {
			t.Errorf("Interpolate(%v) = %+v, want the end %+v", d, p, last)
		}
	}
	if p := Interpolate(nil, nil, 10); p != (Point{}) {
		t.Errorf("Interpolate on an empty track = %+v, want the zero point", p)
	}
}

// TestInterpolateSurvivesRepeatedVertices covers the case a KML full of
// duplicated coordinates produces: a zero-length leg the search can land inside.
func TestInterpolateSurvivesRepeatedVertices(t *testing.T) {
	pts := []Point{
		{Lon: 23.0, Lat: 42.0, Ele: 500},
		{Lon: 23.0, Lat: 42.0, Ele: 500},
		{Lon: 23.0012, Lat: 42.0, Ele: 520},
	}
	dists := Cumulative(pts)
	p := Interpolate(pts, dists, dists[2]/2)
	if p.Lon <= 23.0 || p.Lon >= 23.0012 {
		t.Errorf("got lon %v, want a point inside the real leg", p.Lon)
	}
}

func TestHasElevation(t *testing.T) {
	if !HasElevation(straight) {
		t.Error("HasElevation = false for a track with altitudes")
	}
	flat := []Point{{Lon: 23, Lat: 42}, {Lon: 24, Lat: 43}}
	if HasElevation(flat) {
		t.Error("HasElevation = true for a track exported without altitudes")
	}
	if HasElevation(nil) {
		t.Error("HasElevation = true for no track at all")
	}
}

func TestBoundsBoxesTheTrack(t *testing.T) {
	pts := []Point{
		{Lon: 23.0, Lat: 42.0},
		{Lon: 22.5, Lat: 42.5},
		{Lon: 23.5, Lat: 41.5},
	}
	necLat, necLon, swcLat, swcLon := Bounds(pts)
	if necLat != 42.5 || necLon != 23.5 || swcLat != 41.5 || swcLon != 22.5 {
		t.Errorf("Bounds = (%v, %v)-(%v, %v), want (42.5, 23.5)-(41.5, 22.5)",
			necLat, necLon, swcLat, swcLon)
	}

	if a, b, c, d := Bounds(nil); a != 0 || b != 0 || c != 0 || d != 0 {
		t.Errorf("Bounds(nil) = %v %v %v %v, want zeroes", a, b, c, d)
	}
}
