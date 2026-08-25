package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/simulator"
)

var fieldCmd = &cobra.Command{
	Use:   "field",
	Short: "Simulate a field sport (soccer, basketball, tennis) FIT file.",
	RunE:  fieldSimulate,
}

var (
	fieldDatetime string
	fieldDuration int
	fieldSport    string
	fieldFile     string
)

func init() {
	rootCmd.AddCommand(fieldCmd)
	fieldCmd.Flags().StringVar(&fieldDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	fieldCmd.Flags().IntVar(&fieldDuration, "duration", 3600, "Duration in seconds")
	fieldCmd.Flags().StringVar(&fieldSport, "sport", "soccer", "Sport type: soccer, basketball, tennis")
	fieldCmd.Flags().StringVar(&fieldFile, "file", "", "Output FIT filename")
	fieldCmd.MarkFlagRequired("datetime")
	fieldCmd.MarkFlagRequired("file")
}

func fieldSimulate(cmd *cobra.Command, args []string) error {
	startTime, err := time.Parse("02-01-06 15:04:05", fieldDatetime)
	if err != nil { return err }

	var sportType fit.Sport
	switch fieldSport {
	case "soccer":
		sportType = fit.SportSoccer
	case "basketball":
		sportType = fit.SportBasketball
	case "tennis":
		sportType = fit.SportTennis
	default:
		sportType = fit.SportGeneric
	}

	builder := fitgen.NewBuilder(startTime, 4400, 345000124)
	builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
	builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

	hr := simulator.RandomFloat(80, 100)
	currentTimestamp := startTime
	totalCalories := uint16(float64(fieldDuration) * (700.0 / 3600.0))

	// Bursty heart rate simulation
	inSprint := false
	sprintTimer := 0

	for sec := 0; sec < fieldDuration; sec++ {
		record := fit.NewRecordMsg()
		record.Timestamp = currentTimestamp
		
		if inSprint {
			hr += simulator.RandomFloat(1.0, 3.0)
			sprintTimer--
			if sprintTimer <= 0 { inSprint = false; sprintTimer = int(simulator.RandomFloat(15, 60)) }
		} else {
			hr -= simulator.RandomFloat(0.5, 2.0)
			sprintTimer--
			if sprintTimer <= 0 { inSprint = true; sprintTimer = int(simulator.RandomFloat(5, 15)) }
		}

		if hr < 90 { hr = 90 }
		if hr > 185 { hr = 185 }
		
		record.HeartRate = uint8(hr)
		builder.AddRecord(record)
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	builder.AddEvent(currentTimestamp, fit.EventTimer, fit.EventTypeStop)

	lap := fit.NewLapMsg()
	lap.Timestamp = currentTimestamp
	lap.StartTime = startTime
	lap.TotalElapsedTime = uint32(fieldDuration * 1000)
	lap.TotalTimerTime = uint32(fieldDuration * 1000)
	lap.TotalCalories = totalCalories
	lap.Sport = sportType
	lap.SubSport = fit.SubSportGeneric
	builder.AddLap(lap)

	session := fit.NewSessionMsg()
	session.Timestamp = currentTimestamp
	session.StartTime = startTime
	session.TotalElapsedTime = uint32(fieldDuration * 1000)
	session.TotalTimerTime = uint32(fieldDuration * 1000)
	session.Sport = sportType
	session.SubSport = fit.SubSportGeneric
	session.TotalCalories = totalCalories
	builder.AddSession(session)

	builder.AddActivity(currentTimestamp, uint32(fieldDuration*1000), 1, fit.EventActivity, fit.EventTypeStop)
	
	if err := builder.WriteToFile(fieldFile); err != nil {
		return fmt.Errorf("error writing FIT file: %v", err)
	}
	fmt.Printf("Generated field (%s) FIT: %s\n", fieldSport, fieldFile)
	return nil
}
