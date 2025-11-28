package db

import (
	"context"

	pgxv5 "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgxv5.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgxv5.Row
}

type Tx interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context, q Querier) error) error
}

type tx struct {
	Pool *pgxpool.Pool
}

func NewTx(pool *pgxpool.Pool) Tx {
	return &tx{
		Pool: pool,
	}
}

func (t *tx) RunInTx(ctx context.Context, fn func(ctx context.Context, q Querier) error) error {
	px, err := t.Pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = px.Rollback(ctx)
	}()

	if err := fn(ctx, px); err != nil {
		return err
	}

	if err := px.Commit(ctx); err != nil {
		return err
	}
	
	return nil
}