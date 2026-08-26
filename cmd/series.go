package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"fitsim/pkg/series"
	"fitsim/pkg/simulator"
)

// datetimeLayout is the format every activity command accepts for --datetime.
const datetimeLayout = "02-01-06 15:04:05"

// addCountFlag registers the --count flag shared by every activity command.
func addCountFlag(cmd *cobra.Command, count *int) {
	cmd.Flags().IntVar(count, "count", 1,
		"Number of FIT files to generate; each starts one minute later than the previous one")
}

// generateSeries parses --datetime and calls gen once per requested file, pushing
// the start time one series.Step further along each time.
//
// The random stream is reset to the same seed before every file, so the files in
// a series differ only in their timestamps — which is the point of the flag: the
// same workout, replayed a minute later.
func generateSeries(datetime, file string, count int, gen func(start time.Time, outFile string) error) error {
	if count < 1 {
		return fmt.Errorf("--count must be at least 1, got %d", count)
	}

	start, err := time.Parse(datetimeLayout, datetime)
	if err != nil {
		return err
	}

	seed := time.Now().UnixNano()
	for i := 1; i <= count; i++ {
		simulator.Reseed(seed)
		if err := gen(start, series.Filename(file, i, count)); err != nil {
			return err
		}
		start = start.Add(series.Step)
	}
	return nil
}
