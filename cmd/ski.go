package cmd

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/geo"
	"fitsim/pkg/simulator"
)

var skiCmd = &cobra.Command{
	Use:   "ski",
	Short: "Simulate an Alpine or Cross-Country Skiing FIT file using a KML route.",
	RunE:  skiSimulate,
}

var (
	skiDatetime string
	skiType     string
	skiKML      string
	skiFile     string
)

func init() {
	rootCmd.AddCommand(skiCmd)
	skiCmd.Flags().StringVar(&skiDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	skiCmd.Flags().StringVar(&skiType, "type", "alpine", "Type of skiing: alpine or cross-country")
	skiCmd.Flags().StringVar(&skiKML, "kml", "", "Input KML file containing the route")
	skiCmd.Flags().StringVar(&skiFile, "file", "", "Output FIT filename")
	skiCmd.MarkFlagRequired("datetime")
	skiCmd.MarkFlagRequired("kml")
	skiCmd.MarkFlagRequired("file")
}

func skiSimulate(cmd *cobra.Command, args []string) error {
	startTime, err := time.Parse("02-01-06 15:04:05", skiDatetime)
	if err != nil { return err }

	points, err := geo.ParseKMLCoordinates(skiKML)
	if err != nil { return err }

	hasElevation := false
	for _, p := range points {
		if p.Ele != 0.0 { hasElevation = true; break }
	}
	
	if !hasElevation {
		fmt.Println("Fetching elevation data from Open-Elevation API...")
		points = geo.FetchElevation(points, 100)
	}

	dists := make([]float64, len(points))
	for i := 1; i < len(points); i++ {
		dists[i] = dists[i-1] + geo.Haversine(points[i-1].Lon, points[i-1].Lat, points[i].Lon, points[i].Lat)
	}

	builder := fitgen.NewBuilder(startTime, 4400, 345000124)
	builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
	builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

	hr := simulator.RandomFloat(110, 130)
	currentTimestamp := startTime

	var totalAscent float64
	var totalDescent float64

	totalDistM := 0.0

	baseSpeedMs := 3.0 // Cross-country base speed ~ 11 km/h
	if skiType == "alpine" {
		baseSpeedMs = 8.0 // Alpine base speed ~ 28 km/h
	}

	for i := 1; i < len(points); i++ {
		distSeg := dists[i] - dists[i-1]
		eleDiff := points[i].Ele - points[i-1].Ele

		// Adjust speed based on gradient
		speedMs := baseSpeedMs
		if distSeg > 0 {
			gradient := eleDiff / distSeg
			if skiType == "alpine" {
				if gradient < -0.1 {
					speedMs += math.Abs(gradient) * 30.0 // faster downhill
				} else if gradient > 0.1 {
					speedMs = 1.5 // slow chairlift/uphill
				}
			} else {
				// cross country
				if gradient < -0.05 {
					speedMs += math.Abs(gradient) * 10.0
				} else if gradient > 0.05 {
					speedMs *= 0.5
				}
			}
		}

		if speedMs < 0.5 { speedMs = 0.5 }
		timeForSeg := int(distSeg / speedMs)
		if timeForSeg == 0 && distSeg > 0 { timeForSeg = 1 }

		if eleDiff > 0 { totalAscent += eleDiff } else { totalDescent += math.Abs(eleDiff) }

		for t := 0; t < timeForSeg; t++ {
			fraction := float64(t) / float64(timeForSeg)
			lon := points[i-1].Lon + fraction*(points[i].Lon-points[i-1].Lon)
			lat := points[i-1].Lat + fraction*(points[i].Lat-points[i-1].Lat)
			alt := points[i-1].Ele + fraction*eleDiff

			// HR logic based on gradient
			if eleDiff > 0 {
				if skiType == "alpine" { hr -= simulator.RandomFloat(0, 1.0) } else { hr += simulator.RandomFloat(0.5, 1.5) }
			} else {
				if skiType == "alpine" { hr += simulator.RandomFloat(0.5, 1.5) } else { hr -= simulator.RandomFloat(0.5, 1.0) }
			}
			
			if hr < 80 { hr = 80 }
			if hr > 180 { hr = 180 }

			record := fit.NewRecordMsg()
			record.Timestamp = currentTimestamp
			record.PositionLat = fit.NewLatitudeDegrees(lat)
			record.PositionLong = fit.NewLongitudeDegrees(lon)
			record.Distance = uint32((totalDistM + distSeg*fraction) * 100)
			record.Speed = uint16(speedMs * 1000)
			record.HeartRate = uint8(hr)
			record.Altitude = uint16((alt + 500) * 5)
			builder.AddRecord(record)
			
			currentTimestamp = currentTimestamp.Add(time.Second)
		}
		totalDistM += distSeg
	}

	builder.AddEvent(currentTimestamp, fit.EventTimer, fit.EventTypeStop)

	totalTimeS := int(currentTimestamp.Sub(startTime).Seconds())

	lap := fit.NewLapMsg()
	lap.Timestamp = currentTimestamp
	lap.StartTime = startTime
	lap.TotalElapsedTime = uint32(totalTimeS * 1000)
	lap.TotalTimerTime = uint32(totalTimeS * 1000)
	lap.TotalDistance = uint32(totalDistM * 100)
	lap.TotalAscent = uint16(totalAscent)
	lap.TotalDescent = uint16(totalDescent)
	lap.Sport = fit.SportAlpineSkiing
	if skiType == "cross-country" { lap.Sport = fit.SportCrossCountrySkiing }
	lap.SubSport = fit.SubSportGeneric
	builder.AddLap(lap)

	session := fit.NewSessionMsg()
	session.Timestamp = currentTimestamp
	session.StartTime = startTime
	session.TotalElapsedTime = uint32(totalTimeS * 1000)
	session.TotalTimerTime = uint32(totalTimeS * 1000)
	session.Sport = lap.Sport
	session.SubSport = fit.SubSportGeneric
	session.TotalDistance = uint32(totalDistM * 100)
	session.TotalAscent = uint16(totalAscent)
	session.TotalDescent = uint16(totalDescent)
	builder.AddSession(session)

	builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
	if err := builder.WriteToFile(skiFile); err != nil {
		return fmt.Errorf("error writing FIT file: %v", err)
	}
	fmt.Printf("Generated %s ski FIT: %s\n", skiType, skiFile)
	return nil
}
