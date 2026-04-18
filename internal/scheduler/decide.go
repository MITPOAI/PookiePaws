// Package scheduler implements the research watchlist refresh ticker that
// runs inside the daemon (pookie start). It is intentionally not started
// by one-shot CLI commands.
package scheduler

import (
	"fmt"
	"time"
)

// Decision is the output of a single scheduler tick: should we run, and why?
// The Reason is surfaced in audit events and in pookie research status.
type Decision struct {
	Run    bool
	Reason string
}

// Schedule modes accepted by the scheduler. Anything else is treated as manual.
const (
	ModeManual = "manual"
	ModeHourly = "hourly"
	ModeDaily  = "daily"
)

// Decide is the pure scheduling rule. Given the current time, the configured
// schedule mode, and the timestamp of the last successful run (nil if never),
// it returns whether a run is due and a human-readable reason.
func Decide(now time.Time, schedule string, lastRun *time.Time) Decision {
	switch schedule {
	case ModeManual, "":
		return Decision{Run: false, Reason: "schedule is manual"}
	case ModeHourly:
		return decideInterval(now, lastRun, time.Hour, "hourly")
	case ModeDaily:
		return decideInterval(now, lastRun, 24*time.Hour, "daily")
	default:
		return Decision{Run: false, Reason: fmt.Sprintf("unknown schedule %q, treating as manual", schedule)}
	}
}

func decideInterval(now time.Time, lastRun *time.Time, interval time.Duration, label string) Decision {
	if lastRun == nil {
		return Decision{Run: true, Reason: "no prior run"}
	}
	gap := now.Sub(*lastRun)
	if gap >= interval {
		return Decision{Run: true, Reason: label + " interval elapsed"}
	}
	remaining := interval - gap
	return Decision{Run: false, Reason: fmt.Sprintf("next due in ~%s", remaining.Round(time.Minute))}
}
