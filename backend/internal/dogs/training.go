package dogs

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type TrainingPolicy interface {
	Consistent(context.Context, pgx.Tx, Dog, Dog) (bool, error)
}

func (s *Service) SetTrainingPolicy(policy TrainingPolicy) { s.training = policy }
