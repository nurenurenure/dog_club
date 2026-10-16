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
	if strings.Count(output.String(), "pending") != 3 || scalar(t, ctx, conn,
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
	apply("applied 3 migration(s)\n")
	if count := scalar(t, ctx, conn, `SELECT count(*) FROM pg_tables WHERE schemaname='public'`); count != 41 {
		t.Fatalf("expected 40 application tables and Goose history, got %d", count)
	}
	if count := scalar(t, ctx, conn, `SELECT count(*) FROM pg_constraint c JOIN pg_namespace n
		ON n.oid=c.connamespace WHERE n.nspname='public' AND c.contype='f'`); count != 49 {
		t.Fatalf("expected 37 domain and 12 auth FK, got %d", count)
	}
	checkSeeds(t, ctx, conn)
	apply("applied 0 migration(s)\n")
	output.Reset()
	if err := Run(ctx, "status", url, &output); err != nil || strings.Count(output.String(), "applied") != 3 {
		t.Fatalf("expected three applied versions: %v %q", err, output.String())
	}

	t.Run("constraints and deletion policies", func(t *testing.T) {
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		fixture := `INSERT INTO club_member (club_member_id, club_member_full_name, role_id, membership_status_id)
			VALUES (1001, 'Тестовый участник', 1, 1);
		INSERT INTO dog (dog_id, dog_nickname, dog_date_of_birth, dog_identifier_number,
			dog_status_id, identifier_type_id, dog_breed_id, coat_color_id, sex_id) VALUES
			(1001, 'Тест 1', '2020-01-01', 'CHECK-ONE', 1, 1, 1, 1, 1),
			(1002, 'Тест 2', '2020-01-01', 'CHECK-TWO', 1, 1, 1, 1, 1),
			(1003, 'Тест 3', '2020-01-01', NULL, 1, 1, 1, 1, 1),
			(1004, 'Тест 4', '2020-01-01', NULL, 1, 1, 1, 1, 1);
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
			{"missing dog dictionary", `INSERT INTO dog (dog_nickname,dog_date_of_birth,
				dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id)
				VALUES ('Сирота','2020-01-01',1,1,999999,1,1)`, "23503"},
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
		// Roll back auth in this isolated database before attempting the seed down.
		if err := Run(ctx, "down", url, io.Discard); err != nil {
			t.Fatal(err)
		}
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

	for range 3 {
		if err := Run(ctx, "down", url, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	if count := scalar(t, ctx, conn, `SELECT count(*) FROM pg_tables WHERE schemaname='public'`); count != 1 {
		t.Fatalf("full rollback must leave only Goose history, got %d tables", count)
	}
	apply("applied 3 migration(s)\n")
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
