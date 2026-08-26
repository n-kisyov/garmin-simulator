package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/AlecAivazis/survey/v2"
)

func runInteractiveMenu() error {
	fmt.Println("Welcome to the fitsim interactive mode!")

	var activityType string
	prompt := &survey.Select{
		Message: "Choose an activity to simulate:",
		Options: []string{
			"run", "cycle", "walk", "hike", 
			"cardio", "strength", "yoga", "meditation", 
			"swim", "ski", "row", "ebike", "field",
		},
	}
	if err := survey.AskOne(prompt, &activityType); err != nil {
		return err
	}

	// Common parameters
	var datetimeStr string
	defaultTime := time.Now().Format("02-01-06 15:04:05")
	if err := survey.AskOne(&survey.Input{Message: "Start datetime (dd-mm-yy HH:MM:SS):", Default: defaultTime}, &datetimeStr); err != nil { return err }

	var outFile string
	if err := survey.AskOne(&survey.Input{Message: "Output FIT filename:", Default: activityType + ".fit"}, &outFile); err != nil { return err }

	// More than one file numbers the names (run1.fit, run2.fit, ...) and pushes
	// each start a minute past the one before it. askInt answers 0 for anything it
	// cannot read, which here just means one file rather than an error.
	count := max(askInt("Number of files to generate:", "1"), 1)

	var err error
	switch activityType {
	case "run":
		runDatetime, runFile, runCount = datetimeStr, outFile, count
		runKML = askString("Input KML file path:", "route.kml")
		runSpeed = askFloat("Average speed (km/h):", "10.0")
		err = runSimulate(nil, nil)
	case "cycle":
		cycleDatetime, cycleFile, cycleCount = datetimeStr, outFile, count
		cycleKML = askString("Input KML file path:", "route.kml")
		cycleSpeed = askFloat("Average speed (km/h):", "25.0")
		err = cycleSimulate(nil, nil)
	case "walk":
		walkDatetime, walkFile, walkCount = datetimeStr, outFile, count
		walkKML = askString("Input KML file path:", "route.kml")
		walkSpeed = askFloat("Average speed (km/h):", "5.0")
		err = walkSimulate(nil, nil)
	case "hike":
		hikeDatetime, hikeFile, hikeCount = datetimeStr, outFile, count
		hikeKML = askString("Input KML file path:", "route.kml")
		hikeSpeed = askFloat("Average speed (km/h):", "4.5")
		err = hikeSimulate(nil, nil)
	case "ebike":
		ebikeDatetime, ebikeFile, ebikeCount = datetimeStr, outFile, count
		ebikeKML = askString("Input KML file path:", "route.kml")
		ebikeSpeed = askFloat("Average speed (km/h):", "25.0")
		err = ebikeSimulate(nil, nil)
	case "ski":
		skiDatetime, skiFile, skiCount = datetimeStr, outFile, count
		skiKML = askString("Input KML file path:", "route.kml")
		err = survey.AskOne(&survey.Select{Message: "Skiing type:", Options: []string{"alpine", "cross-country"}, Default: "alpine"}, &skiType)
		if err != nil { return err }
		err = skiSimulate(nil, nil)
	case "cardio":
		cardioDatetime, cardioFile, cardioCount = datetimeStr, outFile, count
		cardioDuration = askInt("Duration in seconds:", "1800")
		err = cardioSimulate(nil, nil)
	case "yoga":
		yogaDatetime, yogaFile, yogaCount = datetimeStr, outFile, count
		yogaDuration = askInt("Duration in seconds:", "1800")
		err = yogaSimulate(nil, nil)
	case "meditation":
		meditationDatetime, meditationFile, meditationCount = datetimeStr, outFile, count
		meditationDuration = askInt("Duration in seconds:", "600")
		err = meditationSimulate(nil, nil)
	case "strength":
		strengthDatetime, strengthFile, strengthCount = datetimeStr, outFile, count
		strengthSets = askInt("Number of sets:", "5")
		strengthReps = askInt("Number of reps per set:", "10")
		err = strengthSimulate(nil, nil)
	case "swim":
		swimDatetime, swimFile, swimCount = datetimeStr, outFile, count
		swimDistance = askFloat("Total distance (km):", "1.5")
		swimSpeed = askFloat("Average speed (km/h):", "3.0")
		err = swimSimulate(nil, nil)
	case "row":
		rowDatetime, rowFile, rowCount = datetimeStr, outFile, count
		rowDuration = askInt("Duration in seconds (if indoor):", "1800")
		rowKML = askString("Optional KML file path (leave blank for indoor):", "")
		err = rowSimulate(nil, nil)
	case "field":
		fieldDatetime, fieldFile, fieldCount = datetimeStr, outFile, count
		fieldDuration = askInt("Duration in seconds:", "3600")
		err = survey.AskOne(&survey.Select{Message: "Sport type:", Options: []string{"soccer", "basketball", "tennis"}, Default: "soccer"}, &fieldSport)
		if err != nil { return err }
		err = fieldSimulate(nil, nil)
	}

	if err != nil {
		fmt.Printf("Simulation failed: %v\n", err)
	}
	return err
}

func askString(msg string, def string) string {
	var res string
	survey.AskOne(&survey.Input{Message: msg, Default: def}, &res)
	return res
}

func askFloat(msg string, def string) float64 {
	var res string
	survey.AskOne(&survey.Input{Message: msg, Default: def}, &res)
	f, _ := strconv.ParseFloat(res, 64)
	return f
}

func askInt(msg string, def string) int {
	var res string
	survey.AskOne(&survey.Input{Message: msg, Default: def}, &res)
	i, _ := strconv.Atoi(res)
	return i
}
