package services

import (
	"os"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestNewTokenService_SecretTooShort(t *testing.T) {
	_, err := NewTokenService("short-secret", time.Minute, time.Hour*24)
	if err == nil {
		t.Fatalf("expected error for short secret, got nil")
	}
}

func TestGenerateAndRevoke(t *testing.T) {
	srv, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer srv.Close()

	os.Setenv("REDIS_ADDR", srv.Addr())

	secret := "012345678901234567890123456789ab"
	svc, err := NewTokenService(secret, time.Second*5, time.Minute*5)
	if err != nil {
		t.Fatalf("failed to create TokenService: %v", err)
	}

	ctx := t.Context()

	if len(srv.Keys()) != 0 {
		t.Fatalf("expected zero keys in redis at start, got %d", len(srv.Keys()))
	}

	_, refresh, _, _, err := svc.GenerateTokens(ctx, "user-123")
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}
	if refresh == "" {
		t.Fatalf("expected a non-empty refresh token")
	}

	if len(srv.Keys()) == 0 {
		t.Fatalf("expected redis to contain keys after GenerateTokens")
	}

	if err := svc.RevokeRefreshByRaw(ctx, refresh); err != nil {
		t.Fatalf("RevokeRefreshByRaw failed: %v", err)
	}

	// check idempotent
	if err := svc.RevokeRefreshByRaw(ctx, refresh); err != nil {
		t.Fatalf("RevokeRefreshByRaw failed on second call: %v", err)
	}

	// check safety
	rdb := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer rdb.Close()

	keys := srv.Keys()
	if len(keys) != 0 {
		t.Logf("remaining keys in miniredis: %v", keys)
	}
}
