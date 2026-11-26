package history

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/models"
)

func day(date string, cost float64) models.DailyCost {
	return models.DailyCost{Date: date, TotalCost: cost}
}

func TestSummariseMonth_ProjectsTheRestOfTheMonthAtTheRunRate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	var days []models.DailyCost
	for d := 1; d <= 17; d++ {
		days = append(days, day(time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), 2))
	}
	days = append(days, day("2026-11-18", 0.5))

	m, ok := SummariseMonth(days, now)
	require.True(t, ok)

	assert.Equal(t, "2026-11", m.Month)
	assert.InDelta(t, 34.5, m.ToDate, 1e-9)
	assert.InDelta(t, 2.0, m.DailyRunRate, 1e-9)
	assert.Equal(t, 7, m.RunRateDays)
	// 17 complete days at 2, today at the run rate (2 > 0.5 so far), 12 days left at 2.
	assert.InDelta(t, 34+2+24, m.Projected, 1e-9)
	assert.Equal(t, 30, m.DaysInMonth)
}

// A job that ran once is not a job that runs every day: one run over seven days is a seventh of it.
func TestSummariseMonth_AJobThatRanOnceIsNotMultipliedByTheMonth(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	days := []models.DailyCost{day("2026-11-01", 0), day("2026-11-12", 7)}

	m, ok := SummariseMonth(days, now)
	require.True(t, ok)

	assert.InDelta(t, 7, m.ToDate, 1e-9)
	assert.InDelta(t, 1.0, m.DailyRunRate, 1e-9)
	assert.InDelta(t, 7+1+12, m.Projected, 1e-9)
}

func TestSummariseMonth_ANewApplicationIsNotDilutedByDaysBeforeIt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	m, ok := SummariseMonth([]models.DailyCost{day("2026-11-16", 3), day("2026-11-17", 3), day("2026-11-18", 1)}, now)
	require.True(t, ok)

	assert.Equal(t, 2, m.RunRateDays)
	assert.InDelta(t, 3.0, m.DailyRunRate, 1e-9)
}

func TestSummariseMonth_EarlyInTheMonthUsesTheDaysBeforeIt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	days := []models.DailyCost{day("2026-10-27", 4), day("2026-10-31", 4), day("2026-11-01", 4), day("2026-11-02", 1)}

	m, ok := SummariseMonth(days, now)
	require.True(t, ok)

	assert.InDelta(t, 5, m.ToDate, 1e-9, "October is not in November's cost")
	assert.Equal(t, MonthFrom(now), time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 6, m.RunRateDays, "from the first day recorded, 27 October")
}

func TestSummariseMonth_NoDays(t *testing.T) {
	t.Parallel()

	_, ok := SummariseMonth(nil, time.Now())
	assert.False(t, ok)
}
