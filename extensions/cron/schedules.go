package cron

import (
	"fmt"
	"strings"
	"time"
)

// Schedule determines when a task should run.
type Schedule interface {
	ShouldRun(lastRun time.Time) bool
	Next(from time.Time) time.Time
	String() string
}

// nowFunc allows testing with fake clocks.
var nowFunc = time.Now

// IntervalSchedule runs every fixed duration.
type IntervalSchedule struct {
	Interval time.Duration
}

func (s IntervalSchedule) ShouldRun(lastRun time.Time) bool {
	if lastRun.IsZero() {
		return true
	}
	return nowFunc().Sub(lastRun) >= s.Interval
}

func (s IntervalSchedule) Next(from time.Time) time.Time {
	return from.Add(s.Interval)
}

func (s IntervalSchedule) String() string {
	return fmt.Sprintf("every %s", s.Interval)
}

// DailySchedule runs once per day at a specific time.
type DailySchedule struct {
	Hour   int
	Minute int
}

func (s DailySchedule) target(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), s.Hour, s.Minute, 0, 0, now.Location())
}

func (s DailySchedule) ShouldRun(lastRun time.Time) bool {
	now := nowFunc()
	target := s.target(now)

	if lastRun.IsZero() {
		// First run: wait until target time arrives.
		return !now.Before(target)
	}

	// Missed run (>24h gap): run immediately.
	if now.Sub(lastRun) > 24*time.Hour {
		return true
	}

	// Normal: run if we haven't run since today's target.
	return lastRun.Before(target) && !now.Before(target)
}

func (s DailySchedule) Next(from time.Time) time.Time {
	target := s.target(from)
	if !from.Before(target) {
		target = target.AddDate(0, 0, 1)
	}
	return target
}

func (s DailySchedule) String() string {
	return fmt.Sprintf("daily at %02d:%02d", s.Hour, s.Minute)
}

// WeeklySchedule runs once per week on a specific day and time.
type WeeklySchedule struct {
	Weekday time.Weekday
	Hour    int
	Minute  int
}

func (s WeeklySchedule) target(now time.Time) time.Time {
	days := int(s.Weekday) - int(now.Weekday())
	if days < 0 {
		days += 7
	}
	d := now.AddDate(0, 0, days)
	return time.Date(d.Year(), d.Month(), d.Day(), s.Hour, s.Minute, 0, 0, now.Location())
}

func (s WeeklySchedule) ShouldRun(lastRun time.Time) bool {
	now := nowFunc()
	target := s.target(now)

	if lastRun.IsZero() {
		return !now.Before(target)
	}

	// Missed run (>7d gap): run immediately.
	if now.Sub(lastRun) > 7*24*time.Hour {
		return true
	}

	// Normal: run if we haven't run since this week's target.
	return lastRun.Before(target) && !now.Before(target)
}

func (s WeeklySchedule) Next(from time.Time) time.Time {
	target := s.target(from)
	if !from.Before(target) {
		target = target.AddDate(0, 0, 7)
	}
	return target
}

func (s WeeklySchedule) String() string {
	return fmt.Sprintf("weekly on %s at %02d:%02d", s.Weekday, s.Hour, s.Minute)
}

// ParseSchedule converts a ScheduleSpec into a Schedule.
func ParseSchedule(spec ScheduleSpec) (Schedule, error) {
	switch {
	case spec.Every != "":
		d, err := time.ParseDuration(spec.Every)
		if err != nil {
			return nil, fmt.Errorf("parse interval %q: %w", spec.Every, err)
		}
		if d < time.Minute {
			return nil, fmt.Errorf("interval %q too short (minimum 1m)", spec.Every)
		}
		return IntervalSchedule{Interval: d}, nil

	case spec.DailyAt != "":
		var hour, minute int
		if _, err := fmt.Sscanf(spec.DailyAt, "%d:%d", &hour, &minute); err != nil {
			return nil, fmt.Errorf("parse daily_at %q (expected HH:MM): %w", spec.DailyAt, err)
		}
		if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return nil, fmt.Errorf("invalid time %q: hour 0-23, minute 0-59", spec.DailyAt)
		}
		return DailySchedule{Hour: hour, Minute: minute}, nil

	case spec.Weekly != "":
		parts := strings.Fields(spec.Weekly)
		if len(parts) != 2 {
			return nil, fmt.Errorf("parse weekly %q (expected \"weekday HH:MM\")", spec.Weekly)
		}
		weekday, err := parseWeekday(parts[0])
		if err != nil {
			return nil, err
		}
		var hour, minute int
		if _, err := fmt.Sscanf(parts[1], "%d:%d", &hour, &minute); err != nil {
			return nil, fmt.Errorf("parse weekly time %q: %w", parts[1], err)
		}
		if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return nil, fmt.Errorf("invalid time in weekly %q", spec.Weekly)
		}
		return WeeklySchedule{Weekday: weekday, Hour: hour, Minute: minute}, nil

	default:
		return nil, fmt.Errorf("no schedule specified (need every, daily_at, or weekly)")
	}
}

func parseWeekday(s string) (time.Weekday, error) {
	switch strings.ToLower(s) {
	case "sunday", "sun":
		return time.Sunday, nil
	case "monday", "mon":
		return time.Monday, nil
	case "tuesday", "tue":
		return time.Tuesday, nil
	case "wednesday", "wed":
		return time.Wednesday, nil
	case "thursday", "thu":
		return time.Thursday, nil
	case "friday", "fri":
		return time.Friday, nil
	case "saturday", "sat":
		return time.Saturday, nil
	default:
		return 0, fmt.Errorf("unknown weekday %q", s)
	}
}
