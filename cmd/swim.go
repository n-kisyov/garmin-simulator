package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/simulator"
)

var swimCmd = &cobra.Command{
	Use:   "swim",
	Short: "Simulate a swimming FIT file.",
	RunE:  swimSimulate,
}

var (
	swimDatetime string
	swimDistance float64
	swimSpeed    float64
	swimFile     string
	swimCount    int
)

func init() {
	rootCmd.AddCommand(swimCmd)
	swimCmd.Flags().StringVar(&swimDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	swimCmd.Flags().Float64Var(&swimDistance, "distance", 0, "Total swim distance in kilometers")
	swimCmd.Flags().Float64Var(&swimSpeed, "speed", 0, "Average swimming speed in km/h")
	swimCmd.Flags().StringVar(&swimFile, "file", "", "Output FIT filename")
	addCountFlag(swimCmd, &swimCount)
	swimCmd.MarkFlagRequired("datetime")
	swimCmd.MarkFlagRequired("distance")
	swimCmd.MarkFlagRequired("speed")
	swimCmd.MarkFlagRequired("file")
}

func swimSimulate(cmd *cobra.Command, args []string) error {
	distanceM := swimDistance * 1000.0
	speedMs := swimSpeed * 1000.0 / 3600.0
	totalTimeS := int(distanceM / speedMs)

	return generateSeries(swimDatetime, swimFile, swimCount, func(startTime time.Time, outFile string) error {
		builder := fitgen.NewBuilder(startTime, 4400, 345000124)
		builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
		builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

		hr := simulator.RandomFloat(80, 95)
		currentTimestamp := startTime
		totalCalories := uint16(float64(totalTimeS) * (500.0 / 3600.0))
		strokeCycles := 0.0

		for t := 0; t <= totalTimeS; t++ {
			targetDist := float64(t) * speedMs
			if targetDist > distanceM { targetDist = distanceM }

			hr += simulator.RandomFloat(-1.0, 1.0)
			if hr < 90 { hr = 90 }
			if hr > 160 { hr = 160 }

			strokeCycles += 0.8 // arbitrary 48 strokes per minute = 0.8 per sec
		
			record := fit.NewRecordMsg()
			record.Timestamp = currentTimestamp
			record.Distance = uint32(targetDist * 100)
			record.Speed = uint16(speedMs * 1000)
			record.HeartRate = uint8(hr)
			record.TotalCycles = uint32(strokeCycles)
			builder.AddRecord(record)
		
			currentTimestamp = currentTimestamp.Add(time.Second)
		}

		builder.AddEvent(currentTimestamp, fit.EventTimer, fit.EventTypeStop)

		lap := fit.NewLapMsg()
		lap.Timestamp = currentTimestamp
		lap.StartTime = startTime
		lap.TotalElapsedTime = uint32(totalTimeS * 1000)
		lap.TotalTimerTime = uint32(totalTimeS * 1000)
		lap.TotalDistance = uint32(distanceM * 100)
		lap.TotalCalories = totalCalories
		lap.TotalCycles = uint32(strokeCycles)
		lap.Sport = fit.SportSwimming
		lap.SubSport = fit.SubSportLapSwimming
		builder.AddLap(lap)

		session := fit.NewSessionMsg()
		session.Timestamp = currentTimestamp
		session.StartTime = startTime
		session.TotalElapsedTime = uint32(totalTimeS * 1000)
		session.TotalTimerTime = uint32(totalTimeS * 1000)
		session.Sport = fit.SportSwimming
		session.SubSport = fit.SubSportLapSwimming
		session.TotalDistance = uint32(distanceM * 100)
		session.TotalCalories = totalCalories
		session.TotalCycles = uint32(strokeCycles)
		builder.AddSession(session)

		builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
		if err := builder.WriteToFile(outFile); err != nil {
			return fmt.Errorf("error writing FIT file: %v", err)
		}
		fmt.Printf("Generated swim FIT: %s\n", outFile)
		return nil
	})
}
