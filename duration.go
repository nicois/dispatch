package dispatch

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Duration is a variant on time.Duration which also understands the
// 'd' unit (for days) in addition to the normal units
type Duration time.Duration

// UnmarshalFlag parses a duration which may begin with a whole number of days,
// for example "3d", "1d12h" or plain "90m". Everything after the optional days
// component is handled by time.ParseDuration.
func (d *Duration) UnmarshalFlag(value string) error {
	if value == "" {
		return errors.New("a duration may not be empty")
	}

	var days int64
	remainder := value

	// A leading run of digits followed by 'd' is a days component. Absent the
	// 'd', those digits belong to the standard duration syntax instead.
	digits := 0
	for digits < len(remainder) && remainder[digits] >= '0' && remainder[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits < len(remainder) && remainder[digits] == 'd' {
		parsed, err := strconv.ParseInt(remainder[:digits], 10, 32)
		if err != nil {
			return fmt.Errorf("could not parse %q as a number of days: %w", remainder[:digits], err)
		}
		days = parsed
		remainder = remainder[digits+1:]
		if remainder == "" {
			*d = Duration(time.Duration(days) * 24 * time.Hour)
			return nil
		}
	}

	duration, err := time.ParseDuration(remainder)
	if err != nil {
		return err
	}
	*d = Duration(duration + time.Duration(days)*24*time.Hour)
	return nil
}
