package policy_test

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/policy"
)

func TestPolicyEngine_ExperiencePlatform(t *testing.T) {
	engine := policy.NewPolicyEngine()

	// Registrar políticas do sistema de experiências - simplificadas para a engine atual
	engine.RegisterPolicy("author_check", "user.role == 'creator'")
	engine.RegisterPolicy("editor_check", "user.role == 'reviewer'")
	engine.RegisterPolicy("publisher_check", "user.role == 'publisher'")
	engine.RegisterPolicy("reader_check", "user.role == 'consumer'")
	engine.RegisterPolicy("author_action", "user.role == 'creator' && action == 'submit_review'")
	engine.RegisterPolicy("editor_action", "user.role == 'reviewer' && action == 'request_clarification'")

	tests := []struct {
		name          string
		policyName    string
		ctx           policy.EvalContext
		expectedAllow bool
	}{
		{
			name:       "Autor identificado corretamente",
			policyName: "author_check",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "creator",
				},
				Action: "read",
				State:  "DRAFT",
			},
			expectedAllow: true,
		},
		{
			name:       "Editor NÃO é autor",
			policyName: "author_check",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "reviewer",
				},
				Action: "read",
				State:  "DRAFT",
			},
			expectedAllow: false,
		},
		{
			name:       "Editor identificado corretamente",
			policyName: "editor_check",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "reviewer",
				},
				Action: "request_clarification",
				State:  "UNDER_REVIEW",
			},
			expectedAllow: true,
		},
		{
			name:       "Publisher identificado corretamente",
			policyName: "publisher_check",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "publisher",
				},
				Action: "approve_publish",
				State:  "DRAFTING",
			},
			expectedAllow: true,
		},
		{
			name:       "Leitor identificado corretamente",
			policyName: "reader_check",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "consumer",
				},
				Action: "read",
				State:  "PUBLISHED",
			},
			expectedAllow: true,
		},
		{
			name:       "Autor pode submeter para review",
			policyName: "author_action",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "creator",
				},
				Action: "submit_review",
				State:  "DRAFT",
			},
			expectedAllow: true,
		},
		{
			name:       "Editor não pode submeter review",
			policyName: "author_action",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "reviewer",
				},
				Action: "submit_review",
				State:  "DRAFT",
			},
			expectedAllow: false,
		},
		{
			name:       "Editor pode solicitar clarificação",
			policyName: "editor_action",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "reviewer",
				},
				Action: "request_clarification",
				State:  "UNDER_REVIEW",
			},
			expectedAllow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := engine.Evaluate(tt.policyName, tt.ctx)
			if err != nil {
				t.Errorf("Erro na avaliação: %v", err)
			}
			if allowed != tt.expectedAllow {
				t.Errorf("Esperava allow=%v, mas obteve allow=%v", tt.expectedAllow, allowed)
			}
		})
	}
}

func TestPolicyEngine_InOperator(t *testing.T) {
	engine := policy.NewPolicyEngine()
	
	engine.RegisterPolicy("role_check", "user.role in ['admin', 'editor', 'reviewer']")
	
	tests := []struct {
		name          string
		ctx           policy.EvalContext
		expectedAllow bool
	}{
		{
			name: "Role admin permitido",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "admin",
				},
			},
			expectedAllow: true,
		},
		{
			name: "Role editor permitido",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "editor",
				},
			},
			expectedAllow: true,
		},
		{
			name: "Role viewer negado",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "viewer",
				},
			},
			expectedAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := engine.Evaluate("role_check", tt.ctx)
			if err != nil {
				t.Errorf("Erro na avaliação: %v", err)
			}
			if allowed != tt.expectedAllow {
				t.Errorf("Esperava allow=%v, mas obteve allow=%v", tt.expectedAllow, allowed)
			}
		})
	}
}

func TestPolicyEngine_AndOrOperators(t *testing.T) {
	engine := policy.NewPolicyEngine()
	
	engine.RegisterPolicy("complex_and", "user.role == 'admin' && state == 'DRAFT'")
	engine.RegisterPolicy("complex_or", "user.role == 'admin' || user.role == 'editor'")
	
	tests := []struct {
		name          string
		policyName    string
		ctx           policy.EvalContext
		expectedAllow bool
	}{
		{
			name:       "AND: ambas condições verdadeiras",
			policyName: "complex_and",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "admin",
				},
				State: "DRAFT",
			},
			expectedAllow: true,
		},
		{
			name:       "AND: uma condição falsa",
			policyName: "complex_and",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "admin",
				},
				State: "PUBLISHED",
			},
			expectedAllow: false,
		},
		{
			name:       "OR: primeira verdadeira",
			policyName: "complex_or",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "admin",
				},
			},
			expectedAllow: true,
		},
		{
			name:       "OR: segunda verdadeira",
			policyName: "complex_or",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "editor",
				},
			},
			expectedAllow: true,
		},
		{
			name:       "OR: ambas falsas",
			policyName: "complex_or",
			ctx: policy.EvalContext{
				User: map[string]interface{}{
					"role": "viewer",
				},
			},
			expectedAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := engine.Evaluate(tt.policyName, tt.ctx)
			if err != nil {
				t.Errorf("Erro na avaliação: %v", err)
			}
			if allowed != tt.expectedAllow {
				t.Errorf("Esperava allow=%v, mas obteve allow=%v", tt.expectedAllow, allowed)
			}
		})
	}
}
