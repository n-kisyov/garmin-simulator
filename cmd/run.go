package cmd

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/geo"
	"fitsim/pkg/simulator"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Simulate a running FIT file using a KML route.",
	RunE:  runSimulate,
}

var (
	runDatetime string
	runSpeed    float64
	runKML      string
	runFile     string
	runCount    int
)

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVar(&runDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	runCmd.Flags().Float64Var(&runSpeed, "speed", 0, "Average running speed in km/h")
	runCmd.Flags().StringVar(&runKML, "kml", "", "Input KML file containing the route")
	runCmd.Flags().StringVar(&runFile, "file", "", "Output FIT filename")
	addCountFlag(runCmd, &runCount)
	runCmd.MarkFlagRequired("datetime")
	runCmd.MarkFlagRequired("speed")
	runCmd.MarkFlagRequired("kml")
	runCmd.MarkFlagRequired("file")
}

func runSimulate(cmd *cobra.Command, args []string) error {
	points, err := geo.ParseKMLCoordinates(runKML)
	if err != nil { return err }

	dists := make([]float64, len(points))
	for i := 1; i < len(points); i++ {
		dists[i] = dists[i-1] + geo.Haversine(points[i-1].Lon, points[i-1].Lat, points[i].Lon, points[i].Lat)
	}

	totalDistM := dists[len(dists)-1]
	speedMs := runSpeed * 1000.0 / 3600.0
	totalTimeS := int(totalDistM / speedMs)

	return generateSeries(runDatetime, runFile, runCount, func(startTime time.Time, outFile string) error {
		builder := fitgen.NewBuilder(startTime, 4400, 345000124)
		builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
		builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

		hr := simulator.RandomFloat(72, 82)
		totalSteps := 0.0
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

			hr += simulator.RandomFloat(-1.5, 1.5)
			if hr < 72 { hr = 72 }
			if hr > 170 { hr = 170 }

			cadence := 170.0 + simulator.RandomFloat(-5, 5)
			totalSteps += cadence / 60.0

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
		lap.Sport = fit.SportRunning
		lap.SubSport = fit.SubSportGeneric
		builder.AddLap(lap)

		session := fit.NewSessionMsg()
		session.Timestamp = currentTimestamp
		session.StartTime = startTime
		session.TotalElapsedTime = uint32(totalTimeS * 1000)
		session.TotalTimerTime = uint32(totalTimeS * 1000)
		session.Sport = fit.SportRunning
		session.SubSport = fit.SubSportGeneric
		session.TotalDistance = uint32(totalDistM * 100)
		builder.AddSession(session)

		builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
		return builder.WriteToFile(outFile)
	})
}
