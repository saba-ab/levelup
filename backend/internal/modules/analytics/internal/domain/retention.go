package domain

import "time"

// CohortCell is the number of distinct players of the cohort starting at
// Cohort who were active in week Offset (0 = the cohort's own week).
type CohortCell struct {
	Cohort  time.Time
	Offset  int
	Players int64
}

// Cohort is one row of the retention matrix. Retained[k] is the percentage
// of Size active k weeks after the cohort week; it only has entries for
// weeks that have started.
type Cohort struct {
	Start    time.Time
	Size     int64
	Retained []float64
}

// CurvePoint is the size-weighted retention k weeks after the first week,
// over the cohorts for which week k has started.
type CurvePoint struct {
	Week    int
	Pct     float64
	Cohorts int
}

// Retention is the cohort matrix and the overall curve.
type Retention struct {
	Cohorts []Cohort
	Curve   []CurvePoint
}

// CohortStarts lists the Monday of each of the last weeks weeks, oldest
// first, ending with the week containing today.
func CohortStarts(today time.Time, weeks int) []time.Time {
	current := WeekStart(today)
	out := make([]time.Time, weeks)
	for i := range weeks {
		out[i] = current.AddDate(0, 0, -7*(weeks-1-i))
	}
	return out
}

// BuildRetention assembles the matrix from cohort sizes and activity cells.
// A player's first-seen week is week 0 by construction, so a non-empty
// cohort retains 100% in week 0.
func BuildRetention(starts []time.Time, sizes map[time.Time]int64, cells []CohortCell) Retention {
	if len(starts) == 0 {
		return Retention{Cohorts: []Cohort{}, Curve: []CurvePoint{}}
	}
	active := map[time.Time]map[int]int64{}
	for _, c := range cells {
		k := c.Cohort.UTC()
		if active[k] == nil {
			active[k] = map[int]int64{}
		}
		active[k][c.Offset] += c.Players
	}
	current := starts[len(starts)-1]
	weeks := len(starts)
	num := make([]int64, weeks)
	den := make([]int64, weeks)
	cnt := make([]int, weeks)

	out := Retention{Cohorts: make([]Cohort, 0, weeks), Curve: make([]CurvePoint, 0, weeks)}
	for _, start := range starts {
		size := sizes[start]
		observable := int(current.Sub(start).Hours()/(24*7)) + 1
		row := Cohort{Start: start, Size: size, Retained: make([]float64, observable)}
		for k := range observable {
			n := active[start][k]
			row.Retained[k] = Pct(n, size)
			if size > 0 {
				num[k] += n
				den[k] += size
				cnt[k]++
			}
		}
		out.Cohorts = append(out.Cohorts, row)
	}
	for k := range weeks {
		out.Curve = append(out.Curve, CurvePoint{Week: k, Pct: Pct(num[k], den[k]), Cohorts: cnt[k]})
	}
	return out
}
