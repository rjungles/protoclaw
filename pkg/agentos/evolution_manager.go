package agentos

import (
	"context"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

// EvolutionManager gerencia a evolução do sistema
type EvolutionManager struct {
	instance      *SystemInstance
	planner       *MigrationPlanner
	executor      *EvolutionExecutor
	backupManager *BackupManager
}

// NewEvolutionManager cria um novo gerenciador de evolução
func NewEvolutionManager(instance *SystemInstance) *EvolutionManager {
	return &EvolutionManager{
		instance:      instance,
		planner:       NewMigrationPlanner(instance),
		executor:      NewEvolutionExecutor(instance),
		backupManager: NewBackupManager(instance),
	}
}

// Evolve executa a evolução completa do sistema
func (em *EvolutionManager) Evolve(ctx context.Context, newManifest *manifest.Manifest) (*EvolutionResult, error) {
	// 1. Detectar mudanças
	currentManifest := em.instance.Manifest
	diff := em.detectChanges(currentManifest, newManifest)

	if !diff.HasChanges() {
		return &EvolutionResult{
			Success:  true,
			Warnings: []string{"No changes detected"},
		}, nil
	}

	// 2. Criar plano de migração
	plan, err := em.planner.CreatePlan(diff)
	if err != nil {
		return nil, fmt.Errorf("failed to create migration plan: %w", err)
	}

	// 3. Fazer backup se necessário
	var backupPath string
	if plan.BackupRequired {
		backupPath, err = em.backupManager.CreateBackup(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// 4. Executar migração
	result, err := em.executor.ExecutePlan(ctx, plan)
	if err != nil {
		// Tentar rollback se possível
		if backupPath != "" {
			if rollbackErr := em.backupManager.RestoreBackup(ctx, backupPath); rollbackErr != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Rollback failed: %v", rollbackErr))
			}
		}
		return result, err
	}

	// 5. Atualizar manifesto na instância
	if result.Success {
		em.instance.Manifest = newManifest
		if em.instance.ManifestStore != nil {
			yamlData, _ := newManifest.ToYAML()
			version := &ManifestVersion{
				Version:      newManifest.Metadata.Version,
				ManifestYAML: string(yamlData),
				CreatedAt:    time.Now(),
				CreatedBy:    "evolution",
				Description:  diff.Summary(),
			}
			if err := em.instance.ManifestStore.SaveVersion(version); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to save version: %v", err))
			}
		}
	}

	return result, nil
}

// detectChanges detecta mudanças entre manifestos
func (em *EvolutionManager) detectChanges(oldManifest, newManifest *manifest.Manifest) *ManifestDiff {
	diff := &ManifestDiff{
		FromVersion: oldManifest.Metadata.Version,
		ToVersion:   newManifest.Metadata.Version,
		Changes:     []Change{},
	}

	// Detectar mudanças em atores
	em.detectActorChanges(oldManifest, newManifest, diff)

	// Detectar mudanças no modelo de dados
	em.detectDataModelChanges(oldManifest, newManifest, diff)

	// Detectar mudanças em workflows
	em.detectWorkflowChanges(oldManifest, newManifest, diff)

	// Detectar mudanças em regras de negócio
	em.detectBusinessRuleChanges(oldManifest, newManifest, diff)

	// Detectar mudanças em segurança
	em.detectSecurityChanges(oldManifest, newManifest, diff)

	// Classificar mudanças
	diff.classifyChanges()

	return diff
}

// detectActorChanges detecta mudanças em atores
func (em *EvolutionManager) detectActorChanges(oldManifest, newManifest *manifest.Manifest, diff *ManifestDiff) {
	oldActors := make(map[string]*manifest.Actor)
	for i := range oldManifest.Actors {
		oldActors[oldManifest.Actors[i].ID] = &oldManifest.Actors[i]
	}

	newActors := make(map[string]*manifest.Actor)
	for i := range newManifest.Actors {
		newActors[newManifest.Actors[i].ID] = &newManifest.Actors[i]
	}

	// Detectar atores adicionados
	for id, actor := range newActors {
		if _, exists := oldActors[id]; !exists {
			diff.Changes = append(diff.Changes, Change{
				Type:              ChangeTypeAdd,
				Severity:          ChangeSeveritySafe,
				Path:              fmt.Sprintf("actors.%s", id),
				NewValue:          actor,
				Description:       fmt.Sprintf("Actor '%s' added", id),
				MigrationStrategy: "ADD_ACTOR",
			})
		}
	}

	// Detectar atores removidos
	for id, actor := range oldActors {
		if _, exists := newActors[id]; !exists {
			diff.Changes = append(diff.Changes, Change{
				Type:              ChangeTypeRemove,
				Severity:          ChangeSeverityBreaking,
				Path:              fmt.Sprintf("actors.%s", id),
				OldValue:          actor,
				Description:       fmt.Sprintf("Actor '%s' removed", id),
				MigrationStrategy: "DEACTIVATE_ACTOR",
			})
		}
	}

	// Detectar mudanças em atores existentes
	for id, oldActor := range oldActors {
		if newActor, exists := newActors[id]; exists {
			if oldActor.Name != newActor.Name {
				diff.Changes = append(diff.Changes, Change{
					Type:        ChangeTypeModify,
					Severity:    ChangeSeverityReview,
					Path:        fmt.Sprintf("actors.%s.name", id),
					OldValue:    oldActor.Name,
					NewValue:    newActor.Name,
					Description: fmt.Sprintf("Actor '%s' name changed", id),
				})
			}

			// Detectar mudanças em permissões
			if !em.comparePermissions(oldActor.Permissions, newActor.Permissions) {
				diff.Changes = append(diff.Changes, Change{
					Type:        ChangeTypeModify,
					Severity:    ChangeSeverityReview,
					Path:        fmt.Sprintf("actors.%s.permissions", id),
					OldValue:    oldActor.Permissions,
					NewValue:    newActor.Permissions,
					Description: fmt.Sprintf("Actor '%s' permissions changed", id),
				})
			}
		}
	}
}

// detectDataModelChanges detecta mudanças no modelo de dados
func (em *EvolutionManager) detectDataModelChanges(oldManifest, newManifest *manifest.Manifest, diff *ManifestDiff) {
	oldEntities := make(map[string]*manifest.Entity)
	for i := range oldManifest.DataModel.Entities {
		oldEntities[oldManifest.DataModel.Entities[i].Name] = &oldManifest.DataModel.Entities[i]
	}

	newEntities := make(map[string]*manifest.Entity)
	for i := range newManifest.DataModel.Entities {
		newEntities[newManifest.DataModel.Entities[i].Name] = &newManifest.DataModel.Entities[i]
	}

	// Detectar entidades adicionadas
	for name, entity := range newEntities {
		if _, exists := oldEntities[name]; !exists {
			diff.Changes = append(diff.Changes, Change{
				Type:              ChangeTypeAdd,
				Severity:          ChangeSeveritySafe,
				Path:              fmt.Sprintf("data_model.entities.%s", name),
				NewValue:          entity,
				Description:       fmt.Sprintf("Entity '%s' added", name),
				MigrationStrategy: "CREATE_TABLE",
			})
		}
	}

	// Detectar entidades removidas
	for name, entity := range oldEntities {
		if _, exists := newEntities[name]; !exists {
			diff.Changes = append(diff.Changes, Change{
				Type:              ChangeTypeRemove,
				Severity:          ChangeSeverityBreaking,
				Path:              fmt.Sprintf("data_model.entities.%s", name),
				OldValue:          entity,
				Description:       fmt.Sprintf("Entity '%s' removed", name),
				MigrationStrategy: "ARCHIVE_TABLE",
			})
		}
	}

	// Detectar mudanças em campos de entidades
	for name, oldEntity := range oldEntities {
		if newEntity, exists := newEntities[name]; exists {
			em.detectFieldChanges(name, oldEntity, newEntity, diff)
		}
	}
}

// detectFieldChanges detecta mudanças em campos de entidades
func (em *EvolutionManager) detectFieldChanges(entityName string, oldEntity, newEntity *manifest.Entity, diff *ManifestDiff) {
	oldFields := make(map[string]*manifest.Field)
	for i := range oldEntity.Fields {
		oldFields[oldEntity.Fields[i].Name] = &oldEntity.Fields[i]
	}

	newFields := make(map[string]*manifest.Field)
	for i := range newEntity.Fields {
		newFields[newEntity.Fields[i].Name] = &newEntity.Fields[i]
	}

	// Detectar campos adicionados
	for name, field := range newFields {
		if _, exists := oldFields[name]; !exists {
			diff.Changes = append(diff.Changes, Change{
				Type:              ChangeTypeAdd,
				Severity:          ChangeSeveritySafe,
				Path:              fmt.Sprintf("data_model.entities.%s.fields.%s", entityName, name),
				NewValue:          field,
				Description:       fmt.Sprintf("Field '%s' added to entity '%s'", name, entityName),
				MigrationStrategy: "ADD_COLUMN",
			})
		}
	}

	// Detectar campos removidos
	for name, field := range oldFields {
		if _, exists := newFields[name]; !exists {
			diff.Changes = append(diff.Changes, Change{
				Type:              ChangeTypeRemove,
				Severity:          ChangeSeverityBreaking,
				Path:              fmt.Sprintf("data_model.entities.%s.fields.%s", entityName, name),
				OldValue:          field,
				Description:       fmt.Sprintf("Field '%s' removed from entity '%s'", name, entityName),
				MigrationStrategy: "DEPRECATE_FIELD",
			})
		}
	}

	// Detectar mudanças em campos existentes
	for name, oldField := range oldFields {
		if newField, exists := newFields[name]; exists {
			if oldField.Type != newField.Type {
				diff.Changes = append(diff.Changes, Change{
					Type:              ChangeTypeModify,
					Severity:          ChangeSeverityReview,
					Path:              fmt.Sprintf("data_model.entities.%s.fields.%s.type", entityName, name),
					OldValue:          oldField.Type,
					NewValue:          newField.Type,
					Description:       fmt.Sprintf("Field '%s' type changed in entity '%s'", name, entityName),
					MigrationStrategy: "ALTER_COLUMN",
				})
			}

			if oldField.Required != newField.Required {
				severity := ChangeSeverityReview
				if newField.Required {
					severity = ChangeSeverityBreaking // Tornar obrigatório pode quebrar dados existentes
				}
				diff.Changes = append(diff.Changes, Change{
					Type:        ChangeTypeModify,
					Severity:    severity,
					Path:        fmt.Sprintf("data_model.entities.%s.fields.%s.required", entityName, name),
					OldValue:    oldField.Required,
					NewValue:    newField.Required,
					Description: fmt.Sprintf("Field '%s' requirement changed in entity '%s'", name, entityName),
				})
			}
		}
	}
}

// detectWorkflowChanges detecta mudanças em workflows
func (em *EvolutionManager) detectWorkflowChanges(oldManifest, newManifest *manifest.Manifest, diff *ManifestDiff) {
	oldWorkflows := make(map[string]*manifest.Workflow)
	for i := range oldManifest.Workflows {
		oldWorkflows[oldManifest.Workflows[i].Entity] = &oldManifest.Workflows[i]
	}

	newWorkflows := make(map[string]*manifest.Workflow)
	for i := range newManifest.Workflows {
		newWorkflows[newManifest.Workflows[i].Entity] = &newManifest.Workflows[i]
	}

	// Detectar workflows adicionados
	for entity, workflow := range newWorkflows {
		if _, exists := oldWorkflows[entity]; !exists {
			diff.Changes = append(diff.Changes, Change{
				Type:        ChangeTypeAdd,
				Severity:    ChangeSeveritySafe,
				Path:        fmt.Sprintf("workflows.%s", entity),
				NewValue:    workflow,
				Description: fmt.Sprintf("Workflow for entity '%s' added", entity),
			})
		}
	}

	// Detectar workflows removidos
	for entity, workflow := range oldWorkflows {
		if _, exists := newWorkflows[entity]; !exists {
			diff.Changes = append(diff.Changes, Change{
				Type:              ChangeTypeRemove,
				Severity:          ChangeSeverityBreaking,
				Path:              fmt.Sprintf("workflows.%s", entity),
				OldValue:          workflow,
				Description:       fmt.Sprintf("Workflow for entity '%s' removed", entity),
				MigrationStrategy: "MARK_INACTIVE",
			})
		}
	}
}

// detectBusinessRuleChanges detecta mudanças em regras de negócio
func (em *EvolutionManager) detectBusinessRuleChanges(oldManifest, newManifest *manifest.Manifest, diff *ManifestDiff) {
	// Implementar detecção de mudanças em regras de negócio
	// Por enquanto, apenas comparar número de regras
	if len(oldManifest.BusinessRules) != len(newManifest.BusinessRules) {
		diff.Changes = append(diff.Changes, Change{
			Type:        ChangeTypeModify,
			Severity:    ChangeSeverityReview,
			Path:        "business_rules",
			OldValue:    len(oldManifest.BusinessRules),
			NewValue:    len(newManifest.BusinessRules),
			Description: fmt.Sprintf("Number of business rules changed from %d to %d", len(oldManifest.BusinessRules), len(newManifest.BusinessRules)),
		})
	}
}

// detectSecurityChanges detecta mudanças em segurança
func (em *EvolutionManager) detectSecurityChanges(oldManifest, newManifest *manifest.Manifest, diff *ManifestDiff) {
	// Detectar mudanças em políticas de segurança
	if oldManifest.Security != newManifest.Security {
		diff.Changes = append(diff.Changes, Change{
			Type:        ChangeTypeModify,
			Severity:    ChangeSeverityReview,
			Path:        "security",
			OldValue:    oldManifest.Security,
			NewValue:    newManifest.Security,
			Description: "Security configuration changed",
		})
	}
}

// comparePermissions compara permissões
func (em *EvolutionManager) comparePermissions(oldPerms, newPerms []manifest.Permission) bool {
	if len(oldPerms) != len(newPerms) {
		return false
	}

	oldMap := make(map[string]manifest.Permission)
	for _, perm := range oldPerms {
		key := fmt.Sprintf("%s:%v", perm.Resource, perm.Actions)
		oldMap[key] = perm
	}

	for _, newPerm := range newPerms {
		key := fmt.Sprintf("%s:%v", newPerm.Resource, newPerm.Actions)
		if oldPerm, exists := oldMap[key]; !exists || oldPerm != newPerm {
			return false
		}
	}

	return true
}

// GetCurrentVersion retorna a versão atual
func (em *EvolutionManager) GetCurrentVersion() string {
	if em.instance != nil && em.instance.Manifest != nil {
		return em.instance.Manifest.Metadata.Version
	}
	return "unknown"
}

// GetVersionHistory retorna o histórico de versões
func (em *EvolutionManager) GetVersionHistory() ([]*ManifestVersion, error) {
	if em.instance.ManifestStore == nil {
		return nil, fmt.Errorf("manifest store not configured")
	}
	return em.instance.ManifestStore.ListVersions()
}

// CanEvolve verifica se pode evoluir para um novo manifesto
func (em *EvolutionManager) CanEvolve(newManifest *manifest.Manifest) (bool, []string, error) {
	diff := em.detectChanges(em.instance.Manifest, newManifest)

	warnings := make([]string, 0)
	for _, change := range diff.Changes {
		if change.Severity == ChangeSeverityBreaking {
			warnings = append(warnings, fmt.Sprintf("Breaking change: %s", change.Description))
		}
	}

	return len(warnings) == 0, warnings, nil
}
