package training

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTrainingPeriodValidation(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-17T12:00:00Z")
	valid := Input{TypeID: 1, StartsAt: now.Add(-time.Hour), EndsAt: now, Timezone: "Asia/Barnaul", Result: "Отработано послушание"}
	for _, c := range []struct {
		name    string
		change  func(*Input)
		invalid bool
	}{{"valid", func(*Input) {}, false}, {"future", func(i *Input) { i.EndsAt = now.Add(time.Second) }, true}, {"reverse", func(i *Input) { i.StartsAt = now.Add(time.Hour) }, true}, {"unknown zone", func(i *Input) { i.Timezone = "NoSuch/Zone" }, true}, {"implicit zone", func(i *Input) { i.Timezone = "Local" }, true}, {"before birth", func(i *Input) { i.StartsAt = time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC) }, true}, {"partial link", func(i *Input) { id := int32(1); i.EventID = &id }, true}, {"empty result", func(i *Input) { i.Result = " " }, true}} {
		t.Run(c.name, func(t *testing.T) {
			in := valid
			c.change(&in)
			if got := validateInput(&in, "2020-01-01", now); (got != nil) != c.invalid {
				t.Fatalf("unexpected validation: %v", got)
			}
		})
	}
}
func TestGrantJSONRequiresExplicitFields(t *testing.T) {
	for _, raw := range []string{`{"version":0,"qualification":"basis","reason":"grant","type_ids":[1]}`, `{"version":0,"enabled":null,"qualification":"basis","reason":"grant","type_ids":[1]}`, `{"version":0,"enabled":true,"qualification":"basis","reason":"grant","type_ids":[1],"unknown":true}`} {
		var in GrantInput
		if json.Unmarshal([]byte(raw), &in) == nil {
			t.Fatal("incomplete or unknown grant fields accepted")
		}
	}
	var in GrantInput
	if err := json.Unmarshal([]byte(`{"version":0,"enabled":true,"qualification":"basis","reason":"grant","type_ids":[1]}`), &in); err != nil {
		t.Fatal(err)
	}
}
