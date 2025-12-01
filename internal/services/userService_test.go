package services

import (
	"context"
	"testing"

	"github.com/andro-kes/auth_service/internal/autherr"
	"github.com/andro-kes/auth_service/internal/models"
	"github.com/andro-kes/auth_service/internal/repo/db"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type fakeTx struct {
	txErr error
}

func (ft *fakeTx) RunInTx(ctx context.Context, fn func(ctx context.Context, q db.Querier) error) error {
	if ft.txErr != nil {
		return ft.txErr
	}
	return fn(ctx, nil)
}

type testUserRepo struct {
	newUser *models.User
	createError error
	notFoundError error
}

func (tur *testUserRepo) Create(ctx context.Context, q db.Querier, user *models.User) error {
	if tur.createError != nil {
		return tur.createError
	}
	tur.newUser = user
	return nil
}

func (tur *testUserRepo) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	if tur.notFoundError != nil {
		return nil, autherr.ErrNotFound
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("supersecret123"), 12)
	if err != nil {
		return nil, err
	}
	
	return &models.User{
		ID: uuid.New().String(),
		Username: username,
		Password: string(hash),
	}, nil
}

func TestRegister(t *testing.T) {
	ctx := t.Context()
	repo := &testUserRepo{}

	us := &UserService{
		Repo: repo,
		Tx: &fakeTx{},
	}

	err := us.Register(ctx, "test_user", "test_password")
	if err != nil {
		t.Fatalf("Failed to register user: %s", err.Error())
	}
	if repo.newUser == nil {
		t.Fatalf("Failed to create new user: %s", err.Error())
	}
	if repo.newUser.Username != "test_user" {
		t.Fatalf("Expected username: test_user, got: %s", repo.newUser.Username)
	}
	if repo.newUser.Password == "test_password" {
		t.Fatalf("Expected password to be hashed, got: %s", repo.newUser.Password)
	}
	if repo.newUser.ID == "" {
		t.Fatal("Expected non-empty user ID")
	}
}

func TestRegisterCreateFails(t *testing.T) {
	ctx := t.Context()
	repo := &testUserRepo{createError: autherr.ErrCreateUser}
	us := &UserService{
		Repo: repo,
		Tx:   &fakeTx{},
	}

	err := us.Register(ctx, "bob", "pwd")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	
	if err != autherr.ErrCreateUser {
		t.Fatalf("expected autherr.ErrCreateUser, got %v", err)
	}
}

func TestLogin(t *testing.T) {
	ctx := t.Context()
	repo := &testUserRepo{}
	us := &UserService{
		Repo: repo,
		Tx:   &fakeTx{},
	}

	user, err := us.Login(ctx, "kevin", "supersecret123")
	if err != nil {
		t.Fatalf("Detected error: %s", err.Error())
	}
	if user.Username != "kevin" {
		t.Fatalf("Expected name 'kevin', got: %s", user.Username)
	}
}

func TestLoginFail(t *testing.T) {
	ctx := t.Context()
	repo := &testUserRepo{notFoundError: autherr.ErrLoginUser}
	us := &UserService{
		Repo: repo,
		Tx:   &fakeTx{},
	}

	user, err := us.Login(ctx, "nick", "supersecret123")
	if err == nil {
		t.Fatal("Expected error")
	}
	if user != nil {
		t.Fatal("User must be nil")
	}
}