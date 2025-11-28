package rpc

import (
	"context"

	"github.com/andro-kes/auth/internal/services"
	pb "github.com/andro-kes/auth/proto"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthServer struct {
	pb.UnimplementedAuthServiceServer
	UserService *services.UserService
}

func NewAuthServer(ctx context.Context, pool *pgxpool.Pool) *AuthServer {
	return &AuthServer{
		UserService: services.NewUserService(ctx, pool),
	}
}

// func (as *AuthServer) Login(ctx context.Context, req *pb.Request) (*pb.Response, error) {

// }

func (as *AuthServer) Register(ctx context.Context, req *pb.Request) (*pb.Status, error) {
	err := as.UserService.Register(ctx, req.Username, req.Password)
	if err != nil {
		return &pb.Status{Status: "Failed"}, err
	}

	return &pb.Status{Status: "Success"}, nil
}