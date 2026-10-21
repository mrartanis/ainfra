package routeros

import (
	"errors"
	"regexp"
	"strconv"
)

var durationPattern = regexp.MustCompile(`^(?:(\d+)w)?(?:(\d+)d)?(?:(\d+)h)?(?:(\d+)m)?(?:(\d+(?:\.\d+)?)s)?$`)
var clockPattern = regexp.MustCompile(`^(?:(\d+)w)?(?:(\d+)d)?(\d+):(\d{2}):(\d{2}(?:\.\d+)?)$`)

func parseUptime(raw string) (float64, error) {
	if raw == "" {
		return 0, errors.New("missing uptime")
	}
	parts := durationPattern.FindStringSubmatch(raw)
	clock := false
	if parts == nil {
		parts = clockPattern.FindStringSubmatch(raw)
		clock = true
	}
	if parts == nil {
		return 0, errors.New("unknown uptime format")
	}
	total := float64(0)
	for i, multiplier := range []float64{604800, 86400, 3600, 60, 1} {
		if parts[i+1] == "" {
			continue
		}
		value, err := strconv.ParseFloat(parts[i+1], 64)
		if err != nil || clock && i >= 3 && value >= 60 {
			return 0, errors.New("invalid uptime")
		}
		total += value * multiplier
	}
	if total > 1e12 {
		return 0, errors.New("uptime too large")
	}
	return total, nil
}
