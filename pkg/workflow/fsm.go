package workflow

import (
	"fmt"
	"slices"
)

// State representa um estado no workflow
type State string

// Action representa uma ação de transição
type Action string

// Transition define uma transição possível entre estados
type Transition struct {
	To           State    `json:"to"`
	Action       Action   `json:"action"`
	AllowedRoles []string `json:"allowed_roles"`
}

// StateConfig configura um estado específico
type StateConfig struct {
	ID          State        `json:"id"`
	Transitions []Transition `json:"transitions"`
}

// FSMConfig configuração completa da máquina de estados
type FSMConfig struct {
	EntityName   string            `json:"entity"`
	InitialState State             `json:"initial_state"`
	States       map[State]StateConfig `json:"states"`
}

// FSM é a máquina de estados finita para gerenciar workflows
type FSM struct {
	config FSMConfig
}

// NewFSM cria uma nova máquina de estados
func NewFSM(config FSMConfig) (*FSM, error) {
	if config.InitialState == "" {
		return nil, fmt.Errorf("estado inicial não definido")
	}
	if config.States == nil || len(config.States) == 0 {
		return nil, fmt.Errorf("nenhum estado configurado")
	}
	
	// Valida se o estado inicial existe
	if _, exists := config.States[config.InitialState]; !exists {
		return nil, fmt.Errorf("estado inicial '%s' não existe na configuração", config.InitialState)
	}
	
	return &FSM{config: config}, nil
}

// CanTransition verifica se uma transição é válida
func (f *FSM) CanTransition(current State, action Action, userRoles []string) (bool, State, error) {
	config, exists := f.config.States[current]
	if !exists {
		return false, "", fmt.Errorf("estado atual desconhecido: %s", current)
	}

	for _, t := range config.Transitions {
		if t.Action == action {
			// Verifica se o usuário tem algum dos papéis permitidos
			hasRole := false
			for _, role := range userRoles {
				if slices.Contains(t.AllowedRoles, role) {
					hasRole = true
					break
				}
			}

			if hasRole {
				return true, t.To, nil
			}
			return false, "", fmt.Errorf("papéis do usuário %v não autorizados para ação %s", userRoles, action)
		}
	}
	
	return false, "", fmt.Errorf("ação %s não é válida no estado %s", action, current)
}

// GetInitialState retorna o estado inicial configurado
func (f *FSM) GetInitialState() State {
	return f.config.InitialState
}

// GetEntityName retorna o nome da entidade gerenciada
func (f *FSM) GetEntityName() string {
	return f.config.EntityName
}

// GetStateConfig retorna a configuração de um estado específico
func (f *FSM) GetStateConfig(state State) (*StateConfig, error) {
	config, exists := f.config.States[state]
	if !exists {
		return nil, fmt.Errorf("estado %s não encontrado", state)
	}
	return &config, nil
}

// ListTransitions lista todas as transições possíveis de um estado
func (f *FSM) ListTransitions(state State, userRoles []string) []Transition {
	config, exists := f.config.States[state]
	if !exists {
		return []Transition{}
	}
	
	validTransitions := []Transition{}
	for _, t := range config.Transitions {
		// Filtra apenas transições que o usuário pode executar
		for _, role := range userRoles {
			if slices.Contains(t.AllowedRoles, role) {
				validTransitions = append(validTransitions, t)
				break
			}
		}
	}
	
	return validTransitions
}
