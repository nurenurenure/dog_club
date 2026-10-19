package veterinary

import (
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func TestCalendarPeriods(t *testing.T) {
	for _, v := range []struct {
		date   string
		period Period
		want   string
	}{
		{"2024-01-31", Period{1, "months"}, "2024-02-29"},
		{"2023-01-31", Period{1, "months"}, "2023-02-28"},
		{"2024-02-29", Period{12, "months"}, "2025-02-28"},
		{"2024-02-29", Period{365, "days"}, "2025-02-28"},
		{"2024-12-31", Period{2, "months"}, "2025-02-28"},
		{"2024-02-28", Period{2, "days"}, "2024-03-01"},
	} {
		got, err := addPeriod(v.date, v.period)
		if err != nil || got != v.want {
			t.Fatalf("%s + %+v = %s, %v; want %s", v.date, v.period, got, err, v.want)
		}
	}
	for _, v := range []struct {
		date string
		p    Period
	}{{"2024-02-30", Period{1, "days"}}, {"2024-01-01", Period{0, "days"}}, {"2024-01-01", Period{-1, "months"}}, {"2024-01-01", Period{1, "years"}}, {"9999-12-31", Period{1, "days"}}, {"9999-12-31", Period{1, "months"}}, {"2024-01-01", Period{2147483647, "days"}}} {
		if _, err := addPeriod(v.date, v.p); err == nil {
			t.Fatal("invalid or overflowing period accepted")
		}
	}
}
func TestMedicalAdmissionChecksOnlyItsBases(t *testing.T) {
	exam := Examination{Date: "2024-01-31"}
	c := Clearance{ID: 1, Decision: "allowed", ValidUntil: ptr("2024-06-30")}
	v := Vaccination{ID: 10, Date: "2024-01-01", ValidUntil: ptr("2024-05-31"), NextDate: ptr("2024-05-15")}
	// A due repeat date is distinct from vaccine expiry. Expiry is inclusive.
	for _, date := range []string{"2024-05-15", "2024-05-31"} {
		out := evaluateAdmission(date, c, exam, []Vaccination{v})
		if out.Decision != "allowed" || out.ValidUntil == nil || *out.ValidUntil != "2024-05-31" {
			t.Fatal("repeat date or inclusive expiry was interpreted as a prohibition")
		}
	}
	out := evaluateAdmission("2024-06-01", c, exam, []Vaccination{v})
	if out.Decision != "review_required" || out.Reasons[0].Code != "vaccination_expired" {
		t.Fatal("expired basis did not require review")
	}
	for _, test := range []struct {
		name      string
		clearance Clearance
		exam      Examination
		v         []Vaccination
		code      string
	}{
		{"void clearance", Clearance{ID: 1, Decision: "allowed", IsVoid: true}, exam, []Vaccination{v}, "clearance_voided"},
		{"void examination", c, Examination{Date: exam.Date, IsVoid: true}, []Vaccination{v}, "examination_voided"},
		{"unknown dates", c, exam, []Vaccination{{ID: 10, Date: v.Date}}, "vaccination_validity_unknown"},
		{"void vaccination", c, exam, []Vaccination{{ID: 10, Date: v.Date, IsVoid: true, ValidUntil: v.ValidUntil}}, "vaccination_voided"},
		{"corrected later dose", c, exam, []Vaccination{{ID: 10, Date: "2024-02-01", ValidUntil: v.ValidUntil}}, "vaccination_after_examination"},
		{"missing doses", c, exam, nil, "vaccinations_missing"},
		{"expired permission", Clearance{ID: 1, Decision: "allowed", ValidUntil: ptr("2024-01-31")}, exam, []Vaccination{v}, "clearance_expired"},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := evaluateAdmission("2024-05-01", test.clearance, test.exam, test.v)
			if out.Decision != "review_required" {
				t.Fatal("invalid admission stayed positive")
			}
			found := false
			for _, r := range out.Reasons {
				found = found || r.Code == test.code
			}
			if !found {
				t.Fatal("specific reason is missing")
			}
		})
	}
	for _, decision := range []string{"denied", "not_assessed"} {
		out := evaluateAdmission("2024-05-01", Clearance{ID: 1, Decision: decision}, exam, nil)
		if out.Decision != decision {
			t.Fatal("explicit veterinary decision was replaced")
		}
	}
}
func TestMedicalInputValidation(t *testing.T) {
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	for _, date := range []string{"2019-12-31", "2026-10-18", "2024-02-30"} {
		if clinicalDate(date, "2020-01-01", now) == nil {
			t.Fatal("invalid clinical date accepted")
		}
	}
	in := ExaminationInput{Date: "2020-01-01", Diagnoses: []DiagnosisInput{{DiseaseID: 1}, {DiseaseID: 1}}}
	if validateExamination(&in, "2020-01-01", now) == nil {
		t.Fatal("duplicate diagnoses accepted")
	}
	in.Diagnoses = []DiagnosisInput{{DiseaseID: 1}}
	if validateExamination(&in, "2020-01-01", now) != nil || in.Diagnoses[0].IsHereditary != nil {
		t.Fatal("unknown heredity was coerced")
	}
	for _, c := range []ClearanceInput{
		{ExaminationID: 1, Purpose: "events", Decision: "allowed", ValidUntil: ptr("2026-10-17")},
		{ExaminationID: 1, Purpose: "events", Decision: "denied", VaccinationIDs: []int32{1}},
		{ExaminationID: 1, Purpose: "events", Decision: "allowed", ValidUntil: ptr("2026-10-17"), VaccinationIDs: []int32{1, 1}},
	} {
		if validateClearance(&c) == nil {
			t.Fatal("invalid clearance accepted")
		}
	}
	reason := " "
	if correction(1, &reason) == nil {
		t.Fatal("blank correction reason accepted")
	}
	reason = strings.Repeat("я", 501)
	if correction(1, &reason) == nil {
		t.Fatal("overlong correction reason accepted")
	}
}
