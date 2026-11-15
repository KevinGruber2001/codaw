package mix

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/KevinGruber2001/codaw/internal/project"
)

// A lane changes one value of a track over time: its gain, its pan, or a parameter of one of its
// effects. Between points the value moves in a straight line; before the first and after the last it
// holds.
type lane struct {
	target  laneTarget
	fxIndex int    // for an effect parameter
	param   string // for an effect parameter
	points  []project.AutomationPoint
}

type laneTarget int

const (
	laneGain laneTarget = iota
	lanePan
	laneFX
)

// lanesOf reads a track's automation. A lane whose target does not parse is left out, with a log
// line (validation catches these first).
func lanesOf(t *project.Track) []lane {
	var out []lane
	for _, a := range t.Automation {
		target, index, param, err := parseTarget(a.Target)
		if err != nil {
			log.Printf("[mix] track %s: skipping automation: %v", t.ID, err)
			continue
		}
		points := append([]project.AutomationPoint(nil), a.Points...)
		sort.SliceStable(points, func(i, j int) bool { return points[i].Beat < points[j].Beat })
		out = append(out, lane{target: target, fxIndex: index, param: param, points: points})
	}
	return out
}

// automate sets the values the lanes give for a beat: gain and pan for this block, an effect's
// parameter as its new target.
func (t *trackState) automate(beat float64) {
	for _, l := range t.lanes {
		v := interpolate(l.points, beat)
		switch l.target {
		case laneGain:
			t.gain = v
		case lanePan:
			t.pan = v
		case laneFX:
			if l.fxIndex < len(t.fx) {
				t.fx[l.fxIndex].effect.SetParam(l.param, v)
			}
		}
	}
}

// parseTarget reads "gain", "pan" or "fx[N].param".
func parseTarget(target string) (laneTarget, int, string, error) {
	switch target {
	case "gain":
		return laneGain, 0, "", nil
	case "pan":
		return lanePan, 0, "", nil
	}
	rest, ok := strings.CutPrefix(target, "fx[")
	if !ok {
		return 0, 0, "", fmt.Errorf("unknown automation target %q", target)
	}
	index, param, ok := strings.Cut(rest, "].")
	if !ok || param == "" {
		return 0, 0, "", fmt.Errorf("malformed target %q (want fx[N].param)", target)
	}
	n, err := strconv.Atoi(index)
	if err != nil || n < 0 {
		return 0, 0, "", fmt.Errorf("bad effect index in %q", target)
	}
	return laneFX, n, param, nil
}

// interpolate is a lane's value at a beat.
func interpolate(points []project.AutomationPoint, beat float64) float64 {
	if len(points) == 0 {
		return 0
	}
	if beat <= points[0].Beat {
		return points[0].Value
	}
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		if beat <= b.Beat {
			if b.Beat == a.Beat {
				return b.Value
			}
			return a.Value + (beat-a.Beat)/(b.Beat-a.Beat)*(b.Value-a.Value)
		}
	}
	return points[len(points)-1].Value
}
