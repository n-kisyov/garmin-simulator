package geo

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Point represents a geographical point with Longitude, Latitude, and optional Elevation.
type Point struct {
	Lon float64
	Lat float64
	Ele float64
}

// seamWarnMeters is the gap between two consecutive geometries that is large
// enough to be worth reporting. Only the seams between geometries are checked,
// never the segments inside one, because exporters legitimately emit sparse
// vertices (several hundred metres apart) along long straight roads.
const seamWarnMeters = 250.0

// ParseKMLCoordinates reads a KML file and extracts its route geometry into a slice of Points.
func ParseKMLCoordinates(kmlFile string) ([]Point, error) {
	file, err := os.Open(kmlFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ParseKMLCoordinatesFromReader(file)
}

// ParseKMLCoordinatesFromReader parses the route geometry from an io.Reader and
// flattens it into a single ordered track.
func ParseKMLCoordinatesFromReader(r io.Reader) ([]Point, error) {
	tracks, err := ParseKMLTracks(r)
	if err != nil {
		return nil, err
	}
	return joinTracks(tracks)
}

// ParseKMLTracks extracts the route geometry from a KML document, returning one
// slice of Points per geometry element, in document order.
//
// Only geometries that describe a path are returned. A KML exported from Google
// My Maps carries the route as a <LineString> followed by standalone <Point>
// placemarks for the start and destination pins; those pins must not be appended
// to the route or the track jumps from its end back to its start in a straight
// line. Geometry kinds are therefore tried in order of preference:
// <LineString>/<gx:Track>, then <LinearRing>, then bare <Point> placemarks as a
// last resort for hand-built waypoint files.
func ParseKMLTracks(r io.Reader) ([][]Point, error) {
	d := xml.NewDecoder(r)
	// Pass unknown encodings straight through rather than failing outright; the
	// coordinate payload of a KML is always ASCII even when the prologue lies.
	d.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}

	var (
		lines  [][]Point // <LineString> and <gx:Track>
		rings  [][]Point // <LinearRing>
		pins   []Point   // standalone <Point> placemarks
		stack  []geomFrame
		xmlErr error
	)

	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Salvage whatever was collected before the markup went bad; a
			// hand-edited KML with one stray tag should still produce a route.
			xmlErr = err
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "LineString", "LinearRing", "Track", "Point":
				stack = append(stack, geomFrame{name: t.Name.Local})

			case "coordinates":
				var block string
				if err := d.DecodeElement(&block, &t); err != nil {
					xmlErr = err
					continue
				}
				appendToTop(&stack, parseCoordBlock(block))

			case "coord":
				// <gx:coord> holds a single space-separated "lon lat ele" tuple.
				var raw string
				if err := d.DecodeElement(&raw, &t); err != nil {
					xmlErr = err
					continue
				}
				if p, ok := pointFromParts(strings.Fields(raw)); ok {
					appendToTop(&stack, []Point{p})
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "LineString", "LinearRing", "Track", "Point":
				if len(stack) == 0 || stack[len(stack)-1].name != t.Name.Local {
					continue
				}
				frame := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if len(frame.pts) == 0 {
					continue
				}
				switch frame.name {
				case "LineString", "Track":
					lines = append(lines, frame.pts)
				case "LinearRing":
					rings = append(rings, frame.pts)
				case "Point":
					pins = append(pins, frame.pts...)
				}
			}
		}
	}

	switch {
	case len(lines) > 0:
		return lines, nil
	case len(rings) > 0:
		return rings, nil
	case len(pins) > 0:
		// Even a single pin is forwarded so that the point-count check in
		// joinTracks owns the "not enough points" error message.
		return [][]Point{pins}, nil
	}

	if xmlErr != nil {
		return nil, fmt.Errorf("could not parse the KML file: %w", xmlErr)
	}
	return nil, errors.New("could not find any route geometry (<LineString>, <LinearRing> or <gx:Track>) inside the KML file")
}

// geomFrame tracks the geometry element currently being read and the points
// gathered inside it.
type geomFrame struct {
	name string
	pts  []Point
}

// appendToTop adds points to the innermost open geometry, discarding coordinates
// that sit outside one.
func appendToTop(stack *[]geomFrame, pts []Point) {
	if len(*stack) == 0 || len(pts) == 0 {
		return
	}
	top := &(*stack)[len(*stack)-1]
	top.pts = append(top.pts, pts...)
}

// parseCoordBlock reads a <coordinates> body: whitespace-separated
// "lon,lat[,ele]" tuples. Malformed tuples are skipped.
func parseCoordBlock(block string) []Point {
	var pts []Point
	for _, chunk := range strings.Fields(block) {
		if p, ok := pointFromParts(strings.Split(chunk, ",")); ok {
			pts = append(pts, p)
		}
	}
	return pts
}

// pointFromParts builds a Point from an already-split "lon lat [ele]" tuple.
func pointFromParts(parts []string) (Point, bool) {
	if len(parts) < 2 {
		return Point{}, false
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return Point{}, false
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return Point{}, false
	}
	ele := 0.0
	if len(parts) >= 3 {
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64); err == nil {
			ele = parsed
		}
	}
	return Point{Lon: lon, Lat: lat, Ele: ele}, true
}

// joinTracks concatenates the geometries into one continuous track, dropping the
// vertex that multi-leg exports repeat at each junction and warning about seams
// that are too wide to be a genuine continuation.
func joinTracks(tracks [][]Point) ([]Point, error) {
	var out []Point
	for i, track := range tracks {
		if len(track) == 0 {
			continue
		}
		if len(out) > 0 {
			last := out[len(out)-1]
			if track[0].Lon == last.Lon && track[0].Lat == last.Lat {
				track = track[1:]
				if len(track) == 0 {
					continue
				}
			} else if gap := Haversine(last.Lon, last.Lat, track[0].Lon, track[0].Lat); gap > seamWarnMeters {
				fmt.Fprintf(os.Stderr, "  Warning: %.0f m gap between KML geometry %d and %d; the track will jump straight across it.\n", gap, i, i+1)
			}
		}
		out = append(out, track...)
	}

	if len(out) < 2 {
		return nil, fmt.Errorf("the KML route needs at least 2 points, found %d", len(out))
	}
	return out, nil
}
