package dogs

import (
	"context"
	"encoding/json"
	"testing"

	"dogclub.local/backend/internal/auth"
)

func TestRegistrationRequiresExplicitDateOrNull(t *testing.T) {
	var in RegistrationInput
	if err := json.Unmarshal([]byte(`{"version":1,"reason":"Исправление"}`), &in); err != nil {
		t.Fatal(err)
	}
	s := new(Service)
	if _, err := s.SetRegistrationDate(context.Background(), auth.Principal{}, 1, in); err == nil {
		t.Fatal("omitted date could erase confirmation")
	}
	if err := json.Unmarshal([]byte(`{"version":1,"reason":"Снято подтверждение","registration_date":null}`), &in); err != nil {
		t.Fatal(err)
	}
	if !in.Date.Set || in.Date.Value != nil {
		t.Fatal("explicit null not distinguished")
	}
}
