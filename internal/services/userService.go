package services

import (
	"context"

	"github.com/andro-kes/auth_service/internal/autherr"
	"github.com/andro-kes/auth_service/internal/logger"
	"github.com/andro-kes/auth_service/internal/models"
	"github.com/andro-kes/auth_service/internal/repo"
	"github.com/andro-kes/auth_service/internal/repo/db"
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

func (us *UserService) Register(ctx context.Context, username, password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		logger.Logger().Error("Failed to hash password", zap.Error(err))
		return "", autherr.ErrHashPassword
	}
	
	user := &models.User{
		ID: uuid.New().String(),
		Username: username,
		Password: string(hash),
	}

	var userId string
	err = us.Tx.RunInTx(ctx, func(ctx context.Context, q db.Querier) error {
		userId, err = us.Repo.Create(ctx, q, user)
		if err != nil {
			logger.Logger().Error("Failed to create user", zap.Error(err))
			return autherr.ErrCreateUser
		}

		logger.Logger().Info("User created", zap.String("user_id", user.ID))
		return nil
	})
	if err != nil {
		return "", err
	}

	return userId, nil
}

func (us *UserService) Login(ctx context.Context, username, password string) (*models.User, error) {
	user, err := us.Repo.FindByUsername(ctx, username)
	if err != nil {
		if err == autherr.ErrNotFound {
			return nil, autherr.ErrNotFound
		}
		logger.Logger().Error("Failed to get user by username", zap.Error(err))
		return nil, autherr.ErrStorageError.WithMessage(err.Error())
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, autherr.ErrLoginUser
	}

	return user, nil
}