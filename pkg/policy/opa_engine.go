package policy

import (
	"fmt"
	"regexp"
	"strings"
)

// PolicyEngine é uma engine de políticas compatível com Go 1.19
// Suporta regras no formato: "role in ['admin', 'editor'] && state == 'DRAFT'"
type PolicyEngine struct {
	policies map[string]*Policy
}

// Policy representa uma política com nome e regra
type Policy struct {
	Name  string
	Rule  string
}

// NewPolicyEngine cria uma nova engine de políticas
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		policies: make(map[string]*Policy),
	}
}

// RegisterPolicy registra uma política na engine
func (e *PolicyEngine) RegisterPolicy(name, rule string) {
	e.policies[name] = &Policy{
		Name: name,
		Rule: rule,
	}
}

// EvalContext contexto para avaliação de políticas
type EvalContext struct {
	User     map[string]interface{}
	Action   string
	Resource map[string]interface{}
	State    string
}

// Evaluate avalia uma política contra o contexto fornecido
func (e *PolicyEngine) Evaluate(policyName string, ctx EvalContext) (bool, error) {
	policy, exists := e.policies[policyName]
	if !exists {
		return false, fmt.Errorf("política não encontrada: %s", policyName)
	}

	result, err := evaluateRule(policy.Rule, ctx)
	if err != nil {
		return false, fmt.Errorf("erro ao avaliar política %s: %w", policyName, err)
	}

	return result, nil
}

// evaluateRule avalia uma regra simples
func evaluateRule(rule string, ctx EvalContext) (bool, error) {
	// Remove espaços extras
	rule = strings.TrimSpace(rule)
	
	// Handle default allow/deny
	if rule == "true" || rule == "allow" {
		return true, nil
	}
	if rule == "false" || rule == "deny" {
		return false, nil
	}

	// Avalia expressões compostas (AND, OR)
	return evalExpression(rule, ctx)
}

func evalExpression(expr string, ctx EvalContext) (bool, error) {
	expr = strings.TrimSpace(expr)
	
	// Handle OR expressions
	orParts := splitByOperator(expr, "||")
	if len(orParts) > 1 {
		for _, part := range orParts {
			result, err := evalExpression(strings.TrimSpace(part), ctx)
			if err != nil {
				return false, err
			}
			if result {
				return true, nil
			}
		}
		return false, nil
	}

	// Handle AND expressions - split by && and evaluate each part
	andParts := splitByOperator(expr, "&&")
	if len(andParts) > 1 {
		for _, part := range andParts {
			result, err := evalExpression(strings.TrimSpace(part), ctx)
			if err != nil {
				return false, err
			}
			if !result {
				return false, nil
			}
		}
		return true, nil
	}

	// Handle NOT expressions
	if strings.HasPrefix(expr, "!") || strings.HasPrefix(expr, "not ") {
		var inner string
		if strings.HasPrefix(expr, "!") {
			inner = strings.TrimSpace(expr[1:])
		} else {
			inner = strings.TrimSpace(expr[4:])
		}
		result, err := evalExpression(inner, ctx)
		if err != nil {
			return false, err
		}
		return !result, nil
	}

	// Handle parenthesized expressions
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return evalExpression(expr[1:len(expr)-1], ctx)
	}

	// Handle comparison operators
	return evalComparison(expr, ctx)
}

func splitByOperator(s, op string) []string {
	var parts []string
	current := ""
	inParens := 0
	inBrackets := 0
	
	for i := 0; i < len(s); i++ {
		char := s[i]
		
		if char == '(' {
			inParens++
			current += string(char)
		} else if char == ')' {
			inParens--
			current += string(char)
		} else if char == '[' {
			inBrackets++
			current += string(char)
		} else if char == ']' {
			inBrackets--
			current += string(char)
		} else if inParens == 0 && inBrackets == 0 && strings.HasPrefix(s[i:], op) {
			parts = append(parts, current)
			current = ""
			i += len(op) - 1
		} else {
			current += string(char)
		}
	}
	
	if current != "" {
		parts = append(parts, current)
	}
	
	return parts
}

func evalComparison(expr string, ctx EvalContext) (bool, error) {
	expr = strings.TrimSpace(expr)
	
	// Handle "in" operator: role in ['admin', 'editor']
	inMatch := regexp.MustCompile(`(\w+(?:\.\w+)*)\s+in\s+\[([^\]]+)\]`).FindStringSubmatch(expr)
	if len(inMatch) == 3 {
		value, err := getValueFromPath(inMatch[1], ctx)
		if err != nil {
			return false, err
		}
		
		values := parseArray(inMatch[2])
		for _, v := range values {
			v = strings.TrimSpace(v)
			if compareValues(value, v) {
				return true, nil
			}
		}
		return false, nil
	}

	// Handle equality/inequality operators - process ALL comparisons in the expression
	operators := []string{"==", "!=", ">=", "<=", ">", "<"}
	allPassed := true
	
	for _, op := range operators {
		parts := strings.SplitN(expr, op, 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			
			leftVal, err := getValueFromPath(left, ctx)
			if err != nil {
				// Se não conseguir obter valor, trata como literal
				leftVal = left
			}
			
			rightVal, err := getValueFromPath(right, ctx)
			if err != nil {
				// Se não conseguir obter valor, trata como literal
				rightVal = strings.Trim(right, "'\"")
			}
			
			result, err := compareWithOperator(leftVal, rightVal, op)
			if err != nil {
				return false, err
			}
			if !result {
				allPassed = false
			}
		}
	}
	
	return allPassed, nil
}

func getValueFromPath(path string, ctx EvalContext) (interface{}, error) {
	path = strings.TrimSpace(path)
	
	// Remove aspas se for string literal
	if (strings.HasPrefix(path, "'") && strings.HasSuffix(path, "'")) ||
	   (strings.HasPrefix(path, "\"") && strings.HasSuffix(path, "\"")) {
		return strings.Trim(path, "'\""), nil
	}

	// Handle numeric literals
	if strings.Contains(path, ".") {
		// Pode ser float ou path
		if _, err := fmt.Sscanf(path, "%f", new(float64)); err == nil && !strings.Contains(path, "user.") && !strings.Contains(path, "resource.") {
			return path, nil
		}
	}
	
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, fmt.Errorf("path inválido: %s", path)
	}

	var current interface{}
	
	switch parts[0] {
	case "user":
		current = ctx.User
	case "resource":
		current = ctx.Resource
	case "state":
		current = ctx.State
	case "action":
		current = ctx.Action
	default:
		return nil, fmt.Errorf("contexto desconhecido: %s", parts[0])
	}

	for i := 1; i < len(parts); i++ {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[parts[i]]
		} else {
			return nil, fmt.Errorf("path inválido em %s", parts[i])
		}
	}

	return current, nil
}

func parseArray(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		result = append(result, strings.TrimSpace(p))
	}
	return result
}

func compareValues(a, b interface{}) bool {
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	
	// Remove quotes from strings
	aStr = strings.Trim(aStr, "'\"")
	bStr = strings.Trim(bStr, "'\"")
	
	return aStr == bStr
}

func compareWithOperator(left, right interface{}, op string) (bool, error) {
	// Tenta converter para números se possível
	var leftNum, rightNum float64
	var leftIsNum, rightIsNum bool
	
	if _, err := fmt.Sscanf(fmt.Sprintf("%v", left), "%f", &leftNum); err == nil {
		leftIsNum = true
	}
	if _, err := fmt.Sscanf(fmt.Sprintf("%v", right), "%f", &rightNum); err == nil {
		rightIsNum = true
	}
	
	if leftIsNum && rightIsNum {
		switch op {
		case "==":
			return leftNum == rightNum, nil
		case "!=":
			return leftNum != rightNum, nil
		case ">=":
			return leftNum >= rightNum, nil
		case "<=":
			return leftNum <= rightNum, nil
		case ">":
			return leftNum > rightNum, nil
		case "<":
			return leftNum < rightNum, nil
		}
	}
	
	// Comparação de strings
	leftStr := fmt.Sprintf("%v", left)
	rightStr := fmt.Sprintf("%v", right)
	leftStr = strings.Trim(leftStr, "'\"")
	rightStr = strings.Trim(rightStr, "'\"")
	
	switch op {
	case "==":
		return leftStr == rightStr, nil
	case "!=":
		return leftStr != rightStr, nil
	default:
		return false, fmt.Errorf("operador %s não suportado para strings", op)
	}
}

func isTruthy(value interface{}) bool {
	if value == nil {
		return false
	}
	
	switch v := value.(type) {
	case bool:
		return v
	case int, int8, int16, int32, int64:
		return v != 0
	case float32, float64:
		return v != 0
	case string:
		return v != "" && v != "false"
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	default:
		return true
	}
}
