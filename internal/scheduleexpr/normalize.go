package scheduleexpr

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	cronv3 "github.com/robfig/cron/v3"
)

func NormalizeExpression(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "every:1h", nil
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "at:") {
		ts := strings.TrimSpace(s[len("at:"):])
		at, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return "", invalidScheduleError(s)
		}
		return "at:" + at.UTC().Format(time.RFC3339), nil
	}
	if strings.HasPrefix(lower, "every:") {
		dur := strings.TrimSpace(s[len("every:"):])
		if dur == "" {
			return "", invalidScheduleError(s)
		}
		if _, err := time.ParseDuration(dur); err != nil {
			return "", invalidScheduleError(s)
		}
		return "every:" + dur, nil
	}
	if strings.HasPrefix(lower, "@every ") {
		dur := strings.TrimSpace(s[len("@every "):])
		if _, err := time.ParseDuration(dur); err != nil {
			return "", invalidScheduleError(s)
		}
		return "@every " + dur, nil
	}
	if _, err := cronv3.ParseStandard(s); err != nil {
		return "", invalidScheduleError(s)
	}
	return s, nil
}

func ResolveSchedule(explicit string, natural string, timezone string, now time.Time) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return NormalizeExpression(explicit)
	}
	natural = strings.TrimSpace(natural)
	if natural == "" {
		return "", fmt.Errorf("natural or schedule is required")
	}
	return ParseNaturalSchedule(natural, timezone, now)
}

var tomorrowHourPattern = regexp.MustCompile(`(오전|오후)?\s*(\d{1,2})시`)
var weeklyPattern = regexp.MustCompile(`매주\s*([월화수목금토일])요일?\s*(오전|오후)?\s*(\d{1,2})시`)
var relativePattern = regexp.MustCompile(`(\d+)\s*(분|시간|일)\s*(뒤|후)`)
var englishRelativePattern = regexp.MustCompile(`(?i)\bin\s+(\d+)\s*(minute|minutes|min|mins|hour|hours|hr|hrs|day|days)\b`)

func ParseNaturalSchedule(natural string, timezone string, now time.Time) (string, error) {
	input := strings.TrimSpace(natural)
	if input == "" {
		return "", fmt.Errorf("natural schedule is required")
	}
	if strings.HasPrefix(strings.ToLower(input), "at:") || strings.HasPrefix(strings.ToLower(input), "every:") {
		return NormalizeExpression(input)
	}
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		loc = time.FixedZone("UTC+9", 9*3600)
	}
	if strings.Contains(input, "매주") {
		match := weeklyPattern.FindStringSubmatch(input)
		if len(match) == 4 {
			dow := weekdayToCron(match[1])
			hour, hourErr := parseHour(match[2], match[3])
			if hourErr != nil {
				return "", hourErr
			}
			return fmt.Sprintf("0 %d * * %d", hour, dow), nil
		}
	}
	if strings.Contains(input, "내일") {
		match := tomorrowHourPattern.FindStringSubmatch(input)
		if len(match) == 3 {
			hour, hourErr := parseHour(match[1], match[2])
			if hourErr != nil {
				return "", hourErr
			}
			base := now.In(loc).AddDate(0, 0, 1)
			at := time.Date(base.Year(), base.Month(), base.Day(), hour, 0, 0, 0, loc)
			return "at:" + at.Format(time.RFC3339), nil
		}
	}
	relative := relativePattern.FindStringSubmatch(input)
	if len(relative) == 4 {
		amount := 0
		if _, err := fmt.Sscanf(strings.TrimSpace(relative[1]), "%d", &amount); err != nil || amount <= 0 {
			return "", fmt.Errorf("invalid relative schedule value: %s", strings.TrimSpace(relative[1]))
		}
		unit := strings.TrimSpace(relative[2])
		base := now.In(loc)
		var target time.Time
		switch unit {
		case "분":
			target = base.Add(time.Duration(amount) * time.Minute)
		case "시간":
			target = base.Add(time.Duration(amount) * time.Hour)
		case "일":
			target = base.AddDate(0, 0, amount)
		}
		if !target.IsZero() {
			return "at:" + target.Format(time.RFC3339), nil
		}
	}
	englishRelative := englishRelativePattern.FindStringSubmatch(input)
	if len(englishRelative) == 3 {
		amount := 0
		if _, err := fmt.Sscanf(strings.TrimSpace(englishRelative[1]), "%d", &amount); err != nil || amount <= 0 {
			return "", fmt.Errorf("invalid relative schedule value: %s", strings.TrimSpace(englishRelative[1]))
		}
		unit := strings.ToLower(strings.TrimSpace(englishRelative[2]))
		base := now.In(loc)
		var target time.Time
		switch unit {
		case "minute", "minutes", "min", "mins":
			target = base.Add(time.Duration(amount) * time.Minute)
		case "hour", "hours", "hr", "hrs":
			target = base.Add(time.Duration(amount) * time.Hour)
		case "day", "days":
			target = base.AddDate(0, 0, amount)
		}
		if !target.IsZero() {
			return "at:" + target.Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("could not parse natural schedule; use at:<rfc3339> or every:<duration> (e.g. at:2026-03-01T09:35:00Z, every:10m)")
}

func invalidScheduleError(raw string) error {
	return fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", raw)
}

func parseHour(marker string, raw string) (int, error) {
	hour := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &hour); err != nil {
		return 0, fmt.Errorf("invalid hour: %s", raw)
	}
	if hour < 0 || hour > 23 {
		if hour < 1 || hour > 12 {
			return 0, fmt.Errorf("invalid hour: %d", hour)
		}
	}
	mark := strings.TrimSpace(marker)
	switch mark {
	case "오후":
		if hour < 12 {
			hour += 12
		}
	case "오전":
		if hour == 12 {
			hour = 0
		}
	}
	if hour > 23 {
		return 0, fmt.Errorf("invalid hour: %d", hour)
	}
	return hour, nil
}

func weekdayToCron(token string) int {
	switch strings.TrimSpace(token) {
	case "월":
		return 1
	case "화":
		return 2
	case "수":
		return 3
	case "목":
		return 4
	case "금":
		return 5
	case "토":
		return 6
	case "일":
		return 0
	default:
		return 0
	}
}
