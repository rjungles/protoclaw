package policy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

// Contexto de avaliação de política
type Context struct {
	ActorID      string                 // ID do ator solicitante
	Roles        []string               // Papéis do ator
	Resource     string                 // Recurso sendo acessado
	Action       string                 // Ação sendo executada (read, write, delete, execute)
	Attributes   map[string]interface{} // Atributos contextuais
	Time         time.Time              // Tempo da requisição
	Metadata     map[string]string      // Metadados adicionais
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
	manifest        *manifest.Manifest
	roleHierarchy   map[string][]string // Mapeia papel -> papéis herdados
	actorRoles      map[string][]string // Mapeia ator -> papéis
	actorPermissions map[string][]manifest.Permission // Mapeia ator -> permissões
	contextConditions []manifest.ContextCondition
	defaultDeny     bool
}

// NovaEngine cria uma nova engine de políticas a partir de um manifesto
func NewEngine(m *manifest.Manifest) (*Engine, error) {
	engine := &Engine{
		manifest:         m,
		roleHierarchy:    make(map[string][]string),
		actorRoles:       make(map[string][]string),
		actorPermissions: make(map[string][]manifest.Permission),
		contextConditions: m.Security.Authorization.ContextConditions,
		defaultDeny:      m.Security.Authorization.DefaultDeny,
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

	allRoles := make(map[string]bool)
	queue := append([]string{}, directRoles...)

	for len(queue) > 0 {
		role := queue[0]
		queue = queue[1:]

		if allRoles[role] {
			continue
		}
		allRoles[role] = true

		// Adiciona papéis herdados
		inherited := e.roleHierarchy[role]
		queue = append(queue, inherited...)
	}

	result := make([]string, 0, len(allRoles))
	for role := range allRoles {
		result = append(result, role)
	}

	return result
}

// CheckPermission verifica se um ator tem permissão para uma ação específica
func (e *Engine) CheckPermission(ctx *Context) *Result {
	// Obtém todos os papéis do ator (com herança)
	allRoles := e.GetAllRoles(ctx.ActorID)
	
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
	// Expressões suportadas:
	// - owner == self
	// - hour >= 8 && hour <= 18
	// - attribute.key == "value"
	// - amount <= 500
	// - shift_active == true
	// - discount_percent <= 50
	
	expression = strings.TrimSpace(expression)
	
	// Caso especial: owner == self
	if expression == "owner == self" {
		if owner, ok := ctx.Attributes["owner"]; ok {
			return owner == ctx.ActorID
		}
		return false
	}
	
	// Verifica hora comercial
	if strings.Contains(expression, "hour") {
		hour := ctx.Time.Hour()
		
		if strings.Contains(expression, ">=") && strings.Contains(expression, "<=") {
			// Extrai limites (implementação simplificada)
			if strings.Contains(expression, "8") && strings.Contains(expression, "18") {
				return hour >= 8 && hour <= 18
			}
		}
	}
	
	// Verifica condições numéricas: amount <= 500, discount_percent <= 50, etc.
	if strings.Contains(expression, "<=") {
		parts := strings.Split(expression, "<=")
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			rightStr := strings.TrimSpace(parts[1])
			
			// Tenta converter o lado direito para número
			var rightVal float64
			if _, err := fmt.Sscanf(rightStr, "%f", &rightVal); err == nil {
				// Busca o valor no contexto
				if leftVal, ok := ctx.Attributes[left]; ok {
					switch v := leftVal.(type) {
					case int:
						return float64(v) <= rightVal
					case int64:
						return float64(v) <= rightVal
					case float32:
						return float64(v) <= rightVal
					case float64:
						return v <= rightVal
					case string:
						// Tenta converter string para número
						if numVal, err := fmt.Sscanf(v, "%f", &rightVal); err == nil && numVal == 1 {
							return rightVal <= rightVal
						}
					}
				}
			}
		}
	}
	
	// Verifica condições booleanas: shift_active == true, scope == "shift", etc.
	if strings.Contains(expression, "==") {
		parts := strings.Split(expression, "==")
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(strings.Trim(parts[1], "\"'"))
			
			// Verifica se é um atributo do contexto
			if val, ok := ctx.Attributes[left]; ok {
				switch v := val.(type) {
				case bool:
					return fmt.Sprintf("%v", v) == right
				case string:
					return v == right
				case int, int64, float32, float64:
					return fmt.Sprintf("%v", v) == right
				}
			}
			
			// Verifica se é um atributo com prefixo attribute.
			if strings.HasPrefix(left, "attribute.") {
				key := strings.TrimPrefix(left, "attribute.")
				if val, ok := ctx.Attributes[key]; ok {
					return fmt.Sprintf("%v", val) == right
				}
			}
		}
	}
	
	// Default: assume verdadeiro para condições não implementadas
	// Em produção, isso deveria retornar falso ou logar um warning
	return true
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
	resources := make(map[string]bool)
	
	for _, perms := range e.actorPermissions {
		for _, perm := range perms {
			resources[perm.Resource] = true
		}
	}
	
	result := make([]string, 0, len(resources))
	for r := range resources {
		result = append(result, r)
	}
	
	return result
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
