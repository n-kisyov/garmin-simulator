package cmd

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tormoder/fit"
)

// generateGolf runs the golf command over the test course and decodes what it
// wrote. The course KML carries elevations, so no test ever reaches for the
// elevation API.
func generateGolf(t *testing.T, holes, par, score int) *fit.ActivityFile {
	t.Helper()

	out := filepath.Join(t.TempDir(), "golf.fit")
	golfDatetime = "01-09-26 08:30:00"
	golfKML = filepath.Join("testdata", "course.kml")
	golfHoles, golfPar, golfScore = holes, par, score
	golfSpeed = 4.5
	golfFile = out
	golfCount = 1

	if err := golfSimulate(nil, nil); err != nil {
		t.Fatalf("golfSimulate: %v", err)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	decoded, err := fit.Decode(f)
	if err != nil {
		t.Fatalf("the generated file does not decode as FIT: %v", err)
	}
	if decoded.FileId.Type != fit.FileTypeActivity {
		t.Errorf("file_id.type = %v, want activity", decoded.FileId.Type)
	}
	activity, err := decoded.Activity()
	if err != nil {
		t.Fatalf("not an activity file: %v", err)
	}
	return activity
}

// TestGolfFileHasEveryRequiredMessage pins the message set the FIT spec calls
// required for an Activity file, plus the device info and timer events Garmin
// documents as best practice.
func TestGolfFileHasEveryRequiredMessage(t *testing.T) {
	activity := generateGolf(t, 18, 0, 0)

	if activity.Activity == nil {
		t.Fatal("no activity message")
	}
	if activity.Activity.NumSessions != 1 {
		t.Errorf("activity.num_sessions = %d, want 1", activity.Activity.NumSessions)
	}
	if activity.Activity.LocalTimestamp.IsZero() {
		t.Error("activity.local_timestamp is unset; a reader cannot recover the time zone")
	}
	if len(activity.Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(activity.Sessions))
	}
	if len(activity.Laps) != 18 {
		t.Errorf("got %d laps, want one per hole (18)", len(activity.Laps))
	}
	if len(activity.Records) == 0 {
		t.Error("no record messages: the file carries no GPS track")
	}
	if len(activity.DeviceInfos) == 0 {
		t.Error("no device_info message")
	}

	var start, stop int
	for _, e := range activity.Events {
		if e.Event != fit.EventTimer {
			continue
		}
		switch e.EventType {
		case fit.EventTypeStart:
			start++
		case fit.EventTypeStop:
			stop++
		}
	}
	if start != 1 || stop != 1 {
		t.Errorf("got %d timer starts and %d stops, want one of each", start, stop)
	}
}

// TestGolfRecordsCarryATrack checks the part of the file a map is drawn from:
// every record needs a timestamp and something else, and for golf that something
// is a position on the course.
func TestGolfRecordsCarryATrack(t *testing.T) {
	activity := generateGolf(t, 18, 0, 0)

	prev := time.Time{}
	var moved int
	for i, r := range activity.Records {
		if r.Timestamp.IsZero() {
			t.Fatalf("record %d has no timestamp", i)
		}
		if i > 0 && !r.Timestamp.After(prev) {
			t.Fatalf("record %d is not after record %d (%v vs %v)", i, i-1, r.Timestamp, prev)
		}
		prev = r.Timestamp

		lat, lon := r.PositionLat.Degrees(), r.PositionLong.Degrees()
		if math.IsNaN(lat) || math.IsNaN(lon) {
			t.Fatalf("record %d has no position", i)
		}
		if r.HeartRate == 0xFF {
			t.Fatalf("record %d has no heart rate", i)
		}
		if r.Speed != 0xFFFF && r.Speed > 0 {
			moved++
		}
	}

	// A round is mostly standing still, but not entirely.
	if moved == 0 {
		t.Error("no record shows the player moving")
	}
	if moved == len(activity.Records) {
		t.Error("every record shows the player moving; nobody stands over a shot")
	}
}

// TestGolfLapsAreTheScorecard covers what makes the file a round of golf rather
// than a walk: sequential, non-overlapping laps that sum to the session, each
// carrying the hole's score against its par.
func TestGolfLapsAreTheScorecard(t *testing.T) {
	const (
		holes = 18
		par   = 71
		score = 84
	)
	activity := generateGolf(t, holes, par, score)
	session := activity.Sessions[0]

	if session.Sport != fit.SportGolf {
		t.Errorf("session.sport = %v, want golf", session.Sport)
	}
	if session.NumLaps != holes {
		t.Errorf("session.num_laps = %d, want %d", session.NumLaps, holes)
	}
	if int(session.PlayerScore) != score {
		t.Errorf("session.player_score = %d, want the round's %d strokes", session.PlayerScore, score)
	}
	if int(session.OpponentScore) != par {
		t.Errorf("session.opponent_score = %d, want the course's par of %d", session.OpponentScore, par)
	}

	var (
		strokes, pars     int
		elapsed, distance uint32
	)
	for i, lap := range activity.Laps {
		if int(lap.MessageIndex) != i {
			t.Errorf("lap %d has message_index %d", i, lap.MessageIndex)
		}
		if lap.Sport != fit.SportGolf {
			t.Errorf("lap %d sport = %v, want golf", i, lap.Sport)
		}
		// Every FIT summary message needs all four of these.
		if lap.StartTime.IsZero() || lap.Timestamp.IsZero() ||
			lap.TotalElapsedTime == 0xFFFFFFFF || lap.TotalTimerTime == 0xFFFFFFFF {
			t.Errorf("lap %d is missing one of start_time, timestamp, total_elapsed_time, total_timer_time", i)
		}
		if i > 0 && lap.StartTime.Before(activity.Laps[i-1].Timestamp) {
			t.Errorf("lap %d starts before lap %d ended", i, i-1)
		}
		if lap.PlayerScore == 0 || lap.PlayerScore == 0xFFFF {
			t.Errorf("lap %d has no score", i)
		}

		strokes += int(lap.PlayerScore)
		pars += int(lap.OpponentScore)
		elapsed += lap.TotalElapsedTime
		distance += lap.TotalDistance
	}

	if strokes != score {
		t.Errorf("the laps add up to %d strokes, want %d", strokes, score)
	}
	if pars != par {
		t.Errorf("the laps add up to a par of %d, want %d", pars, par)
	}
	if elapsed != session.TotalElapsedTime {
		t.Errorf("laps total %d ms elapsed, session says %d", elapsed, session.TotalElapsedTime)
	}
	// The laps split the route between them, so their distances should land on
	// the session's; a centimetre either way is rounding.
	if diff := int64(distance) - int64(session.TotalDistance); diff > 100 || diff < -100 {
		t.Errorf("laps total %d cm, session says %d cm", distance, session.TotalDistance)
	}
}

// TestGolfSessionSpansTheRound checks the summary values a viewer shows next to
// the map: the round has to start when the first record does and end with the
// last, and cover the whole route.
func TestGolfSessionSpansTheRound(t *testing.T) {
	activity := generateGolf(t, 9, 0, 0)
	session := activity.Sessions[0]
	records := activity.Records

	if !session.StartTime.Equal(records[0].Timestamp) {
		t.Errorf("session starts at %v, first record is %v", session.StartTime, records[0].Timestamp)
	}
	last := records[len(records)-1].Timestamp
	if session.Timestamp.Before(last) {
		t.Errorf("session ends at %v, before the last record at %v", session.Timestamp, last)
	}
	if session.TotalDistance == 0 || session.TotalDistance == 0xFFFFFFFF {
		t.Error("session has no distance")
	}
	if session.TotalCalories == 0xFFFF {
		t.Error("session has no calories")
	}
	if session.TotalAscent == 0xFFFF || session.TotalDescent == 0xFFFF {
		t.Error("session has no climb; the course KML carries elevations")
	}
	if math.IsNaN(session.NecLat.Degrees()) || math.IsNaN(session.SwcLat.Degrees()) {
		t.Error("session has no bounding box")
	}
	if session.NecLat.Degrees() <= session.SwcLat.Degrees() {
		t.Error("the bounding box corners are the wrong way round")
	}
}

// TestGolfRejectsAnImpossibleCard keeps the flag validation honest, since a
// nonsense card would otherwise be quietly rounded into a plausible one.
func TestGolfRejectsAnImpossibleCard(t *testing.T) {
	cases := []struct {
		name              string
		holes, par, score int
		speed             float64
	}{
		{name: "no holes", holes: 0},
		{name: "par below three a hole", holes: 18, par: 40},
		{name: "par above five a hole", holes: 18, par: 100},
		{name: "fewer strokes than holes", holes: 18, score: 12},
		{name: "score off the card", holes: 18, score: 500},
		{name: "standing still", holes: 18, speed: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			golfDatetime = "01-09-26 08:30:00"
			golfKML = filepath.Join("testdata", "course.kml")
			golfHoles, golfPar, golfScore = tc.holes, tc.par, tc.score
			golfSpeed = 4.5
			if tc.speed != 0 {
				golfSpeed = tc.speed
			}
			golfFile = filepath.Join(t.TempDir(), "golf.fit")
			golfCount = 1

			if err := golfSimulate(nil, nil); err == nil {
				t.Error("the card was accepted; want an error")
			}
		})
	}
}
