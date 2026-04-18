package scheduler

import (
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	hourAgo := now.Add(-1 * time.Hour)
	thirtyMinAgo := now.Add(-30 * time.Minute)
	dayAgo := now.Add(-24 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	cases := []struct {
		name     string
		schedule string
		lastRun  *time.Time
		want     Decision
	}{
		{"manual never runs", "manual", &hourAgo, Decision{Run: false, Reason: "schedule is manual"}},
		{"hourly never run before", "hourly", nil, Decision{Run: true, Reason: "no prior run"}},
		{"hourly due exactly at boundary", "hourly", &hourAgo, Decision{Run: true, Reason: "hourly interval elapsed"}},
		{"hourly not yet due", "hourly", &thirtyMinAgo, Decision{Run: false, Reason: "next due in ~30m0s"}},
		{"daily never run", "daily", nil, Decision{Run: true, Reason: "no prior run"}},
		{"daily exactly at boundary", "daily", &dayAgo, Decision{Run: true, Reason: "daily interval elapsed"}},
		{"daily not yet due", "daily", &twoHoursAgo, Decision{Run: false, Reason: "next due in ~22h0m0s"}},
		{"unknown mode treated as manual", "weekly", &dayAgo, Decision{Run: false, Reason: `unknown schedule "weekly", treating as manual`}},
		{"empty mode treated as manual", "", &dayAgo, Decision{Run: false, Reason: "schedule is manual"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(now, tc.schedule, tc.lastRun)
			if got.Run != tc.want.Run || got.Reason != tc.want.Reason {
				t.Fatalf("Decide = %+v, want %+v", got, tc.want)
			}
		})
	}
}
