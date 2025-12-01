package repo

import (
	"context"
	"errors"

	"github.com/andro-kes/auth_service/internal/autherr"
	"github.com/andro-kes/auth_service/internal/models"
	"github.com/andro-kes/auth_service/internal/repo/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepo interface {
	Create(ctx context.Context, q db.Querier, user *models.User) error
	FindByUsername(ctx context.Context, username string) (*models.User, error)
}

type userRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(ctx context.Context, pool *pgxpool.Pool) UserRepo {
	return &userRepo{
		pool: pool,
	}
}

func (ur *userRepo) Create(ctx context.Context, q db.Querier, user *models.User) error {
	ib := db.NewInsertBuilder(ctx, ur.pool).
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

func (ur *userRepo) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	sb := db.NewSelectBuilder(ctx, ur.pool).
		Select("id", "username", "password").
		From("users").
		Where("username = ?", username).
		Limit(1)

	row := sb.QueryRow()

	var user models.User
	err := row.Scan(&user.ID, &user.Username, &user.Password)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, autherr.ErrNotFound
		}
		return nil, err
	}

	return &user, nil
}