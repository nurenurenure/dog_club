package reports

import (
	"bytes"
	"encoding/csv"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dogclub.local/backend/internal/events"
	"dogclub.local/backend/internal/platform/reporting"
)

func exampleReport() Report {
	return Report{Period: reporting.Period{From: "2024-02-01", To: "2024-02-29"}, GeneratedAt: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC), Methodology: methodology,
		Activity: Activity{MembersJoined: 3, DogsRegistered: 2, EventsCompleted: 2, EventsByType: []events.ActivityEventType{{TypeID: 1, TypeName: "Выставка", Count: 1}, {TypeID: 2, TypeName: "Семинар, лекция", Count: 1}}, MatingsPerformed: 1, LittersBorn: 1, PuppiesBorn: 8},
		Current:  Current{MemberCards: 4, ActiveMembers: 2, DogCards: 5, DogsOnRegister: 3}, DataQuality: DataQuality{MembersWithoutDate: 1, DogsWithoutDate: 2, MatingsWithoutDate: 1, EventsWithoutPeriod: 1}}
}
func TestCSVPreservesReportNumbersAndMetadata(t *testing.T) {
	r := exampleReport()
	data, err := renderCSV(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("missing UTF-8 BOM")
	}
	records, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 17 || strings.Join(records[0], ",") != "scope,metric,event_type_id,label,value,from,to,generated_at,methodology" {
		t.Fatal("CSV columns or metric rows missing")
	}
	expected := map[string]string{"members_joined": "3", "dogs_registered": "2", "events_completed": "2", "matings_performed": "1", "litters_born": "1", "puppies_born": "8", "member_cards": "4", "active_members": "2", "dog_cards": "5", "dogs_on_register": "3", "members_without_membership_date": "1", "dogs_without_registration_date": "2", "performed_matings_without_date": "1", "events_without_period": "1"}
	typeCounts := 0
	for _, row := range records[1:] {
		if len(row) != 9 || row[5] != r.Period.From || row[6] != r.Period.To || row[7] != r.GeneratedAt.Format(time.RFC3339) || row[8] != methodology {
			t.Fatal("CSV metadata differs from report")
		}
		if row[1] == "events_completed_by_type" {
			typeCounts++
			if row[4] != "1" {
				t.Fatal("type count differs")
			}
			continue
		}
		if expected[row[1]] != row[4] {
			t.Fatalf("unexpected value of %s", row[1])
		}
		delete(expected, row[1])
	}
	if len(expected) != 0 || typeCounts != 2 {
		t.Fatal("metrics omitted")
	}
}
func TestCSVFormulaEscaping(t *testing.T) {
	for _, s := range []string{"=1+1", " +cmd()", "\tvalue", "\rvalue", "\nvalue", "-1", "@SUM(1)"} {
		if !strings.HasPrefix(csvText(s), "'") {
			t.Fatal("unsafe spreadsheet text")
		}
	}
	if csvText("Клуб, выставка") != "Клуб, выставка" {
		t.Fatal("ordinary text changed")
	}
}
func TestReportQueryStrictness(t *testing.T) {
	for _, c := range []struct {
		query         string
		export, valid bool
	}{
		{"from=2024-02-29&to=2024-02-29", false, true},
		{"from=2024-02-29&to=2024-02-29&format=csv", true, true},
		{"from=2024-02-29&to=2024-02-29&format=pdf", true, true},
		{"from=2024-02-29&to=2024-02-29&format=csv", false, false},
		{"from=2024-02-29&to=2024-02-29&extra=1", false, false},
		{"from=2024-02-29&from=2024-02-28&to=2024-02-29", false, false},
		{"from=2024-02-29&to=2024-02-29&format=csv&format=pdf", true, false},
		{"from=2024-02-29&to=2024-02-29&format=json", true, false},
		{"from=2024-02-29&to=2024-02-29", true, false},
		{"from=2024-03-01&to=2024-02-29", false, false},
		{"from=2024-02-29&to=2024-02-29&bad=%zz", false, false},
		{"from=2024-02-29;to=2024-02-29", false, false},
	} {
		req := httptest.NewRequest("GET", "http://localhost/report?"+c.query, nil)
		_, _, err := parseQuery(req, c.export)
		if (err == nil) != c.valid {
			t.Fatalf("query %s, export=%v: %v", c.query, c.export, err)
		}
	}
}
func TestReportPDFUnicodeAndPagination(t *testing.T) {
	r := exampleReport()
	for _, long := range []bool{false, true} {
		if long {
			for i := range 50 {
				r.Activity.EventsByType = append(r.Activity.EventsByType, events.ActivityEventType{TypeID: int32(100 + i), TypeName: strings.Repeat("Длинное название мероприятия ", 5), Count: int64(i)})
			}
		}
		data, err := renderPDF(r)
		if err != nil || !bytes.HasPrefix(data, []byte("%PDF-")) {
			t.Fatal("invalid report PDF", err)
		}
		if long && bytes.Count(data, []byte("/Type /Page\n")) < 2 {
			t.Fatal("long PDF did not paginate")
		}
		if dir := os.Getenv("DOGCLUB_REPORT_REVIEW_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			name := "club_activity.pdf"
			if long {
				name = "club_activity_long.pdf"
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
