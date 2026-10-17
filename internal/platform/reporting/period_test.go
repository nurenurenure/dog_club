package reporting

import "testing"

func TestCalendarPeriod(t *testing.T) {
	for _, c := range []struct {
		from, to string
		valid    bool
	}{
		{"2024-02-29", "2024-02-29", true}, {"2023-02-29", "2023-03-01", false},
		{"2024-01-02", "2024-01-01", false}, {"", "2024-01-01", false}, {"0000-01-01", "2024-01-01", false},
		{"2024-1-01", "2024-02-01", false}, {"0001-01-01", "9999-12-31", true},
	} {
		if (Period{c.from, c.to}).Valid() != c.valid {
			t.Fatalf("unexpected validity for %q - %q", c.from, c.to)
		}
	}
}
