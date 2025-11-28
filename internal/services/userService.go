package services

import (
	"context"

	"github.com/andro-kes/auth/internal/autherr"
	"github.com/andro-kes/auth/internal/logger"
	"github.com/andro-kes/auth/internal/models"
	"github.com/andro-kes/auth/internal/repo"
	"github.com/andro-kes/auth/internal/repo/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	Repo repo.UserRepo
	Tx db.Tx
}

func NewUserService(ctx context.Context, pool *pgxpool.Pool) *UserService {
	return &UserService{
		Repo: repo.NewUserRepo(ctx, pool),
		Tx: db.NewTx(pool),
	}
}

func (us *UserService) Register(ctx context.Context, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		logger.Logger().Error("Failed to hash password", zap.Error(err))
		return autherr.ErrHashPassword
	}
	
	user := &models.User{
		ID: uuid.New().String(),
		Username: username,
		Password: string(hash),
	}

	return us.Tx.RunInTx(ctx, func(ctx context.Context, q db.Querier) error {
		if err := us.Repo.Create(ctx, q, user); err != nil {
			logger.Logger().Error("Failed to create user", zap.Error(err))
			return autherr.ErrCreateUser
		}

		logger.Logger().Info("User created", zap.String("user_id", user.ID))
		return nil
	})
}