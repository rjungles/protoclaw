package db

import (
	"context"
	"database/sql"
)

type SQLDB struct {
	db *sql.DB
}

func NewSQLDB(db *sql.DB) *SQLDB {
	return &SQLDB{db: db}
}

func (s *SQLDB) Exec(query string, args ...interface{}) (ResultInterface, error) {
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return nil, err
	}
	return sqlResult{res: res}, nil
}

func (s *SQLDB) Query(query string, args ...interface{}) (RowsInterface, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows: rows}, nil
}

func (s *SQLDB) QueryRow(query string, args ...interface{}) RowInterface {
	return &sqlRow{row: s.db.QueryRow(query, args...)}
}

func (s *SQLDB) ExecContext(ctx context.Context, query string, args ...interface{}) (ResultInterface, error) {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return sqlResult{res: res}, nil
}

type sqlResult struct {
	res sql.Result
}

func (r sqlResult) LastInsertId() (int64, error) {
	return r.res.LastInsertId()
}

func (r sqlResult) RowsAffected() (int64, error) {
	return r.res.RowsAffected()
}

type sqlRows struct {
	rows *sql.Rows
}

func (r *sqlRows) Next() bool {
	return r.rows.Next()
}

func (r *sqlRows) Scan(dest ...interface{}) error {
	return r.rows.Scan(dest...)
}

func (r *sqlRows) Close() error {
	return r.rows.Close()
}

func (r *sqlRows) Err() error {
	return r.rows.Err()
}

type sqlRow struct {
	row *sql.Row
}

func (r *sqlRow) Scan(dest ...interface{}) error {
	return r.row.Scan(dest...)
}
