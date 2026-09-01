// Package golf models a round well enough to write a believable FIT activity for
// it: what each hole was worth, what it cost, and which stretch of the walked
// route it covered.
//
// The scorecard has nowhere official to go. The FIT profile defines no par or
// stroke field for golf — Garmin's own watches write the card into private,
// undocumented messages (global numbers 191-193) that the published SDK cannot
// express. What the profile does give is one lap per hole plus player_score and
// opponent_score, so fitsim puts the hole's score in the first and its par in
// the second, and totals the same pair on the session.
package golf

import (
	"fmt"

	"fitsim/pkg/simulator"
)

// A hole is worth no less than a par 3 and no more than a par 5; anything
// outside that is a driving range, not a golf course.
const (
	MinHolePar = 3
	MaxHolePar = 5
)

// MaxHoles is the longest round fitsim will lay out. Two eighteens and a nine is
// already more golf than anyone plays in a day.
const MaxHoles = 45

// maxOverPar caps how badly a single hole may go before the extra strokes start
// landing on its neighbours, so one hole cannot swallow a whole bad round.
const maxOverPar = 5

// layout is the shape of an ordinary par-72 eighteen: two nines of 36 that
// alternate their short and long holes instead of clumping them. Rounds of other
// lengths take the first n holes of it, wrapping round once past 18.
var layout = [18]int{4, 5, 3, 4, 4, 3, 4, 5, 4, 4, 3, 5, 4, 4, 4, 3, 5, 4}

// yardsByPar is how long a hole of each par typically plays. The numbers are only
// ever used as ratios, to share the route out between the holes so that a par 5
// takes more walking than a par 3.
var yardsByPar = map[int]float64{3: 175, 4: 400, 5: 530}

// Hole is one hole of a round, together with the stretch of route it occupies.
type Hole struct {
	Number  int
	Par     int
	Strokes int

	// Start and End are distances in metres along the walked route.
	Start float64
	End   float64
}

// Over returns the hole's score relative to its par.
func (h Hole) Over() int { return h.Strokes - h.Par }

// Round is a full scorecard laid out along a route.
type Round struct {
	Holes   []Hole
	Par     int
	Strokes int
}

// Over returns the round's score relative to par.
func (r *Round) Over() int { return r.Strokes - r.Par }

// String summarises the card the way a golfer would say it out loud.
func (r *Round) String() string {
	rel := fmt.Sprintf("%+d", r.Over())
	if r.Over() == 0 {
		rel = "level"
	}
	return fmt.Sprintf("%d holes, par %d, %d strokes (%s)", len(r.Holes), r.Par, r.Strokes, rel)
}

// DefaultPar is the par of a course with this many holes, taken from the standard
// layout.
func DefaultPar(holes int) int {
	total := 0
	for i := 0; i < holes; i++ {
		total += layout[i%len(layout)]
	}
	return total
}

// DefaultStrokes is what a mid-handicap golfer goes round in: a shade under a
// bogey a hole.
func DefaultStrokes(holes, par int) int {
	return par + (holes*3+2)/4
}

// NewRound builds a scorecard for the given number of holes and lays it out along
// a route of routeLength metres. A par or score of zero or less asks for the
// default; every other value is taken literally and validated.
func NewRound(holes, par, strokes int, routeLength float64) (*Round, error) {
	if holes < 1 || holes > MaxHoles {
		return nil, fmt.Errorf("--holes must be between 1 and %d, got %d", MaxHoles, holes)
	}
	if routeLength <= 0 {
		return nil, fmt.Errorf("the KML route has no length to lay %d holes along", holes)
	}

	if par <= 0 {
		par = DefaultPar(holes)
	}
	if par < MinHolePar*holes || par > MaxHolePar*holes {
		return nil, fmt.Errorf("--par for %d holes must be between %d and %d, got %d",
			holes, MinHolePar*holes, MaxHolePar*holes, par)
	}

	if strokes <= 0 {
		strokes = DefaultStrokes(holes, par)
	}
	// One stroke a hole is the floor a scorecard can express; the ceiling keeps a
	// mistyped score from stretching the round into a day-long walk.
	maxStrokes := (MaxHolePar + maxOverPar) * holes
	if strokes < holes || strokes > maxStrokes {
		return nil, fmt.Errorf("--score for %d holes must be between %d and %d, got %d",
			holes, holes, maxStrokes, strokes)
	}

	pars := parsFor(holes, par)
	scores := scoresFor(pars, strokes)

	round := &Round{Holes: make([]Hole, holes), Par: par, Strokes: strokes}
	for i := range round.Holes {
		round.Holes[i] = Hole{Number: i + 1, Par: pars[i], Strokes: scores[i]}
	}
	layOut(round.Holes, routeLength)
	return round, nil
}

// parsFor shares total par out over the holes, starting from the standard layout
// and nudging individual holes up or down until the total matches. The caller has
// already checked that total is reachable within MinHolePar..MaxHolePar.
func parsFor(holes, total int) []int {
	pars := make([]int, holes)
	sum := 0
	for i := range pars {
		pars[i] = layout[i%len(layout)]
		sum += pars[i]
	}

	for sum < total {
		i, ok := pickHole(pars, func(_, p int) bool { return p < MaxHolePar })
		if !ok {
			break
		}
		pars[i]++
		sum++
	}
	for sum > total {
		i, ok := pickHole(pars, func(_, p int) bool { return p > MinHolePar })
		if !ok {
			break
		}
		pars[i]--
		sum--
	}
	return pars
}

// scoresFor turns the pars into a per-hole score summing to total, keeping every
// hole within a stroke or two of what it is worth before it starts piling the
// damage onto one hole.
func scoresFor(pars []int, total int) []int {
	scores := make([]int, len(pars))
	sum := 0
	for i, p := range pars {
		scores[i] = p
		sum += p
	}

	// The ceiling rises a stroke at a time, so a wildly high score spreads itself
	// over the card instead of landing entirely on whichever hole is picked first.
	for limit := 1; sum < total && limit <= maxOverPar; limit++ {
		for sum < total {
			i, ok := pickHole(scores, func(i, s int) bool { return s-pars[i] < limit })
			if !ok {
				break
			}
			scores[i]++
			sum++
		}
	}
	if sum < total {
		// Every hole is at the cap and the remainder still has to go somewhere.
		i, ok := pickHole(scores, func(int, int) bool { return true })
		if ok {
			scores[i] += total - sum
			sum = total
		}
	}

	for sum > total {
		// Two under par is an eagle; below that a hole stops being believable, so
		// only take strokes off elsewhere once every hole has reached the floor.
		i, ok := pickHole(scores, func(i, s int) bool { return s > 1 && s > pars[i]-2 })
		if !ok {
			if i, ok = pickHole(scores, func(_, s int) bool { return s > 1 }); !ok {
				break
			}
		}
		scores[i]--
		sum--
	}
	return scores
}

// pickHole returns the index of a random hole satisfying want, reporting whether
// there was one at all. It walks the card from a random starting hole so that
// repeated calls do not keep landing on the same one.
func pickHole(values []int, want func(idx, value int) bool) (int, bool) {
	n := len(values)
	if n == 0 {
		return 0, false
	}
	start := int(simulator.RandomFloat(0, float64(n)))
	if start >= n {
		start = n - 1
	}
	for off := 0; off < n; off++ {
		i := (start + off) % n
		if want(i, values[i]) {
			return i, true
		}
	}
	return 0, false
}

// layOut divides the route between the holes in proportion to how long a hole of
// each par plays, so the par 5 gets the long walk and the par 3 the short one.
// The last hole is pinned to the end of the route so that rounding cannot leave a
// sliver of it unwalked.
func layOut(holes []Hole, routeLength float64) {
	weight := 0.0
	for _, h := range holes {
		weight += yardsByPar[h.Par]
	}

	covered := 0.0
	for i := range holes {
		holes[i].Start = covered
		covered += routeLength * yardsByPar[holes[i].Par] / weight
		holes[i].End = covered
	}
	holes[len(holes)-1].End = routeLength
}
