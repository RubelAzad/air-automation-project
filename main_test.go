package main

import (
	"testing"
	"time"

	"air-automation/internal/action"
)

func TestFillBookingIDAndSafeArg(t *testing.T) {
	plan := action.Plan{Steps: []action.Action{{Kind: action.Navigate, URL: "https://x.test/b?id={{BOOKING_ID}}"}}}
	fillBookingID(&plan, "ABC123")
	if got := plan.Steps[0].URL; got != "https://x.test/b?id=ABC123" {
		t.Fatalf("url = %q", got)
	}
	plan = action.Plan{Steps: []action.Action{{Kind: action.Fill, Value: "{{TODAY+7}}"}}}
	fillBookingID(&plan, "x")
	if want := time.Now().AddDate(0, 0, 7).Format("02/01/2006"); plan.Steps[0].Value != want {
		t.Fatalf("date = %q, want %q", plan.Steps[0].Value, want)
	}
	for _, bad := range []string{"../etc", "a&b=c", "a/b", ""} {
		if safeArg.MatchString(bad) {
			t.Errorf("safeArg accepted %q", bad)
		}
	}
}
