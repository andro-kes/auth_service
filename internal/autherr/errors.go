package autherr

import (
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthError is a small error type intended for use in RPC responses.
// It carries a human-readable message (serialized to JSON as {"message":"..."})
// and a gRPC code to be returned from gRPC handlers.
type AuthError struct {
	// Message is safe to expose to clients and will be serialized to JSON.
	Message string `json:"message"`

	// grpcCode is not serialized to JSON but is used when converting to gRPC status/errors.
	grpcCode codes.Code `json:"-"`
}

// Ensure AuthError implements error.
func (e *AuthError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Message
}

// MarshalJSON ensures only the message (and optionally code name) are exposed to JSON clients.
func (e *AuthError) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	type payload struct {
		Message string `json:"message"`
		Code    string `json:"code,omitempty"`
	}
	p := payload{Message: e.Message}
	if e.grpcCode != codes.OK && e.grpcCode != 0 {
		p.Code = e.grpcCode.String()
	}
	return json.Marshal(p)
}

// New creates a new AuthError with the provided message and gRPC code.
func New(message string, code codes.Code) *AuthError {
	if message == "" {
		message = code.String()
	}
	return &AuthError{
		Message:  message,
		grpcCode: code,
	}
}

// WithMessage returns a copy of the error with the message replaced (keeps the same gRPC code).
func (e *AuthError) WithMessage(msg string) *AuthError {
	if e == nil {
		return New(msg, codes.Internal)
	}
	return &AuthError{Message: msg, grpcCode: e.grpcCode}
}

// GRPCStatus returns a *status.Status suitable for returning from gRPC handlers.
func (e *AuthError) GRPCStatus() *status.Status {
	if e == nil {
		return status.New(codes.Internal, "internal error")
	}
	return status.New(e.grpcCode, e.Message)
}

// GRPCError returns an error that can be returned from a gRPC method (status.Error).
func (e *AuthError) GRPCError() error {
	return e.GRPCStatus().Err()
}

// ToGRPCError converts any error into a gRPC error. If err is *AuthError it preserves its code/message,
// otherwise it returns a status with codes.Internal and the original error message.
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}
	if ae, ok := err.(*AuthError); ok {
		return ae.GRPCError()
	}
	// If it's already a status error, return as-is
	if _, ok := status.FromError(err); ok {
		return err
	}
	// Default mapping
	return status.Error(codes.Internal, err.Error())
}

// Predefined common errors for the auth microservice.
// You may use these directly or create copies with WithMessage when you need contextual text.
var (
	// user creation/login issues
	ErrCreateUser = New("failed to create user", codes.Internal)
	ErrLoginUser  = New("invalid credentials", codes.Unauthenticated)

	// token related
	ErrInvalidToken = New("invalid token", codes.Unauthenticated)
	ErrTokenExpired = New("token expired", codes.Unauthenticated)
	ErrNoToken      = New("no token provided", codes.Unauthenticated)

	// authorization / access
	ErrForbidden = New("forbidden", codes.PermissionDenied)
	ErrNotFound  = New("not found", codes.NotFound)

	// generic
	ErrBadRequest = New("bad request", codes.InvalidArgument)
	ErrHashPassword = New("failed to hash password", codes.Internal)
)