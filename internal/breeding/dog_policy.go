package breeding

import (
	"context"
	"errors"

	"dogclub.local/backend/internal/dogs"

	"github.com/jackc/pgx/v5"
)

// DogPolicy reads only breeding-owned tables; dogs calls it in the same locked
// transaction after an edit, rolling back both card and history on conflict.
type DogPolicy struct{}

func (DogPolicy) Consistent(ctx context.Context, tx pgx.Tx, before, after dogs.Dog) (bool, error) {
	return (&repository{tx}).consistentDog(ctx, before, after)
}
func (q *repository) consistentDog(ctx context.Context, before, after dogs.Dog) (bool, error) {
	var parent, wrongDates, wrongSex bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM breeding b JOIN breeding_status s USING(breeding_status_id) WHERE s.breeding_status_name='Проведена' AND (b.female_dog_id=$1 OR b.male_dog_id=$1)),EXISTS(SELECT 1 FROM breeding b JOIN breeding_status s USING(breeding_status_id) WHERE s.breeding_status_name='Проведена' AND (b.female_dog_id=$1 OR b.male_dog_id=$1) AND b.breeding_date::date<=$2::date),EXISTS(SELECT 1 FROM breeding b JOIN breeding_status s USING(breeding_status_id) WHERE s.breeding_status_name='Проведена' AND ((b.female_dog_id=$1 AND $3::text<>'Сука') OR (b.male_dog_id=$1 AND $3::text<>'Кобель')))`, after.ID, after.DateOfBirth, after.Sex.Name).Scan(&parent, &wrongDates, &wrongSex)
	if err != nil {
		return false, storageError(err)
	}
	if before.Breed.ID != after.Breed.ID && parent {
		return false, nil
	}
	if before.DateOfBirth != after.DateOfBirth && wrongDates {
		return false, nil
	}
	if before.Sex.ID != after.Sex.ID && wrongSex {
		return false, nil
	}
	var birth string
	var mother, father int32
	err = q.tx.QueryRow(ctx, `SELECT to_char(l.date_of_birth,'YYYY-MM-DD'),b.female_dog_id,b.male_dog_id FROM litter_dog ld JOIN litter l USING(litter_id) JOIN breeding b USING(breeding_id) WHERE ld.dog_id=$1`, after.ID).Scan(&birth, &mother, &father)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, storageError(err)
	}
	p := after.Pedigree
	return after.DateOfBirth == birth && before.Breed.ID == after.Breed.ID && p.Mother != nil && p.Mother.DogID != nil && *p.Mother.DogID == mother && p.Father != nil && p.Father.DogID != nil && *p.Father.DogID == father, nil
}
