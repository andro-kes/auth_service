package repo

import (
	"context"

	"github.com/andro-kes/auth/internal/models"
	"github.com/andro-kes/auth/internal/repo/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepo interface {
	Create(ctx context.Context, q db.Querier, user *models.User) error
}

type userRepo struct {
	SelectBuilder *db.SelectBuilder
	UpdateBuilder *db.UpdateBuilder
	InsertBuilder *db.InsertBuilder
	DeleteBuilder *db.DeleteBuilder
}

func NewUserRepo(ctx context.Context, pool *pgxpool.Pool) UserRepo {
	return &userRepo{
		SelectBuilder: db.NewSelectBuilder(ctx, pool),
		UpdateBuilder: db.NewUpdateBuilder(ctx, pool),
		InsertBuilder: db.NewInsertBuilder(ctx, pool),
		DeleteBuilder: db.NewDeleteBuilder(ctx, pool),
	}
}

func (ur *userRepo) Create(ctx context.Context, q db.Querier, user *models.User) error {
	ib := ur.InsertBuilder.
		Into("users").
		Columns("id", "username", "password").
		Values(user.ID, user.Username, user.Password)

	sql, args, err := ib.Build()
	if err != nil {
		return err
	}

	if _, err := q.Exec(ctx, sql, args...); err != nil {
		return err
	}

	return nil
}