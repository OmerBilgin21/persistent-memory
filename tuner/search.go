package main

import (
	"fmt"
	"math"
	"persistent-memory/util"
)

const falseRejectCap = 0.10

type label struct {
	id     string
	reject bool
	check  bool
}

type rates struct {
	falseRejects int
	shouldAccept int
	misses       int
	shouldReject int
	wrong        []string
}

func (r rates) falseRejectRate() float64 {
	if r.shouldAccept == 0 {
		return 0
	}
	return float64(r.falseRejects) / float64(r.shouldAccept)
}

func (r rates) missRate() float64 {
	if r.shouldReject == 0 {
		return 0
	}
	return float64(r.misses) / float64(r.shouldReject)
}

func score(labels []label, rejected []bool, check bool) rates {
	var r rates
	for i, l := range labels {
		if l.check != check {
			continue
		}

		if l.reject {
			r.shouldReject++
			if !rejected[i] {
				r.misses++
				r.wrong = append(r.wrong, l.id+" should be rejected, was accepted")
			}
			continue
		}

		r.shouldAccept++
		if rejected[i] {
			r.falseRejects++
			r.wrong = append(r.wrong, l.id+" should be accepted, was rejected")
		}
	}

	return r
}

func round3(value float64) float32 {
	return float32(math.Round(value*1000) / 1000)
}

func grid(low, high, step float64) []float32 {
	count := int(math.Round((high - low) / step))
	values := make([]float32, 0, count+1)
	for i := 0; i <= count; i++ {
		values = append(values, round3(low+float64(i)*step))
	}
	return values
}

type binaryPick struct {
	value       float32
	plateauLow  float32
	plateauHigh float32
	tuning      rates
	check       rates
}

func pickBinary(values []float32, rejected [][]bool, labels []label) (binaryPick, bool) {
	feasible := make([]bool, len(values))
	missRates := make([]float64, len(values))
	best := -1.0

	for v := range values {
		r := score(labels, rejected[v], false)
		feasible[v] = r.falseRejectRate() <= falseRejectCap
		missRates[v] = r.missRate()
		if feasible[v] && (best < 0 || missRates[v] < best) {
			best = missRates[v]
		}
	}

	if best < 0 {
		return binaryPick{}, false
	}

	runStart, runLength := -1, 0
	currentStart, currentLength := -1, 0
	for v := range values {
		if feasible[v] && missRates[v] == best {
			if currentLength == 0 {
				currentStart = v
			}
			currentLength++
			if currentLength > runLength {
				runStart, runLength = currentStart, currentLength
			}
			continue
		}
		currentLength = 0
	}

	middle := runStart + (runLength-1)/2

	return binaryPick{
		value:       values[middle],
		plateauLow:  values[runStart],
		plateauHigh: values[runStart+runLength-1],
		tuning:      score(labels, rejected[middle], false),
		check:       score(labels, rejected[middle], true),
	}, true
}

type evaluateFunc func(values []float32) ([][]bool, error)

func tuneBinary(name string, low, high float64, labels []label, evaluate evaluateFunc) (binaryPick, error) {
	values := grid(low, high, 0.001)
	rejected, err := evaluate(values)
	if err != nil {
		return binaryPick{}, err
	}

	pick, ok := pickBinary(values, rejected, labels)
	if !ok {
		return binaryPick{}, fmt.Errorf("%s: no value rejects at most %.0f%% of the should-accept cases",
			name, falseRejectCap*100)
	}

	return pick, nil
}

type injectLabel struct {
	id       string
	expected []string
	check    bool
}

const (
	hitWeight   = 1.25
	wrongWeight = 1.0
)

type factScore struct {
	hits     int
	wrong    int
	expected int
}

func (f factScore) value() float64 {
	return hitWeight*float64(f.hits) - wrongWeight*float64(f.wrong)
}

func (f factScore) add(other factScore) factScore {
	return factScore{
		hits:     f.hits + other.hits,
		wrong:    f.wrong + other.wrong,
		expected: f.expected + other.expected,
	}
}

func scoreFacts(injected, expected []string) factScore {
	want := map[string]bool{}
	for _, e := range expected {
		want[util.NormalizeStr(e)] = true
	}

	score := factScore{expected: len(want)}
	for _, i := range injected {
		if want[i] {
			score.hits++
		} else {
			score.wrong++
		}
	}

	return score
}

type injectPick struct {
	minimum      float32
	gap          float32
	regionSize   int
	tuningFacts  factScore
	checkFacts   factScore
	tuningRight  int
	tuningTotal  int
	checkRight   int
	checkTotal   int
	wrongTuning  []int
	wrongCheck   []int
	injectedHere [][]string
}

type scoreGrid struct {
	scores [][]float64
	best   float64
}

func newScoreGrid(results [][][][]string, labels []injectLabel) scoreGrid {
	sg := scoreGrid{scores: make([][]float64, len(results)), best: math.Inf(-1)}
	for m := range results {
		sg.scores[m] = make([]float64, len(results[m]))
		for g := range results[m] {
			for c, l := range labels {
				if !l.check {
					sg.scores[m][g] += scoreFacts(results[m][g][c], l.expected).value()
				}
			}
			sg.best = max(sg.best, sg.scores[m][g])
		}
	}
	return sg
}

func (s scoreGrid) isBest(m, g int) bool {
	return m >= 0 && m < len(s.scores) && g >= 0 && g < len(s.scores[m]) && s.scores[m][g] == s.best
}

func (s scoreGrid) radius(m, g int) int {
	r := 0
	for {
		next := r + 1
		for dm := -next; dm <= next; dm++ {
			for dg := -next; dg <= next; dg++ {
				if !s.isBest(m+dm, g+dg) {
					return r
				}
			}
		}
		r = next
	}
}

func (s scoreGrid) mostInteriorBest() (int, int, int) {
	pickM, pickG, pickRadius, regionSize := -1, -1, -1, 0
	for m := range s.scores {
		for g := range s.scores[m] {
			if !s.isBest(m, g) {
				continue
			}
			regionSize++
			if r := s.radius(m, g); r > pickRadius {
				pickM, pickG, pickRadius = m, g, r
			}
		}
	}
	return pickM, pickG, regionSize
}

func (p *injectPick) record(c int, injected []string, l injectLabel) {
	facts := scoreFacts(injected, l.expected)
	right := sameSet(injected, l.expected)

	if l.check {
		p.checkFacts = p.checkFacts.add(facts)
		p.checkTotal++
		if right {
			p.checkRight++
		} else {
			p.wrongCheck = append(p.wrongCheck, c)
		}
		return
	}

	p.tuningFacts = p.tuningFacts.add(facts)
	p.tuningTotal++
	if right {
		p.tuningRight++
	} else {
		p.wrongTuning = append(p.wrongTuning, c)
	}
}

func pickInject(minimums, gaps []float32, results [][][][]string, labels []injectLabel) injectPick {
	pickM, pickG, regionSize := newScoreGrid(results, labels).mostInteriorBest()

	pick := injectPick{
		minimum:      minimums[pickM],
		gap:          gaps[pickG],
		regionSize:   regionSize,
		injectedHere: results[pickM][pickG],
	}
	for c, l := range labels {
		pick.record(c, results[pickM][pickG][c], l)
	}

	return pick
}

func injectGrid(runner *injectRunner, minimums, gaps []float32) ([][][][]string, error) {
	results := make([][][][]string, len(minimums))
	for m, minimum := range minimums {
		results[m] = make([][][]string, len(gaps))
		for g, gap := range gaps {
			injected, err := runner.run(minimum, gap)
			if err != nil {
				return nil, err
			}
			results[m][g] = injected
		}
	}
	return results, nil
}

func tuneInject(runner *injectRunner, labels []injectLabel) (injectPick, error) {
	minimums := grid(0.70, 0.95, 0.005)
	gaps := grid(0.0, 0.20, 0.005)

	results, err := injectGrid(runner, minimums, gaps)
	if err != nil {
		return injectPick{}, err
	}

	return pickInject(minimums, gaps, results, labels), nil
}
