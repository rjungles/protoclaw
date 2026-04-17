package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type RuleExecutor struct {
	manifest *manifest.Manifest
}

func NewRuleExecutor(m *manifest.Manifest) *RuleExecutor {
	return &RuleExecutor{manifest: m}
}

func (e *RuleExecutor) ExecuteBefore(ctx context.Context, event, entityName string, data map[string]interface{}) error {
	rules := e.findRules(event, entityName, true, false)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.Condition != "" && !e.evaluateCondition(rule.Condition, data) {
			continue
		}
		for _, action := range rule.Actions {
			if err := e.executeAction(ctx, action, data); err != nil {
				return fmt.Errorf("rule %s action failed: %w", rule.ID, err)
			}
		}
	}
	return nil
}

func (e *RuleExecutor) ExecuteAfter(ctx context.Context, event, entityName string, data map[string]interface{}) error {
	rules := e.findRules(event, entityName, false, true)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.Condition != "" && !e.evaluateCondition(rule.Condition, data) {
			continue
		}
		for _, action := range rule.Actions {
			if err := e.executeAction(ctx, action, data); err != nil {
				return fmt.Errorf("rule %s action failed: %w", rule.ID, err)
			}
		}
	}
	return nil
}

func (e *RuleExecutor) findRules(event, entityName string, before, after bool) []manifest.BusinessRule {
	var result []manifest.BusinessRule
	for _, rule := range e.manifest.BusinessRules {
		if rule.Trigger.Event != event {
			continue
		}
		if rule.Trigger.Before != before || rule.Trigger.After != after {
			continue
		}
		for _, entity := range rule.Trigger.Entities {
			if entity == entityName {
				result = append(result, rule)
				break
			}
		}
	}
	return result
}

func (e *RuleExecutor) evaluateCondition(condition string, data map[string]interface{}) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" || condition == "true" {
		return true
	}
	if condition == "false" {
		return false
	}
	return e.evalSimpleExpression(condition, data)
}

func (e *RuleExecutor) evalSimpleExpression(expr string, data map[string]interface{}) bool {
	expr = strings.TrimSpace(expr)

	if strings.Contains(expr, " && ") {
		parts := strings.Split(expr, " && ")
		for _, part := range parts {
			if !e.evalSimpleExpression(part, data) {
				return false
			}
		}
		return true
	}

	if strings.Contains(expr, " || ") {
		parts := strings.Split(expr, " || ")
		for _, part := range parts {
			if e.evalSimpleExpression(part, data) {
				return true
			}
		}
		return false
	}

	switch {
	case strings.Contains(expr, " == "):
		parts := strings.SplitN(expr, " == ", 2)
		return e.compareValues(parts[0], parts[1], data, "==")
	case strings.Contains(expr, " != "):
		parts := strings.SplitN(expr, " != ", 2)
		return e.compareValues(parts[0], parts[1], data, "!=")
	case strings.Contains(expr, " >= "):
		parts := strings.SplitN(expr, " >= ", 2)
		return e.compareValues(parts[0], parts[1], data, ">=")
	case strings.Contains(expr, " <= "):
		parts := strings.SplitN(expr, " <= ", 2)
		return e.compareValues(parts[0], parts[1], data, "<=")
	case strings.Contains(expr, " > "):
		parts := strings.SplitN(expr, " > ", 2)
		return e.compareValues(parts[0], parts[1], data, ">")
	case strings.Contains(expr, " < "):
		parts := strings.SplitN(expr, " < ", 2)
		return e.compareValues(parts[0], parts[1], data, "<")
	}

	return true
}

func (e *RuleExecutor) compareValues(left, right string, data map[string]interface{}, op string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	leftVal := e.resolveValue(left, data)
	rightVal := e.resolveValue(right, data)

	leftStr := fmt.Sprintf("%v", leftVal)
	rightStr := fmt.Sprintf("%v", rightVal)

	switch op {
	case "==":
		return leftStr == rightStr
	case "!=":
		return leftStr != rightStr
	case ">=", "<=", ">", "<":
		return e.compareNumeric(leftStr, rightStr, op)
	}

	return false
}

func (e *RuleExecutor) resolveValue(token string, data map[string]interface{}) interface{} {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, `"'`)

	if val, ok := data[token]; ok {
		return val
	}

	return token
}

func (e *RuleExecutor) compareNumeric(left, right, op string) bool {
	var leftNum, rightNum float64
	var leftIsNum, rightIsNum bool

	if n, err := fmt.Sscanf(left, "%f", &leftNum); err == nil && n == 1 {
		leftIsNum = true
	}
	if n, err := fmt.Sscanf(right, "%f", &rightNum); err == nil && n == 1 {
		rightIsNum = true
	}

	if leftIsNum && rightIsNum {
		switch op {
		case ">=":
			return leftNum >= rightNum
		case "<=":
			return leftNum <= rightNum
		case ">":
			return leftNum > rightNum
		case "<":
			return leftNum < rightNum
		}
	}

	return false
}

func (e *RuleExecutor) executeAction(ctx context.Context, action manifest.RuleAction, data map[string]interface{}) error {
	switch action.Type {
	case "validate":
		return nil
	case "transform":
		if action.Target != "" && action.Script != "" {
			data[action.Target] = action.Script
		}
		return nil
	case "notify":
		return nil
	case "execute":
		return nil
	case "reject":
		return fmt.Errorf("action rejected: %s", action.Target)
	default:
		return nil
	}
}
