package veterinary

import (
	"testing"
	"time"
)

func TestReminderStagesAndUTCDates(t *testing.T) {
	for _, c := range []struct{ now, want string }{{"2026-10-01T00:00:00Z", ""}, {"2026-10-10T23:59:59Z", "week"}, {"2026-10-15T12:00:00Z", "week"}, {"2026-10-16T12:00:00Z", "tomorrow"}, {"2026-10-17T23:00:00Z", "today"}, {"2026-10-18T00:00:00Z", "overdue"}, {"2026-11-18T00:00:00Z", "overdue"}, {"2026-10-18T01:00:00+07:00", "today"}} {
		now, _ := time.Parse(time.RFC3339, c.now)
		if got := ReminderStage("2026-10-17", now); got != c.want {
			t.Errorf("%s: %s != %s", c.now, got, c.want)
		}
	}
	if ReminderStage("unknown", time.Now()) != "" {
		t.Fatal("unknown next date must not be guessed")
	}
}
