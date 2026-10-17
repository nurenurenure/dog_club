package notifications

import (
	"testing"
	"time"

	"dogclub.local/backend/internal/platform/notify"
)

func TestRetryScheduleAndStableMessageID(t *testing.T) {
	for i, want := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, time.Hour} {
		if retryDelay(i+1) != want {
			t.Fatal("retry policy changed")
		}
	}
	m := notify.Message{Module: "dogs", Kind: "registration", ID: 1, Revision: 2}
	if messageID(m, "user1") != messageID(m, "user1") || messageID(m, "user1") == messageID(m, "user2") {
		t.Fatal("unstable or nonrecipient-specific ID")
	}
	m.Generation = 1
	a := messageID(m, "user1")
	m.Generation = 2
	if a == messageID(m, "user1") {
		t.Fatal("transitions need separate IDs")
	}
}
