package stateful

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

// QueryBuilder constrói queries SQL com filtros contextuais
type QueryBuilder struct {
	manifest *manifest.Manifest
	db       *sql.DB
}

// NewQueryBuilder cria um novo QueryBuilder
func NewQueryBuilder(manifest *manifest.Manifest, db *sql.DB) *QueryBuilder {
	return &QueryBuilder{
		manifest: manifest,
		db:       db,
	}
}

// BuildEntityQuery constrói uma query completa para uma entidade com todos os filtros
func (qb *QueryBuilder) BuildEntityQuery(entityType string, q *ContextualQuery, filters map[string]interface{}) (string, []interface{}) {
	// Query base
	query := fmt.Sprintf("SELECT * FROM %s", entityType)
	args := []interface{}{}
	whereClauses := []string{}

	// Adicionar filtros contextuais
	if authorFilter, authorArgs := q.GetAuthorFilter(); authorFilter != "" {
		whereClauses = append(whereClauses, authorFilter)
		args = append(args, authorArgs...)
	}

	if visibilityFilter, visibilityArgs := q.GetVisibilityFilter(); visibilityFilter != "" {
		whereClauses = append(whereClauses, visibilityFilter)
		args = append(args, visibilityArgs...)
	}

	// Adicionar filtros customizados
	for field, value := range filters {
		if value != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("%s = ?", field))
			args = append(args, value)
		}
	}

	// Construir WHERE clause
	if len(whereClauses) > 0 {
		query = fmt.Sprintf("%s WHERE %s", query, strings.Join(whereClauses, " AND "))
	}

	return query, args
}

// BuildAggregationQuery constrói queries de agregação com filtros contextuais
func (qb *QueryBuilder) BuildAggregationQuery(entityType string, q *ContextualQuery, groupBy string) (string, []interface{}) {
	// Query base com agregação
	query := fmt.Sprintf("SELECT %s, COUNT(*) as count, MAX(updated_at) as last_updated FROM %s", groupBy, entityType)
	args := []interface{}{}
	whereClauses := []string{}

	// Adicionar filtros contextuais
	if authorFilter, authorArgs := q.GetAuthorFilter(); authorFilter != "" {
		whereClauses = append(whereClauses, authorFilter)
		args = append(args, authorArgs...)
	}

	if visibilityFilter, visibilityArgs := q.GetVisibilityFilter(); visibilityFilter != "" {
		whereClauses = append(whereClauses, visibilityFilter)
		args = append(args, visibilityArgs...)
	}

	// Construir WHERE clause
	if len(whereClauses) > 0 {
		query = fmt.Sprintf("%s WHERE %s", query, strings.Join(whereClauses, " AND "))
	}

	// Adicionar GROUP BY e ORDER BY
	query = fmt.Sprintf("%s GROUP BY %s ORDER BY count DESC", query, groupBy)

	return query, args
}
