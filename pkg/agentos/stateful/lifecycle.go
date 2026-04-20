package stateful

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type LifecycleHook struct {
	Event   string // "OnCreate", "OnUpdate", "OnDelete", "OnStateChange"
	Entity  string
	Actions []manifest.WorkflowAction
}

type LifecycleManager struct {
	manifest *manifest.Manifest
	hooks    map[string][]LifecycleHook
	executor *SideEffectExecutor
}

func NewLifecycleManager(manifest *manifest.Manifest, notifyBus NotificationBus) *LifecycleManager {
	lm := &LifecycleManager{
		manifest: manifest,
		hooks:    make(map[string][]LifecycleHook),
		executor: NewSideEffectExecutor(manifest, nil, notifyBus),
	}

	// Initialize hooks from manifest
	lm.initializeHooks()

	return lm
}

func (m *LifecycleManager) initializeHooks() {
	// For now, lifecycle hooks are not defined in the manifest
	// This is a placeholder for future implementation
	// The hooks can be loaded from configuration or database
}

func (m *LifecycleManager) hookKey(event, entity string) string {
	return fmt.Sprintf("%s:%s", event, entity)
}

func (m *LifecycleManager) TriggerOnCreate(ctx context.Context, entityID, actorID string) error {
	if m.manifest == nil {
		return nil
	}

	// Find entity type from workflows or data model
	entityType := m.findEntityType(entityID)
	if entityType == "" {
		return fmt.Errorf("could not determine entity type for ID: %s", entityID)
	}

	hooks := m.getHooks("OnCreate", entityType)
	for _, hook := range hooks {
		for _, action := range hook.Actions {
			if err := m.executor.executeAction(&action, entityID, actorID); err != nil {
				return fmt.Errorf("failed to execute OnCreate action: %w", err)
			}
		}
	}

	return nil
}

func (m *LifecycleManager) TriggerOnUpdate(ctx context.Context, entityID, actorID string, changes map[string]interface{}) error {
	if m.manifest == nil {
		return nil
	}

	entityType := m.findEntityType(entityID)
	if entityType == "" {
		return fmt.Errorf("could not determine entity type for ID: %s", entityID)
	}

	hooks := m.getHooks("OnUpdate", entityType)
	for _, hook := range hooks {
		// Check if this hook should be triggered based on changes
		if m.shouldTriggerUpdateHook(hook, changes) {
			for _, action := range hook.Actions {
				// Add changes to action config for template processing
				if action.Config == nil {
					action.Config = make(map[string]interface{})
				}
				action.Config["changes"] = changes

				if err := m.executor.executeAction(&action, entityID, actorID); err != nil {
					return fmt.Errorf("failed to execute OnUpdate action: %w", err)
				}
			}
		}
	}

	return nil
}

func (m *LifecycleManager) TriggerOnStateChange(ctx context.Context, entityID, fromState, toState, actorID string) error {
	if m.manifest == nil {
		return nil
	}

	entityType := m.findEntityType(entityID)
	if entityType == "" {
		return fmt.Errorf("could not determine entity type for ID: %s", entityID)
	}

	hooks := m.getHooks("OnStateChange", entityType)
	for _, hook := range hooks {
		for _, action := range hook.Actions {
			// Add state change info to action config
			if action.Config == nil {
				action.Config = make(map[string]interface{})
			}
			action.Config["from_state"] = fromState
			action.Config["to_state"] = toState

			if err := m.executor.executeAction(&action, entityID, actorID); err != nil {
				return fmt.Errorf("failed to execute OnStateChange action: %w", err)
			}
		}
	}

	return nil
}

func (m *LifecycleManager) TriggerOnDelete(ctx context.Context, entityID, actorID string) error {
	if m.manifest == nil {
		return nil
	}

	entityType := m.findEntityType(entityID)
	if entityType == "" {
		return fmt.Errorf("could not determine entity type for ID: %s", entityID)
	}

	hooks := m.getHooks("OnDelete", entityType)
	for _, hook := range hooks {
		for _, action := range hook.Actions {
			if err := m.executor.executeAction(&action, entityID, actorID); err != nil {
				return fmt.Errorf("failed to execute OnDelete action: %w", err)
			}
		}
	}

	return nil
}

func (m *LifecycleManager) getHooks(event, entity string) []LifecycleHook {
	key := m.hookKey(event, entity)
	return m.hooks[key]
}

func (m *LifecycleManager) findEntityType(entityID string) string {
	// Try to find entity type from workflows
	for _, wf := range m.manifest.Workflows {
		if wf.Entity != "" {
			return wf.Entity
		}
	}

	// Try to find from data model entities
	for _, entity := range m.manifest.DataModel.Entities {
		if entity.Name != "" {
			return entity.Name
		}
	}

	return ""
}

func (m *LifecycleManager) shouldTriggerUpdateHook(hook LifecycleHook, changes map[string]interface{}) bool {
	// For now, trigger all update hooks
	// In a more sophisticated implementation, we could check specific field conditions
	return len(changes) > 0
}

// LifecycleHookConfig represents configuration for lifecycle hooks in the manifest
type LifecycleHookConfig struct {
	Event   string                    `yaml:"event" json:"event"`
	Entity  string                    `yaml:"entity" json:"entity"`
	Actions []manifest.WorkflowAction `yaml:"actions" json:"actions"`
}

// LifecycleConfig represents the lifecycle configuration section in the manifest
type LifecycleConfig struct {
	Hooks []LifecycleHookConfig `yaml:"hooks" json:"hooks"`
}
