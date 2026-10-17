package training

import (
	"context"

	"dogclub.local/backend/internal/dogs"

	"github.com/jackc/pgx/v5"
)

type DogPolicy struct{}

func (DogPolicy) Consistent(ctx context.Context, tx pgx.Tx, before, after dogs.Dog) (bool, error) {
	if before.DateOfBirth == after.DateOfBirth {
		return true, nil
	}
	return (&repository{tx}).datesConsistent(ctx, after.ID, after.DateOfBirth)
}
