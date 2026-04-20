package evolution

import (
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type ChangeType string

const (
	ChangeTypeAdd    ChangeType = "add"
	ChangeTypeModify ChangeType = "modify"
	ChangeTypeRemove ChangeType = "remove"
)

type ChangeSeverity string

const (
	SeveritySafe     ChangeSeverity = "safe"
	SeverityReview   ChangeSeverity = "review"
	SeverityBreaking ChangeSeverity = "breaking"
)

type Change struct {
	Type        ChangeType
	Path        string
	Severity    ChangeSeverity
	OldValue    interface{}
	NewValue    interface{}
	Description string
}

type ManifestDiff struct {
	FromVersion string
	ToVersion   string
	Changes     []Change
}

func DiffManifests(old, new *manifest.Manifest) *ManifestDiff {
	diff := &ManifestDiff{
		FromVersion: old.Metadata.Version,
		ToVersion:   new.Metadata.Version,
		Changes:     make([]Change, 0),
	}

	diff.diffEntities(old, new)
	diff.diffActors(old, new)
	diff.diffWorkflows(old, new)
	diff.diffBusinessRules(old, new)
	diff.diffIntegrations(old, new)

	return diff
}

func (d *ManifestDiff) diffEntities(old, new *manifest.Manifest) {
	oldEntities := make(map[string]*manifest.Entity)
	for i := range old.DataModel.Entities {
		e := &old.DataModel.Entities[i]
		oldEntities[e.Name] = e
	}

	newEntities := make(map[string]*manifest.Entity)
	for i := range new.DataModel.Entities {
		e := &new.DataModel.Entities[i]
		newEntities[e.Name] = e
	}

	for name, newEntity := range newEntities {
		if oldEntity, exists := oldEntities[name]; exists {
			d.diffFields(name, oldEntity, newEntity)
		} else {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeAdd,
				Path:        fmt.Sprintf("data_model.entities.%s", name),
				Severity:    SeveritySafe,
				NewValue:    newEntity,
				Description: fmt.Sprintf("New entity '%s' added", name),
			})
		}
	}

	for name, oldEntity := range oldEntities {
		if _, exists := newEntities[name]; !exists {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeRemove,
				Path:        fmt.Sprintf("data_model.entities.%s", name),
				Severity:    SeverityBreaking,
				OldValue:    oldEntity,
				Description: fmt.Sprintf("Entity '%s' removed (data will be archived)", name),
			})
		}
	}
}

func (d *ManifestDiff) diffFields(entityName string, oldEntity, newEntity *manifest.Entity) {
	oldFields := make(map[string]*manifest.Field)
	for i := range oldEntity.Fields {
		f := &oldEntity.Fields[i]
		oldFields[f.Name] = f
	}

	newFields := make(map[string]*manifest.Field)
	for i := range newEntity.Fields {
		f := &newEntity.Fields[i]
		newFields[f.Name] = f
	}

	for name, newField := range newFields {
		if oldField, exists := oldFields[name]; exists {
			if oldField.Type != newField.Type {
				d.Changes = append(d.Changes, Change{
					Type:        ChangeTypeModify,
					Path:        fmt.Sprintf("data_model.entities.%s.fields.%s.type", entityName, name),
					Severity:    SeverityReview,
					OldValue:    oldField.Type,
					NewValue:    newField.Type,
					Description: fmt.Sprintf("Field '%s' type changed from '%s' to '%s'", name, oldField.Type, newField.Type),
				})
			}
			if oldField.Required != newField.Required {
				d.Changes = append(d.Changes, Change{
					Type:        ChangeTypeModify,
					Path:        fmt.Sprintf("data_model.entities.%s.fields.%s.required", entityName, name),
					Severity:    SeverityReview,
					OldValue:    oldField.Required,
					NewValue:    newField.Required,
					Description: fmt.Sprintf("Field '%s' required changed from %v to %v", name, oldField.Required, newField.Required),
				})
			}
		} else {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeAdd,
				Path:        fmt.Sprintf("data_model.entities.%s.fields.%s", entityName, name),
				Severity:    SeveritySafe,
				NewValue:    newField,
				Description: fmt.Sprintf("New field '%s' added to entity '%s'", name, entityName),
			})
		}
	}

	for name, oldField := range oldFields {
		if _, exists := newFields[name]; !exists {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeRemove,
				Path:        fmt.Sprintf("data_model.entities.%s.fields.%s", entityName, name),
				Severity:    SeverityBreaking,
				OldValue:    oldField,
				Description: fmt.Sprintf("Field '%s' removed from entity '%s' (data will be deprecated)", name, entityName),
			})
		}
	}
}

func (d *ManifestDiff) diffActors(old, new *manifest.Manifest) {
	oldActors := make(map[string]*manifest.Actor)
	for i := range old.Actors {
		a := &old.Actors[i]
		oldActors[a.ID] = a
	}

	newActors := make(map[string]*manifest.Actor)
	for i := range new.Actors {
		a := &new.Actors[i]
		newActors[a.ID] = a
	}

	for id, newActor := range newActors {
		if _, exists := oldActors[id]; !exists {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeAdd,
				Path:        fmt.Sprintf("actors.%s", id),
				Severity:    SeveritySafe,
				NewValue:    newActor,
				Description: fmt.Sprintf("New actor '%s' added", id),
			})
		}
	}

	for id, oldActor := range oldActors {
		if _, exists := newActors[id]; !exists {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeRemove,
				Path:        fmt.Sprintf("actors.%s", id),
				Severity:    SeverityBreaking,
				OldValue:    oldActor,
				Description: fmt.Sprintf("Actor '%s' removed (will be deactivated)", id),
			})
		}
	}
}

func (d *ManifestDiff) diffWorkflows(old, new *manifest.Manifest) {
	oldWorkflows := make(map[string]*manifest.WorkflowConfig)
	for i := range old.Workflows {
		w := &old.Workflows[i]
		oldWorkflows[w.Entity] = w
	}

	newWorkflows := make(map[string]*manifest.WorkflowConfig)
	for i := range new.Workflows {
		w := &new.Workflows[i]
		newWorkflows[w.Entity] = w
	}

	for entity, newWorkflow := range newWorkflows {
		if _, exists := oldWorkflows[entity]; !exists {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeAdd,
				Path:        fmt.Sprintf("workflows.%s", entity),
				Severity:    SeveritySafe,
				NewValue:    newWorkflow,
				Description: fmt.Sprintf("New workflow for entity '%s' added", entity),
			})
		}
	}

	for entity, oldWorkflow := range oldWorkflows {
		if _, exists := newWorkflows[entity]; !exists {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeRemove,
				Path:        fmt.Sprintf("workflows.%s", entity),
				Severity:    SeverityBreaking,
				OldValue:    oldWorkflow,
				Description: fmt.Sprintf("Workflow for entity '%s' removed", entity),
			})
		}
	}
}

func (d *ManifestDiff) diffBusinessRules(old, new *manifest.Manifest) {
	oldRules := make(map[string]bool)
	for _, r := range old.BusinessRules {
		oldRules[r.ID] = true
	}

	newRules := make(map[string]bool)
	for _, r := range new.BusinessRules {
		newRules[r.ID] = true
	}

	for id := range newRules {
		if !oldRules[id] {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeAdd,
				Path:        fmt.Sprintf("business_rules.%s", id),
				Severity:    SeveritySafe,
				Description: fmt.Sprintf("New business rule '%s' added", id),
			})
		}
	}

	for id := range oldRules {
		if !newRules[id] {
			d.Changes = append(d.Changes, Change{
				Type:        ChangeTypeRemove,
				Path:        fmt.Sprintf("business_rules.%s", id),
				Severity:    SeverityReview,
				Description: fmt.Sprintf("Business rule '%s' removed", id),
			})
		}
	}
}

func (d *ManifestDiff) diffIntegrations(old, new *manifest.Manifest) {
	if len(new.Integrations.APIs) > len(old.Integrations.APIs) {
		d.Changes = append(d.Changes, Change{
			Type:        ChangeTypeAdd,
			Path:        "integrations.apis",
			Severity:    SeveritySafe,
			Description: "New API integration added",
		})
	}

	if len(new.Integrations.MCPs) > len(old.Integrations.MCPs) {
		d.Changes = append(d.Changes, Change{
			Type:        ChangeTypeAdd,
			Path:        "integrations.mcps",
			Severity:    SeveritySafe,
			Description: "New MCP integration added",
		})
	}
}

func (d *ManifestDiff) HasBreakingChanges() bool {
	for _, c := range d.Changes {
		if c.Severity == SeverityBreaking {
			return true
		}
	}
	return false
}

func (d *ManifestDiff) GetSafeChanges() []Change {
	return d.filterBySeverity(SeveritySafe)
}

func (d *ManifestDiff) GetReviewChanges() []Change {
	return d.filterBySeverity(SeverityReview)
}

func (d *ManifestDiff) GetBreakingChanges() []Change {
	return d.filterBySeverity(SeverityBreaking)
}

func (d *ManifestDiff) filterBySeverity(severity ChangeSeverity) []Change {
	result := make([]Change, 0)
	for _, c := range d.Changes {
		if c.Severity == severity {
			result = append(result, c)
		}
	}
	return result
}

func (d *ManifestDiff) GetChangesByType(changeType ChangeType) []Change {
	result := make([]Change, 0)
	for _, c := range d.Changes {
		if c.Type == changeType {
			result = append(result, c)
		}
	}
	return result
}

func (d *ManifestDiff) Summary() string {
	safe := len(d.GetSafeChanges())
	review := len(d.GetReviewChanges())
	breaking := len(d.GetBreakingChanges())

	return fmt.Sprintf("Changes: %d safe, %d review, %d breaking", safe, review, breaking)
}

func (d *ManifestDiff) HasChanges() bool {
	return len(d.Changes) > 0
}

func (d *ManifestDiff) GetChangesForEntity(entityName string) []Change {
	result := make([]Change, 0)
	for _, c := range d.Changes {
		if strings.Contains(c.Path, entityName) {
			result = append(result, c)
		}
	}
	return result
}
