package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/andro-kes/auth_service/internal/autherr"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type TokenService struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	rdb        *redis.Client
}

type tokenClaims struct {
	UserID string `json:"uid"`
	Typ    string `json:"typ"`
	jwt.RegisteredClaims
}

func NewTokenService(secret string, accessTTL, refreshTTL time.Duration) (*TokenService, error) {
	if len(secret) < 32 {
		return nil, autherr.ErrBadRequest.WithMessage("secret must be at least 32 bytes")
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, autherr.ErrStorageError.WithMessage(err.Error())
	}
	return &TokenService{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		rdb:        rdb,
	}, nil
}

func (s *TokenService) Close() error {
	return s.rdb.Close()
}

func (s *TokenService) GenerateTokens(ctx context.Context, userID string) (accessToken, refreshToken string, accessExp, refreshExp time.Time, err error) {
	now := time.Now().UTC()
	accessExp = now.Add(s.accessTTL)
	atJti, err := randomHex(16)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, autherr.ErrTokenGeneration.WithMessage(err.Error())
	}
	accessClaims := tokenClaims{
		UserID: userID,
		Typ:    "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        atJti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExp),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	signedAccess, err := at.SignedString(s.secret)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, autherr.ErrTokenGeneration.WithMessage(err.Error())
	}

	refreshExp = now.Add(s.refreshTTL)
	rawRefresh, err := randomBase64(64)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, autherr.ErrTokenGeneration.WithMessage(err.Error())
	}
	refreshHash := sha256Hex(rawRefresh)
	key := redisKey(refreshHash)

	if err := s.rdb.HSet(ctx, key, map[string]any{
		"user_id":   userID,
		"issued_at": now.Unix(),
	}).Err(); err != nil {
		return "", "", time.Time{}, time.Time{}, autherr.ErrStorageError.WithMessage(err.Error())
	}
	if err := s.rdb.Expire(ctx, key, s.refreshTTL).Err(); err != nil {
		return "", "", time.Time{}, time.Time{}, autherr.ErrStorageError.WithMessage(err.Error())
	}

	return signedAccess, rawRefresh, accessExp, refreshExp, nil
}

func (s *TokenService) ValidateAccess(tokenStr string) (string, error) {
	claims, err := s.parseAndMapErr(tokenStr)
	if err != nil {
		return "", err
	}
	if claims.Typ != "access" {
		return "", autherr.ErrInvalidToken
	}
	return claims.UserID, nil
}

func (s *TokenService) ValidateRefresh(ctx context.Context, rawRefresh string) (string, error) {
	h := sha256Hex(rawRefresh)
	key := redisKey(h)
	exists, err := s.rdb.Exists(ctx, key).Result()
	if err != nil {
		return "", autherr.ErrStorageError.WithMessage(err.Error())
	}
	if exists == 0 {
		return "", autherr.ErrInvalidToken
	}
	userID, err := s.rdb.HGet(ctx, key, "user_id").Result()
	if err == redis.Nil || userID == "" {
		return "", autherr.ErrInvalidToken
	}
	if err != nil {
		return "", autherr.ErrStorageError.WithMessage(err.Error())
	}
	return userID, nil
}

var rotateScript = `
if redis.call("EXISTS", KEYS[1]) == 0 then
  return {err="old_not_found"}
end
local uid = redis.call("HGET", KEYS[1], "user_id")
if ARGV[1] ~= "" and uid ~= ARGV[1] then
  return {err="user_mismatch"}
end
redis.call("HSET", KEYS[2], "user_id", ARGV[1], "issued_at", ARGV[2])
redis.call("EXPIRE", KEYS[2], tonumber(ARGV[3]))
redis.call("DEL", KEYS[1])
return {ok="ok"}
`

func (s *TokenService) RotateRefresh(ctx context.Context, oldRaw string, expectedUserID string) (newAccess, newRefresh string, accessExp, refreshExp time.Time, err error) {
	userID, err := s.ValidateRefresh(ctx, oldRaw)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	if expectedUserID != "" && userID != expectedUserID {
		return "", "", time.Time{}, time.Time{}, autherr.ErrInvalidToken
	}

	now := time.Now().UTC()
	newAccess, newRefresh, accessExp, refreshExp, err = s.GenerateTokens(ctx, userID)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}

	newHash := sha256Hex(newRefresh)
	oldHash := sha256Hex(oldRaw)
	oldKey := redisKey(oldHash)
	newKey := redisKey(newHash)
	issuedAt := now.Unix()
	ttl := int(s.refreshTTL.Seconds())

	cmd := s.rdb.Eval(ctx, rotateScript, []string{oldKey, newKey}, userID, issuedAt, ttl)
	if cmd.Err() != nil {
		// rollback attempt: delete newKey if created
		_ = s.rdb.Del(ctx, newKey).Err()
		// map specific errors
		if cmd.Err().Error() == "ERR old_not_found" || cmd.Err().Error() == "old_not_found" {
			return "", "", time.Time{}, time.Time{}, autherr.ErrInvalidToken
		}
		if cmd.Err().Error() == "ERR user_mismatch" || cmd.Err().Error() == "user_mismatch" {
			return "", "", time.Time{}, time.Time{}, autherr.ErrInvalidToken
		}
		return "", "", time.Time{}, time.Time{}, autherr.ErrStorageError.WithMessage(cmd.Err().Error())
	}

	return newAccess, newRefresh, accessExp, refreshExp, nil
}

func (s *TokenService) RevokeRefreshByRaw(ctx context.Context, raw string) error {
	h := sha256Hex(raw)
	key := redisKey(h)
	_, err := s.rdb.Del(ctx, key).Result()
	if err != nil {
		return autherr.ErrStorage.WithMessage(err.Error())
	}
	return nil
}

func (s *TokenService) parseAndMapErr(tokenStr string) (*tokenClaims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &tokenClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, autherr.ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, autherr.ErrTokenExpired
		}
		return nil, autherr.ErrInvalidToken
	}
	claims, ok := tok.Claims.(*tokenClaims)
	if !ok || !tok.Valid {
		return nil, autherr.ErrInvalidToken
	}
	return claims, nil
}

func redisKey(hash string) string {
	return "refresh:th:" + hash
}

func randomBase64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}