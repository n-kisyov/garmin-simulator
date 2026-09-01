package golf

import (
	"math"
	"testing"

	"fitsim/pkg/simulator"
)

// newRound seeds the shared random stream first, so that a failure names a card
// that can be reproduced rather than one that only appeared once.
func newRound(t *testing.T, seed int64, holes, par, strokes int, routeLength float64) *Round {
	t.Helper()
	simulator.Reseed(seed)
	r, err := NewRound(holes, par, strokes, routeLength)
	if err != nil {
		t.Fatalf("NewRound(%d, %d, %d): %v", holes, par, strokes, err)
	}
	return r
}

func TestDefaultsAreAnOrdinaryCourse(t *testing.T) {
	if got := DefaultPar(18); got != 72 {
		t.Errorf("DefaultPar(18) = %d, want 72", got)
	}
	if got := DefaultPar(9); got != 36 {
		t.Errorf("DefaultPar(9) = %d, want 36", got)
	}
	// A mid handicap, which is who the simulator plays as unless told otherwise.
	if got := DefaultStrokes(18, 72); got < 80 || got > 92 {
		t.Errorf("DefaultStrokes(18, 72) = %d, want a mid-handicap round", got)
	}
}

// TestCardAddsUp is the property that matters: however the strokes and pars fall
// across the holes, they have to total what was asked for, or the scorecard in
// the FIT file disagrees with its own session summary.
func TestCardAddsUp(t *testing.T) {
	cases := []struct{ holes, par, strokes int }{
		{18, 0, 0},    // defaults
		{9, 0, 0},     //
		{18, 72, 72},  // level par
		{18, 72, 61},  // eleven under, which pushes every hole to its floor
		{18, 72, 126}, // seven a hole, which pushes every hole to its cap
		{18, 54, 54},  // all par 3s
		{18, 90, 95},  // all par 5s
		{27, 0, 0},    // past one time round the standard layout
		{1, 3, 9},     // a single hole, badly played
		{45, 0, 0},    // the longest round allowed
	}

	for _, tc := range cases {
		for seed := int64(1); seed <= 25; seed++ {
			r := newRound(t, seed, tc.holes, tc.par, tc.strokes, 6000)

			var par, strokes int
			for _, h := range r.Holes {
				if h.Par < MinHolePar || h.Par > MaxHolePar {
					t.Fatalf("holes=%d par=%d score=%d seed=%d: hole %d is a par %d",
						tc.holes, tc.par, tc.strokes, seed, h.Number, h.Par)
				}
				if h.Strokes < 1 {
					t.Fatalf("holes=%d par=%d score=%d seed=%d: hole %d took %d strokes",
						tc.holes, tc.par, tc.strokes, seed, h.Number, h.Strokes)
				}
				par += h.Par
				strokes += h.Strokes
			}

			if par != r.Par {
				t.Fatalf("holes=%d par=%d score=%d seed=%d: holes total par %d, round says %d",
					tc.holes, tc.par, tc.strokes, seed, par, r.Par)
			}
			if strokes != r.Strokes {
				t.Fatalf("holes=%d par=%d score=%d seed=%d: holes total %d strokes, round says %d",
					tc.holes, tc.par, tc.strokes, seed, strokes, r.Strokes)
			}
			if tc.par > 0 && r.Par != tc.par {
				t.Fatalf("asked for par %d, got %d", tc.par, r.Par)
			}
			if tc.strokes > 0 && r.Strokes != tc.strokes {
				t.Fatalf("asked for %d strokes, got %d", tc.strokes, r.Strokes)
			}
		}
	}
}

// TestHolesCoverTheRouteInOrder pins the layout: the holes tile the route from
// end to end without gaps or overlaps, since each one becomes a FIT lap whose
// distances have to sum to the session's.
func TestHolesCoverTheRouteInOrder(t *testing.T) {
	const routeLength = 7400.0
	r := newRound(t, 7, 18, 0, 0, routeLength)

	if r.Holes[0].Start != 0 {
		t.Errorf("the first hole starts %v m along the route, want 0", r.Holes[0].Start)
	}
	if last := r.Holes[len(r.Holes)-1].End; last != routeLength {
		t.Errorf("the last hole ends at %v m, want the end of the route at %v", last, routeLength)
	}

	for i, h := range r.Holes {
		if h.End <= h.Start {
			t.Errorf("hole %d covers no ground (%v to %v)", h.Number, h.Start, h.End)
		}
		if i > 0 && h.Start != r.Holes[i-1].End {
			t.Errorf("hole %d starts at %v but hole %d ended at %v",
				h.Number, h.Start, r.Holes[i-1].Number, r.Holes[i-1].End)
		}
	}
}

// TestLongerHolesGetMoreRoute checks the point of weighting the layout by par: a
// par 5 has to be a longer walk than a par 3 on the same course.
func TestLongerHolesGetMoreRoute(t *testing.T) {
	r := newRound(t, 3, 18, 72, 0, 6800)

	shortest := map[int]float64{}
	longest := map[int]float64{}
	for _, h := range r.Holes {
		span := h.End - h.Start
		if v, ok := shortest[h.Par]; !ok || span < v {
			shortest[h.Par] = span
		}
		if v, ok := longest[h.Par]; !ok || span > v {
			longest[h.Par] = span
		}
	}

	for _, par := range []int{3, 4} {
		if longest[par] >= shortest[par+1] {
			t.Errorf("a par %d walks %.0f m but a par %d only %.0f m",
				par, longest[par], par+1, shortest[par+1])
		}
	}
}

func TestRoundRejectsWhatCannotBePlayed(t *testing.T) {
	cases := []struct {
		name              string
		holes, par, score int
		routeLength       float64
	}{
		{name: "no holes", holes: 0, routeLength: 6000},
		{name: "more holes than a day allows", holes: MaxHoles + 1, routeLength: 6000},
		{name: "no route", holes: 18, routeLength: 0},
		{name: "par under three a hole", holes: 18, par: 53, routeLength: 6000},
		{name: "par over five a hole", holes: 18, par: 91, routeLength: 6000},
		{name: "fewer strokes than holes", holes: 18, score: 17, routeLength: 6000},
		{name: "more strokes than the card holds", holes: 18, score: 181, routeLength: 6000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewRound(tc.holes, tc.par, tc.score, tc.routeLength); err == nil {
				t.Error("accepted; want an error")
			}
		})
	}
}

func TestRoundReadsBackAsAScore(t *testing.T) {
	r := newRound(t, 11, 18, 72, 85, 6000)
	if got, want := r.String(), "18 holes, par 72, 85 strokes (+13)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	level := newRound(t, 11, 18, 72, 72, 6000)
	if got, want := level.String(), "18 holes, par 72, 72 strokes (level)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if level.Over() != 0 {
		t.Errorf("Over() = %d, want 0", level.Over())
	}
}

// TestScoresStaySpreadOut guards the rule that keeps a card believable: a bad
// round is a lot of bogeys, not one hole taking twenty shots.
func TestScoresStaySpreadOut(t *testing.T) {
	r := newRound(t, 5, 18, 72, 108, 6000) // two over a hole, on average

	worst := 0
	for _, h := range r.Holes {
		worst = int(math.Max(float64(worst), float64(h.Over())))
	}
	if worst > maxOverPar {
		t.Errorf("the worst hole ran to %d over par, want no more than %d", worst, maxOverPar)
	}
}
