package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/simulator"
)

var strengthCmd = &cobra.Command{
	Use:   "strength",
	Short: "Simulate a strength training FIT file.",
	RunE:  strengthSimulate,
}

var (
	strengthDatetime string
	strengthFile     string
	strengthReps     int
	strengthSets     int
)

func init() {
	rootCmd.AddCommand(strengthCmd)
	strengthCmd.Flags().StringVar(&strengthDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	strengthCmd.Flags().StringVar(&strengthFile, "file", "", "Output FIT filename")
	strengthCmd.Flags().IntVar(&strengthReps, "reps", 10, "Number of reps per set")
	strengthCmd.Flags().IntVar(&strengthSets, "sets", 6, "Total number of sets")
	strengthCmd.MarkFlagRequired("datetime")
	strengthCmd.MarkFlagRequired("file")
}

func strengthSimulate(cmd *cobra.Command, args []string) error {
	startTime, err := time.Parse("02-01-06 15:04:05", strengthDatetime)
	if err != nil { return err }

	setDuration := strengthReps * 3
	restDuration := 60
	totalTimeS := strengthSets * setDuration + (strengthSets-1)*restDuration

	builder := fitgen.NewBuilder(startTime, 4400, 345000124)
	builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
	builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

	hr := simulator.RandomFloat(68, 75)
	currentTimestamp := startTime
	totalCalories := uint16(float64(totalTimeS) * (450.0 / 3600.0))

	for s := 0; s < strengthSets; s++ {
		// Active set
		for sec := 0; sec < setDuration; sec++ {
			record := fit.NewRecordMsg()
			record.Timestamp = currentTimestamp
			
			hr += simulator.RandomFloat(0.5, 1.5)
			if hr > 165 { hr = 165 }
			
			record.HeartRate = uint8(hr)
			builder.AddRecord(record)
			currentTimestamp = currentTimestamp.Add(time.Second)
		}

		// Rest period
		if s < strengthSets-1 {
			for sec := 0; sec < restDuration; sec++ {
				record := fit.NewRecordMsg()
				record.Timestamp = currentTimestamp
				
				hr -= simulator.RandomFloat(0.1, 0.8)
				if hr < 75 { hr = 75 }
				
				record.HeartRate = uint8(hr)
				builder.AddRecord(record)
				currentTimestamp = currentTimestamp.Add(time.Second)
			}
		}
	}

	builder.AddEvent(currentTimestamp, fit.EventTimer, fit.EventTypeStop)

	lap := fit.NewLapMsg()
	lap.Timestamp = currentTimestamp
	lap.StartTime = startTime
	lap.TotalElapsedTime = uint32(totalTimeS * 1000)
	lap.TotalTimerTime = uint32(totalTimeS * 1000)
	lap.TotalCalories = totalCalories
	lap.Sport = fit.SportTraining
	lap.SubSport = fit.SubSportStrengthTraining
	builder.AddLap(lap)

	session := fit.NewSessionMsg()
	session.Timestamp = currentTimestamp
	session.StartTime = startTime
	session.TotalElapsedTime = uint32(totalTimeS * 1000)
	session.TotalTimerTime = uint32(totalTimeS * 1000)
	session.Sport = fit.SportTraining
	session.SubSport = fit.SubSportStrengthTraining
	session.TotalCalories = totalCalories
	builder.AddSession(session)

	builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
	if err := builder.WriteToFile(strengthFile); err != nil {
		return fmt.Errorf("error writing FIT file: %v", err)
	}
	fmt.Printf("Generated strength FIT: %s\n", strengthFile)
	return nil
}
