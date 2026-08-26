package cmd

import (
	"fmt"
	"time"
	"math"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/geo"
	"fitsim/pkg/simulator"
)

var hikeCmd = &cobra.Command{
	Use:   "hike",
	Short: "Simulate a hike FIT file using a KML route.",
	RunE:  hikeSimulate,
}

var (
	hikeDatetime string
	hikeSpeed    float64
	hikeKML      string
	hikeFile     string
	hikeCount    int
)

func init() {
	rootCmd.AddCommand(hikeCmd)
	hikeCmd.Flags().StringVar(&hikeDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	hikeCmd.Flags().Float64Var(&hikeSpeed, "speed", 0, "Average hiking speed in km/h")
	hikeCmd.Flags().StringVar(&hikeKML, "kml", "", "Input KML file containing the route")
	hikeCmd.Flags().StringVar(&hikeFile, "file", "", "Output FIT filename")
	addCountFlag(hikeCmd, &hikeCount)
	hikeCmd.MarkFlagRequired("datetime")
	hikeCmd.MarkFlagRequired("speed")
	hikeCmd.MarkFlagRequired("kml")
	hikeCmd.MarkFlagRequired("file")
}

func hikeSimulate(cmd *cobra.Command, args []string) error {
	points, err := geo.ParseKMLCoordinates(hikeKML)
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

	totalDistM := dists[len(dists)-1]
	speedMs := hikeSpeed * 1000.0 / 3600.0
	totalTimeS := int(totalDistM / speedMs)

	return generateSeries(hikeDatetime, hikeFile, hikeCount, func(startTime time.Time, outFile string) error {
		builder := fitgen.NewBuilder(startTime, 4400, 345000124)
		builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
		builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

		hr := simulator.RandomFloat(80, 95)
		currentTimestamp := startTime
		ptIdx := 0

		var totalAscent float64
		var totalDescent float64
		prevAltitude := points[0].Ele

		for t := 0; t <= totalTimeS; t++ {
			targetDist := float64(t) * speedMs
			for ptIdx < len(points)-2 && dists[ptIdx+1] < targetDist { ptIdx++ }
		
			dStart, dEnd := dists[ptIdx], dists[ptIdx+1]
			fraction := 0.0
			if dEnd > dStart { fraction = (targetDist - dStart) / (dEnd - dStart) }
		
			lon := points[ptIdx].Lon + fraction*(points[ptIdx+1].Lon-points[ptIdx].Lon)
			lat := points[ptIdx].Lat + fraction*(points[ptIdx+1].Lat-points[ptIdx].Lat)
			alt := points[ptIdx].Ele + fraction*(points[ptIdx+1].Ele-points[ptIdx].Ele)

			eleDiff := alt - prevAltitude
			if eleDiff > 0 { totalAscent += eleDiff } else { totalDescent += math.Abs(eleDiff) }
			prevAltitude = alt

			hr += simulator.RandomFloat(-1.5, 1.5)
			if hr < 70 { hr = 70 }
			if hr > 165 { hr = 165 }

			record := fit.NewRecordMsg()
			record.Timestamp = currentTimestamp
			record.PositionLat = fit.NewLatitudeDegrees(lat)
			record.PositionLong = fit.NewLongitudeDegrees(lon)
			record.Distance = uint32(targetDist * 100)
			record.Speed = uint16(speedMs * 1000)
			record.HeartRate = uint8(hr)
			record.Cadence = uint8(105 + simulator.RandomFloat(-5, 5))
			record.Altitude = uint16((alt + 500) * 5)
			builder.AddRecord(record)
		
			currentTimestamp = currentTimestamp.Add(time.Second)
		}

		builder.AddEvent(currentTimestamp, fit.EventTimer, fit.EventTypeStop)

		lap := fit.NewLapMsg()
		lap.Timestamp = currentTimestamp
		lap.StartTime = startTime
		lap.TotalElapsedTime = uint32(totalTimeS * 1000)
		lap.TotalTimerTime = uint32(totalTimeS * 1000)
		lap.TotalDistance = uint32(totalDistM * 100)
		lap.TotalAscent = uint16(totalAscent)
		lap.TotalDescent = uint16(totalDescent)
		lap.Sport = fit.SportHiking
		lap.SubSport = fit.SubSportGeneric
		builder.AddLap(lap)

		session := fit.NewSessionMsg()
		session.Timestamp = currentTimestamp
		session.StartTime = startTime
		session.TotalElapsedTime = uint32(totalTimeS * 1000)
		session.TotalTimerTime = uint32(totalTimeS * 1000)
		session.Sport = fit.SportHiking
		session.SubSport = fit.SubSportGeneric
		session.TotalDistance = uint32(totalDistM * 100)
		session.TotalAscent = uint16(totalAscent)
		session.TotalDescent = uint16(totalDescent)
		builder.AddSession(session)

		builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
		if err := builder.WriteToFile(outFile); err != nil {
			return fmt.Errorf("error writing FIT file: %v", err)
		}
		fmt.Printf("Generated hike FIT: %s\n", outFile)
		return nil
	})
}
