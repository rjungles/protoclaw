package evolution

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

func testManifestV1() *manifest.Manifest {
	return &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "TestSystem",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Task",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "title", Type: "string", Required: true},
						{Name: "status", Type: "string", Required: false},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{ID: "admin", Name: "Admin", Roles: []string{"admin"}},
		},
	}
}

func testManifestV2() *manifest.Manifest {
	return &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "TestSystem",
			Version: "1.1.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Task",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "title", Type: "string", Required: true},
						{Name: "status", Type: "string", Required: false},
						{Name: "priority", Type: "integer", Required: false},
					},
				},
				{
					Name: "Project",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "name", Type: "string", Required: true},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{ID: "admin", Name: "Admin", Roles: []string{"admin"}},
			{ID: "user", Name: "User", Roles: []string{"user"}},
		},
	}
}

func testManifestV3() *manifest.Manifest {
	return &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "TestSystem",
			Version: "1.2.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Task",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "title", Type: "string", Required: true},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{ID: "admin", Name: "Admin", Roles: []string{"admin"}},
		},
	}
}

func TestManifestDiff_NoChanges(t *testing.T) {
	m := testManifestV1()
	diff := DiffManifests(m, m)

	if diff.HasChanges() {
		t.Error("expected no changes for identical manifests")
	}

	if diff.HasBreakingChanges() {
		t.Error("expected no breaking changes")
	}
}

func TestManifestDiff_AddEntity(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)

	if !diff.HasChanges() {
		t.Error("expected changes")
	}

	addChanges := diff.GetChangesByType(ChangeTypeAdd)
	if len(addChanges) < 2 {
		t.Errorf("expected at least 2 add changes, got %d", len(addChanges))
	}

	hasProject := false
	for _, c := range addChanges {
		if c.Path == "data_model.entities.Project" {
			hasProject = true
			break
		}
	}
	if !hasProject {
		t.Error("expected Project entity to be added")
	}
}

func TestManifestDiff_AddField(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)

	addChanges := diff.GetChangesByType(ChangeTypeAdd)
	hasPriority := false
	for _, c := range addChanges {
		if c.Path == "data_model.entities.Task.fields.priority" {
			hasPriority = true
			if c.Severity != SeveritySafe {
				t.Errorf("expected safe severity for new field, got %s", c.Severity)
			}
			break
		}
	}
	if !hasPriority {
		t.Error("expected priority field to be added")
	}
}

func TestManifestDiff_AddActor(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)

	addChanges := diff.GetChangesByType(ChangeTypeAdd)
	hasUser := false
	for _, c := range addChanges {
		if c.Path == "actors.user" {
			hasUser = true
			break
		}
	}
	if !hasUser {
		t.Error("expected user actor to be added")
	}
}

func TestManifestDiff_RemoveField(t *testing.T) {
	old := testManifestV2()
	new := testManifestV3()

	diff := DiffManifests(old, new)

	removeChanges := diff.GetChangesByType(ChangeTypeRemove)
	hasStatus := false
	for _, c := range removeChanges {
		if c.Path == "data_model.entities.Task.fields.status" {
			hasStatus = true
			if c.Severity != SeverityBreaking {
				t.Errorf("expected breaking severity for removed field, got %s", c.Severity)
			}
			break
		}
	}
	if !hasStatus {
		t.Error("expected status field to be removed")
	}
}

func TestManifestDiff_ModifyField(t *testing.T) {
	old := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Task",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
					},
				},
			},
		},
	}

	new := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.1.0"},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Task",
					Fields: []manifest.Field{
						{Name: "id", Type: "integer", Required: true},
					},
				},
			},
		},
	}

	diff := DiffManifests(old, new)

	modifyChanges := diff.GetChangesByType(ChangeTypeModify)
	if len(modifyChanges) != 1 {
		t.Errorf("expected 1 modify change, got %d", len(modifyChanges))
	}

	if len(modifyChanges) > 0 {
		c := modifyChanges[0]
		if c.OldValue != "string" || c.NewValue != "integer" {
			t.Errorf("expected type change from string to integer, got %v to %v", c.OldValue, c.NewValue)
		}
	}
}

func TestManifestDiff_HasBreakingChanges(t *testing.T) {
	old := testManifestV2()
	new := testManifestV3()

	diff := DiffManifests(old, new)

	if !diff.HasBreakingChanges() {
		t.Error("expected breaking changes when removing fields")
	}
}

func TestManifestDiff_Summary(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)
	summary := diff.Summary()

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	t.Logf("Summary: %s", summary)
}

func TestMigrationPlan_CreatePlan(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)
	plan := CreateMigrationPlan(diff, nil, new)

	if len(plan.Steps) == 0 {
		t.Error("expected migration steps")
	}

	t.Logf("Plan: %s", plan.Summary())
	for _, step := range plan.Steps {
		t.Logf("  - %s: %s", step.Action, step.Description)
	}
}

func TestMigrationPlan_CanApplyAutomatically(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)
	plan := CreateMigrationPlan(diff, nil, new)

	if !plan.CanApplyAutomatically() {
		t.Error("expected plan to be auto-applicable (only safe changes)")
	}
}

func TestMigrationPlan_RequiresConfirmation(t *testing.T) {
	old := testManifestV2()
	new := testManifestV3()

	diff := DiffManifests(old, new)
	plan := CreateMigrationPlan(diff, nil, new)

	if !plan.RequiresConfirmation() {
		t.Error("expected plan to require confirmation (has breaking changes)")
	}
}

func TestMigrationPlan_GetExecutableSteps(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)
	plan := CreateMigrationPlan(diff, nil, new)

	executable := plan.GetExecutableSteps()
	if len(executable) != len(plan.Safe) {
		t.Errorf("expected %d executable steps, got %d", len(plan.Safe), len(executable))
	}
}

func TestEvolutionExecutor_NewEvolutionExecutor(t *testing.T) {
	m := testManifestV1()
	executor := NewEvolutionExecutor(m, nil)

	if executor == nil {
		t.Fatal("executor should not be nil")
	}

	if executor.GetCurrentVersion() != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", executor.GetCurrentVersion())
	}
}

func TestEvolutionExecutor_DiffWith(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	executor := NewEvolutionExecutor(old, nil)
	diff := executor.DiffWith(new)

	if !diff.HasChanges() {
		t.Error("expected changes")
	}
}

func TestEvolutionExecutor_CreatePlanFor(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	executor := NewEvolutionExecutor(old, nil)
	plan := executor.CreatePlanFor(new)

	if plan == nil {
		t.Fatal("plan should not be nil")
	}

	if len(plan.Steps) == 0 {
		t.Error("expected migration steps")
	}
}

func TestEvolutionExecutor_Evolve_NoChanges(t *testing.T) {
	m := testManifestV1()
	executor := NewEvolutionExecutor(m, nil)

	result, err := executor.Evolve(nil, m)
	if err != nil {
		t.Fatalf("Evolve: %v", err)
	}

	if !result.Success {
		t.Error("expected success for no changes")
	}
}

func TestEvolutionExecutor_GetCurrentManifest(t *testing.T) {
	m := testManifestV1()
	executor := NewEvolutionExecutor(m, nil)

	current := executor.GetCurrentManifest()
	if current != m {
		t.Error("expected same manifest reference")
	}
}

func TestManifestDiff_GetChangesForEntity(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)

	taskChanges := diff.GetChangesForEntity("Task")
	if len(taskChanges) == 0 {
		t.Error("expected changes for Task entity")
	}

	projectChanges := diff.GetChangesForEntity("Project")
	if len(projectChanges) == 0 {
		t.Error("expected changes for Project entity")
	}
}

func TestManifestDiff_GetSafeChanges(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)
	safe := diff.GetSafeChanges()

	if len(safe) == 0 {
		t.Error("expected safe changes")
	}

	for _, c := range safe {
		if c.Severity != SeveritySafe {
			t.Errorf("expected safe severity, got %s", c.Severity)
		}
	}
}

func TestManifestDiff_GetBreakingChanges(t *testing.T) {
	old := testManifestV2()
	new := testManifestV3()

	diff := DiffManifests(old, new)
	breaking := diff.GetBreakingChanges()

	if len(breaking) == 0 {
		t.Error("expected breaking changes")
	}

	for _, c := range breaking {
		if c.Severity != SeverityBreaking {
			t.Errorf("expected breaking severity, got %s", c.Severity)
		}
	}
}

func TestMigrationPlan_SafeSteps(t *testing.T) {
	old := testManifestV1()
	new := testManifestV2()

	diff := DiffManifests(old, new)
	plan := CreateMigrationPlan(diff, nil, new)

	if len(plan.Safe) == 0 {
		t.Error("expected safe steps")
	}

	for _, step := range plan.Safe {
		if step.Severity != SeveritySafe {
			t.Errorf("expected safe severity, got %s", step.Severity)
		}
	}
}

func TestMigrationPlan_BreakingSteps(t *testing.T) {
	old := testManifestV2()
	new := testManifestV3()

	diff := DiffManifests(old, new)
	plan := CreateMigrationPlan(diff, nil, new)

	if len(plan.Breaking) == 0 {
		t.Error("expected breaking steps")
	}

	for _, step := range plan.Breaking {
		if step.Severity != SeverityBreaking {
			t.Errorf("expected breaking severity, got %s", step.Severity)
		}
	}
}
