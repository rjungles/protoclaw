package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type Repository struct {
	db       *sql.DB
	manifest *manifest.Manifest
}

func NewRepository(db *sql.DB, m *manifest.Manifest) *Repository {
	return &Repository{db: db, manifest: m}
}

func (r *Repository) FindByID(ctx context.Context, entityName string, id interface{}) (map[string]interface{}, error) {
	tableName := toSnakeCase(entityName)
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = ?", tableName)
	rows, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, ErrNotFound
	}

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns error: %w", err)
	}

	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, fmt.Errorf("scan error: %w", err)
	}

	result := make(map[string]interface{})
	for i, col := range columns {
		result[col] = values[i]
	}

	return result, nil
}

func (r *Repository) FindAll(ctx context.Context, entityName string, limit, offset int) ([]map[string]interface{}, error) {
	tableName := toSnakeCase(entityName)
	query := fmt.Sprintf("SELECT * FROM %s LIMIT ? OFFSET ?", tableName)
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		columns, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("columns error: %w", err)
		}

		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}

		result := make(map[string]interface{})
		for i, col := range columns {
			result[col] = values[i]
		}
		results = append(results, result)
	}

	return results, nil
}

func (r *Repository) Create(ctx context.Context, entityName string, data map[string]interface{}) (int64, error) {
	tableName := toSnakeCase(entityName)
	entity := r.findEntity(entityName)
	if entity == nil {
		return 0, fmt.Errorf("entity %s not found", entityName)
	}

	now := time.Now()

	columns := make([]string, 0, len(data)+2)
	placeholders := make([]string, 0, len(data)+2)
	values := make([]interface{}, 0, len(data)+2)

	for col, val := range data {
		if col == "id" {
			continue
		}
		columns = append(columns, toSnakeCase(col))
		placeholders = append(placeholders, "?")
		values = append(values, val)
	}

	if _, ok := data["created_at"]; !ok {
		columns = append(columns, "created_at")
		placeholders = append(placeholders, "?")
		values = append(values, now)
	}
	if _, ok := data["updated_at"]; !ok {
		columns = append(columns, "updated_at")
		placeholders = append(placeholders, "?")
		values = append(values, now)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	result, err := r.db.ExecContext(ctx, query, values...)
	if err != nil {
		return 0, fmt.Errorf("insert error: %w", err)
	}

	return result.LastInsertId()
}

func (r *Repository) Update(ctx context.Context, entityName string, id interface{}, data map[string]interface{}) error {
	tableName := toSnakeCase(entityName)

	setClauses := make([]string, 0, len(data)+1)
	values := make([]interface{}, 0, len(data)+2)

	for col, val := range data {
		if col == "id" {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", toSnakeCase(col)))
		values = append(values, val)
	}

	if _, ok := data["updated_at"]; !ok {
		setClauses = append(setClauses, "updated_at = ?")
		values = append(values, time.Now())
	}

	values = append(values, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?",
		tableName,
		strings.Join(setClauses, ", "))

	result, err := r.db.ExecContext(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("update error: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected error: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) Delete(ctx context.Context, entityName string, id interface{}) error {
	tableName := toSnakeCase(entityName)
	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", tableName)

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete error: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected error: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) findEntity(name string) *manifest.Entity {
	for i := range r.manifest.DataModel.Entities {
		if r.manifest.DataModel.Entities[i].Name == name {
			return &r.manifest.DataModel.Entities[i]
		}
	}
	return nil
}

var ErrNotFound = errors.New("record not found")

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, []rune(strings.ToLower(string(r)))...)
	}
	return string(result)
}
