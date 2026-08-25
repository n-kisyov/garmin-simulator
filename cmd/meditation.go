package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/simulator"
)

var meditationCmd = &cobra.Command{
	Use:   "meditation",
	Short: "Simulate a meditation FIT file.",
	RunE:  meditationSimulate,
}

var (
	meditationDatetime string
	meditationDuration int
	meditationFile     string
)

func init() {
	rootCmd.AddCommand(meditationCmd)
	meditationCmd.Flags().StringVar(&meditationDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	meditationCmd.Flags().IntVar(&meditationDuration, "duration", 600, "Duration in seconds")
	meditationCmd.Flags().StringVar(&meditationFile, "file", "", "Output FIT filename")
	meditationCmd.MarkFlagRequired("datetime")
	meditationCmd.MarkFlagRequired("file")
}

func meditationSimulate(cmd *cobra.Command, args []string) error {
	startTime, err := time.Parse("02-01-06 15:04:05", meditationDatetime)
	if err != nil { return err }

	totalTimeS := meditationDuration
	builder := fitgen.NewBuilder(startTime, 4400, 345000124)
	builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
	builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

	hr := simulator.RandomFloat(55, 65)
	currentTimestamp := startTime

	for sec := 0; sec < totalTimeS; sec++ {
		record := fit.NewRecordMsg()
		record.Timestamp = currentTimestamp
		
		hr += simulator.RandomFloat(-0.3, 0.3)
		if hr < 45 { hr = 45 }
		if hr > 80 { hr = 80 }
		
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
	lap.Sport = fit.SportTraining
	lap.SubSport = fit.SubSportGeneric
	builder.AddLap(lap)

	session := fit.NewSessionMsg()
	session.Timestamp = currentTimestamp
	session.StartTime = startTime
	session.TotalElapsedTime = uint32(totalTimeS * 1000)
	session.TotalTimerTime = uint32(totalTimeS * 1000)
	session.Sport = fit.SportTraining
	session.SubSport = fit.SubSportGeneric
	builder.AddSession(session)

	builder.AddActivity(currentTimestamp, uint32(totalTimeS*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
	if err := builder.WriteToFile(meditationFile); err != nil {
		return fmt.Errorf("error writing FIT file: %v", err)
	}
	fmt.Printf("Generated meditation FIT: %s\n", meditationFile)
	return nil
}
