package documents

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
