package policy

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

// Contexto de avaliação de política
type Context struct {
	ActorID    string                 // ID do ator solicitante
	Roles      []string               // Papéis do ator
	Resource   string                 // Recurso sendo acessado
	Action     string                 // Ação sendo executada (read, write, delete, execute)
	Attributes map[string]interface{} // Atributos contextuais
	Time       time.Time              // Tempo da requisição
	Metadata   map[string]string      // Metadados adicionais
}

// Resultado da avaliação de política
type Result struct {
	Allowed   bool     // Se o acesso é permitido
	Denied    bool     // Se o acesso é negado (explicitamente)
	Reason    string   // Motivo da decisão
	Roles     []string // Papéis que justificam a decisão
	Condition string   // Condição aplicada (se houver)
}

// Engine de políticas RBAC/ABAC
type Engine struct {
	manifest          *manifest.Manifest
	roleHierarchy     map[string][]string              // Mapeia papel -> papéis herdados
	actorRoles        map[string][]string              // Mapeia ator -> papéis
	actorPermissions  map[string][]manifest.Permission // Mapeia ator -> permissões
	contextConditions []manifest.ContextCondition
	defaultDeny       bool
}

// NovaEngine cria uma nova engine de políticas a partir de um manifesto
func NewEngine(m *manifest.Manifest) (*Engine, error) {
	engine := &Engine{
		manifest:          m,
		roleHierarchy:     make(map[string][]string),
		actorRoles:        make(map[string][]string),
		actorPermissions:  make(map[string][]manifest.Permission),
		contextConditions: m.Security.Authorization.ContextConditions,
		defaultDeny:       m.Security.Authorization.DefaultDeny,
	}

	// Constrói hierarquia de papéis
	for _, rh := range m.Security.Authorization.RoleHierarchy {
		engine.roleHierarchy[rh.Role] = rh.Inherits
	}

	// Mapeia atores para seus papéis e permissões
	for _, actor := range m.Actors {
		engine.actorRoles[actor.ID] = actor.Roles
		engine.actorPermissions[actor.ID] = actor.Permissions
	}

	return engine, nil
}

// GetAllRoles retorna todos os papéis de um ator (incluindo herdados)
func (e *Engine) GetAllRoles(actorID string) []string {
	directRoles := e.actorRoles[actorID]
	if len(directRoles) == 0 {
		return []string{}
	}

	allRoles := make(map[string]struct{})
	queue := append([]string{}, directRoles...)

	for len(queue) > 0 {
		role := queue[0]
		queue = queue[1:]

		if _, ok := allRoles[role]; ok {
			continue
		}
		allRoles[role] = struct{}{}

		// Adiciona papéis herdados
		inherited := e.roleHierarchy[role]
		queue = append(queue, inherited...)
	}

	result := make([]string, 0, len(allRoles))
	for role := range allRoles {
		result = append(result, role)
	}

	slices.Sort(result)
	return result
}

// CheckPermission verifica se um ator tem permissão para uma ação específica
func (e *Engine) CheckPermission(ctx *Context) *Result {
	// Obtém todos os papéis do ator (com herança)
	allRoles := e.getAllRolesForContext(ctx)

	if len(allRoles) == 0 {
		return &Result{
			Allowed: !e.defaultDeny,
			Denied:  e.defaultDeny,
			Reason:  fmt.Sprintf("actor %s has no roles", ctx.ActorID),
		}
	}

	// Verifica permissões diretas do ator (prioridade máxima)
	actorPerms := e.actorPermissions[ctx.ActorID]
	for _, perm := range actorPerms {
		if e.matchesResource(perm.Resource, ctx.Resource) &&
			e.matchesAction(perm.Actions, ctx.Action) {

			// Verifica condição se existir
			if perm.Condition != "" {
				if !e.evaluateCondition(perm.Condition, ctx) {
					continue // Condição não satisfeita
				}
			}

			return &Result{
				Allowed:   true,
				Denied:    false,
				Reason:    fmt.Sprintf("permission granted for %s on %s", ctx.Action, ctx.Resource),
				Roles:     allRoles,
				Condition: perm.Condition,
			}
		}
	}

	// Nenhuma permissão direta encontrada - verifica default policy
	if e.defaultDeny {
		return &Result{
			Allowed: false,
			Denied:  true,
			Reason:  fmt.Sprintf("access denied by default policy for %s on %s (no explicit permission found)", ctx.Action, ctx.Resource),
			Roles:   allRoles,
		}
	}

	return &Result{
		Allowed: true,
		Denied:  false,
		Reason:  "access allowed by default (default_deny is false)",
		Roles:   allRoles,
	}
}

// CheckAccess verifica acesso com condições contextuais
func (e *Engine) CheckAccess(ctx *Context) *Result {
	// Primeiro verifica permissão básica
	result := e.CheckPermission(ctx)

	if !result.Allowed || result.Denied {
		return result
	}

	// Verifica condições contextuais globais
	for _, cond := range e.contextConditions {
		if !e.evaluateCondition(cond.Expression, ctx) {
			return &Result{
				Allowed:   false,
				Denied:    true,
				Reason:    cond.Message,
				Roles:     result.Roles,
				Condition: cond.Name,
			}
		}
	}

	return result
}

// matchesResource verifica se um padrão de recurso corresponde ao recurso solicitado
func (e *Engine) matchesResource(pattern, resource string) bool {
	if pattern == "*" {
		return true
	}

	// Suporte a wildcards simples
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(resource, prefix)
	}

	return pattern == resource
}

// matchesAction verifica se uma ação está na lista de ações permitidas
func (e *Engine) matchesAction(allowedActions []string, action string) bool {
	for _, a := range allowedActions {
		if a == "*" || a == action {
			return true
		}
	}
	return false
}

// evaluateCondition avalia uma expressão de condição
// Implementação simplificada - em produção usaria um motor de expressões real
func (e *Engine) evaluateCondition(expression string, ctx *Context) bool {
	expression = strings.TrimSpace(expression)

	if expression == "" {
		return false
	}

	orParts := splitByOperator(expression, "||")
	if len(orParts) > 1 {
		for _, part := range orParts {
			if e.evaluateCondition(part, ctx) {
				return true
			}
		}
		return false
	}

	andParts := splitByOperator(expression, "&&")
	if len(andParts) > 1 {
		for _, part := range andParts {
			if !e.evaluateCondition(part, ctx) {
				return false
			}
		}
		return true
	}

	for strings.HasPrefix(expression, "(") && strings.HasSuffix(expression, ")") {
		inner := strings.TrimSpace(expression[1 : len(expression)-1])
		if inner == expression {
			break
		}
		expression = inner
	}

	if strings.HasPrefix(expression, "!") {
		return !e.evaluateCondition(strings.TrimSpace(expression[1:]), ctx)
	}

	if strings.HasPrefix(expression, "not ") {
		return !e.evaluateCondition(strings.TrimSpace(expression[4:]), ctx)
	}

	return e.evaluateComparison(expression, ctx)
}

// GetActorRoles retorna os papéis diretos de um ator
func (e *Engine) GetActorRoles(actorID string) []string {
	return e.actorRoles[actorID]
}

// GetRoleHierarchy retorna a hierarquia de um papel específico
func (e *Engine) GetRoleHierarchy(role string) []string {
	return e.roleHierarchy[role]
}

// ListResources retorna todos os recursos definidos nas permissões
func (e *Engine) ListResources() []string {
	resources := make(map[string]struct{})

	for _, perms := range e.actorPermissions {
		for _, perm := range perms {
			resources[perm.Resource] = struct{}{}
		}
	}

	result := make([]string, 0, len(resources))
	for r := range resources {
		result = append(result, r)
	}

	slices.Sort(result)
	return result
}

func (e *Engine) getAllRolesForContext(ctx *Context) []string {
	directRoles := ctx.Roles
	if len(directRoles) == 0 {
		directRoles = e.actorRoles[ctx.ActorID]
	}
	if len(directRoles) == 0 {
		return []string{}
	}

	allRoles := make(map[string]struct{})
	queue := append([]string{}, directRoles...)
	for len(queue) > 0 {
		role := queue[0]
		queue = queue[1:]

		if _, ok := allRoles[role]; ok {
			continue
		}
		allRoles[role] = struct{}{}

		queue = append(queue, e.roleHierarchy[role]...)
	}

	result := make([]string, 0, len(allRoles))
	for role := range allRoles {
		result = append(result, role)
	}
	slices.Sort(result)
	return result
}

func splitByOperator(s, op string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{""}
	}

	var parts []string
	var current strings.Builder
	inParens := 0
	inBrackets := 0
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		switch ch {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '(':
			if !inSingleQuote && !inDoubleQuote {
				inParens++
			}
		case ')':
			if !inSingleQuote && !inDoubleQuote {
				inParens--
			}
		case '[':
			if !inSingleQuote && !inDoubleQuote {
				inBrackets++
			}
		case ']':
			if !inSingleQuote && !inDoubleQuote {
				inBrackets--
			}
		}

		if !inSingleQuote && !inDoubleQuote && inParens == 0 && inBrackets == 0 && strings.HasPrefix(s[i:], op) {
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
			i += len(op) - 1
			continue
		}

		current.WriteByte(ch)
	}

	parts = append(parts, strings.TrimSpace(current.String()))
	return parts
}

func (e *Engine) evaluateComparison(expr string, ctx *Context) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}

	operators := []string{"<=", ">=", "==", "!=", "<", ">"}
	var op string
	var idx int
	for _, candidate := range operators {
		if i := strings.Index(expr, candidate); i >= 0 {
			op = candidate
			idx = i
			break
		}
	}
	if op == "" {
		return false
	}

	left := strings.TrimSpace(expr[:idx])
	right := strings.TrimSpace(expr[idx+len(op):])

	leftVal, ok := resolveValue(left, ctx)
	if !ok {
		return false
	}

	rightVal, ok := resolveLiteral(right, ctx)
	if !ok {
		return false
	}

	if leftNum, lok := toFloat(leftVal); lok {
		if rightNum, rok := toFloat(rightVal); rok {
			switch op {
			case "<=":
				return leftNum <= rightNum
			case ">=":
				return leftNum >= rightNum
			case "<":
				return leftNum < rightNum
			case ">":
				return leftNum > rightNum
			case "==":
				return leftNum == rightNum
			case "!=":
				return leftNum != rightNum
			}
		}
	}

	if leftBool, lok := leftVal.(bool); lok {
		if rightBool, rok := rightVal.(bool); rok {
			switch op {
			case "==":
				return leftBool == rightBool
			case "!=":
				return leftBool != rightBool
			default:
				return false
			}
		}
	}

	leftStr := fmt.Sprintf("%v", leftVal)
	rightStr := fmt.Sprintf("%v", rightVal)
	switch op {
	case "==":
		return leftStr == rightStr
	case "!=":
		return leftStr != rightStr
	default:
		return false
	}
}

func resolveValue(token string, ctx *Context) (interface{}, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}

	if token == "hour" {
		return ctx.Time.Hour(), true
	}

	if strings.HasPrefix(token, "attribute.") {
		key := strings.TrimPrefix(token, "attribute.")
		val, ok := ctx.Attributes[key]
		return val, ok
	}

	val, ok := ctx.Attributes[token]
	if ok {
		if s, isString := val.(string); isString && s == "self" {
			return ctx.ActorID, true
		}
		return val, true
	}

	if token == "owner" {
		val, ok := ctx.Attributes["owner"]
		if ok {
			if s, isString := val.(string); isString && s == "self" {
				return ctx.ActorID, true
			}
		}
		return val, ok
	}

	return nil, false
}

func resolveLiteral(token string, ctx *Context) (interface{}, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}

	if token == "self" {
		return ctx.ActorID, true
	}

	if strings.HasPrefix(token, "'") && strings.HasSuffix(token, "'") && len(token) >= 2 {
		return token[1 : len(token)-1], true
	}
	if strings.HasPrefix(token, "\"") && strings.HasSuffix(token, "\"") && len(token) >= 2 {
		return token[1 : len(token)-1], true
	}

	if b, err := strconv.ParseBool(token); err == nil {
		return b, true
	}

	if f, err := strconv.ParseFloat(token, 64); err == nil {
		return f, true
	}

	if v, ok := ctx.Attributes[token]; ok {
		if s, isString := v.(string); isString && s == "self" {
			return ctx.ActorID, true
		}
		return v, true
	}

	return token, true
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// ValidateManifest valida se o manifesto tem configurações de segurança válidas
func ValidateManifest(m *manifest.Manifest) error {
	var errs []error

	// Valida modelo de autorização
	auth := m.Security.Authorization
	if auth.Model == "" {
		errs = append(errs, errors.New("authorization model is required"))
	}

	if auth.Model != "rbac" && auth.Model != "abac" && auth.Model != "acl" {
		errs = append(errs, fmt.Errorf("invalid authorization model: %s (must be rbac, abac, or acl)", auth.Model))
	}

	// Valida hierarquia de papéis (verifica ciclos)
	if err := validateRoleHierarchy(auth.RoleHierarchy); err != nil {
		errs = append(errs, err)
	}

	// Valida autenticação
	if len(m.Security.Authentication.Methods) == 0 {
		errs = append(errs, errors.New("at least one authentication method is required"))
	}

	if len(errs) > 0 {
		return fmt.Errorf("security validation failed: %v", errs)
	}

	return nil
}

// validateRoleHierarchy verifica se há ciclos na hierarquia de papéis
func validateRoleHierarchy(hierarchy []manifest.RoleHierarchy) error {
	graph := make(map[string][]string)
	for _, rh := range hierarchy {
		graph[rh.Role] = rh.Inherits
	}

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		if recStack[node] {
			return true
		}
		if visited[node] {
			return false
		}

		visited[node] = true
		recStack[node] = true

		for _, neighbor := range graph[node] {
			if hasCycle(neighbor) {
				return true
			}
		}

		recStack[node] = false
		return false
	}

	for node := range graph {
		if hasCycle(node) {
			return fmt.Errorf("cycle detected in role hierarchy involving role: %s", node)
		}
	}

	return nil
}
