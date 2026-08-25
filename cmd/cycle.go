package cmd

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/geo"
	"fitsim/pkg/simulator"
)

var cycleCmd = &cobra.Command{
	Use:   "cycle",
	Short: "Simulate a cycling FIT file using a KML route.",
	RunE:  cycleSimulate,
}

var (
	cycleDatetime string
	cycleSpeed    float64
	cycleKML      string
	cycleFile     string
)

func init() {
	rootCmd.AddCommand(cycleCmd)
	cycleCmd.Flags().StringVar(&cycleDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	cycleCmd.Flags().Float64Var(&cycleSpeed, "speed", 0, "Average cycling speed in km/h")
	cycleCmd.Flags().StringVar(&cycleKML, "kml", "", "Input KML file containing the route")
	cycleCmd.Flags().StringVar(&cycleFile, "file", "", "Output FIT filename")
	cycleCmd.MarkFlagRequired("datetime")
	cycleCmd.MarkFlagRequired("speed")
	cycleCmd.MarkFlagRequired("kml")
	cycleCmd.MarkFlagRequired("file")
}

func cycleSimulate(cmd *cobra.Command, args []string) error {
	startTime, err := time.Parse("02-01-06 15:04:05", cycleDatetime)
	if err != nil { return err }

	points, err := geo.ParseKMLCoordinates(cycleKML)
	if err != nil { return err }

	dists := make([]float64, len(points))
	for i := 1; i < len(points); i++ {
		dists[i] = dists[i-1] + geo.Haversine(points[i-1].Lon, points[i-1].Lat, points[i].Lon, points[i].Lat)
	}

	totalDistM := dists[len(dists)-1]
	speedMs := cycleSpeed * 1000.0 / 3600.0
	totalTimeS := int(totalDistM / speedMs)

	builder := fitgen.NewBuilder(startTime, 4400, 345000124)
	builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
	builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

	hr := simulator.RandomFloat(60, 72)
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
		if hr < 60 { hr = 60 }
		if hr > 180 { hr = 180 }
		cadence := 85.0 + simulator.RandomFloat(-7, 7)

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
	lap.Sport = fit.SportCycling
	lap.SubSport = fit.SubSportGeneric
	builder.AddLap(lap)

	session := fit.NewSessionMsg()
	session.Timestamp = currentTimestamp
	session.StartTime = startTime
	session.TotalElapsedTime = uint32(totalTimeS * 1000)
	session.TotalTimerTime = uint32(totalTimeS * 1000)
	session.Sport = fit.SportCycling
	session.SubSport = fit.SubSportGeneric
	session.TotalDistance = uint32(totalDistM * 100)
	builder.AddSession(session)

	builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	return builder.WriteToFile(cycleFile)
}
