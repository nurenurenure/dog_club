//go:build integration

package migrations

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	neturl "net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var seedCounts = map[string]int{
	"dog_status": 2, "breeding_status": 3, "membership_status": 2,
	"dog_breed": 3, "coat_color": 4, "identifier_type": 3, "document_type": 7,
	"event_type": 6, "event_class": 7, "grade": 4, "title": 5, "training_type": 4,
	"contact_type": 2, "address_type": 1, "role": 6, "sex": 2, "disease": 7,
}

func TestPostgresMigrations(t *testing.T) {
	ctx, url, conn := testDatabase(t)
	var output bytes.Buffer
	if err := Run(ctx, "status", url, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "pending") != 8 || scalar(t, ctx, conn,
		`SELECT count(*) FROM pg_tables WHERE schemaname='public'`) != 0 {
		t.Fatal("status on an empty database must list pending migrations without creating tables")
	}
	apply := func(want string) {
		t.Helper()
		output.Reset()
		if err := Run(ctx, "up", url, &output); err != nil {
			t.Fatal(err)
		}
		if output.String() != want {
			t.Fatalf("unexpected migration result: %q", output.String())
		}
	}
	apply("applied 8 migration(s)\n")
	if count := scalar(t, ctx, conn, `SELECT count(*) FROM pg_tables WHERE schemaname='public'`); count != 57 {
		t.Fatalf("expected 56 application tables and Goose history, got %d", count)
	}
	if count := scalar(t, ctx, conn, `SELECT count(*) FROM pg_constraint c JOIN pg_namespace n
		ON n.oid=c.connamespace WHERE n.nspname='public' AND c.contype='f'`); count != 99 {
		t.Fatalf("expected 80 previous and 19 event FK, got %d", count)
	}
	checkSeeds(t, ctx, conn)
	apply("applied 0 migration(s)\n")
	output.Reset()
	if err := Run(ctx, "status", url, &output); err != nil || strings.Count(output.String(), "applied") != 8 {
		t.Fatalf("expected eight applied versions: %v %q", err, output.String())
	}

	t.Run("constraints and deletion policies", func(t *testing.T) {
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		fixture := `INSERT INTO club_member (club_member_id, club_member_full_name, membership_status_id)
			VALUES (1001, 'Тестовый участник', 1);
		INSERT INTO dog (dog_id, dog_nickname, dog_date_of_birth, dog_identifier_number,
			dog_status_id, identifier_type_id, dog_breed_id, coat_color_id, sex_id) VALUES
			(1001, 'Тест 1', '2020-01-01', 'CHECK-ONE', 1, 1, 1, 1, 1),
			(1002, 'Тест 2', '2020-01-01', 'CHECK-TWO', 1, 1, 1, 1, 1),
			(1003, 'Тест 3', '2020-01-01', 'CHECK-THREE', 1, 1, 1, 1, 1),
			(1004, 'Тест 4', '2020-01-01', 'CHECK-FOUR', 1, 1, 1, 1, 1);
		INSERT INTO veterinary_examination (veterinary_examination_id, dog_id,
			veterinary_examination_date, club_member_id) VALUES (1001, 1001, now(), 1001);
		INSERT INTO dog_club_member VALUES (1001,1001);
		INSERT INTO title_dog VALUES (1,1001);
		INSERT INTO veterinary_examination_disease VALUES (1001,1001,1);`
		if _, err := tx.Exec(ctx, fixture); err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct{ name, query, code string }{
			{"duplicate owner", `INSERT INTO dog_club_member VALUES (1001,1001)`, "23505"},
			{"duplicate title", `INSERT INTO title_dog VALUES (1,1001)`, "23505"},
			{"missing identifier", `INSERT INTO dog(dog_nickname,dog_date_of_birth,dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id) VALUES('Missing identifier','2020-01-01',1,1,1,1,1)`, "23502"},
			{"blank identifier", `INSERT INTO dog(dog_nickname,dog_date_of_birth,dog_identifier_number,dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id) VALUES('Blank identifier','2020-01-01',' ',1,1,1,1,1)`, "23514"},
			{"missing parent", `INSERT INTO pedigree(dog_id,pedigree_mother) VALUES(1001,999999)`, "23503"},
			{"self parent", `INSERT INTO pedigree(dog_id,pedigree_mother) VALUES(1001,1001)`, "23514"},
			{"duplicate diagnosis", `INSERT INTO veterinary_examination_disease VALUES (1001,1001,1)`, "23505"},
			{"missing dog", `INSERT INTO dog_club_member VALUES (999999,1001)`, "23503"},
			{"missing member", `INSERT INTO dog_club_member VALUES (1001,999999)`, "23503"},
			{"missing title", `INSERT INTO title_dog VALUES (999999,1001)`, "23503"},
			{"wrong examination dog", `INSERT INTO veterinary_examination_disease VALUES (1001,1002,1)`, "23503"},
			{"missing disease", `INSERT INTO veterinary_examination_disease VALUES (1001,1001,999999)`, "23503"},
			{"referenced breed", `DELETE FROM dog_breed WHERE dog_breed_id=1`, "23001"},
			{"referenced title", `DELETE FROM title WHERE title_id=1`, "23001"},
			{"referenced examiner", `DELETE FROM club_member WHERE club_member_id=1001`, "23001"},
			{"duplicate identifier", `INSERT INTO dog (dog_nickname,dog_date_of_birth,dog_identifier_number,
				dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id)
				VALUES ('Дубликат','2020-01-01','CHECK-ONE',1,1,1,1,1)`, "23505"},
			{"missing dog dictionary", `INSERT INTO dog (dog_nickname,dog_date_of_birth,dog_identifier_number,
				dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id)
				VALUES ('Сирота','2020-01-01','ORPHAN-CHECK',1,1,999999,1,1)`, "23503"},
		} {
			t.Run(test.name, func(t *testing.T) {
				savepoint, err := tx.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer savepoint.Rollback(context.Background())
				_, err = savepoint.Exec(ctx, test.query)
				var postgres *pgconn.PgError
				if !errors.As(err, &postgres) || postgres.Code != test.code {
					t.Fatalf("expected SQLSTATE %s, got %v", test.code, err)
				}
			})
		}
		if _, err := tx.Exec(ctx, `DELETE FROM dog WHERE dog_id=1001`); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"dog_club_member", "title_dog", "veterinary_examination_disease", "veterinary_examination"} {
			var count int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
				t.Fatalf("expected cascaded removal from %s, count=%d, error=%v", table, count, err)
			}
		}
	})

	t.Run("seed sequence values", func(t *testing.T) {
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		for table, count := range seedCounts {
			var id int
			query := fmt.Sprintf("INSERT INTO %s (%s) VALUES ($1) RETURNING %s",
				pgx.Identifier{table}.Sanitize(), pgx.Identifier{table + "_name"}.Sanitize(), pgx.Identifier{table + "_id"}.Sanitize())
			if table == "role" {
				query = `INSERT INTO role(role_name,code) VALUES($1,'test_role') RETURNING role_id`
			}
			if err := tx.QueryRow(ctx, query, "Тест").Scan(&id); err != nil || id <= count {
				t.Fatalf("sequence for %s: id=%d, error=%v", table, id, err)
			}
		}
	})

	t.Run("referenced seeds prevent partial rollback", func(t *testing.T) {
		rollbackTo(t, ctx, url, 2)
		defer func() {
			if err := Run(ctx, "up", url, io.Discard); err != nil {
				t.Error(err)
			}
		}()
		if _, err := conn.Exec(ctx, `INSERT INTO club_member (club_member_id,club_member_full_name,role_id,membership_status_id)
			VALUES (2001,'Тест защиты отката',1,1)`); err != nil {
			t.Fatal(err)
		}
		defer conn.Exec(ctx, `DELETE FROM club_member WHERE club_member_id=2001`)
		err := Run(ctx, "down", url, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "23001") {
			t.Fatalf("expected referenced seed rollback to fail: %v", err)
		}
		checkSeeds(t, ctx, conn)
	})

	for range 8 {
		if err := Run(ctx, "down", url, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	if count := scalar(t, ctx, conn, `SELECT count(*) FROM pg_tables WHERE schemaname='public'`); count != 1 {
		t.Fatalf("full rollback must leave only Goose history, got %d tables", count)
	}
	apply("applied 8 migration(s)\n")
	checkSeeds(t, ctx, conn)
}

func TestBaselineRefusesExistingUnversionedData(t *testing.T) {
	ctx, url, conn := testDatabase(t)
	if _, err := conn.Exec(ctx, `CREATE TABLE existing_data (id integer); INSERT INTO existing_data VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	err := Run(ctx, "up", url, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "baseline refused") {
		t.Fatalf("expected baseline refusal, got %v", err)
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM existing_data`) != 1 ||
		scalar(t, ctx, conn, `SELECT count(*) FROM pg_tables WHERE schemaname='public'`) != 1 {
		t.Fatal("existing schema was changed")
	}
}

func TestMembersUpgradePreservesCardsAndRefusesDestructiveRollback(t *testing.T) {
	ctx, url, conn := testDatabase(t)
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	rollbackTo(t, ctx, url, 3)
	if _, err := conn.Exec(ctx, `INSERT INTO club_member(club_member_full_name,role_id,membership_status_id) VALUES('Legacy card',1,1)`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM club_member WHERE club_member_full_name='Legacy card' AND version=1`) != 1 {
		t.Fatal("upgrade lost the member card")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM information_schema.columns WHERE table_name='club_member' AND column_name='role_id'`) != 0 {
		t.Fatal("legacy role column was not removed")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM users`) != 0 || scalar(t, ctx, conn, `SELECT count(*) FROM user_profiles`) != 0 {
		t.Fatal("legacy role was turned into access privileges")
	}
	rollbackTo(t, ctx, url, 4)
	if err := Run(ctx, "down", url, io.Discard); err == nil {
		t.Fatal("rollback invented a role for an existing card")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM club_member`) != 1 || scalar(t, ctx, conn, `SELECT count(*) FROM member_changes`) != 0 {
		t.Fatal("failed rollback changed the member schema or data")
	}
}

func TestDogsUpgradeRefusesMissingIdentifiersAndKeepsData(t *testing.T) {
	ctx, url, conn := testDatabase(t)
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	rollbackTo(t, ctx, url, 4)
	if _, err := conn.Exec(ctx, `INSERT INTO dog(dog_nickname,dog_date_of_birth,dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id) VALUES('Legacy dog','2020-01-01',1,1,1,1,1)`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err == nil || !strings.Contains(err.Error(), "23502") {
		t.Fatal("upgrade fabricated an identifier for a legacy dog")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM dog WHERE dog_nickname='Legacy dog' AND dog_identifier_number IS NULL`) != 1 || scalar(t, ctx, conn, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='dog_changes'`) != 0 {
		t.Fatal("failed dogs upgrade changed data or partially applied schema")
	}
	if _, err := conn.Exec(ctx, `UPDATE dog SET dog_identifier_number='LEGACY-0001' WHERE dog_nickname='Legacy dog'`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM dog WHERE dog_nickname='Legacy dog' AND dog_identifier_number='LEGACY-0001' AND version=1`) != 1 {
		t.Fatal("upgrade lost a corrected legacy card")
	}
	rollbackTo(t, ctx, url, 5)
	if err := Run(ctx, "down", url, io.Discard); err == nil {
		t.Fatal("rollback silently dropped added dog data")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM dog`) != 1 || scalar(t, ctx, conn, `SELECT count(*) FROM dog_changes`) != 0 {
		t.Fatal("refused dogs rollback changed schema or data")
	}
}

func TestDogsUpgradeRejectsOrphanParentsAndDuplicatePedigrees(t *testing.T) {
	ctx, url, conn := testDatabase(t)
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	rollbackTo(t, ctx, url, 4)
	if _, err := conn.Exec(ctx, `INSERT INTO dog(dog_id,dog_nickname,dog_date_of_birth,dog_identifier_number,dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id) VALUES(101,'Legacy dog','2020-01-01','OLD-101',1,1,1,1,1); INSERT INTO pedigree(dog_id,pedigree_mother,pedigree_father) VALUES(101,9999,9998)`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err == nil || !strings.Contains(err.Error(), "23503") {
		t.Fatal("upgrade accepted missing parents")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM pedigree`) != 1 {
		t.Fatal("upgrade removed invalid pedigree instead of refusing")
	}
	if _, err := conn.Exec(ctx, `DELETE FROM pedigree; INSERT INTO pedigree(dog_id,pedigree_mother,pedigree_father) VALUES(101,101,101),(101,101,101)`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err == nil || !strings.Contains(err.Error(), "23505") {
		t.Fatal("upgrade accepted multiple pedigrees for one dog")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM pedigree`) != 2 {
		t.Fatal("upgrade silently discarded duplicate pedigree data")
	}
}

func TestVeterinaryUpgradePreservesUnknownLegacyFacts(t *testing.T) {
	ctx, url, conn := testDatabase(t)
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	rollbackTo(t, ctx, url, 5)
	_, err := conn.Exec(ctx, `INSERT INTO club_member(club_member_id,club_member_full_name,membership_status_id) VALUES(301,'Legacy physician',1);
	INSERT INTO dog(dog_id,dog_nickname,dog_date_of_birth,dog_identifier_number,dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id) VALUES(301,'Legacy patient','2020-01-01','LEGACY-MED-301',1,1,1,1,1);
	INSERT INTO vaccine_type(vaccine_type_id,vaccine_type_name,validity_period) VALUES(301,'Legacy product',365);
	INSERT INTO vaccination(vaccination_id,dog_id,vaccination_date,vaccination_next_date,vaccine_type_id) VALUES(301,301,'2024-01-01 12:34:56.123456','2024-12-01',301);
	INSERT INTO veterinary_examination(veterinary_examination_id,dog_id,veterinary_examination_date,veterinary_examination_result,club_member_id) VALUES(301,301,'2024-01-02 15:16:17.123456','Legacy result',301);
	INSERT INTO veterinary_examination_disease VALUES(301,301,1);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	queries := []string{
		`SELECT count(*) FROM vaccine_type WHERE vaccine_type_id=301 AND validity_period=365 AND validity_period_unit IS NULL AND revaccination_period IS NULL`,
		`SELECT count(*) FROM vaccination WHERE vaccination_id=301 AND vaccination_date='2024-01-01 12:34:56.123456' AND vaccination_next_date='2024-12-01' AND vaccination_valid_until IS NULL AND recorded_by_user_id IS NULL AND vaccine_snapshot IS NULL AND version=1 AND NOT is_void`,
		`SELECT count(*) FROM veterinary_examination WHERE veterinary_examination_id=301 AND veterinary_examination_date='2024-01-02 15:16:17.123456' AND veterinary_examination_result='Legacy result' AND club_member_id=301 AND recorded_by_user_id IS NULL`,
		`SELECT count(*) FROM veterinary_examination_disease WHERE veterinary_examination_id=301 AND is_hereditary IS NULL`,
	}
	for _, query := range queries {
		if scalar(t, ctx, conn, query) != 1 {
			t.Fatal("upgrade changed legacy clinical facts or fabricated metadata")
		}
	}
	rollbackTo(t, ctx, url, 6)
	if err := Run(ctx, "down", url, io.Discard); err == nil {
		t.Fatal("down silently removed clinical fields")
	}
	for _, query := range queries {
		if scalar(t, ctx, conn, query) != 1 {
			t.Fatal("refused rollback changed clinical data")
		}
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM veterinary_changes`) != 0 {
		t.Fatal("migration invented clinical authors/history")
	}
}

func TestBreedingUpgradePreservesLegacyFactsAndRefusesAmbiguousIDs(t *testing.T) {
	ctx, url, conn := testDatabase(t)
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	rollbackTo(t, ctx, url, 6)
	_, err := conn.Exec(ctx, `INSERT INTO dog(dog_id,dog_nickname,dog_date_of_birth,dog_identifier_number,dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id) VALUES
	(401,'Legacy female','2020-01-01','BREED-401',1,1,1,1,1),
	(402,'Legacy male','2020-01-01','BREED-402',1,1,1,1,2),
	(403,'Legacy other female','2020-01-01','BREED-403',1,1,1,1,1);
	INSERT INTO breeding(breeding_id,female_dog_id,male_dog_id,breeding_date,breeding_number_of_puppies,breeding_status_id) VALUES
	(401,401,402,'2024-01-01 12:34:56.123456',4,2),(401,403,402,NULL,NULL,1);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err == nil || !strings.Contains(err.Error(), "23505") {
		t.Fatal("migration accepted ambiguous breeding IDs")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM breeding`) != 2 || scalar(t, ctx, conn, `SELECT count(*) FROM information_schema.tables WHERE table_name='litter'`) != 0 {
		t.Fatal("failed upgrade changed legacy rows/schema")
	}
	if _, err := conn.Exec(ctx, `DELETE FROM breeding WHERE female_dog_id=403`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM breeding WHERE breeding_id=401 AND breeding_date='2024-01-01 12:34:56.123456' AND breeding_number_of_puppies=4 AND request_id IS NULL AND outcome='unknown' AND assessment IS NULL`) != 1 || scalar(t, ctx, conn, `SELECT count(*) FROM litter`) != 0 || scalar(t, ctx, conn, `SELECT count(*) FROM breeding_request`) != 0 {
		t.Fatal("migration invented litter/date/decision or changed legacy facts")
	}
	if _, err := conn.Exec(ctx, `INSERT INTO breeding_request(female_dog_id,male_dog_id,planned_date) VALUES(401,402,'2025-01-01')`); err != nil {
		t.Fatal(err)
	}
	rollbackTo(t, ctx, url, 7)
	if err := Run(ctx, "down", url, io.Discard); err == nil {
		t.Fatal("rollback removed new breeding information")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM breeding_request`) != 1 {
		t.Fatal("failed rollback lost request")
	}
}

func TestEventsUpgradePreservesDatesAddressesAndResults(t *testing.T) {
	ctx, url, conn := testDatabase(t)
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	rollbackTo(t, ctx, url, 7)
	_, err := conn.Exec(ctx, `INSERT INTO club_member(club_member_id,club_member_full_name,membership_status_id) VALUES(501,'Legacy event participant',1);
    INSERT INTO address(address_id,club_member_id,address_type_id,address_locality,address_street,address_building_number,address_apartment_number) VALUES(501,501,1,'Legacy locality','Legacy street','12','3');
    INSERT INTO dog(dog_id,dog_nickname,dog_date_of_birth,dog_identifier_number,dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id) VALUES
    (501,'Legacy attendee','2020-01-01','EVENT-501',1,1,1,1,1),(502,'Legacy other','2020-01-01','EVENT-502',1,1,1,1,1);
    INSERT INTO event(event_id,event_name,event_start_date,event_end_date,address_id,event_type_id,event_class_id) VALUES(501,'Legacy show','2024-01-01 12:34:56.123456','2024-01-02 15:16:17.123456',501,1,3);
    INSERT INTO achievement(achievement_id,event_id,dog_id,achievement_award_date,achievement_judge_full_name,grade_id) VALUES(501,501,501,'2024-01-02','Legacy judge',1),(501,501,502,'2024-01-02','Other judge',2);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err == nil || !strings.Contains(err.Error(), "23505") {
		t.Fatal("upgrade accepted ambiguous achievement IDs")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM achievement`) != 2 || scalar(t, ctx, conn, `SELECT count(*) FROM information_schema.tables WHERE table_name='event_venue'`) != 0 {
		t.Fatal("failed upgrade changed source data")
	}
	if _, err := conn.Exec(ctx, `DELETE FROM achievement WHERE dog_id=502`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	checks := []string{
		`SELECT count(*) FROM event WHERE event_id=501 AND event_start_date='2024-01-01 12:34:56.123456' AND event_end_date='2024-01-02 15:16:17.123456' AND address_id=501 AND event_class_id=3 AND starts_at IS NULL AND ends_at IS NULL AND timezone IS NULL AND audience IS NULL AND status='draft' AND version=1 AND rules_version=1`,
		`SELECT count(*) FROM event_venue WHERE legacy_address_id=501 AND locality='Legacy locality' AND street='Legacy street' AND building_number='12' AND room='3'`,
		`SELECT count(*) FROM event_class_rule WHERE event_id=501 AND catalog_class_id=3 AND min_age_months IS NULL AND max_age_months IS NULL AND NOT requires_expert`,
		`SELECT count(*) FROM achievement WHERE achievement_id=501 AND dog_id=501 AND achievement_award_date='2024-01-02' AND achievement_judge_full_name='Legacy judge' AND grade_id=1 AND registration_id IS NULL AND recorded_by_profile_id IS NULL AND entry_snapshot IS NULL`,
		`SELECT count(*) FROM address WHERE address_id=501 AND club_member_id=501 AND address_locality='Legacy locality'`,
	}
	for _, query := range checks {
		if scalar(t, ctx, conn, query) != 1 {
			t.Fatal("upgrade invented timezone, age, judge account or changed old facts")
		}
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM event_registration`) != 0 || scalar(t, ctx, conn, `SELECT count(*) FROM event_changes`) != 0 {
		t.Fatal("upgrade invented participation/history")
	}
	if err := Run(ctx, "down", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM achievement WHERE achievement_id=501 AND achievement_judge_full_name='Legacy judge' AND grade_id=1`) != 1 {
		t.Fatal("rollback changed old result")
	}
	if err := Run(ctx, "up", url, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO event_venue(name,locality) VALUES('New venue','New locality')`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "down", url, io.Discard); err == nil {
		t.Fatal("rollback discarded a new venue")
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM event_venue`) != 2 {
		t.Fatal("failed rollback lost data")
	}
}

func checkSeeds(t *testing.T, ctx context.Context, conn *pgx.Conn) {
	t.Helper()
	for table, count := range seedCounts {
		if got := scalar(t, ctx, conn, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()); got != count {
			t.Errorf("%s: expected %d seed rows, got %d", table, count, got)
		}
	}
	if scalar(t, ctx, conn, `SELECT count(*) FROM vaccine_type`) != 0 {
		t.Error("unapproved vaccine products were inserted")
	}
}

func scalar(t *testing.T, ctx context.Context, conn *pgx.Conn, query string) int {
	t.Helper()
	var count int
	if err := conn.QueryRow(ctx, query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// The supplied URL is used only to create and remove fresh random test databases.
// No tables in the supplied database are changed, and no existing database is dropped.
func testDatabase(t *testing.T) (context.Context, string, *pgx.Conn) {
	t.Helper()
	url := os.Getenv("DOGCLUB_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("DOGCLUB_TEST_DATABASE_URL is required for -tags=integration; the role must have CREATEDB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal("cannot connect to integration PostgreSQL")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	name := "dogclub_test_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal("cannot create isolated integration database")
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Error("cannot remove isolated integration database")
		}
	})
	target, err := neturl.Parse(url)
	if err != nil || (target.Scheme != "postgres" && target.Scheme != "postgresql") {
		t.Fatal("integration configuration must be a postgres:// or postgresql:// URL")
	}
	target.Path = "/" + name
	target.RawPath = ""
	query := target.Query()
	query.Del("database")
	query.Del("dbname")
	target.RawQuery = query.Encode()
	isolatedURL := target.String()
	conn, err := pgx.Connect(ctx, isolatedURL)
	if err != nil {
		t.Fatal("cannot connect to isolated integration database")
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	var connected string
	if err := conn.QueryRow(ctx, `SELECT current_database()`).Scan(&connected); err != nil || connected != name {
		t.Fatal("integration connection is not isolated; refusing to run migrations")
	}
	return ctx, isolatedURL, conn
}

// Reduce to a known prior schema without changing earlier migration files.
func rollbackTo(t *testing.T, ctx context.Context, url string, target int) {
	t.Helper()
	for {
		var output bytes.Buffer
		if err := Run(ctx, "status", url, &output); err != nil {
			t.Fatal(err)
		}
		if strings.Count(output.String(), "applied") <= target {
			return
		}
		if err := Run(ctx, "down", url, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
}
