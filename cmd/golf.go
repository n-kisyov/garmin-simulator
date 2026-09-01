package cmd

import (
	"fmt"
	"math"
	"time"

	"fitsim/pkg/fitgen"
	"fitsim/pkg/geo"
	"fitsim/pkg/golf"
	"fitsim/pkg/simulator"

	"github.com/spf13/cobra"
	"github.com/tormoder/fit"
)

var golfCmd = &cobra.Command{
	Use:   "golf",
	Short: "Simulate a round of golf FIT file using a KML route.",
	Long: `Simulate a round of golf as a Garmin FIT activity file.

The KML route is the path walked during the round, tee to final green. It is
divided between the holes in proportion to how long a hole of each par plays, and
the round is then walked a second at a time: the player stands still over every
shot, walks on to the next one, and pauses again on the green.

Each hole becomes a FIT lap carrying its own distance, duration, heart rate and
position, with the hole's score in player_score and its par in opponent_score.
The session sums the card and reports the round as sport "golf".`,
	RunE: golfSimulate,
}

var (
	golfDatetime string
	golfKML      string
	golfHoles    int
	golfPar      int
	golfScore    int
	golfSpeed    float64
	golfFile     string
	golfCount    int
)

func init() {
	rootCmd.AddCommand(golfCmd)
	golfCmd.Flags().StringVar(&golfDatetime, "datetime", "", "Start datetime in format 'dd-mm-yy HH:MM:SS'")
	golfCmd.Flags().StringVar(&golfKML, "kml", "", "Input KML file containing the route walked during the round")
	golfCmd.Flags().IntVar(&golfHoles, "holes", 18, "Number of holes played")
	golfCmd.Flags().IntVar(&golfPar, "par", 0, "Par for the course; 0 uses the standard layout (72 for 18 holes)")
	golfCmd.Flags().IntVar(&golfScore, "score", 0, "Total strokes for the round; 0 plays it to a mid handicap")
	golfCmd.Flags().Float64Var(&golfSpeed, "speed", 4.5, "Walking speed between shots in km/h")
	golfCmd.Flags().StringVar(&golfFile, "file", "", "Output FIT filename")
	addCountFlag(golfCmd, &golfCount)
	golfCmd.MarkFlagRequired("datetime")
	golfCmd.MarkFlagRequired("kml")
	golfCmd.MarkFlagRequired("file")
}

// Golf is walked, and a Garmin watch on the wrist counts the walking, not the
// swings. Cadence and cycle counts are therefore in strides, which is the unit
// the FIT profile uses for foot sports and which Garmin Connect doubles when it
// shows steps.
const (
	golfStridesPerMin = 52.0 // ~104 steps/min, an unhurried walk between shots
	golfKcalWalking   = 5.5 / 60.0
	golfKcalStanding  = 1.5 / 60.0
)

// How long the player stands still, in seconds. Standing is most of a round: a
// tee or approach shot takes a look at the yardage, a practice swing and a club
// change; a putt is quicker; holing out and walking off the green costs a little
// more on top; and every tee has the group ahead still to clear it. Together with
// the walking these put an eighteen at three to four hours, which is what a round
// on foot actually takes.
var (
	golfTeeWait    = [2]float64{15, 150}
	golfShotPause  = [2]float64{35, 70}
	golfPuttPause  = [2]float64{18, 34}
	golfGreenPause = [2]float64{30, 80}
)

func golfSimulate(cmd *cobra.Command, args []string) error {
	if golfSpeed <= 0 {
		return fmt.Errorf("--speed must be greater than 0, got %v", golfSpeed)
	}

	points, err := geo.ParseKMLCoordinates(golfKML)
	if err != nil {
		return err
	}
	if !geo.HasElevation(points) {
		fmt.Println("Fetching elevation data from Open-Elevation API...")
		points = geo.FetchElevation(points, 100)
	}

	dists := geo.Cumulative(points)
	routeLen := dists[len(dists)-1]

	// The card is drawn once and every file in a --count series plays the same
	// round, matching how the series only ever varies the start time.
	round, err := golf.NewRound(golfHoles, golfPar, golfScore, routeLen)
	if err != nil {
		return err
	}

	walkMs := golfSpeed * 1000.0 / 3600.0
	necLat, necLon, swcLat, swcLon := geo.Bounds(points)

	return generateSeries(golfDatetime, golfFile, golfCount, func(startTime time.Time, outFile string) error {
		builder := fitgen.NewBuilder(startTime, 4400, 345000124)
		builder.AddDeviceInfo(startTime, 4400, 345000124, 14.50)
		builder.AddEvent(startTime, fit.EventTimer, fit.EventTypeStart)

		g := &golfWalk{
			builder: builder,
			points:  points,
			dists:   dists,
			now:     startTime,
			walkMs:  walkMs,
			hr:      simulator.RandomFloat(78, 88),
			temp:    simulator.RandomFloat(15, 23),
		}

		var total golfStats
		for i := range round.Holes {
			hole := round.Holes[i]
			holeStart := g.now
			from := geo.Interpolate(points, dists, hole.Start)

			var played golfStats
			g.playHole(hole, &played)

			to := geo.Interpolate(points, dists, hole.End)
			lap := fit.NewLapMsg()
			lap.MessageIndex = fit.MessageIndex(i)
			lap.Timestamp = g.now
			lap.StartTime = holeStart
			lap.Event = fit.EventLap
			lap.EventType = fit.EventTypeStop
			// A hole ends where the player decides it does, not on a timer or a
			// distance, which is what "position marked" means in the FIT profile.
			lap.LapTrigger = fit.LapTriggerPositionMarked
			lap.Sport = fit.SportGolf
			lap.SubSport = fit.SubSportGeneric
			lap.StartPositionLat = fit.NewLatitudeDegrees(from.Lat)
			lap.StartPositionLong = fit.NewLongitudeDegrees(from.Lon)
			lap.EndPositionLat = fit.NewLatitudeDegrees(to.Lat)
			lap.EndPositionLong = fit.NewLongitudeDegrees(to.Lon)
			lap.PlayerScore = uint16(hole.Strokes)
			lap.OpponentScore = uint16(hole.Par)
			played.fillLap(lap)
			builder.AddLap(lap)

			total.merge(&played)
		}

		builder.AddEvent(g.now, fit.EventTimer, fit.EventTypeStop)

		start := geo.Interpolate(points, dists, 0)
		finish := geo.Interpolate(points, dists, routeLen)

		session := fit.NewSessionMsg()
		session.MessageIndex = 0
		session.Timestamp = g.now
		session.StartTime = startTime
		session.Event = fit.EventSession
		session.EventType = fit.EventTypeStop
		session.Trigger = fit.SessionTriggerActivityEnd
		session.Sport = fit.SportGolf
		session.SubSport = fit.SubSportGeneric
		session.SportProfileName = "Golf"
		session.FirstLapIndex = 0
		session.NumLaps = uint16(len(round.Holes))
		session.StartPositionLat = fit.NewLatitudeDegrees(start.Lat)
		session.StartPositionLong = fit.NewLongitudeDegrees(start.Lon)
		session.EndPositionLat = fit.NewLatitudeDegrees(finish.Lat)
		session.EndPositionLong = fit.NewLongitudeDegrees(finish.Lon)
		session.NecLat = fit.NewLatitudeDegrees(necLat)
		session.NecLong = fit.NewLongitudeDegrees(necLon)
		session.SwcLat = fit.NewLatitudeDegrees(swcLat)
		session.SwcLong = fit.NewLongitudeDegrees(swcLon)
		session.AvgLapTime = uint32(float64(total.seconds) / float64(len(round.Holes)) * 1000)
		session.PlayerScore = uint16(round.Strokes)
		session.OpponentScore = uint16(round.Par)
		total.fillSession(session)
		builder.AddSession(session)

		builder.AddActivity(g.now, uint32(total.seconds*1000), 1, fit.EventActivity, fit.EventTypeStop)
		// local_timestamp is what a reader subtracts from the UTC timestamp to
		// recover the time zone. fitsim reads --datetime as UTC throughout, so the
		// two are the same instant and the round shows at the hour that was asked
		// for rather than shifted into the reader's own zone.
		builder.SetLocalTimestamp(g.now)

		if err := builder.WriteToFile(outFile); err != nil {
			return fmt.Errorf("error writing FIT file: %v", err)
		}
		fmt.Printf("Generated golf FIT: %s — %s, %.2f km in %s\n",
			outFile, round, total.distance/1000, formatGolfDuration(total.seconds))
		return nil
	})
}

// golfWalk carries the state that runs the length of a round: where the player
// is on the route, what time it is, and the values that drift rather than being
// recomputed from scratch each second.
type golfWalk struct {
	builder *fitgen.Builder
	points  []geo.Point
	dists   []float64

	now    time.Time
	dist   float64 // metres of route covered so far
	walkMs float64

	hr      float64
	temp    float64
	prevAlt float64
	hasPrev bool
}

// golfWalkOff is the share of a hole's route that is the walk off the green to
// the next tee rather than ground the ball is played over.
const golfWalkOff = 0.12

// playHole walks one hole: a wait on the tee, then a pause over each stroke with
// a walk on to where the next one is played from, and finally the pause for
// holing out before the walk to the next tee.
func (g *golfWalk) playHole(hole golf.Hole, st *golfStats) {
	g.stand(golfTeeWait, st)

	// The ball works its way from the tee to the green over the hole's strokes;
	// what is left of the hole's route afterwards is the walk to the next tee.
	green := hole.End - (hole.End-hole.Start)*golfWalkOff

	// The last stroke is holed and the one before it is played on the green too,
	// unless the hole was short enough that there was no second putt.
	putts := 1
	if hole.Strokes >= 3 {
		putts = 2
	}

	for k := 0; k < hole.Strokes; k++ {
		pause := golfShotPause
		if k >= hole.Strokes-putts {
			pause = golfPuttPause
		}
		g.stand(pause, st)

		if k < hole.Strokes-1 {
			g.walk(hole.Start+(green-hole.Start)*float64(k+1)/float64(hole.Strokes-1), st)
		}
	}

	g.stand(golfGreenPause, st)
	g.walk(hole.End, st)
}

// stand holds position for a randomised number of seconds drawn from the given
// range, recording all the while: the timer keeps running over a golf shot.
func (g *golfWalk) stand(pause [2]float64, st *golfStats) {
	for secs := int(simulator.RandomFloat(pause[0], pause[1])); secs > 0; secs-- {
		g.sample(false, 0, st)
	}
}

// walk advances along the route to the given distance at walking pace, one
// record a second.
func (g *golfWalk) walk(to float64, st *golfStats) {
	remaining := to - g.dist
	if remaining <= 0 {
		return
	}
	secs := int(math.Round(remaining / g.walkMs))
	if secs < 1 {
		secs = 1
	}
	step := remaining / float64(secs)
	for i := 0; i < secs; i++ {
		g.dist += step
		g.sample(true, step, st)
	}
	g.dist = to
}

// sample writes one record and folds it into the running totals. moving says
// whether the second was spent walking, and advanced is how far it covered.
func (g *golfWalk) sample(moving bool, advanced float64, st *golfStats) {
	p := geo.Interpolate(g.points, g.dists, g.dist)

	grade := 0.0
	if g.hasPrev {
		climb := p.Ele - g.prevAlt
		if climb > 0 {
			st.ascent += climb
		} else {
			st.descent -= climb
		}
		if advanced > 0 {
			grade = climb / advanced
		}
	}
	g.prevAlt, g.hasPrev = p.Ele, true

	// Heart rate chases a target rather than jumping to it, so the trace settles
	// over the half minute a player spends standing over the ball.
	target := 84.0
	speed := 0.0
	cadence := 0.0
	if moving {
		speed = advanced
		cadence = golfStridesPerMin + simulator.RandomFloat(-3, 3)
		target = 104.0 + math.Max(-0.15, math.Min(0.15, grade))*160.0
		st.calories += golfKcalWalking
		st.steps += cadence / 60.0
	} else {
		st.calories += golfKcalStanding
	}
	g.hr += (target-g.hr)*0.06 + simulator.RandomFloat(-1.2, 1.2)
	g.hr = math.Max(58, math.Min(178, g.hr))

	g.temp += simulator.RandomFloat(-0.03, 0.03)
	g.temp = math.Max(-20, math.Min(45, g.temp))

	record := fit.NewRecordMsg()
	record.Timestamp = g.now
	record.PositionLat = fit.NewLatitudeDegrees(p.Lat)
	record.PositionLong = fit.NewLongitudeDegrees(p.Lon)
	record.Distance = uint32(g.dist * 100)
	record.Altitude = uint16((p.Ele + 500) * 5)
	record.Speed = uint16(speed * 1000)
	record.HeartRate = uint8(g.hr)
	record.Cadence = uint8(cadence)
	record.Temperature = int8(math.Round(g.temp))
	g.builder.AddRecord(record)

	st.add(advanced, speed, g.hr, cadence, p.Ele)
	g.now = g.now.Add(time.Second)
}

// golfStats accumulates what a FIT summary message needs out of a stretch of
// records. One is filled per hole and then merged into the round's.
type golfStats struct {
	seconds  int
	distance float64
	steps    float64
	calories float64
	ascent   float64
	descent  float64

	hrSum float64
	hrMin float64
	hrMax float64

	cadSum float64
	cadN   int
	cadMax float64

	speedMax float64

	altSum float64
	altMin float64
	altMax float64
}

func (s *golfStats) add(advanced, speed, hr, cadence, alt float64) {
	if s.seconds == 0 {
		s.hrMin, s.hrMax = hr, hr
		s.altMin, s.altMax = alt, alt
	}
	s.seconds++
	s.distance += advanced
	s.hrSum += hr
	s.hrMin = math.Min(s.hrMin, hr)
	s.hrMax = math.Max(s.hrMax, hr)
	s.speedMax = math.Max(s.speedMax, speed)
	s.altSum += alt
	s.altMin = math.Min(s.altMin, alt)
	s.altMax = math.Max(s.altMax, alt)
	if cadence > 0 {
		// Averaged over the strides actually taken; a player stood over a putt has
		// no cadence, and folding those seconds in would report a limp.
		s.cadSum += cadence
		s.cadN++
		s.cadMax = math.Max(s.cadMax, cadence)
	}
}

func (s *golfStats) merge(o *golfStats) {
	if s.seconds == 0 {
		*s = *o
		return
	}
	if o.seconds == 0 {
		return
	}
	s.seconds += o.seconds
	s.distance += o.distance
	s.steps += o.steps
	s.calories += o.calories
	s.ascent += o.ascent
	s.descent += o.descent
	s.hrSum += o.hrSum
	s.hrMin = math.Min(s.hrMin, o.hrMin)
	s.hrMax = math.Max(s.hrMax, o.hrMax)
	s.cadSum += o.cadSum
	s.cadN += o.cadN
	s.cadMax = math.Max(s.cadMax, o.cadMax)
	s.speedMax = math.Max(s.speedMax, o.speedMax)
	s.altSum += o.altSum
	s.altMin = math.Min(s.altMin, o.altMin)
	s.altMax = math.Max(s.altMax, o.altMax)
}

func (s *golfStats) avgHR() float64 {
	if s.seconds == 0 {
		return 0
	}
	return s.hrSum / float64(s.seconds)
}

func (s *golfStats) avgCadence() float64 {
	if s.cadN == 0 {
		return 0
	}
	return s.cadSum / float64(s.cadN)
}

func (s *golfStats) avgSpeed() float64 {
	if s.seconds == 0 {
		return 0
	}
	return s.distance / float64(s.seconds)
}

func (s *golfStats) avgAltitude() float64 {
	if s.seconds == 0 {
		return 0
	}
	return s.altSum / float64(s.seconds)
}

// fitAltitude encodes metres the way the FIT profile stores altitude: five
// counts per metre, offset 500 so that anywhere below sea level still fits.
func fitAltitude(m float64) uint16 { return uint16((m + 500) * 5) }

func (s *golfStats) fillLap(lap *fit.LapMsg) {
	lap.TotalElapsedTime = uint32(s.seconds * 1000)
	lap.TotalTimerTime = uint32(s.seconds * 1000)
	lap.TotalMovingTime = uint32(s.movingSeconds() * 1000)
	lap.TotalDistance = uint32(s.distance * 100)
	lap.TotalCycles = uint32(s.steps)
	lap.TotalCalories = uint16(s.calories)
	lap.AvgSpeed = uint16(s.avgSpeed() * 1000)
	lap.MaxSpeed = uint16(s.speedMax * 1000)
	lap.AvgHeartRate = uint8(s.avgHR())
	lap.MaxHeartRate = uint8(s.hrMax)
	lap.MinHeartRate = uint8(s.hrMin)
	lap.AvgCadence = uint8(s.avgCadence())
	lap.MaxCadence = uint8(s.cadMax)
	lap.TotalAscent = uint16(s.ascent)
	lap.TotalDescent = uint16(s.descent)
	lap.AvgAltitude = fitAltitude(s.avgAltitude())
	lap.MaxAltitude = fitAltitude(s.altMax)
	lap.MinAltitude = fitAltitude(s.altMin)
}

func (s *golfStats) fillSession(session *fit.SessionMsg) {
	session.TotalElapsedTime = uint32(s.seconds * 1000)
	session.TotalTimerTime = uint32(s.seconds * 1000)
	session.TotalMovingTime = uint32(s.movingSeconds() * 1000)
	session.TotalDistance = uint32(s.distance * 100)
	session.TotalCycles = uint32(s.steps)
	session.TotalCalories = uint16(s.calories)
	session.AvgSpeed = uint16(s.avgSpeed() * 1000)
	session.MaxSpeed = uint16(s.speedMax * 1000)
	session.AvgHeartRate = uint8(s.avgHR())
	session.MaxHeartRate = uint8(s.hrMax)
	session.MinHeartRate = uint8(s.hrMin)
	session.AvgCadence = uint8(s.avgCadence())
	session.MaxCadence = uint8(s.cadMax)
	session.TotalAscent = uint16(s.ascent)
	session.TotalDescent = uint16(s.descent)
	session.AvgAltitude = fitAltitude(s.avgAltitude())
	session.MaxAltitude = fitAltitude(s.altMax)
	session.MinAltitude = fitAltitude(s.altMin)
}

// movingSeconds is how much of the round was spent walking rather than standing
// over a shot. Every walking second covers one stride's worth of cadence, so the
// count of cadence samples is exactly it.
func (s *golfStats) movingSeconds() int { return s.cadN }

// formatGolfDuration renders a round length as h:mm:ss, which is how long enough
// a walk deserves to be read.
func formatGolfDuration(seconds int) string {
	return fmt.Sprintf("%d:%02d:%02d", seconds/3600, (seconds/60)%60, seconds%60)
}
