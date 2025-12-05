package rpc

import (
	"context"
	"os"
	"time"

	"github.com/andro-kes/auth_service/internal/autherr"
	"github.com/andro-kes/auth_service/internal/logger"
	"github.com/andro-kes/auth_service/internal/services"
	pb "github.com/andro-kes/auth_service/proto"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
)

type AuthServer struct {
	pb.UnimplementedAuthServiceServer
	UserService *services.UserService
	TokenService *services.TokenService
}

func NewAuthServer(ctx context.Context, pool *pgxpool.Pool) (*AuthServer, error) {
	tsvc, err := services.NewTokenService(
		os.Getenv("SECRET_KEY"),
		time.Minute * 5,
		time.Hour * 24 * 7,
	)
	if err != nil {
		// return the actual error so callers see the real cause
		return nil, err
	}

	return &AuthServer{
		UserService: services.NewUserService(ctx, pool),
		TokenService: tsvc,
	}, nil
}

func (as *AuthServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.TokenResponse, error) {
	user, err := as.UserService.Login(ctx, req.Username, req.Password)
	if err != nil {
		logger.Logger().Error("Failed to login", zap.Error(err))
		return nil, err
	}
	logger.Logger().Info("User logged in", zap.String("username", user.Username))

	accessToken, refreshToken, accessExp, refreshExp, err := as.TokenService.GenerateTokens(ctx, user.ID)
	if err != nil {
		logger.Logger().Error("Failed to generate tokens", zap.Error(err))
		return nil, autherr.ErrBadRequest
	}

	accessTTL := time.Until(accessExp)
	refreshTTL := time.Until(refreshExp)

	return &pb.TokenResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		AccessExpiresIn:  durationpb.New(accessTTL),
		RefreshExpiresIn: durationpb.New(refreshTTL),
		UserId:           user.ID,
	}, nil
}

func (as *AuthServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	userId, err := as.UserService.Register(ctx, req.Username, req.Password)
	if err != nil {
		return &pb.RegisterResponse{UserId: ""}, err
	}

	return &pb.RegisterResponse{UserId: userId}, nil
}

func (as *AuthServer) Refresh(ctx context.Context, req *pb.RefreshRequest) (resp *pb.TokenResponse, err error) {
	newAccess, newRefresh, accessExp, refreshExp, err := as.TokenService.RotateRefresh(ctx, req.RefreshToken, req.ExpectedUserId)
	if err != nil {
		return nil, err
	}

	resp = &pb.TokenResponse{
		AccessToken:      newAccess,
		RefreshToken:     newRefresh,
		AccessExpiresIn:  durationpb.New(time.Until(accessExp)),
		RefreshExpiresIn: durationpb.New(time.Until(refreshExp)),
		UserId:           req.ExpectedUserId,
	}

	return resp, nil
}

func (as *AuthServer) Revoke(ctx context.Context, req *pb.RevokeRequest) (*pb.RevokeResponse, error) {
	if err := as.TokenService.RevokeRefreshByRaw(ctx, req.RefreshToken); err != nil {
		return &pb.RevokeResponse{Error: "failed to revoke token"}, err
	}
	return &pb.RevokeResponse{Error: "Token revoked"}, nil
}