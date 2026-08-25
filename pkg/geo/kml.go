package geo

import (
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Point represents a geographical point with Longitude, Latitude, and optional Elevation.
type Point struct {
	Lon float64
	Lat float64
	Ele float64
}

// ParseKMLCoordinates reads a KML file and extracts the <coordinates> tags into a slice of Points.
func ParseKMLCoordinates(kmlFile string) ([]Point, error) {
	file, err := os.Open(kmlFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`(?s)<coordinates>(.*?)</coordinates>`)
	matches := re.FindAllStringSubmatch(string(content), -1)

	if len(matches) == 0 {
		return nil, errors.New("could not find any <coordinates> inside the KML file")
	}

	var points []Point
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		
		// The matched content is in match[1]
		coordBlock := strings.TrimSpace(match[1])
		chunks := strings.Fields(coordBlock)
		
		for _, chunk := range chunks {
			parts := strings.Split(chunk, ",")
			if len(parts) >= 2 {
				lon, err := strconv.ParseFloat(parts[0], 64)
				if err != nil {
					continue
				}
				lat, err := strconv.ParseFloat(parts[1], 64)
				if err != nil {
					continue
				}
				
				ele := 0.0
				if len(parts) >= 3 {
					if parsedEle, err := strconv.ParseFloat(parts[2], 64); err == nil {
						ele = parsedEle
					}
				}
				
				points = append(points, Point{Lon: lon, Lat: lat, Ele: ele})
			}
		}
	}

	return points, nil
}
