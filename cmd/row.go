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

var rowCmd = &cobra.Command{
	Use:   "row",
	Short: "Simulate a rowing FIT file (indoor or outdoor).",
	RunE:  rowSimulate,
}

var (
	rowDatetime string
	rowDuration int
	rowKML      string
	rowFile     string
)

func init() {
	rootCmd.AddCommand(rowCmd)
	rowCmd.Flags().StringVar(&rowDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	rowCmd.Flags().IntVar(&rowDuration, "duration", 1800, "Duration of the activity in seconds (if indoor)")
	rowCmd.Flags().StringVar(&rowKML, "kml", "", "Optional input KML file for outdoor rowing")
	rowCmd.Flags().StringVar(&rowFile, "file", "", "Output FIT filename")
	rowCmd.MarkFlagRequired("datetime")
	rowCmd.MarkFlagRequired("file")
}

func rowSimulate(cmd *cobra.Command, args []string) error {
	startTime, err := time.Parse("02-01-06 15:04:05", rowDatetime)
	if err != nil { return err }

	var points []geo.Point
	isOutdoor := rowKML != ""

	if isOutdoor {
		points, err = geo.ParseKMLCoordinates(rowKML)
		if err != nil { return err }
	}

	builder := fitgen.NewBuilder(startTime, 4400, 345000124)
	builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
	builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

	hr := simulator.RandomFloat(110, 130)
	currentTimestamp := startTime

	strokeRate := 24.0 // strokes per min
	totalStrokes := 0.0
	speedMs := 2.5 // ~9 km/h

	totalDistM := 0.0
	totalTimeS := rowDuration

	if isOutdoor {
		dists := make([]float64, len(points))
		for i := 1; i < len(points); i++ {
			dists[i] = dists[i-1] + geo.Haversine(points[i-1].Lon, points[i-1].Lat, points[i].Lon, points[i].Lat)
		}
		totalDistM = dists[len(dists)-1]
		totalTimeS = int(totalDistM / speedMs)

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
			if hr < 100 { hr = 100 }
			if hr > 160 { hr = 160 }

			strokeRate += simulator.RandomFloat(-0.5, 0.5)
			if strokeRate < 18 { strokeRate = 18 }
			if strokeRate > 32 { strokeRate = 32 }
			totalStrokes += strokeRate / 60.0

			record := fit.NewRecordMsg()
			record.Timestamp = currentTimestamp
			record.PositionLat = fit.NewLatitudeDegrees(lat)
			record.PositionLong = fit.NewLongitudeDegrees(lon)
			record.Distance = uint32(targetDist * 100)
			record.Speed = uint16(speedMs * 1000)
			record.HeartRate = uint8(hr)
			record.Cadence = uint8(strokeRate)
			builder.AddRecord(record)
			
			currentTimestamp = currentTimestamp.Add(time.Second)
		}
	} else {
		for t := 0; t < totalTimeS; t++ {
			hr += simulator.RandomFloat(-1.0, 1.0)
			if hr < 100 { hr = 100 }
			if hr > 160 { hr = 160 }

			strokeRate += simulator.RandomFloat(-0.5, 0.5)
			if strokeRate < 18 { strokeRate = 18 }
			if strokeRate > 32 { strokeRate = 32 }
			totalStrokes += strokeRate / 60.0

			totalDistM += speedMs

			record := fit.NewRecordMsg()
			record.Timestamp = currentTimestamp
			record.Distance = uint32(totalDistM * 100)
			record.Speed = uint16(speedMs * 1000)
			record.HeartRate = uint8(hr)
			record.Cadence = uint8(strokeRate)
			builder.AddRecord(record)
			
			currentTimestamp = currentTimestamp.Add(time.Second)
		}
	}

	builder.AddEvent(currentTimestamp, fit.EventTimer, fit.EventTypeStop)
	totalCalories := uint16(float64(totalTimeS) * (600.0 / 3600.0))

	lap := fit.NewLapMsg()
	lap.Timestamp = currentTimestamp
	lap.StartTime = startTime
	lap.TotalElapsedTime = uint32(totalTimeS * 1000)
	lap.TotalTimerTime = uint32(totalTimeS * 1000)
	lap.TotalDistance = uint32(totalDistM * 100)
	lap.TotalCycles = uint32(totalStrokes)
	lap.TotalCalories = totalCalories
	lap.Sport = fit.SportRowing
	lap.SubSport = fit.SubSportIndoorRowing
	if isOutdoor { lap.SubSport = fit.SubSportGeneric }
	builder.AddLap(lap)

	session := fit.NewSessionMsg()
	session.Timestamp = currentTimestamp
	session.StartTime = startTime
	session.TotalElapsedTime = uint32(totalTimeS * 1000)
	session.TotalTimerTime = uint32(totalTimeS * 1000)
	session.Sport = lap.Sport
	session.SubSport = lap.SubSport
	session.TotalDistance = uint32(totalDistM * 100)
	session.TotalCycles = uint32(totalStrokes)
	session.TotalCalories = totalCalories
	builder.AddSession(session)

	builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
	if err := builder.WriteToFile(rowFile); err != nil {
		return fmt.Errorf("error writing FIT file: %v", err)
	}
	fmt.Printf("Generated row FIT: %s\n", rowFile)
	return nil
}
