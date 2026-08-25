package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/simulator"
)

var yogaCmd = &cobra.Command{
	Use:   "yoga",
	Short: "Simulate a yoga session FIT file.",
	RunE:  yogaSimulate,
}

var (
	yogaDatetime string
	yogaDuration int
	yogaFile     string
)

func init() {
	rootCmd.AddCommand(yogaCmd)
	yogaCmd.Flags().StringVar(&yogaDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	yogaCmd.Flags().IntVar(&yogaDuration, "duration", 1800, "Duration in seconds")
	yogaCmd.Flags().StringVar(&yogaFile, "file", "", "Output FIT filename")
	yogaCmd.MarkFlagRequired("datetime")
	yogaCmd.MarkFlagRequired("file")
}

func yogaSimulate(cmd *cobra.Command, args []string) error {
	startTime, err := time.Parse("02-01-06 15:04:05", yogaDatetime)
	if err != nil { return err }

	totalTimeS := yogaDuration
	builder := fitgen.NewBuilder(startTime, 4400, 345000124)
	builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
	builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

	hr := simulator.RandomFloat(62, 70)
	currentTimestamp := startTime
	totalCalories := uint16(float64(totalTimeS) * (200.0 / 3600.0))

	for sec := 0; sec < totalTimeS; sec++ {
		record := fit.NewRecordMsg()
		record.Timestamp = currentTimestamp
		
		hr += simulator.RandomFloat(-0.8, 0.8)
		if hr < 60 { hr = 60 }
		if hr > 110 { hr = 110 }
		
		record.HeartRate = uint8(hr)
		builder.AddRecord(record)
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	builder.AddEvent(currentTimestamp, fit.EventTimer, fit.EventTypeStop)

	lap := fit.NewLapMsg()
	lap.Timestamp = currentTimestamp
	lap.StartTime = startTime
	lap.TotalElapsedTime = uint32(totalTimeS * 1000)
	lap.TotalTimerTime = uint32(totalTimeS * 1000)
	lap.TotalCalories = totalCalories
	lap.Sport = fit.SportTraining
	lap.SubSport = fit.SubSportYoga
	builder.AddLap(lap)

	session := fit.NewSessionMsg()
	session.Timestamp = currentTimestamp
	session.StartTime = startTime
	session.TotalElapsedTime = uint32(totalTimeS * 1000)
	session.TotalTimerTime = uint32(totalTimeS * 1000)
	session.Sport = fit.SportTraining
	session.SubSport = fit.SubSportYoga
	session.TotalCalories = totalCalories
	builder.AddSession(session)

	builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
	if err := builder.WriteToFile(yogaFile); err != nil {
		return fmt.Errorf("error writing FIT file: %v", err)
	}
	fmt.Printf("Generated yoga FIT: %s\n", yogaFile)
	return nil
}
