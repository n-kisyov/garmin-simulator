package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/geo"
	"fitsim/pkg/simulator"
)

var ebikeCmd = &cobra.Command{
	Use:   "ebike",
	Short: "Simulate an E-Biking FIT file using a KML route.",
	RunE:  ebikeSimulate,
}

var (
	ebikeDatetime string
	ebikeSpeed    float64
	ebikeKML      string
	ebikeFile     string
	ebikeCount    int
)

func init() {
	rootCmd.AddCommand(ebikeCmd)
	ebikeCmd.Flags().StringVar(&ebikeDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	ebikeCmd.Flags().Float64Var(&ebikeSpeed, "speed", 25.0, "Average e-biking speed in km/h")
	ebikeCmd.Flags().StringVar(&ebikeKML, "kml", "", "Input KML file containing the route")
	ebikeCmd.Flags().StringVar(&ebikeFile, "file", "", "Output FIT filename")
	addCountFlag(ebikeCmd, &ebikeCount)
	ebikeCmd.MarkFlagRequired("datetime")
	ebikeCmd.MarkFlagRequired("kml")
	ebikeCmd.MarkFlagRequired("file")
}

func ebikeSimulate(cmd *cobra.Command, args []string) error {
	points, err := geo.ParseKMLCoordinates(ebikeKML)
	if err != nil { return err }

	dists := make([]float64, len(points))
	for i := 1; i < len(points); i++ {
		dists[i] = dists[i-1] + geo.Haversine(points[i-1].Lon, points[i-1].Lat, points[i].Lon, points[i].Lat)
	}

	totalDistM := dists[len(dists)-1]
	speedMs := ebikeSpeed * 1000.0 / 3600.0
	totalTimeS := int(totalDistM / speedMs)

	return generateSeries(ebikeDatetime, ebikeFile, ebikeCount, func(startTime time.Time, outFile string) error {
		builder := fitgen.NewBuilder(startTime, 4400, 345000124)
		builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
		builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

		hr := simulator.RandomFloat(80, 95) // Lower HR than normal cycling
		currentTimestamp := startTime
		ptIdx := 0

		for t := 0; t <= totalTimeS; t++ {
			targetDist := float64(t) * speedMs
			for ptIdx < len(points)-2 && dists[ptIdx+1] < targetDist { ptIdx++ }
		
			dStart, dEnd := dists[ptIdx], dists[ptIdx+1]
			fraction := 0.0
			if dEnd > dStart { fraction = (targetDist - dStart) / (dEnd - dStart) }
		
			lon := points[ptIdx].Lon + fraction*(points[ptIdx+1].Lon-points[ptIdx].Lon)
			lat := points[ptIdx].Lat + fraction*(points[ptIdx+1].Lat-points[ptIdx].Lat)

			hr += simulator.RandomFloat(-1.0, 1.0)
			if hr < 70 { hr = 70 }
			if hr > 130 { hr = 130 }
			cadence := 75.0 + simulator.RandomFloat(-5, 5) // Slightly lower exertion cadence

			record := fit.NewRecordMsg()
			record.Timestamp = currentTimestamp
			record.PositionLat = fit.NewLatitudeDegrees(lat)
			record.PositionLong = fit.NewLongitudeDegrees(lon)
			record.Distance = uint32(targetDist * 100)
			record.Speed = uint16(speedMs * 1000)
			record.HeartRate = uint8(hr)
			record.Cadence = uint8(cadence)
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
		lap.Sport = fit.SportEBiking
		lap.SubSport = fit.SubSportGeneric
		builder.AddLap(lap)

		session := fit.NewSessionMsg()
		session.Timestamp = currentTimestamp
		session.StartTime = startTime
		session.TotalElapsedTime = uint32(totalTimeS * 1000)
		session.TotalTimerTime = uint32(totalTimeS * 1000)
		session.Sport = fit.SportEBiking
		session.SubSport = fit.SubSportGeneric
		session.TotalDistance = uint32(totalDistM * 100)
		builder.AddSession(session)

		builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
		if err := builder.WriteToFile(outFile); err != nil {
			return fmt.Errorf("error writing FIT file: %v", err)
		}
		fmt.Printf("Generated ebike FIT: %s\n", outFile)
		return nil
	})
}
