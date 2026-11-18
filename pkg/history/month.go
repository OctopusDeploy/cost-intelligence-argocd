package history

import (
	"sort"
	"time"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/models"
)

// runRateDays is how many complete days before today the run rate averages.
const runRateDays = 7

// Month is an application's cost in a calendar month (UTC): what it has cost so far and what the
// month will cost at the current run rate, the figure AWS bills at the end of the month.
type Month struct {
	// Month is the calendar month, "2006-01".
	Month string `json:"month"`
	// ToDate is the cost recorded from the first of the month through today, today's part included.
	ToDate float64 `json:"toDate"`
	// Projected is ToDate plus the days left at the run rate.
	Projected float64 `json:"projected"`
	// DailyRunRate is the average cost per day over the last complete days; RunRateDays is how many
	// days it averages (fewer than seven for an application whose history is shorter).
	DailyRunRate float64 `json:"dailyRunRate"`
	RunRateDays  int     `json:"runRateDays"`
	// DaysElapsed counts the days of the month up to and including today, DaysInMonth all of them.
	DaysElapsed int `json:"daysElapsed"`
	DaysInMonth int `json:"daysInMonth"`
}

// MonthFrom is the first day Daily must be asked for to summarise now's month: the first of the month,
// or earlier when the run rate needs the days before it.
func MonthFrom(now time.Time) time.Time {
	today := utcDay(now)
	first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	if back := today.AddDate(0, 0, -runRateDays); back.Before(first) {
		return back
	}
	return first
}

// SummariseMonth builds the Month of now from the daily costs, which must cover MonthFrom(now) through
// today. It returns false when there are no days at all.
//
// A day with no cost is not in the series, so the run rate counts calendar days, starting no earlier
// than the first day recorded: a job that ran once in the last week averages one run over seven days,
// not a run per day, and an application a day old is not diluted by days before it existed. Today is
// partly measured: it counts in ToDate, and in Projected as the larger of what it has cost so far and
// the run rate.
func SummariseMonth(days []models.DailyCost, now time.Time) (Month, bool) {
	if len(days) == 0 {
		return Month{}, false
	}
	today := utcDay(now)
	first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := first.AddDate(0, 1, -1).Day()

	byDay := make(map[string]float64, len(days))
	dates := make([]string, 0, len(days))
	for _, d := range days {
		if _, seen := byDay[d.Date]; !seen {
			dates = append(dates, d.Date)
		}
		byDay[d.Date] += d.TotalCost
	}
	sort.Strings(dates)
	oldest, err := time.Parse(time.DateOnly, dates[0])
	if err != nil {
		return Month{}, false
	}

	m := Month{Month: first.Format("2006-01"), DaysElapsed: today.Day(), DaysInMonth: daysInMonth}
	var completeInMonth float64
	for date, cost := range byDay {
		day, err := time.Parse(time.DateOnly, date)
		if err != nil || day.Before(first) || day.After(today) {
			continue
		}
		m.ToDate += cost
		if day.Before(today) {
			completeInMonth += cost
		}
	}

	start := today.AddDate(0, 0, -runRateDays)
	if oldest.After(start) {
		start = oldest
	}
	var windowCost float64
	for d := start; d.Before(today); d = d.AddDate(0, 0, 1) {
		windowCost += byDay[formatDay(d)]
		m.RunRateDays++
	}
	if m.RunRateDays > 0 {
		m.DailyRunRate = windowCost / float64(m.RunRateDays)
	}

	todayCost := byDay[formatDay(today)]
	m.Projected = completeInMonth + max(todayCost, m.DailyRunRate) + m.DailyRunRate*float64(daysInMonth-today.Day())
	return m, true
}
