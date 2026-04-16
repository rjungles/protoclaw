package workflow_test

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/workflow"
)

func TestFSM_ExperiencePlatform(t *testing.T) {
	// Configuração do workflow de experiências
	config := workflow.FSMConfig{
		EntityName:   "experience",
		InitialState: "DRAFT",
		States: map[workflow.State]workflow.StateConfig{
			"DRAFT": {
				ID: "DRAFT",
				Transitions: []workflow.Transition{
					{
						To:           "UNDER_REVIEW",
						Action:       "submit_review",
						AllowedRoles: []string{"creator"},
					},
				},
			},
			"UNDER_REVIEW": {
				ID: "UNDER_REVIEW",
				Transitions: []workflow.Transition{
					{
						To:           "CLARIFICATION_NEEDED",
						Action:       "request_clarification",
						AllowedRoles: []string{"reviewer"},
					},
					{
						To:           "DRAFTING",
						Action:       "start_drafting",
						AllowedRoles: []string{"reviewer"},
					},
					{
						To:           "REJECTED",
						Action:       "reject",
						AllowedRoles: []string{"reviewer"},
					},
				},
			},
			"CLARIFICATION_NEEDED": {
				ID: "CLARIFICATION_NEEDED",
				Transitions: []workflow.Transition{
					{
						To:           "UNDER_REVIEW",
						Action:       "resubmit_for_review",
						AllowedRoles: []string{"creator"},
					},
					{
						To:           "DRAFT",
						Action:       "cancel_clarification",
						AllowedRoles: []string{"creator"},
					},
				},
			},
			"DRAFTING": {
				ID: "DRAFTING",
				Transitions: []workflow.Transition{
					{
						To:           "PUBLISHED",
						Action:       "approve_publish",
						AllowedRoles: []string{"publisher"},
					},
					{
						To:           "UNDER_REVIEW",
						Action:       "return_to_review",
						AllowedRoles: []string{"reviewer"},
					},
				},
			},
			"PUBLISHED": {
				ID: "PUBLISHED",
				Transitions: []workflow.Transition{
					{
						To:           "ARCHIVED",
						Action:       "archive",
						AllowedRoles: []string{"publisher"},
					},
				},
			},
			"REJECTED": {
				ID: "REJECTED",
				Transitions: []workflow.Transition{
					{
						To:           "DRAFT",
						Action:       "revise_and_resubmit",
						AllowedRoles: []string{"creator"},
					},
				},
			},
			"ARCHIVED": {
				ID: "ARCHIVED",
				Transitions: []workflow.Transition{
					{
						To:           "DRAFT",
						Action:       "restore",
						AllowedRoles: []string{"publisher"},
					},
				},
			},
		},
	}

	fsm, err := workflow.NewFSM(config)
	if err != nil {
		t.Fatalf("Falha ao criar FSM: %v", err)
	}

	tests := []struct {
		name           string
		currentState   workflow.State
		action         workflow.Action
		userRoles      []string
		expectedAllow  bool
		expectedNext   workflow.State
		expectedError  bool
	}{
		// Workflow do Autor
		{
			name:          "Autor pode submeter para review",
			currentState:  "DRAFT",
			action:        "submit_review",
			userRoles:     []string{"creator"},
			expectedAllow: true,
			expectedNext:  "UNDER_REVIEW",
		},
		{
			name:          "Autor NÃO pode submeter se não for creator",
			currentState:  "DRAFT",
			action:        "submit_review",
			userRoles:     []string{"consumer"},
			expectedAllow: false,
			expectedError: true,
		},
		{
			name:          "Autor pode resubmeter após clarificação",
			currentState:  "CLARIFICATION_NEEDED",
			action:        "resubmit_for_review",
			userRoles:     []string{"creator"},
			expectedAllow: true,
			expectedNext:  "UNDER_REVIEW",
		},
		{
			name:          "Autor pode cancelar clarificação",
			currentState:  "CLARIFICATION_NEEDED",
			action:        "cancel_clarification",
			userRoles:     []string{"creator"},
			expectedAllow: true,
			expectedNext:  "DRAFT",
		},
		{
			name:          "Autor pode revisar e resubmeter após rejeição",
			currentState:  "REJECTED",
			action:        "revise_and_resubmit",
			userRoles:     []string{"creator"},
			expectedAllow: true,
			expectedNext:  "DRAFT",
		},

		// Workflow do Editor (Reviewer)
		{
			name:          "Editor pode solicitar clarificação",
			currentState:  "UNDER_REVIEW",
			action:        "request_clarification",
			userRoles:     []string{"reviewer"},
			expectedAllow: true,
			expectedNext:  "CLARIFICATION_NEEDED",
		},
		{
			name:          "Editor pode iniciar drafting",
			currentState:  "UNDER_REVIEW",
			action:        "start_drafting",
			userRoles:     []string{"reviewer"},
			expectedAllow: true,
			expectedNext:  "DRAFTING",
		},
		{
			name:          "Editor pode rejeitar",
			currentState:  "UNDER_REVIEW",
			action:        "reject",
			userRoles:     []string{"reviewer"},
			expectedAllow: true,
			expectedNext:  "REJECTED",
		},
		{
			name:          "Editor pode retornar para review",
			currentState:  "DRAFTING",
			action:        "return_to_review",
			userRoles:     []string{"reviewer"},
			expectedAllow: true,
			expectedNext:  "UNDER_REVIEW",
		},

		// Workflow do Publisher
		{
			name:          "Publisher pode aprovar publicação",
			currentState:  "DRAFTING",
			action:        "approve_publish",
			userRoles:     []string{"publisher"},
			expectedAllow: true,
			expectedNext:  "PUBLISHED",
		},
		{
			name:          "Publisher pode arquivar",
			currentState:  "PUBLISHED",
			action:        "archive",
			userRoles:     []string{"publisher"},
			expectedAllow: true,
			expectedNext:  "ARCHIVED",
		},
		{
			name:          "Publisher pode restaurar arquivado",
			currentState:  "ARCHIVED",
			action:        "restore",
			userRoles:     []string{"publisher"},
			expectedAllow: true,
			expectedNext:  "DRAFT",
		},

		// Casos de erro
		{
			name:          "Ação inválida no estado atual",
			currentState:  "DRAFT",
			action:        "approve_publish",
			userRoles:     []string{"publisher"},
			expectedAllow: false,
			expectedError: true,
		},
		{
			name:          "Estado desconhecido",
			currentState:  "UNKNOWN_STATE",
			action:        "read",
			userRoles:     []string{"creator"},
			expectedAllow: false,
			expectedError: true,
		},
		{
			name:          "Leitor não pode executar ações de transição",
			currentState:  "PUBLISHED",
			action:        "archive",
			userRoles:     []string{"consumer"},
			expectedAllow: false,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, nextState, err := fsm.CanTransition(tt.currentState, tt.action, tt.userRoles)

			if tt.expectedError && err == nil {
				t.Errorf("Esperava erro, mas não ocorreu")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Erro inesperado: %v", err)
			}

			if allowed != tt.expectedAllow {
				t.Errorf("Esperava allow=%v, mas obteve allow=%v", tt.expectedAllow, allowed)
			}

			if tt.expectedAllow && nextState != tt.expectedNext {
				t.Errorf("Esperava next_state=%s, mas obteve next_state=%s", tt.expectedNext, nextState)
			}
		})
	}
}

func TestFSM_ListTransitions(t *testing.T) {
	config := workflow.FSMConfig{
		EntityName:   "experience",
		InitialState: "DRAFT",
		States: map[workflow.State]workflow.StateConfig{
			"DRAFT": {
				ID: "DRAFT",
				Transitions: []workflow.Transition{
					{To: "UNDER_REVIEW", Action: "submit_review", AllowedRoles: []string{"creator"}},
				},
			},
			"UNDER_REVIEW": {
				ID: "UNDER_REVIEW",
				Transitions: []workflow.Transition{
					{To: "CLARIFICATION_NEEDED", Action: "request_clarification", AllowedRoles: []string{"reviewer"}},
					{To: "DRAFTING", Action: "start_drafting", AllowedRoles: []string{"reviewer"}},
					{To: "REJECTED", Action: "reject", AllowedRoles: []string{"reviewer"}},
				},
			},
			"CLARIFICATION_NEEDED": {
				ID: "CLARIFICATION_NEEDED",
				Transitions: []workflow.Transition{
					{To: "UNDER_REVIEW", Action: "resubmit_for_review", AllowedRoles: []string{"creator"}},
					{To: "DRAFT", Action: "cancel_clarification", AllowedRoles: []string{"creator"}},
				},
			},
		},
	}

	fsm, err := workflow.NewFSM(config)
	if err != nil {
		t.Fatalf("Falha ao criar FSM: %v", err)
	}

	tests := []struct {
		name               string
		state              workflow.State
		userRoles          []string
		expectedCount      int
		expectedActions    []workflow.Action
	}{
		{
			name:            "Autor vê apenas submit_review no DRAFT",
			state:           "DRAFT",
			userRoles:       []string{"creator"},
			expectedCount:   1,
			expectedActions: []workflow.Action{"submit_review"},
		},
		{
			name:            "Editor vê todas as transições de UNDER_REVIEW",
			state:           "UNDER_REVIEW",
			userRoles:       []string{"reviewer"},
			expectedCount:   3,
			expectedActions: []workflow.Action{"request_clarification", "start_drafting", "reject"},
		},
		{
			name:            "Leitor não vê transições em CLARIFICATION_NEEDED",
			state:           "CLARIFICATION_NEEDED",
			userRoles:       []string{"consumer"},
			expectedCount:   0,
			expectedActions: []workflow.Action{},
		},
		{
			name:            "Autor vê transições de CLARIFICATION_NEEDED",
			state:           "CLARIFICATION_NEEDED",
			userRoles:       []string{"creator"},
			expectedCount:   2,
			expectedActions: []workflow.Action{"resubmit_for_review", "cancel_clarification"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transitions := fsm.ListTransitions(tt.state, tt.userRoles)

			if len(transitions) != tt.expectedCount {
				t.Errorf("Esperava %d transições, mas obteve %d", tt.expectedCount, len(transitions))
			}

			// Verifica se todas as ações esperadas estão presentes
			actionMap := make(map[workflow.Action]bool)
			for _, tr := range transitions {
				actionMap[tr.Action] = true
			}

			for _, expectedAction := range tt.expectedActions {
				if !actionMap[expectedAction] {
					t.Errorf("Ação esperada %s não encontrada nas transições", expectedAction)
				}
			}
		})
	}
}

func TestFSM_InitialState(t *testing.T) {
	config := workflow.FSMConfig{
		EntityName:   "experience",
		InitialState: "DRAFT",
		States: map[workflow.State]workflow.StateConfig{
			"DRAFT": {
				ID: "DRAFT",
				Transitions: []workflow.Transition{
					{To: "UNDER_REVIEW", Action: "submit_review", AllowedRoles: []string{"creator"}},
				},
			},
		},
	}

	fsm, err := workflow.NewFSM(config)
	if err != nil {
		t.Fatalf("Falha ao criar FSM: %v", err)
	}

	initialState := fsm.GetInitialState()
	if initialState != "DRAFT" {
		t.Errorf("Esperava estado inicial DRAFT, mas obteve %s", initialState)
	}

	entityName := fsm.GetEntityName()
	if entityName != "experience" {
		t.Errorf("Esperava entidade experience, mas obteve %s", entityName)
	}
}

func TestFSM_InvalidConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        workflow.FSMConfig
		expectedError bool
	}{
		{
			name: "Estado inicial vazio",
			config: workflow.FSMConfig{
				EntityName:   "test",
				InitialState: "",
				States:       map[workflow.State]workflow.StateConfig{},
			},
			expectedError: true,
		},
		{
			name: "Sem estados configurados",
			config: workflow.FSMConfig{
				EntityName:   "test",
				InitialState: "DRAFT",
				States:       map[workflow.State]workflow.StateConfig{},
			},
			expectedError: true,
		},
		{
			name: "Estado inicial não existe",
			config: workflow.FSMConfig{
				EntityName:   "test",
				InitialState: "NONEXISTENT",
				States: map[workflow.State]workflow.StateConfig{
					"DRAFT": {
						ID: "DRAFT",
						Transitions: []workflow.Transition{},
					},
				},
			},
			expectedError: true,
		},
		{
			name: "Configuração válida",
			config: workflow.FSMConfig{
				EntityName:   "test",
				InitialState: "DRAFT",
				States: map[workflow.State]workflow.StateConfig{
					"DRAFT": {
						ID: "DRAFT",
						Transitions: []workflow.Transition{},
					},
				},
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := workflow.NewFSM(tt.config)
			
			if tt.expectedError && err == nil {
				t.Errorf("Esperava erro na configuração, mas não ocorreu")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Erro inesperado na configuração: %v", err)
			}
		})
	}
}

func TestFSM_GetStateConfig(t *testing.T) {
	config := workflow.FSMConfig{
		EntityName:   "experience",
		InitialState: "DRAFT",
		States: map[workflow.State]workflow.StateConfig{
			"DRAFT": {
				ID: "DRAFT",
				Transitions: []workflow.Transition{
					{To: "UNDER_REVIEW", Action: "submit_review", AllowedRoles: []string{"creator"}},
				},
			},
		},
	}

	fsm, err := workflow.NewFSM(config)
	if err != nil {
		t.Fatalf("Falha ao criar FSM: %v", err)
	}

	// Teste estado existente
	stateConfig, err := fsm.GetStateConfig("DRAFT")
	if err != nil {
		t.Errorf("Erro ao obter configuração do estado DRAFT: %v", err)
	}
	if stateConfig == nil {
		t.Error("Configuração do estado DRAFT é nil")
	}
	if len(stateConfig.Transitions) != 1 {
		t.Errorf("Esperava 1 transição, mas obteve %d", len(stateConfig.Transitions))
	}

	// Teste estado inexistente
	_, err = fsm.GetStateConfig("NONEXISTENT")
	if err == nil {
		t.Error("Esperava erro para estado inexistente, mas não ocorreu")
	}
}
