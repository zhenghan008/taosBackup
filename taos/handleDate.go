package taos

import (
	"time"
)

// TimestampPrecision define time precision
type TimestampPrecision int

const (
	PrecisionMillisecond TimestampPrecision = iota
	PrecisionMicrosecond
	PrecisionNanosecond
)

type TimeRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

func convertTimestamp(ms int64, precision TimestampPrecision) int64 {
	switch precision {
	case PrecisionMicrosecond:
		return ms * 1000
	case PrecisionNanosecond:
		return ms * 1000000
	default:
		return ms
	}
}

func SplitTimeRangeByDay(startMs int64, endMs int64, intervalDays int, precision TimestampPrecision) []*TimeRange {
	now := time.Now().UnixMilli()

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
		t := time.UnixMilli(currentStart)
		nextDay := time.Date(t.Year(), t.Month(), t.Day()+intervalDays, 0, 0, 0, 0, t.Location())
		nextDayUnix := nextDay.UnixMilli()
		currentEnd := nextDayUnix
		if currentEnd > endMs {
			currentEnd = endMs
		}

		ranges = append(ranges, &TimeRange{
			Start: convertTimestamp(currentStart, precision),
			End:   convertTimestamp(currentEnd, precision),
		})
		currentStart = nextDayUnix
	}

	return ranges
}
