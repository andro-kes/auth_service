package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This file provides a small Postgres SQL builder targeted for use with *pgxpool.Pool.
// It is intentionally lightweight and focuses on safety around placeholder numbering
// and composing query fragments. It supports simple Select/Insert/Update/Delete builders,
// WHERE clauses with "?" placeholders that are converted to $n, and execution helpers.
//
// Usage examples (simple):
//
//   // SELECT
//   b := NewSelectBuilder(ctx, pool).
//         Select("id", "name").
//         From("users").
//         Where("email = ?", email).
//         Limit(1)
//   row := b.QueryRow()
//   err := row.Scan(&id, &name)
//
//   // INSERT
//   ib := NewInsertBuilder(ctx, pool).
//         Into("users").
//         Columns("email","name").
//         Values(email, name).
//         Returning("id")
//   row := ib.QueryRow()
//   var newID int
//   _ = row.Scan(&newID)
//
// Notes:
// - WHERE/SET/VALUES methods accept "?" placeholders; they will be replaced by
//   $1, $2, ... in the final SQL and the corresponding args appended.
// - For IN-lists prefer Postgres ANY/ARRAY syntax (e.g. "col = ANY($1)") and pass a slice.
// - Builders are not thread-safe; use per-goroutine instances.

type baseBuilder struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	args     []any
	argCount int
}

func (b *baseBuilder) addArgs(values ...any) {
	if len(values) == 0 {
		return
	}
	b.args = append(b.args, values...)
	b.argCount += len(values)
}

// replaceQuestionPlaceholders replaces each "?" in fragment with a numbered $n placeholder
// using b.argCount to continue numbering. It also appends provided args to the builder.
func (b *baseBuilder) replaceQuestionPlaceholders(fragment string, args ...interface{}) (string, error) {
	if len(args) == 0 && !strings.Contains(fragment, "?") {
		return fragment, nil
	}

	var out strings.Builder
	argIdx := 0
	for i := 0; i < len(fragment); i++ {
		ch := fragment[i]
		if ch == '?' {
			// ensure we have a corresponding arg
			if argIdx >= len(args) {
				return "", fmt.Errorf("not enough arguments for placeholders: fragment=%q", fragment)
			}
			b.argCount++
			out.WriteString(fmt.Sprintf("$%d", b.argCount))
			argIdx++
		} else {
			out.WriteByte(ch)
		}
	}
	if argIdx != len(args) {
		return "", fmt.Errorf("too many args for fragment: fragment=%q", fragment)
	}
	// append the args in the same order
	b.addArgs(args...)
	return out.String(), nil
}

// -- Select Builder --

type SelectBuilder struct {
	baseBuilder

	columns  []string
	from     string
	joins    []string
	wheres   []string
	groupBy  []string
	orderBy  []string
	limit    *int
	offset   *int
	distinct bool
}

// NewSelectBuilder creates a SelectBuilder bound to ctx and pool.
func NewSelectBuilder(ctx context.Context, pool *pgxpool.Pool) *SelectBuilder {
	return &SelectBuilder{
		baseBuilder: baseBuilder{ctx: ctx, pool: pool},
		columns:     []string{},
		joins:       []string{},
		wheres:      []string{},
		groupBy:     []string{},
		orderBy:     []string{},
	}
}

func (s *SelectBuilder) Select(cols ...string) *SelectBuilder {
	s.columns = append(s.columns, cols...)
	return s
}

func (s *SelectBuilder) Distinct() *SelectBuilder {
	s.distinct = true
	return s
}

func (s *SelectBuilder) From(table string) *SelectBuilder {
	s.from = table
	return s
}

func (s *SelectBuilder) Join(joinClause string) *SelectBuilder {
	s.joins = append(s.joins, joinClause)
	return s
}

func (s *SelectBuilder) Where(cond string, args ...interface{}) *SelectBuilder {
	fragment, err := s.replaceQuestionPlaceholders(cond, args...)
	if err != nil {
		panic(err)
	}
	s.wheres = append(s.wheres, fragment)
	return s
}

func (s *SelectBuilder) GroupBy(cols ...string) *SelectBuilder {
	s.groupBy = append(s.groupBy, cols...)
	return s
}

func (s *SelectBuilder) OrderBy(exprs ...string) *SelectBuilder {
	s.orderBy = append(s.orderBy, exprs...)
	return s
}

func (s *SelectBuilder) Limit(n int) *SelectBuilder {
	s.limit = &n
	return s
}

func (s *SelectBuilder) Offset(n int) *SelectBuilder {
	s.offset = &n
	return s
}

func (s *SelectBuilder) Build() (string, []interface{}) {
	if len(s.columns) == 0 {
		s.columns = append(s.columns, "*")
	}
	var b strings.Builder
	b.WriteString("SELECT ")
	if s.distinct {
		b.WriteString("DISTINCT ")
	}
	b.WriteString(strings.Join(s.columns, ", "))
	if s.from != "" {
		b.WriteString(" FROM ")
		b.WriteString(s.from)
	}
	if len(s.joins) > 0 {
		b.WriteString(" ")
		b.WriteString(strings.Join(s.joins, " "))
	}
	if len(s.wheres) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(s.wheres, " AND "))
	}
	if len(s.groupBy) > 0 {
		b.WriteString(" GROUP BY ")
		b.WriteString(strings.Join(s.groupBy, ", "))
	}
	if len(s.orderBy) > 0 {
		b.WriteString(" ORDER BY ")
		b.WriteString(strings.Join(s.orderBy, ", "))
	}
	if s.limit != nil {
		b.WriteString(fmt.Sprintf(" LIMIT %d", *s.limit))
	}
	if s.offset != nil {
		b.WriteString(fmt.Sprintf(" OFFSET %d", *s.offset))
	}
	return b.String(), s.args
}

func (s *SelectBuilder) Query() (pgx.Rows, error) {
	sql, args := s.Build()
	return s.pool.Query(s.ctx, sql, args...)
}

// QueryRow executes the built SELECT and returns a single row (pgx.Row).
func (s *SelectBuilder) QueryRow() pgx.Row {
	sql, args := s.Build()
	return s.pool.QueryRow(s.ctx, sql, args...)
}

func (s *SelectBuilder) Exec() (pgconn.CommandTag, error) {
	sql, args := s.Build()
	return s.pool.Exec(s.ctx, sql, args...)
}

// -- Insert Builder --

type InsertBuilder struct {
	baseBuilder

	table     string
	columns   []string
	values    [][]interface{} // multiple rows support
	returning []string
}

func NewInsertBuilder(ctx context.Context, pool *pgxpool.Pool) *InsertBuilder {
	return &InsertBuilder{
		baseBuilder: baseBuilder{ctx: ctx, pool: pool},
		columns:     []string{},
		values:      [][]interface{}{},
		returning:   []string{},
	}
}

func (i *InsertBuilder) Into(table string) *InsertBuilder {
	i.table = table
	return i
}

func (i *InsertBuilder) Columns(cols ...string) *InsertBuilder {
	i.columns = append(i.columns, cols...)
	return i
}

// Values adds a row of values. Number of values must match number of columns.
func (i *InsertBuilder) Values(vals ...interface{}) *InsertBuilder {
	i.values = append(i.values, vals)
	return i
}

func (i *InsertBuilder) Returning(cols ...string) *InsertBuilder {
	i.returning = append(i.returning, cols...)
	return i
}

func (i *InsertBuilder) Build() (string, []any, error) {
	if i.table == "" {
		return "", nil, fmt.Errorf("insert: missing table")
	}
	if len(i.columns) == 0 {
		return "", nil, fmt.Errorf("insert: missing columns")
	}
	if len(i.values) == 0 {
		return "", nil, fmt.Errorf("insert: missing values")
	}

	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(i.table)
	b.WriteString(" (")
	b.WriteString(strings.Join(i.columns, ", "))
	b.WriteString(") VALUES ")

	// build values with placeholders
	rowsFragments := make([]string, 0, len(i.values))
	for _, row := range i.values {
		if len(row) != len(i.columns) {
			return "", nil, fmt.Errorf("insert: values count %d does not match columns count %d", len(row), len(i.columns))
		}
		var frag strings.Builder
		frag.WriteString("(")
		for j := range row {
			i.argCount++
			if j > 0 {
				frag.WriteString(", ")
			}
			frag.WriteString(fmt.Sprintf("$%d", i.argCount))
		}
		frag.WriteString(")")
		rowsFragments = append(rowsFragments, frag.String())
		i.addArgs(row...)
	}
	b.WriteString(strings.Join(rowsFragments, ", "))
	if len(i.returning) > 0 {
		b.WriteString(" RETURNING ")
		b.WriteString(strings.Join(i.returning, ", "))
	}
	return b.String(), i.args, nil
}

func (i *InsertBuilder) Exec() (pgconn.CommandTag, error) {
	sql, args, err := i.Build()
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	return i.pool.Exec(i.ctx, sql, args...)
}

func (i *InsertBuilder) QueryRow() pgx.Row {
	sql, args, _ := i.Build()
	return i.pool.QueryRow(i.ctx, sql, args...)
}

// -- Update Builder --

type UpdateBuilder struct {
	baseBuilder

	table     string
	sets      []string // fragments like "col = $n"
	wheres    []string
	returning []string
}

func NewUpdateBuilder(ctx context.Context, pool *pgxpool.Pool) *UpdateBuilder {
	return &UpdateBuilder{
		baseBuilder: baseBuilder{ctx: ctx, pool: pool},
		sets:        []string{},
		wheres:      []string{},
	}
}

func (u *UpdateBuilder) Table(table string) *UpdateBuilder {
	u.table = table
	return u
}

// Set adds a "col = value" pair; value is provided as an argument (use ? placeholder semantics).
func (u *UpdateBuilder) Set(col string, value interface{}) *UpdateBuilder {
	fragment, err := u.replaceQuestionPlaceholders("?", value)
	if err != nil {
		panic(err)
	}
	// fragment is something like "$N"
	u.sets = append(u.sets, fmt.Sprintf("%s = %s", col, fragment))
	return u
}

func (u *UpdateBuilder) Where(cond string, args ...interface{}) *UpdateBuilder {
	fragment, err := u.replaceQuestionPlaceholders(cond, args...)
	if err != nil {
		panic(err)
	}
	u.wheres = append(u.wheres, fragment)
	return u
}

func (u *UpdateBuilder) Returning(cols ...string) *UpdateBuilder {
	u.returning = append(u.returning, cols...)
	return u
}

func (u *UpdateBuilder) Build() (string, []interface{}, error) {
	if u.table == "" {
		return "", nil, fmt.Errorf("update: missing table")
	}
	if len(u.sets) == 0 {
		return "", nil, fmt.Errorf("update: no sets provided")
	}
	var b strings.Builder
	b.WriteString("UPDATE ")
	b.WriteString(u.table)
	b.WriteString(" SET ")
	b.WriteString(strings.Join(u.sets, ", "))
	if len(u.wheres) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(u.wheres, " AND "))
	}
	if len(u.returning) > 0 {
		b.WriteString(" RETURNING ")
		b.WriteString(strings.Join(u.returning, ", "))
	}
	return b.String(), u.args, nil
}

func (u *UpdateBuilder) Exec() (pgconn.CommandTag, error) {
	sql, args, err := u.Build()
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	return u.pool.Exec(u.ctx, sql, args...)
}

func (u *UpdateBuilder) QueryRow() pgx.Row {
	sql, args, _ := u.Build()
	return u.pool.QueryRow(u.ctx, sql, args...)
}

// -- Delete Builder --

type DeleteBuilder struct {
	baseBuilder

	table     string
	wheres    []string
	returning []string
}

func NewDeleteBuilder(ctx context.Context, pool *pgxpool.Pool) *DeleteBuilder {
	return &DeleteBuilder{
		baseBuilder: baseBuilder{ctx: ctx, pool: pool},
		wheres:      []string{},
	}
}

func (d *DeleteBuilder) From(table string) *DeleteBuilder {
	d.table = table
	return d
}

func (d *DeleteBuilder) Where(cond string, args ...interface{}) *DeleteBuilder {
	fragment, err := d.replaceQuestionPlaceholders(cond, args...)
	if err != nil {
		panic(err)
	}
	d.wheres = append(d.wheres, fragment)
	return d
}

func (d *DeleteBuilder) Returning(cols ...string) *DeleteBuilder {
	d.returning = append(d.returning, cols...)
	return d
}

func (d *DeleteBuilder) Build() (string, []interface{}, error) {
	if d.table == "" {
		return "", nil, fmt.Errorf("delete: missing table")
	}
	var b strings.Builder
	b.WriteString("DELETE FROM ")
	b.WriteString(d.table)
	if len(d.wheres) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(d.wheres, " AND "))
	}
	if len(d.returning) > 0 {
		b.WriteString(" RETURNING ")
		b.WriteString(strings.Join(d.returning, ", "))
	}
	return b.String(), d.args, nil
}

func (d *DeleteBuilder) Exec() (pgconn.CommandTag, error) {
	sql, args, err := d.Build()
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	return d.pool.Exec(d.ctx, sql, args...)
}

func (d *DeleteBuilder) QueryRow() pgx.Row {
	sql, args, _ := d.Build()
	return d.pool.QueryRow(d.ctx, sql, args...)
}