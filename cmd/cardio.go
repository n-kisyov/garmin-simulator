package cmd

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
	"fitsim/pkg/fitgen"
	"fitsim/pkg/simulator"
)

var cardioCmd = &cobra.Command{
	Use:   "cardio",
	Short: "Simulate a generic cardio workout FIT file.",
	RunE:  cardioSimulate,
}

var (
	cardioDatetime string
	cardioDuration int
	cardioFile     string
	cardioCount    int
)

func init() {
	rootCmd.AddCommand(cardioCmd)
	cardioCmd.Flags().StringVar(&cardioDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	cardioCmd.Flags().IntVar(&cardioDuration, "duration", 0, "Duration of the activity in seconds")
	cardioCmd.Flags().StringVar(&cardioFile, "file", "", "Output FIT filename")
	addCountFlag(cardioCmd, &cardioCount)
	cardioCmd.MarkFlagRequired("datetime")
	cardioCmd.MarkFlagRequired("duration")
	cardioCmd.MarkFlagRequired("file")
}

func cardioSimulate(cmd *cobra.Command, args []string) error {
	return generateSeries(cardioDatetime, cardioFile, cardioCount, func(startTime time.Time, outFile string) error {
		builder := fitgen.NewBuilder(startTime, 4400, 345000123)
		builder.AddDeviceInfo(startTime, 4400, 345000123, 14.50)
		builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

		currentTimestamp := startTime
		hr := 80.0
		totalCalories := uint16(float64(cardioDuration) * (600.0 / 3600.0))

		for i := 0; i < cardioDuration; i++ {
			record := fit.NewRecordMsg()
			record.Timestamp = currentTimestamp

			if hr < 140 { hr += simulator.RandomFloat(0.1, 0.6)
			} else { hr += simulator.RandomFloat(-2.0, 2.0) }
			if hr < 60 { hr = 60 }
			if hr > 160 { hr = 160 }

			record.HeartRate = uint8(hr)
			builder.AddRecord(record)
			currentTimestamp = currentTimestamp.Add(time.Second)
		}

		builder.AddEvent(currentTimestamp, fit.EventTimer, fit.EventTypeStop)

		lap := fit.NewLapMsg()
		lap.Timestamp = currentTimestamp
		lap.StartTime = startTime
		lap.TotalElapsedTime = uint32(cardioDuration * 1000)
		lap.TotalTimerTime = uint32(cardioDuration * 1000)
		lap.TotalCalories = totalCalories
		lap.Sport = fit.SportTraining
		lap.SubSport = fit.SubSportCardioTraining
		builder.AddLap(lap)

		session := fit.NewSessionMsg()
		session.Timestamp = currentTimestamp
		session.StartTime = startTime
		session.TotalElapsedTime = uint32(cardioDuration * 1000)
		session.TotalTimerTime = uint32(cardioDuration * 1000)
		session.Sport = fit.SportTraining
		session.SubSport = fit.SubSportCardioTraining
		session.TotalCalories = totalCalories
		builder.AddSession(session)

		builder.AddActivity(currentTimestamp, uint32(cardioDuration*1000), 1, fit.EventActivity, fit.EventTypeStop)
		return builder.WriteToFile(outFile)
	})
}
