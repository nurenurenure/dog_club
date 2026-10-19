//go:build integration

package breeding

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/veterinary"
)

// TestPostgresLab3BreedingRules covers lab 3 items 9, 12, 15–16 and 18.
func TestPostgresLab3BreedingRules(t *testing.T) {
	ctx, pool := isolatedBreedingDB(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	mail := new(testMail)
	a, err := auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "breeding-test", Audience: "backend", IOTimeout: 10 * time.Second})
	must(err)
	m := members.NewService(a)
	d := dogs.NewService(a, m, veterinary.DogDatesPolicy{})
	d.SetBreedingPolicy(DogPolicy{})
	v := veterinary.NewService(a, d, m)
	s := NewService(a, d, v)
	s.now = func() time.Time { return time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC) }
	password := "Breeding integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@breeding.test")
	must(err)
	must(a.Activate(ctx, mail.code(t), password))
	login := func(profile string) auth.Principal {
		t.Helper()
		c, err := a.Login(ctx, "admin", password, profile)
		must(err)
		p, err := a.Authenticate(ctx, c.AccessToken)
		must(err)
		return p
	}
	admin := login("")
	roles := map[string]auth.Principal{}
	for _, role := range []string{"staff", "expert", "veterinarian"} {
		profile, err := a.AddProfile(ctx, admin, admin.User.ID, role)
		must(err)
		roles[role] = login(profile.ID)
	}
	staff, expert := roles["staff"], roles["expert"]
	md, err := m.Dictionaries(ctx, staff)
	must(err)
	person, err := m.Create(ctx, staff, members.CreateInput{FullName: "Lab owner", MembershipStatusID: md.MembershipStatuses[0].ID})
	must(err)
	must(a.BindMember(ctx, admin, admin.User.ID, person.ID))
	vet := login(roles["veterinarian"].Profile.ID)
	dict, err := d.Dictionaries(ctx, staff)
	must(err)
	sexes, statuses := map[string]int32{}, map[string]int32{}
	for _, x := range dict.Sexes {
		sexes[x.Name] = x.ID
	}
	for _, x := range dict.Statuses {
		statuses[x.Name] = x.ID
		if x.IsActive != (x.Name == "На учёте") {
			t.Fatalf("status %q has is_active=%v", x.Name, x.IsActive)
		}
	}
	for _, name := range []string{"Снята с учёта", "Умерла", "Потеряна", "Исключена"} {
		if statuses[name] == 0 {
			t.Fatalf("status %q is missing", name)
		}
	}
	count := 0
	dog := func(sex, birth string, breed int32) dogs.Dog {
		t.Helper()
		count++
		number := "LAB3-" + string(rune('A'+count/26)) + string(rune('A'+count%26))
		out, err := d.Create(ctx, staff, dogs.CreateInput{Nickname: number, DateOfBirth: birth, Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: number}, StatusID: statuses["На учёте"], BreedID: breed, CoatColorID: dict.CoatColors[0].ID, SexID: sexes[sex], OwnerIDs: []int32{person.ID}})
		must(err)
		return out
	}
	breed := dict.Breeds[0].ID
	female := func(birth string) dogs.Dog { return dog("Сука", birth, breed) }
	male := func(birth string) dogs.Dog { return dog("Кобель", birth, breed) }
	parent := func(x *dogs.Dog) *dogs.ParentInput {
		if x == nil {
			return &dogs.ParentInput{External: &dogs.ExternalParent{Nickname: "Внешний предок", PedigreeNumber: "EXT-1", Issuer: "Клуб"}}
		}
		return &dogs.ParentInput{DogID: &x.ID}
	}
	pedigree := func(child *dogs.Dog, mother, father *dogs.ParentInput) {
		t.Helper()
		out, err := d.SetPedigree(ctx, staff, child.ID, dogs.PedigreeInput{Version: child.Version, Mother: mother, Father: father})
		must(err)
		*child = out
	}
	check := func(f, mm dogs.Dog) Assessment {
		t.Helper()
		r, err := s.CreateRequest(ctx, staff, RequestInput{FemaleID: f.ID, MaleID: mm.ID, PlannedDate: "2026-01-15"})
		must(err)
		out, err := s.Check(ctx, expert, r.ID)
		must(err)
		_, err = s.Cancel(ctx, staff, r.ID, Correction{r.Version, "Проверка правила"})
		must(err)
		return out
	}
	kin := []string{"parent_child", "shared_parent", "common_ancestor_3_generations"}
	expectKin := func(a Assessment, want string) {
		t.Helper()
		for _, reason := range kin {
			if slices.Contains(a.BlockingReasons, reason) != (reason == want) {
				t.Fatalf("want kinship %q, got %v", want, a.BlockingReasons)
			}
		}
	}

	// Founders P×Q, their children S1 and S2, grandchildren C1 (S1×X) and C2 (Y×S2).
	p, q := male("2016-06-01"), female("2016-06-01")
	x, y := male("2016-06-01"), female("2016-06-01")
	s1, s2 := female("2019-03-01"), male("2019-03-01")
	pedigree(&s1, parent(&q), parent(&p))
	pedigree(&s2, parent(&q), parent(&p))
	c1, c2 := female("2021-06-01"), male("2021-06-01")
	pedigree(&c1, parent(&s1), parent(&x))
	pedigree(&c2, parent(&y), parent(&s2))
	unrelated := female("2019-03-01")
	expectKin(check(s1, p), "parent_child")
	expectKin(check(s1, s2), "shared_parent")
	expectKin(check(c1, c2), "common_ancestor_3_generations")
	expectKin(check(c1, p), "common_ancestor_3_generations")
	expectKin(check(unrelated, s2), "")

	// A pedigree is known to three generations only with card parents and
	// grandparents; great-grandparents may be external records.
	grand := []dogs.Dog{female("2012-01-01"), male("2012-01-01"), female("2012-01-01"), male("2012-01-01")}
	for i := range grand {
		pedigree(&grand[i], parent(nil), parent(nil))
	}
	mother, father := female("2016-01-01"), male("2016-01-01")
	pedigree(&mother, parent(&grand[0]), parent(&grand[1]))
	pedigree(&father, parent(&grand[2]), parent(&grand[3]))
	complete := female("2019-01-01")
	pedigree(&complete, parent(&mother), parent(&father))
	known := func(a Assessment) bool { return a.Female.PedigreeKnown3 != nil && *a.Female.PedigreeKnown3 }
	if !known(check(complete, s2)) {
		t.Fatal("complete three-generation pedigree reported unknown")
	}
	pedigree(&grand[3], parent(nil), nil)
	if known(check(complete, s2)) {
		t.Fatal("missing great-grandparent not detected")
	}
	if known(check(c1, s2)) {
		t.Fatal("grandparent without own pedigree not detected")
	}
	// A recorded pedigree document is accepted without cards of every ancestor.
	c1, err = d.SetPedigree(ctx, staff, c1.ID, dogs.PedigreeInput{Version: c1.Version, Number: "LAB3-PEDIGREE-1", Issuer: "Кинологическая организация", Mother: parent(&s1), Father: parent(&x)})
	must(err)
	if !known(check(c1, s2)) {
		t.Fatal("pedigree document not accepted")
	}

	// Inactive partners are blocked separately and the expert cannot approve.
	lost := male("2019-03-01")
	lost, err = d.Patch(ctx, staff, lost.ID, dogs.PatchInput{Version: lost.Version, StatusID: dogs.Field[int32]{Set: true, Value: ptr(statuses["Потеряна"])}})
	must(err)
	inactive := check(unrelated, lost)
	if !slices.Contains(inactive.BlockingReasons, "male_inactive") || slices.Contains(inactive.BlockingReasons, "female_inactive") {
		t.Fatalf("inactive male not blocked: %v", inactive.BlockingReasons)
	}
	r, err := s.CreateRequest(ctx, staff, RequestInput{FemaleID: unrelated.ID, MaleID: lost.ID, PlannedDate: "2026-01-15"})
	must(err)
	_, err = s.Decide(ctx, expert, r.ID, DecisionInput{Correction: Correction{r.Version, "Пара проверена"}, Decision: "approved"})
	var invalidErr *ValidationError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("approval of an inactive partner: %v", err)
	}

	// Mate suggestion: opposite sex, same breed, active, 24–120 months, no
	// confirmed hereditary diagnosis and no close kinship.
	young, old := male("2024-06-01"), male("2014-01-01")
	otherBreed := dog("Кобель", "2019-03-01", dict.Breeds[1].ID)
	sick := male("2019-03-01")
	_, err = v.CreateExamination(ctx, vet, sick.ID, veterinary.ExaminationInput{Date: "2025-01-10", Result: "Наследственный диагноз", Diagnoses: []veterinary.DiagnosisInput{{DiseaseID: 1, IsHereditary: ptr(true)}}})
	must(err)
	suspected := male("2019-03-01")
	_, err = v.CreateExamination(ctx, vet, suspected.ID, veterinary.ExaminationInput{Date: "2025-01-10", Result: "Не уточнено", Diagnoses: []veterinary.DiagnosisInput{{DiseaseID: 1}}})
	must(err)
	ids := func(target dogs.Dog) []int32 {
		t.Helper()
		out, err := s.MateCandidates(ctx, expert, target.ID)
		must(err)
		if out.Truncated || out.AsOf != "2025-12-31" {
			t.Fatalf("unexpected suggestion metadata: %+v", out)
		}
		found := []int32{}
		for _, c := range out.Candidates {
			found = append(found, c.ID)
		}
		return found
	}
	got := ids(unrelated)
	for _, want := range []dogs.Dog{p, x, s2, c2, suspected} {
		if !slices.Contains(got, want.ID) {
			t.Fatalf("suitable male %s missing from %v", want.Nickname, got)
		}
	}
	for _, unwanted := range []dogs.Dog{lost, young, old, otherBreed, sick, q, s1} {
		if slices.Contains(got, unwanted.ID) {
			t.Fatalf("unsuitable dog %s suggested", unwanted.Nickname)
		}
	}
	got = ids(p)
	for _, unwanted := range []dogs.Dog{s1, c1} {
		if slices.Contains(got, unwanted.ID) {
			t.Fatalf("relative %s suggested for its ancestor", unwanted.Nickname)
		}
	}
	if !slices.Contains(got, q.ID) || !slices.Contains(got, unrelated.ID) {
		t.Fatalf("unrelated females missing: %v", got)
	}
	_, err = s.MateCandidates(ctx, expert, lost.ID)
	if !errors.As(err, &invalidErr) {
		t.Fatalf("suggestion for an inactive dog: %v", err)
	}
	_, err = s.MateCandidates(ctx, vet, p.ID)
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("veterinarian must not list candidates: %v", err)
	}
}
