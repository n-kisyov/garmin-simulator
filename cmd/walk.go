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

var walkCmd = &cobra.Command{
	Use:   "walk",
	Short: "Simulate a walk FIT file using a KML route.",
	RunE:  walkSimulate,
}

var (
	walkDatetime string
	walkSpeed    float64
	walkKML      string
	walkFile     string
	walkCount    int
)

func init() {
	rootCmd.AddCommand(walkCmd)
	walkCmd.Flags().StringVar(&walkDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	walkCmd.Flags().Float64Var(&walkSpeed, "speed", 0, "Average walking speed in km/h")
	walkCmd.Flags().StringVar(&walkKML, "kml", "", "Input KML file containing the route")
	walkCmd.Flags().StringVar(&walkFile, "file", "", "Output FIT filename")
	addCountFlag(walkCmd, &walkCount)
	walkCmd.MarkFlagRequired("datetime")
	walkCmd.MarkFlagRequired("speed")
	walkCmd.MarkFlagRequired("kml")
	walkCmd.MarkFlagRequired("file")
}

func walkSimulate(cmd *cobra.Command, args []string) error {
	points, err := geo.ParseKMLCoordinates(walkKML)
	if err != nil { return err }

	dists := make([]float64, len(points))
	for i := 1; i < len(points); i++ {
		dists[i] = dists[i-1] + geo.Haversine(points[i-1].Lon, points[i-1].Lat, points[i].Lon, points[i].Lat)
	}

	totalDistM := dists[len(dists)-1]
	speedMs := walkSpeed * 1000.0 / 3600.0
	totalTimeS := int(totalDistM / speedMs)

	return generateSeries(walkDatetime, walkFile, walkCount, func(startTime time.Time, outFile string) error {
		builder := fitgen.NewBuilder(startTime, 4400, 345000124)
		builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
		builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

		hr := simulator.RandomFloat(80, 95)
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
			if hr > 140 { hr = 140 }

			record := fit.NewRecordMsg()
			record.Timestamp = currentTimestamp
			record.PositionLat = fit.NewLatitudeDegrees(lat)
			record.PositionLong = fit.NewLongitudeDegrees(lon)
			record.Distance = uint32(targetDist * 100)
			record.Speed = uint16(speedMs * 1000)
			record.HeartRate = uint8(hr)
			record.Cadence = uint8(110 + simulator.RandomFloat(-5, 5))
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
		lap.Sport = fit.SportWalking
		lap.SubSport = fit.SubSportGeneric
		builder.AddLap(lap)

		session := fit.NewSessionMsg()
		session.Timestamp = currentTimestamp
		session.StartTime = startTime
		session.TotalElapsedTime = uint32(totalTimeS * 1000)
		session.TotalTimerTime = uint32(totalTimeS * 1000)
		session.Sport = fit.SportWalking
		session.SubSport = fit.SubSportGeneric
		session.TotalDistance = uint32(totalDistM * 100)
		builder.AddSession(session)

		builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
		if err := builder.WriteToFile(outFile); err != nil {
			return fmt.Errorf("error writing FIT file: %v", err)
		}
		fmt.Printf("Generated walk FIT: %s\n", outFile)
		return nil
	})
}
