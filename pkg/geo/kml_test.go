package geo

import (
	"math"
	"os"
	"strings"
	"testing"
)

// wantPoints is the LineString of testdata/mymaps_point_to_point.kml. The two
// marker pins that follow it in the document must never reach the route.
var wantPoints = []Point{
	{Lon: 23.30000, Lat: 42.70000},
	{Lon: 23.30500, Lat: 42.70100},
	{Lon: 23.31000, Lat: 42.70250},
	{Lon: 23.31600, Lat: 42.70400},
	{Lon: 23.32100, Lat: 42.70520},
	{Lon: 23.32700, Lat: 42.70610},
}

func parseFile(t *testing.T, name string) []Point {
	t.Helper()
	pts, err := ParseKMLCoordinates("testdata/" + name)
	if err != nil {
		t.Fatalf("ParseKMLCoordinates(%s): %v", name, err)
	}
	return pts
}

func pointsEqual(a, b Point) bool {
	const eps = 1e-9
	return math.Abs(a.Lon-b.Lon) < eps && math.Abs(a.Lat-b.Lat) < eps && math.Abs(a.Ele-b.Ele) < eps
}

func assertPoints(t *testing.T, got, want []Point) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d points, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if !pointsEqual(got[i], want[i]) {
			t.Errorf("point %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// assertNoOutlierSegment guards against a stray point being spliced onto the
// track: no single segment may carry more than maxShare of the total distance.
func assertNoOutlierSegment(t *testing.T, pts []Point, maxShare float64) {
	t.Helper()
	total := 0.0
	worstIdx, worstDist := 0, 0.0
	for i := 1; i < len(pts); i++ {
		d := Haversine(pts[i-1].Lon, pts[i-1].Lat, pts[i].Lon, pts[i].Lat)
		total += d
		if d > worstDist {
			worstIdx, worstDist = i, d
		}
	}
	if total == 0 {
		t.Fatal("track has zero length")
	}
	if share := worstDist / total; share > maxShare {
		t.Errorf("segment ending at point %d is %.0f m = %.1f%% of the %.0f m track (max %.1f%%); a non-route point was probably appended",
			worstIdx, worstDist, share*100, total, maxShare*100)
	}
}

// TestPointToPointDropsMarkerPins is the regression test for the reported bug:
// a Google My Maps export appends start/destination <Point> placemarks after the
// <LineString>, which used to be concatenated onto the route and drew a straight
// line from the route's end back to its start.
func TestPointToPointDropsMarkerPins(t *testing.T) {
	got := parseFile(t, "mymaps_point_to_point.kml")
	assertPoints(t, got, wantPoints)

	for _, pin := range []Point{
		{Lon: 23.3000012, Lat: 42.6999988},
		{Lon: 23.3270031, Lat: 42.7061042},
	} {
		for i, p := range got {
			if pointsEqual(p, pin) {
				t.Errorf("marker pin %v leaked into the route at index %d", pin, i)
			}
		}
	}

	assertNoOutlierSegment(t, got, 0.5)
}

// TestLoopRouteExcludesPins covers the real-world export the bug was found in.
// The 664-point LineString measures ~24712 m; parsing used to yield 666 points
// and ~24720 m because of the two trailing marker pins.
func TestLoopRouteExcludesPins(t *testing.T) {
	got := parseFile(t, "mymaps_loop.kml")

	if len(got) != 664 {
		t.Errorf("got %d points, want 664 (the LineString only, without the 2 marker pins)", len(got))
	}

	total := 0.0
	for i := 1; i < len(got); i++ {
		total += Haversine(got[i-1].Lon, got[i-1].Lat, got[i].Lon, got[i].Lat)
	}
	if math.Abs(total-24712.5) > 1.0 {
		t.Errorf("route length = %.1f m, want ~24712.5 m", total)
	}

	last := got[len(got)-1]
	for _, pin := range []Point{
		{Lon: 23.3633706, Lat: 42.6105772},
		{Lon: 23.3633471, Lat: 42.6106085},
	} {
		if pointsEqual(last, pin) {
			t.Errorf("route still ends on marker pin %v", pin)
		}
	}
}

func TestMultiLineStringJoinsLegsWithoutDuplicateJunction(t *testing.T) {
	got := parseFile(t, "multi_linestring.kml")
	assertPoints(t, got, wantPoints[:5])
}

func TestMultiGeometryIgnoresSiblingPoint(t *testing.T) {
	got := parseFile(t, "multigeometry.kml")
	assertPoints(t, got, wantPoints[:3])
}

func TestGxTrack(t *testing.T) {
	got := parseFile(t, "gx_track.kml")
	assertPoints(t, got, []Point{
		{Lon: 23.30000, Lat: 42.70000, Ele: 612},
		{Lon: 23.30500, Lat: 42.70100, Ele: 618},
		{Lon: 23.31000, Lat: 42.70250, Ele: 625},
	})
}

// TestPointsOnlyFallback keeps hand-built waypoint files working: with no line
// geometry at all, the <Point> placemarks are the route.
func TestPointsOnlyFallback(t *testing.T) {
	got := parseFile(t, "points_only.kml")
	assertPoints(t, got, wantPoints[:3])
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		wantSub string
	}{
		{"empty document", "empty.kml", "route geometry"},
		{"styles but no geometry", "styles_only.kml", "route geometry"},
		{"single point is not a route", "single_point.kml", "at least 2 points"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseKMLCoordinates("testdata/" + tc.file)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantSub)
			}
		})
	}
}

func TestSingleVertexLineStringIsNotARoute(t *testing.T) {
	const doc = `<kml><Document><Placemark><LineString>
		<coordinates>23.30000,42.70000,0</coordinates>
	</LineString></Placemark></Document></kml>`

	_, err := ParseKMLCoordinatesFromReader(strings.NewReader(doc))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "at least 2 points") {
		t.Errorf("error = %q, want it to mention %q", err, "at least 2 points")
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := ParseKMLCoordinates("testdata/does_not_exist.kml"); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestMalformedTuplesAreSkipped(t *testing.T) {
	const doc = `<kml><Document><Placemark><LineString><coordinates>
		23.30000,42.70000,0
		not-a-number,42.70100,0
		23.30500,42.70100,0
		23.31000
		23.31000,42.70250,not-an-elevation
	</coordinates></LineString></Placemark></Document></kml>`

	got, err := ParseKMLCoordinatesFromReader(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	assertPoints(t, got, []Point{
		{Lon: 23.30000, Lat: 42.70000},
		{Lon: 23.30500, Lat: 42.70100},
		{Lon: 23.31000, Lat: 42.70250},
	})
}

// TestSalvagesTruncatedDocument checks that a KML cut short mid-file still
// yields the route it did contain, rather than failing outright.
func TestSalvagesTruncatedDocument(t *testing.T) {
	const doc = `<kml><Document><Placemark><LineString><coordinates>
		23.30000,42.70000,0
		23.30500,42.70100,0
	</coordinates></LineString></Placemark><Placemark><LineString>`

	got, err := ParseKMLCoordinatesFromReader(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	assertPoints(t, got, wantPoints[:2])
}

func TestParseKMLTracksReturnsOneSlicePerGeometry(t *testing.T) {
	tracks, err := ParseKMLTracks(strings.NewReader(mustReadFile(t, "testdata/multi_linestring.kml")))
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(tracks))
	}
	assertPoints(t, tracks[0], wantPoints[:3])
	assertPoints(t, tracks[1], wantPoints[2:5])
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
