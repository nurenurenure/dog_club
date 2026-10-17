package documents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dogclub.local/backend/internal/breeding"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/veterinary"
)

func ptr[T any](v T) *T { return &v }
func TestPDFFormsCyrillicPaginationAndLongValues(t *testing.T) {
	s := sourceSnapshot{Document: Input{Number: "КЛ-2026/001", Issuer: "Клуб собаководства", IssueDate: ptr("2026-10-17")}, Member: &members.Member{Summary: members.Summary{ID: 1, FullName: "Анна Муравьёва", MembershipStatus: members.Dictionary{Name: "Активный"}}}, Dog: &dogs.Dog{Summary: dogs.Summary{ID: 1, Nickname: "Верный друг", DateOfBirth: "2023-01-01", Breed: dogs.Dictionary{Name: "Немецкая овчарка"}, Sex: dogs.Dictionary{Name: "Кобель"}}}, Medical: &veterinary.DocumentSource{Examination: veterinary.Examination{ID: 1, Date: "2026-10-01", Version: 1, Description: strings.Repeat("Состояние собаки удовлетворительное. ", 250), Result: "Без отклонений", Recommendations: "Плановый осмотр"}}}
	for _, kind := range []string{"member_card", "dog_card", "veterinary_conclusion"} {
		copy := s
		if kind == "member_card" {
			copy.Dog = nil
			copy.Medical = nil
		}
		if kind == "dog_card" {
			copy.Member = nil
			copy.Medical = nil
		}
		if kind == "veterinary_conclusion" {
			copy.Member = nil
		}
		data, err := render(copy, kind)
		if err != nil || !bytes.HasPrefix(data, []byte("%PDF-")) {
			t.Fatal("PDF generation failed", err)
		}
		if kind == "veterinary_conclusion" && bytes.Count(data, []byte("/Type /Page\n")) < 2 {
			t.Fatal("long clinical text did not paginate")
		}
		if dir := os.Getenv("DOGCLUB_PDF_REVIEW_DIR"); dir != "" {
			if err = os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, kind+".pdf"), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestFileTypesNamesAndDates(t *testing.T) {
	data, err := render(sourceSnapshot{Document: Input{Issuer: "Клуб"}}, "dog_card")
	if err != nil {
		t.Fatal(err)
	}
	if err = validateFile("копия.pdf", "application/pdf", data); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, media string
		data        []byte
	}{{"../copy.pdf", "application/pdf", data}, {"copy.exe", "application/pdf", data}, {"copy.pdf", "application/pdf", []byte("not PDF")}, {"image.png", "image/png", data}, {"copy.pdf", "text/html", data}} {
		if validateFile(tc.name, tc.media, tc.data) == nil {
			t.Fatal("unsafe file accepted")
		}
	}
	in := Input{TypeID: 1, DogID: ptr(int32(1)), IssueDate: ptr("2026-02-30")}
	if validate(&in) == nil {
		t.Fatal("impossible date accepted")
	}
	in.IssueDate = ptr("2026-10-17")
	in.ExpiresOn = ptr("2026-10-16")
	if validate(&in) == nil {
		t.Fatal("reversed validity accepted")
	}
}

func TestBreedingFormsLongLitterAndV1SourceCompatibility(t *testing.T) {
	unknown := sourceSnapshot{Document: Input{TypeID: 1}, Dog: &dogs.Dog{}}
	data, err := json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"breeding_decision_id", "litter_id", "registration_date", "breeding", "expert_name"} {
		if bytes.Contains(data, []byte(`"`+field+`"`)) {
			t.Fatal("new empty fields would mark old PDF sources stale")
		}
	}
	b := &breeding.DocumentSource{Personal: true, Female: dogs.Summary{Nickname: "Мать", Identifier: dogs.Identifier{Number: "DAM-001"}}, Male: dogs.Summary{Nickname: "Отец", Identifier: dogs.Identifier{Number: "SIRE-001"}}, Litter: &breeding.Litter{ID: 1, BreedingID: 1, DateOfBirth: "2025-01-01", TotalPuppies: 80, IntervalDays: ptr(int32(300)), IntervalViolation: true}}
	for i := range 80 {
		b.Puppies = append(b.Puppies, dogs.Summary{Nickname: fmt.Sprintf("Щенок %d с длинной кличкой для проверки печати", i+1), Identifier: dogs.Identifier{Type: dogs.Dictionary{Name: "Микрочип"}, Number: fmt.Sprintf("LONG-PUPPY-%d", i+1)}})
	}
	data, err = render(sourceSnapshot{Document: Input{Number: "АКТ-001", Issuer: "Клуб"}, Breeding: b, Recipient: &members.EventParticipant{FullName: "Получатель"}}, "litter_act")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(data, []byte("/Type /Page\n")) < 2 {
		t.Fatal("long litter did not paginate")
	}
	if dir := os.Getenv("DOGCLUB_PDF_REVIEW_DIR"); dir != "" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "litter_act_long.pdf"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
