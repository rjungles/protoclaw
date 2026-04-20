package evolution

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type MigrationAction string

const (
	ActionAddColumn      MigrationAction = "ADD_COLUMN"
	ActionAddEntity      MigrationAction = "ADD_ENTITY"
	ActionAddActor       MigrationAction = "ADD_ACTOR"
	ActionModifyField    MigrationAction = "MODIFY_FIELD"
	ActionRemoveField    MigrationAction = "REMOVE_FIELD"
	ActionRemoveEntity   MigrationAction = "REMOVE_ENTITY"
	ActionDeprecateField MigrationAction = "DEPRECATE_FIELD"
	ActionArchiveEntity  MigrationAction = "ARCHIVE_ENTITY"
	ActionMigrateData    MigrationAction = "MIGRATE_DATA"
	ActionAddWorkflow    MigrationAction = "ADD_WORKFLOW"
)

type MigrationStep struct {
	Action      MigrationAction
	Entity      string
	Field       string
	SQL         string
	RollbackSQL string
	Severity    ChangeSeverity
	Description string
}

type MigrationPlan struct {
	Steps    []MigrationStep
	Safe     []MigrationStep
	Review   []MigrationStep
	Breaking []MigrationStep
}

func CreateMigrationPlan(diff *ManifestDiff, db *sql.DB, manifest *manifest.Manifest) *MigrationPlan {
	plan := &MigrationPlan{
		Steps:    make([]MigrationStep, 0),
		Safe:     make([]MigrationStep, 0),
		Review:   make([]MigrationStep, 0),
		Breaking: make([]MigrationStep, 0),
	}

	for _, change := range diff.Changes {
		step := plan.createStepForChange(change, db, manifest)
		if step != nil {
			plan.Steps = append(plan.Steps, *step)
			switch step.Severity {
			case SeveritySafe:
				plan.Safe = append(plan.Safe, *step)
			case SeverityReview:
				plan.Review = append(plan.Review, *step)
			case SeverityBreaking:
				plan.Breaking = append(plan.Breaking, *step)
			}
		}
	}

	return plan
}

func (p *MigrationPlan) createStepForChange(change Change, db *sql.DB, m *manifest.Manifest) *MigrationStep {
	switch change.Type {
	case ChangeTypeAdd:
		return p.createAddStep(change, m)
	case ChangeTypeModify:
		return p.createModifyStep(change, m)
	case ChangeTypeRemove:
		return p.createRemoveStep(change, m)
	}
	return nil
}

func (p *MigrationPlan) createAddStep(change Change, m *manifest.Manifest) *MigrationStep {
	if strings.Contains(change.Path, "data_model.entities.") {
		if strings.Contains(change.Path, "fields.") {
			parts := strings.Split(change.Path, ".")
			if len(parts) >= 2 {
				entityName := parts[2]
				fieldName := parts[4]

				field := p.findField(m, entityName, fieldName)
				if field == nil {
					return nil
				}

				sqlType := mapManifestTypeToSQL(field.Type)

				return &MigrationStep{
					Action:      ActionAddColumn,
					Entity:      entityName,
					Field:       fieldName,
					SQL:         fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", toSnakeCase(entityName), toSnakeCase(fieldName), sqlType),
					RollbackSQL: fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", toSnakeCase(entityName), toSnakeCase(fieldName)),
					Severity:    change.Severity,
					Description: change.Description,
				}
			}
		} else if strings.Count(change.Path, ".") == 2 {
			parts := strings.Split(change.Path, ".")
			if len(parts) >= 1 {
				entityName := parts[2]

				entity := p.findEntity(m, entityName)
				if entity != nil {
					sql := p.generateCreateTableSQL(entity)
					return &MigrationStep{
						Action:      ActionAddEntity,
						Entity:      entityName,
						SQL:         sql,
						RollbackSQL: fmt.Sprintf("DROP TABLE IF EXISTS %s", toSnakeCase(entityName)),
						Severity:    change.Severity,
						Description: change.Description,
					}
				}

				return &MigrationStep{
					Action:      ActionAddEntity,
					Entity:      entityName,
					SQL:         fmt.Sprintf("-- Create table for entity %s", entityName),
					Severity:    change.Severity,
					Description: change.Description,
				}
			}
		}
	}

	if strings.Contains(change.Path, "actors.") {
		parts := strings.Split(change.Path, ".")
		if len(parts) >= 1 {
			actorID := parts[1]

			actor := p.findActor(m, actorID)
			if actor != nil {
				rolesJSON, _ := json.Marshal(actor.Roles)
				return &MigrationStep{
					Action:      ActionAddActor,
					Entity:      actorID,
					SQL:         fmt.Sprintf("INSERT INTO _actors (actor_id, actor_type, api_key_hash, roles, is_active) VALUES ('%s', '%s', '', '%s', TRUE)", actor.ID, actor.Name, string(rolesJSON)),
					RollbackSQL: fmt.Sprintf("DELETE FROM _actors WHERE actor_id = '%s'", actorID),
					Severity:    change.Severity,
					Description: change.Description,
				}
			}

			return &MigrationStep{
				Action:      ActionAddActor,
				Entity:      actorID,
				SQL:         fmt.Sprintf("-- Provision actor %s", actorID),
				Severity:    change.Severity,
				Description: change.Description,
			}
		}
	}

	if strings.Contains(change.Path, "workflows.") {
		parts := strings.Split(change.Path, ".")
		if len(parts) >= 1 {
			entityName := parts[1]

			return &MigrationStep{
				Action:      ActionAddWorkflow,
				Entity:      entityName,
				SQL:         fmt.Sprintf("-- Add workflow for entity %s", entityName),
				Severity:    change.Severity,
				Description: change.Description,
			}
		}
	}

	return nil
}

func (p *MigrationPlan) createModifyStep(change Change, m *manifest.Manifest) *MigrationStep {
	if strings.Contains(change.Path, "fields.") && strings.Contains(change.Path, ".type") {
		parts := strings.Split(change.Path, ".")
		if len(parts) >= 2 {
			entityName := parts[2]
			fieldName := parts[4]

			return &MigrationStep{
				Action:      ActionModifyField,
				Entity:      entityName,
				Field:       fieldName,
				SQL:         fmt.Sprintf("-- Modify field %s.%s type", entityName, fieldName),
				Severity:    change.Severity,
				Description: change.Description,
			}
		}
	}

	return nil
}

func (p *MigrationPlan) createRemoveStep(change Change, m *manifest.Manifest) *MigrationStep {
	if strings.Contains(change.Path, "data_model.entities.") {
		if strings.Contains(change.Path, "fields.") {
			parts := strings.Split(change.Path, ".")
			if len(parts) >= 2 {
				entityName := parts[2]
				fieldName := parts[4]

				return &MigrationStep{
					Action:      ActionDeprecateField,
					Entity:      entityName,
					Field:       fieldName,
					SQL:         fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO _deprecated_%s", toSnakeCase(entityName), toSnakeCase(fieldName), toSnakeCase(fieldName)),
					RollbackSQL: fmt.Sprintf("ALTER TABLE %s RENAME COLUMN _deprecated_%s TO %s", toSnakeCase(entityName), toSnakeCase(fieldName), toSnakeCase(fieldName)),
					Severity:    change.Severity,
					Description: change.Description,
				}
			}
		} else if strings.Count(change.Path, ".") == 2 {
			parts := strings.Split(change.Path, ".")
			if len(parts) >= 1 {
				entityName := parts[2]

				return &MigrationStep{
					Action:      ActionArchiveEntity,
					Entity:      entityName,
					SQL:         fmt.Sprintf("ALTER TABLE %s RENAME TO _archived_%s", toSnakeCase(entityName), toSnakeCase(entityName)),
					RollbackSQL: fmt.Sprintf("ALTER TABLE _archived_%s RENAME TO %s", toSnakeCase(entityName), toSnakeCase(entityName)),
					Severity:    change.Severity,
					Description: change.Description,
				}
			}
		}
	}

	if strings.Contains(change.Path, "actors.") {
		parts := strings.Split(change.Path, ".")
		if len(parts) >= 1 {
			actorID := parts[1]

			return &MigrationStep{
				Action:      ActionRemoveField,
				Entity:      actorID,
				SQL:         fmt.Sprintf("UPDATE _actors SET is_active = FALSE WHERE actor_id = '%s'", actorID),
				RollbackSQL: fmt.Sprintf("UPDATE _actors SET is_active = TRUE WHERE actor_id = '%s'", actorID),
				Severity:    change.Severity,
				Description: change.Description,
			}
		}
	}

	return nil
}

func (p *MigrationPlan) findField(m *manifest.Manifest, entityName, fieldName string) *manifest.Field {
	for _, entity := range m.DataModel.Entities {
		if entity.Name == entityName {
			for _, field := range entity.Fields {
				if field.Name == fieldName {
					return &field
				}
			}
		}
	}
	return nil
}

func (p *MigrationPlan) findEntity(m *manifest.Manifest, entityName string) *manifest.Entity {
	for i := range m.DataModel.Entities {
		if m.DataModel.Entities[i].Name == entityName {
			return &m.DataModel.Entities[i]
		}
	}
	return nil
}

func (p *MigrationPlan) findActor(m *manifest.Manifest, actorID string) *manifest.Actor {
	for i := range m.Actors {
		if m.Actors[i].ID == actorID {
			return &m.Actors[i]
		}
	}
	return nil
}

func (p *MigrationPlan) generateCreateTableSQL(entity *manifest.Entity) string {
	columns := make([]string, 0, len(entity.Fields))
	for _, field := range entity.Fields {
		colName := toSnakeCase(field.Name)
		colType := mapManifestTypeToSQL(field.Type)
		colDef := colName + " " + colType
		if field.Required {
			colDef += " NOT NULL"
		}
		if field.Name == "id" {
			colDef += " PRIMARY KEY"
		}
		columns = append(columns, colDef)
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", toSnakeCase(entity.Name), strings.Join(columns, ", "))
}

func (p *MigrationPlan) CanApplyAutomatically() bool {
	return len(p.Review) == 0 && len(p.Breaking) == 0
}

func (p *MigrationPlan) RequiresConfirmation() bool {
	return len(p.Review) > 0 || len(p.Breaking) > 0
}

func (p *MigrationPlan) GetExecutableSteps() []MigrationStep {
	return p.Safe
}

func (p *MigrationPlan) GetAllSteps() []MigrationStep {
	return p.Steps
}

func (p *MigrationPlan) Summary() string {
	return fmt.Sprintf("Plan: %d safe, %d review, %d breaking", len(p.Safe), len(p.Review), len(p.Breaking))
}

func mapManifestTypeToSQL(mtype string) string {
	switch strings.ToLower(mtype) {
	case "string", "text":
		return "TEXT"
	case "integer", "int":
		return "INTEGER"
	case "float", "number", "decimal":
		return "REAL"
	case "boolean", "bool":
		return "BOOLEAN"
	case "datetime", "timestamp", "date":
		return "DATETIME"
	case "array", "array<string>", "array<int>":
		return "TEXT"
	default:
		return "TEXT"
	}
}

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, []rune(strings.ToLower(string(r)))...)
	}
	return string(result)
}
