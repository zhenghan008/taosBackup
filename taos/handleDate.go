package taos

import (
	"time"
)

type TimeRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

func SplitTimeRangeByDay(startMs int64, endMs int64, intervalDays int) []*TimeRange {
	now := time.Now().UnixMilli()

	// Boundary handling: If the start time is later than the current time, return null or the current point directly
	if startMs >= now {
		return nil
	}
	if endMs == 0 || endMs > now {
		endMs = now
	}
	if intervalDays <= 0 {
		intervalDays = 1
	}
	const msPerDay = int64(24 * time.Hour / time.Millisecond)
	estimateSize := int((endMs-startMs)/(msPerDay*int64(intervalDays))) + 1
	ranges := make([]*TimeRange, 0, estimateSize)

	currentStart := startMs
	for currentStart < endMs {
		//Calculate the end time of the day containing the current time (00:00:00 the next day)
		t := time.UnixMilli(currentStart)
		nextDay := time.Date(t.Year(), t.Month(), t.Day()+intervalDays, 0, 0, 0, 0, t.Location())
		nextDayUnix := nextDay.UnixMilli()
		currentEnd := nextDayUnix
		//If the calculated end time exceeds the current time, the current time shall prevail
		if currentEnd > endMs {
			currentEnd = endMs
		}

		ranges = append(ranges, &TimeRange{
			Start: currentStart,
			End:   currentEnd,
		})
		currentStart = nextDayUnix
	}

	return ranges
}
