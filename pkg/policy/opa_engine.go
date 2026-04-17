package policy

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/open-policy-agent/opa/rego"
)

type PolicyEngine struct {
	policies map[string]*compiledPolicy
}

type compiledPolicy struct {
	name       string
	rule       string
	packageRef string
	prepared   rego.PreparedEvalQuery
	compileErr error
}

func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		policies: make(map[string]*compiledPolicy),
	}
}

func (e *PolicyEngine) RegisterPolicy(name, rule string) {
	cp := &compiledPolicy{name: name, rule: rule}
	cp.packageRef = "picoclaw.policy." + sanitizePackageSegment(name)
	module, err := buildRegoModule(cp.packageRef, rule)
	if err != nil {
		cp.compileErr = err
		e.policies[name] = cp
		return
	}

	query := "data." + cp.packageRef + ".allow"
	prepared, err := rego.New(
		rego.Query(query),
		rego.Module("policy_"+sanitizeModuleName(name)+".rego", module),
	).PrepareForEval(context.Background())
	if err != nil {
		cp.compileErr = err
		e.policies[name] = cp
		return
	}

	cp.prepared = prepared
	e.policies[name] = cp
}

type EvalContext struct {
	User     map[string]interface{}
	Action   string
	Resource map[string]interface{}
	State    string
}

func (e *PolicyEngine) Evaluate(policyName string, ctx EvalContext) (bool, error) {
	policy, exists := e.policies[policyName]
	if !exists {
		return false, fmt.Errorf("política não encontrada: %s", policyName)
	}

	if policy.compileErr != nil {
		return false, fmt.Errorf("erro ao compilar política %s: %w", policyName, policy.compileErr)
	}

	input := map[string]interface{}{
		"user":     ctx.User,
		"action":   ctx.Action,
		"resource": ctx.Resource,
		"state":    ctx.State,
	}

	rs, err := policy.prepared.Eval(context.Background(), rego.EvalInput(input))
	if err != nil {
		return false, fmt.Errorf("erro ao avaliar política %s: %w", policyName, err)
	}

	for _, r := range rs {
		for _, exp := range r.Expressions {
			if b, ok := exp.Value.(bool); ok && b {
				return true, nil
			}
		}
	}

	return false, nil
}

type exprKind int

const (
	exprAtom exprKind = iota
	exprAnd
	exprOr
	exprNot
)

type exprNode struct {
	kind     exprKind
	atom     string
	children []*exprNode
}

type atomCond struct {
	expr string
	neg  bool
}

func buildRegoModule(packageRef, rule string) (string, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", fmt.Errorf("regra vazia")
	}

	bodies, err := toRegoBodies(rule)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("package ")
	b.WriteString(packageRef)
	b.WriteString("\n\n")
	b.WriteString("default allow = false\n\n")
	b.WriteString("user := input.user\n")
	b.WriteString("action := input.action\n")
	b.WriteString("resource := input.resource\n")
	b.WriteString("state := input.state\n\n")

	for _, body := range bodies {
		b.WriteString("allow {\n")
		if body == "" {
			b.WriteString("  true\n")
		} else {
			for _, line := range strings.Split(body, "\n") {
				b.WriteString("  ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		b.WriteString("}\n\n")
	}

	return b.String(), nil
}

func toRegoBodies(rule string) ([]string, error) {
	switch strings.TrimSpace(rule) {
	case "true", "allow":
		return []string{""}, nil
	case "false", "deny":
		return []string{"false"}, nil
	}

	normalized, err := normalizeRule(rule)
	if err != nil {
		return nil, err
	}

	parsed, err := parseExpr(normalized)
	if err != nil {
		return nil, err
	}

	nnf := toNNF(parsed, false)
	dnf := toDNF(nnf)
	if len(dnf) == 0 {
		return []string{"false"}, nil
	}

	bodies := make([]string, 0, len(dnf))
	for _, conj := range dnf {
		if len(conj) == 0 {
			bodies = append(bodies, "")
			continue
		}
		lines := make([]string, 0, len(conj))
		for _, a := range conj {
			if strings.TrimSpace(a.expr) == "" {
				continue
			}
			if a.neg {
				lines = append(lines, "not ("+a.expr+")")
			} else {
				lines = append(lines, a.expr)
			}
		}
		bodies = append(bodies, strings.Join(lines, "\n"))
	}

	return bodies, nil
}

func normalizeRule(rule string) (string, error) {
	rule = strings.TrimSpace(rule)
	rule = strings.ReplaceAll(rule, "\r\n", "\n")
	rule = strings.ReplaceAll(rule, "\r", "\n")

	var out strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(rule); i++ {
		ch := rule[i]
		if escape {
			out.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' {
			out.WriteByte(ch)
			escape = true
			continue
		}
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
				out.WriteByte('"')
				continue
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		}
		out.WriteByte(ch)
	}
	if inSingle || inDouble {
		return "", fmt.Errorf("string literal não terminada")
	}
	return out.String(), nil
}

func parseExpr(s string) (*exprNode, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("expressão vazia")
	}
	return parseOr(s)
}

func parseOr(s string) (*exprNode, error) {
	parts := splitByOperator(s, "||")
	if len(parts) > 1 {
		children := make([]*exprNode, 0, len(parts))
		for _, p := range parts {
			n, err := parseAnd(p)
			if err != nil {
				return nil, err
			}
			children = append(children, n)
		}
		return &exprNode{kind: exprOr, children: children}, nil
	}
	return parseAnd(s)
}

func parseAnd(s string) (*exprNode, error) {
	parts := splitByOperator(s, "&&")
	if len(parts) > 1 {
		children := make([]*exprNode, 0, len(parts))
		for _, p := range parts {
			n, err := parseUnary(p)
			if err != nil {
				return nil, err
			}
			children = append(children, n)
		}
		return &exprNode{kind: exprAnd, children: children}, nil
	}
	return parseUnary(s)
}

func parseUnary(s string) (*exprNode, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("expressão vazia")
	}

	for {
		inner, ok := stripOuterParens(s)
		if !ok {
			break
		}
		s = strings.TrimSpace(inner)
	}

	if strings.HasPrefix(s, "!") {
		child, err := parseUnary(strings.TrimSpace(s[1:]))
		if err != nil {
			return nil, err
		}
		return &exprNode{kind: exprNot, children: []*exprNode{child}}, nil
	}
	if strings.HasPrefix(s, "not ") {
		child, err := parseUnary(strings.TrimSpace(s[4:]))
		if err != nil {
			return nil, err
		}
		return &exprNode{kind: exprNot, children: []*exprNode{child}}, nil
	}

	if inLeft, inRight, ok := splitInOperator(s); ok {
		alts, err := parseInList(inRight)
		if err != nil {
			return nil, err
		}
		if len(alts) == 0 {
			return &exprNode{kind: exprAtom, atom: "false"}, nil
		}
		children := make([]*exprNode, 0, len(alts))
		for _, lit := range alts {
			children = append(children, &exprNode{kind: exprAtom, atom: strings.TrimSpace(inLeft) + " == " + lit})
		}
		if len(children) == 1 {
			return children[0], nil
		}
		return &exprNode{kind: exprOr, children: children}, nil
	}

	return &exprNode{kind: exprAtom, atom: strings.TrimSpace(s)}, nil
}

func stripOuterParens(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return "", false
	}
	if !balancedParens(s[1 : len(s)-1]) {
		return "", false
	}
	return s[1 : len(s)-1], true
}

func balancedParens(s string) bool {
	inSingle := false
	inDouble := false
	inBrackets := 0
	depth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		}
		if inSingle || inDouble {
			continue
		}
		switch ch {
		case '[':
			inBrackets++
		case ']':
			inBrackets--
		case '(':
			if inBrackets == 0 {
				depth++
			}
		case ')':
			if inBrackets == 0 {
				depth--
				if depth < 0 {
					return false
				}
			}
		}
	}
	return depth == 0 && inBrackets == 0 && !inSingle && !inDouble
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
	inSingle := false
	inDouble := false
	escape := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escape {
			current.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' {
			current.WriteByte(ch)
			escape = true
			continue
		}

		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		}

		if !inSingle && !inDouble {
			switch ch {
			case '(':
				inParens++
			case ')':
				inParens--
			case '[':
				inBrackets++
			case ']':
				inBrackets--
			}
		}

		if !inSingle && !inDouble && inParens == 0 && inBrackets == 0 && strings.HasPrefix(s[i:], op) {
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

func splitInOperator(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}

	inParens := 0
	inBrackets := 0
	inSingle := false
	inDouble := false
	escape := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escape {
			escape = false
			continue
		}
		if ch == '\\' {
			escape = true
			continue
		}

		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		}

		if inSingle || inDouble {
			continue
		}

		switch ch {
		case '(':
			inParens++
		case ')':
			inParens--
		case '[':
			inBrackets++
		case ']':
			inBrackets--
		}

		if inParens == 0 && inBrackets == 0 && strings.HasPrefix(s[i:], " in ") {
			left := strings.TrimSpace(s[:i])
			right := strings.TrimSpace(s[i+4:])
			return left, right, left != "" && right != ""
		}
	}

	return "", "", false
}

func parseInList(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	matches := regexp.MustCompile(`^\[(.*)\]$`).FindStringSubmatch(s)
	if len(matches) != 2 {
		return nil, fmt.Errorf("lista inválida em operador in: %s", s)
	}

	inner := strings.TrimSpace(matches[1])
	if inner == "" {
		return []string{}, nil
	}

	values := splitCommaSeparated(inner)
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func splitCommaSeparated(s string) []string {
	var parts []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	inParens := 0
	inBrackets := 0

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escape {
			cur.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' {
			cur.WriteByte(ch)
			escape = true
			continue
		}

		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		}

		if !inSingle && !inDouble {
			switch ch {
			case '(':
				inParens++
			case ')':
				inParens--
			case '[':
				inBrackets++
			case ']':
				inBrackets--
			}
		}

		if !inSingle && !inDouble && inParens == 0 && inBrackets == 0 && ch == ',' {
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(ch)
	}

	parts = append(parts, strings.TrimSpace(cur.String()))
	return parts
}

func toNNF(n *exprNode, neg bool) *exprNode {
	switch n.kind {
	case exprNot:
		return toNNF(n.children[0], !neg)
	case exprAtom:
		if neg {
			return &exprNode{kind: exprNot, children: []*exprNode{{kind: exprAtom, atom: n.atom}}}
		}
		return &exprNode{kind: exprAtom, atom: n.atom}
	case exprAnd:
		children := make([]*exprNode, 0, len(n.children))
		if neg {
			for _, c := range n.children {
				children = append(children, toNNF(c, true))
			}
			return &exprNode{kind: exprOr, children: children}
		}
		for _, c := range n.children {
			children = append(children, toNNF(c, false))
		}
		return &exprNode{kind: exprAnd, children: children}
	case exprOr:
		children := make([]*exprNode, 0, len(n.children))
		if neg {
			for _, c := range n.children {
				children = append(children, toNNF(c, true))
			}
			return &exprNode{kind: exprAnd, children: children}
		}
		for _, c := range n.children {
			children = append(children, toNNF(c, false))
		}
		return &exprNode{kind: exprOr, children: children}
	default:
		if neg {
			return &exprNode{kind: exprNot, children: []*exprNode{{kind: exprAtom, atom: "false"}}}
		}
		return &exprNode{kind: exprAtom, atom: "false"}
	}
}

func toDNF(n *exprNode) [][]atomCond {
	switch n.kind {
	case exprAtom:
		if strings.TrimSpace(n.atom) == "" {
			return [][]atomCond{}
		}
		return [][]atomCond{{{expr: strings.TrimSpace(n.atom)}}}
	case exprNot:
		if len(n.children) != 1 || n.children[0].kind != exprAtom {
			return [][]atomCond{}
		}
		atom := strings.TrimSpace(n.children[0].atom)
		if atom == "" {
			return [][]atomCond{}
		}
		return [][]atomCond{{{expr: atom, neg: true}}}
	case exprOr:
		var out [][]atomCond
		for _, c := range n.children {
			out = append(out, toDNF(c)...)
		}
		return out
	case exprAnd:
		out := [][]atomCond{{}}
		for _, c := range n.children {
			right := toDNF(c)
			if len(right) == 0 {
				return [][]atomCond{}
			}
			var next [][]atomCond
			for _, a := range out {
				for _, b := range right {
					merged := make([]atomCond, 0, len(a)+len(b))
					merged = append(merged, a...)
					merged = append(merged, b...)
					next = append(next, merged)
				}
			}
			out = next
		}
		return out
	default:
		return [][]atomCond{}
	}
}

func sanitizePackageSegment(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "policy"
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
			b.WriteByte(ch)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if out[0] >= '0' && out[0] <= '9' {
		out = "p_" + out
	}
	out = strings.Trim(out, "_")
	if out == "" {
		return "policy"
	}
	return out
}

func sanitizeModuleName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "policy"
	}
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, " ", "_")
	if s == "" {
		return "policy"
	}
	return s
}
