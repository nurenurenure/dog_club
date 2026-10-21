//go:build integration

package dogs

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/filestore"
)

func TestPostgresDogPhotos(t *testing.T) {
	ctx, pool := isolatedDogsDB(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	expect := func(err, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("expected %v, got %v", want, err)
		}
	}
	validation := func(err error) {
		t.Helper()
		var v *ValidationError
		if !errors.As(err, &v) {
			t.Fatalf("expected validation failure, got %v", err)
		}
	}
	mail := new(testMail)
	a, err := auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "dogs-test", Audience: "backend", IOTimeout: 5 * time.Second})
	must(err)
	m := members.NewService(a)
	s := NewService(a, m)
	dir := filepath.Join(t.TempDir(), "files")
	files, err := filestore.New(dir)
	must(err)
	t.Cleanup(func() { _ = files.Close() })
	s.SetPhotoStore(files)

	password := "Dogs integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@dogs.test")
	must(err)
	must(a.Activate(ctx, mail.code(t), password))
	login := func(name, profile string) auth.Principal {
		t.Helper()
		c, err := a.Login(ctx, name, password, profile)
		must(err)
		p, err := a.Authenticate(ctx, c.AccessToken)
		must(err)
		return p
	}
	admin := login("admin", "")
	profile := func(role string) auth.Principal {
		t.Helper()
		created, err := a.AddProfile(ctx, admin, admin.User.ID, role)
		must(err)
		return login("admin", created.ID)
	}
	staff, leader, vet, expert := profile("staff"), profile("leader"), profile("veterinarian"), profile("expert")
	md, err := m.Dictionaries(ctx, staff)
	must(err)
	person, err := m.Create(ctx, staff, members.CreateInput{FullName: "Photo Owner", MembershipStatusID: md.MembershipStatuses[0].ID})
	must(err)
	other, err := m.Create(ctx, staff, members.CreateInput{FullName: "Other Owner", MembershipStatusID: md.MembershipStatuses[0].ID})
	must(err)
	owner := func(name string, memberID int32) auth.Principal {
		t.Helper()
		_, err := a.Invite(ctx, admin, auth.InviteInput{Login: name, Email: name + "@dogs.test", Role: "owner", ClubMemberID: &memberID})
		must(err)
		must(a.Activate(ctx, mail.code(t), password))
		return login(name, "")
	}
	anna, boris := owner("anna", person.ID), owner("boris", other.ID)
	d, err := s.Dictionaries(ctx, staff)
	must(err)
	dog, err := s.Create(ctx, staff, CreateInput{Nickname: "Бублик", DateOfBirth: "2020-01-01", Identifier: IdentifierInput{TypeID: d.IdentifierTypes[0].ID, Number: "PHOTO-1"},
		StatusID: d.Statuses[0].ID, BreedID: d.Breeds[0].ID, CoatColorID: d.CoatColors[0].ID, SexID: d.Sexes[0].ID, OwnerIDs: []int32{person.ID}})
	must(err)

	picture := func(w, h int) []byte {
		t.Helper()
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		img.Set(0, 0, color.RGBA{R: 200, A: 255})
		var buf bytes.Buffer
		must(png.Encode(&buf, img))
		return buf.Bytes()
	}
	stored := func() int {
		t.Helper()
		entries, err := os.ReadDir(dir)
		must(err)
		return len(entries)
	}

	// Only staff uploads; the declared type must match the bytes.
	first := picture(4, 3)
	_, err = s.SetPhoto(ctx, anna, dog.ID, 0, "image/png", first)
	expect(err, auth.ErrForbidden)
	_, err = s.SetPhoto(ctx, leader, dog.ID, 0, "image/png", first)
	expect(err, auth.ErrForbidden)
	_, err = s.SetPhoto(ctx, staff, dog.ID, 0, "image/jpeg", first)
	validation(err)
	_, err = s.SetPhoto(ctx, staff, dog.ID, 0, "image/png", []byte("not a picture"))
	validation(err)
	_, err = s.SetPhoto(ctx, staff, dog.ID, 0, "image/png", make([]byte, MaxPhotoSize+1))
	validation(err)
	_, err = s.Photo(ctx, staff, dog.ID)
	expect(err, ErrNotFound)
	if stored() != 0 {
		t.Fatal("refused uploads left files behind")
	}

	photo, err := s.SetPhoto(ctx, staff, dog.ID, 0, "image/png", first)
	must(err)
	if photo.Version != 1 || photo.Width != 4 || photo.Height != 3 || photo.SizeBytes != int64(len(first)) {
		t.Fatalf("unexpected photo %+v", photo)
	}

	// The own owner, staff, leader, the vet and the expert see it; another owner does not.
	for _, p := range []auth.Principal{anna, staff, leader, vet, expert} {
		meta, data, err := s.PhotoFile(ctx, p, dog.ID)
		must(err)
		if meta.Version != 1 || !bytes.Equal(data, first) {
			t.Fatal("photo bytes differ")
		}
	}
	_, _, err = s.PhotoFile(ctx, boris, dog.ID)
	expect(err, ErrNotFound)

	// A stale version is refused; a replacement removes the old bytes.
	second := picture(2, 2)
	_, err = s.SetPhoto(ctx, staff, dog.ID, 0, "image/png", second)
	expect(err, ErrConflict)
	photo, err = s.SetPhoto(ctx, staff, dog.ID, 1, "image/png", second)
	must(err)
	if photo.Version != 2 || stored() != 1 {
		t.Fatalf("replacement kept stale state: version %d, files %d", photo.Version, stored())
	}

	// Removal needs the version and a reason; versions keep growing afterwards.
	validation(s.RemovePhoto(ctx, staff, dog.ID, PhotoRemoval{Version: 2}))
	expect(s.RemovePhoto(ctx, staff, dog.ID, PhotoRemoval{Version: 1, Reason: "Чужая собака"}), ErrConflict)
	must(s.RemovePhoto(ctx, staff, dog.ID, PhotoRemoval{Version: 2, Reason: "Чужая собака"}))
	_, err = s.Photo(ctx, anna, dog.ID)
	expect(err, ErrNotFound)
	if stored() != 0 {
		t.Fatal("removed photo bytes remain")
	}
	photo, err = s.SetPhoto(ctx, staff, dog.ID, 0, "image/png", first)
	must(err)
	if photo.Version != 4 {
		t.Fatalf("a new photo after removal reused an old version: %d", photo.Version)
	}

	// The card version is untouched: photos do not stale fingerprints or snapshots.
	card, err := s.Get(ctx, staff, dog.ID)
	must(err)
	if card.Version != dog.Version {
		t.Fatalf("photo changed the card version: %d -> %d", dog.Version, card.Version)
	}
	history, err := s.History(ctx, staff, dog.ID, 0, 50)
	must(err)
	actions := []string{}
	for _, change := range history.Changes {
		actions = append(actions, change.Action)
	}
	if got := strings.Join(actions, ","); got != "dog_created,photo_set,photo_set,photo_removed,photo_set" {
		t.Fatalf("unexpected history %s", got)
	}
}
