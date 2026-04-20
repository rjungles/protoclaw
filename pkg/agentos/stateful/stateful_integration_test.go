package stateful

import (
	"context"
	"database/sql"
	"testing"

	"github.com/sipeed/picoclaw/pkg/manifest"
	_ "modernc.org/sqlite"
)

// TestWorkflowEngine_ExperiencePlatform testa o fluxo editorial completo do experience-platform
func TestWorkflowEngine_ExperiencePlatform(t *testing.T) {
	// Criar manifesto com fluxo editorial
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "ExperiencePlatform",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Experience",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "title", Type: "string", Required: true},
						{Name: "content", Type: "string", Required: false},
						{Name: "author_id", Type: "string", Required: true},
						{Name: "editor_id", Type: "string", Required: false},
						{Name: "reviewer_id", Type: "string", Required: false},
						{Name: "state", Type: "string", Required: true},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{ID: "author", Name: "Author", Roles: []string{"author"}},
			{ID: "editor", Name: "Editor", Roles: []string{"editor"}},
			{ID: "reviewer", Name: "Reviewer", Roles: []string{"reviewer"}},
			{ID: "admin", Name: "Administrator", Roles: []string{"admin"}},
		},
		Workflows: []manifest.WorkflowConfig{
			{
				Entity:       "Experience",
				InitialState: "DRAFT",
				States: []manifest.WorkflowState{
					{
						ID:          "DRAFT",
						Description: "Initial draft state",
						Transitions: []manifest.WorkflowTransition{
							{To: "UNDER_REVIEW", Action: "submit", AllowedRoles: []string{"author"}},
						},
					},
					{
						ID:          "UNDER_REVIEW",
						Description: "Under editorial review",
						Transitions: []manifest.WorkflowTransition{
							{To: "DRAFT", Action: "return", AllowedRoles: []string{"editor", "reviewer"}},
							{To: "CLARIFICATION", Action: "request_clarification", AllowedRoles: []string{"editor"}},
							{To: "APPROVED", Action: "approve", AllowedRoles: []string{"editor", "reviewer"}},
						},
					},
					{
						ID:          "CLARIFICATION",
						Description: "Awaiting author clarification",
						Transitions: []manifest.WorkflowTransition{
							{To: "UNDER_REVIEW", Action: "resubmit", AllowedRoles: []string{"author"}},
						},
					},
					{
						ID:          "APPROVED",
						Description: "Approved for publication",
						Transitions: []manifest.WorkflowTransition{
							{To: "PUBLISHED", Action: "publish", AllowedRoles: []string{"admin", "editor"}},
						},
					},
					{
						ID:          "PUBLISHED",
						Description: "Published experience",
						Transitions: []manifest.WorkflowTransition{
							{To: "ARCHIVED", Action: "archive", AllowedRoles: []string{"admin"}},
						},
					},
					{
						ID:          "ARCHIVED",
						Description: "Archived experience",
						Transitions: []manifest.WorkflowTransition{},
					},
				},
			},
		},
	}

	// Criar engine com banco de dados em memória
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	engine := NewWorkflowEngine(m, db)
	if engine == nil {
		t.Fatal("Engine should not be nil")
	}

	ctx := context.Background()

	// Testar fluxo completo
	expID := "exp-123"

	// 1. Autor cria experiência
	instance, err := engine.InitializeState("Experience", expID, "author")
	if err != nil {
		t.Fatalf("Failed to initialize state: %v", err)
	}
	if instance.CurrentState != "DRAFT" {
		t.Errorf("Expected state DRAFT, got %s", instance.CurrentState)
	}

	// 2. Autor submete para revisão
	instance, err = engine.Transition(ctx, "Experience", expID, "submit", "author", []string{"author"})
	if err != nil {
		t.Fatalf("Failed to submit: %v", err)
	}
	if instance.CurrentState != "UNDER_REVIEW" {
		t.Errorf("Expected state UNDER_REVIEW, got %s", instance.CurrentState)
	}

	// 3. Editor solicita clarificação
	instance, err = engine.Transition(ctx, "Experience", expID, "request_clarification", "editor", []string{"editor"})
	if err != nil {
		t.Fatalf("Failed to request clarification: %v", err)
	}
	if instance.CurrentState != "CLARIFICATION" {
		t.Errorf("Expected state CLARIFICATION, got %s", instance.CurrentState)
	}

	// 4. Autor resubmete
	instance, err = engine.Transition(ctx, "Experience", expID, "resubmit", "author", []string{"author"})
	if err != nil {
		t.Fatalf("Failed to resubmit: %v", err)
	}
	if instance.CurrentState != "UNDER_REVIEW" {
		t.Errorf("Expected state UNDER_REVIEW, got %s", instance.CurrentState)
	}

	// 5. Editor aprova
	instance, err = engine.Transition(ctx, "Experience", expID, "approve", "editor", []string{"editor"})
	if err != nil {
		t.Fatalf("Failed to approve: %v", err)
	}
	if instance.CurrentState != "APPROVED" {
		t.Errorf("Expected state APPROVED, got %s", instance.CurrentState)
	}

	// 6. Admin publica
	instance, err = engine.Transition(ctx, "Experience", expID, "publish", "admin", []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}
	if instance.CurrentState != "PUBLISHED" {
		t.Errorf("Expected state PUBLISHED, got %s", instance.CurrentState)
	}

	// 7. Verificar histórico completo
	history, err := engine.GetHistory("Experience", expID)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 6 { // initialize + submit + request_clarification + resubmit + approve + publish
		t.Errorf("Expected 6 history records, got %d", len(history))
	}

	// Verificar ações disponíveis no estado atual
	actions := engine.ListAvailableActions("Experience", expID, []string{"admin"})
	if len(actions) != 1 || actions[0] != "archive" {
		t.Errorf("Expected [archive] actions, got %v", actions)
	}
}

// TestWorkflowEngine_ClarificationFlow testa o fluxo de esclarecimento
func TestWorkflowEngine_ClarificationFlow(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "ClarificationFlow",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Document",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "content", Type: "string", Required: true},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{ID: "author", Name: "Author", Roles: []string{"author"}},
			{ID: "reviewer", Name: "Reviewer", Roles: []string{"reviewer"}},
		},
		Workflows: []manifest.WorkflowConfig{
			{
				Entity:       "Document",
				InitialState: "DRAFT",
				States: []manifest.WorkflowState{
					{
						ID:          "DRAFT",
						Description: "Draft state",
						Transitions: []manifest.WorkflowTransition{
							{To: "PENDING_REVIEW", Action: "submit", AllowedRoles: []string{"author"}},
						},
					},
					{
						ID:          "PENDING_REVIEW",
						Description: "Pending review",
						Transitions: []manifest.WorkflowTransition{
							{To: "CLARIFICATION_NEEDED", Action: "request_clarification", AllowedRoles: []string{"reviewer"}},
							{To: "APPROVED", Action: "approve", AllowedRoles: []string{"reviewer"}},
						},
					},
					{
						ID:          "CLARIFICATION_NEEDED",
						Description: "Clarification needed",
						Transitions: []manifest.WorkflowTransition{
							{To: "PENDING_REVIEW", Action: "provide_clarification", AllowedRoles: []string{"author"}},
						},
					},
					{
						ID:          "APPROVED",
						Description: "Approved",
						Transitions: []manifest.WorkflowTransition{},
					},
				},
			},
		},
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	engine := NewWorkflowEngine(m, db)
	ctx := context.Background()

	docID := "doc-456"

	// Inicializar documento
	engine.InitializeState("Document", docID, "author")

	// Fluxo de esclarecimento
	engine.Transition(ctx, "Document", docID, "submit", "author", []string{"author"})
	engine.Transition(ctx, "Document", docID, "request_clarification", "reviewer", []string{"reviewer"})
	engine.Transition(ctx, "Document", docID, "provide_clarification", "author", []string{"author"})

	// Verificar estado final
	instance, _ := engine.GetCurrentState("Document", docID)
	if instance.CurrentState != "PENDING_REVIEW" {
		t.Errorf("Expected PENDING_REVIEW after clarification, got %s", instance.CurrentState)
	}
}

// TestWorkflowEngine_TimeoutAutoReturn testa timeout com retorno automático
func TestWorkflowEngine_TimeoutAutoReturn(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "TimeoutTest",
			Version: "1.0.0",
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
			{ID: "user", Name: "User", Roles: []string{"user"}},
		},
		Workflows: []manifest.WorkflowConfig{
			{
				Entity:       "Task",
				InitialState: "NEW",
				States: []manifest.WorkflowState{
					{
						ID:          "NEW",
						Description: "New task",
						Transitions: []manifest.WorkflowTransition{
							{To: "IN_PROGRESS", Action: "start", AllowedRoles: []string{"user"}},
						},
					},
					{
						ID:          "IN_PROGRESS",
						Description: "In progress",
						Transitions: []manifest.WorkflowTransition{
							{To: "COMPLETED", Action: "complete", AllowedRoles: []string{"user"}},
							{To: "NEW", Action: "auto_return", AllowedRoles: []string{"system"}},
						},
					},
					{
						ID:          "COMPLETED",
						Description: "Completed",
						Transitions: []manifest.WorkflowTransition{},
					},
				},
			},
		},
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	engine := NewWorkflowEngine(m, db)
	ctx := context.Background()

	taskID := "task-789"

	// Inicializar e iniciar tarefa
	engine.InitializeState("Task", taskID, "user")
	engine.Transition(ctx, "Task", taskID, "start", "user", []string{"user"})

	// Simular verificação de timeout (em produção seria um cron job)
	// Aqui apenas verificamos que o mecanismo está implementado
	if engine.timeouts != nil {
		// O timeout manager foi criado corretamente
		timeouts, err := engine.timeouts.CheckTimeouts(ctx)
		if err != nil {
			t.Logf("Timeout check error (expected in test): %v", err)
		}
		if timeouts != nil {
			t.Logf("Found %d timeouts", len(timeouts))
		}
	}
}

// TestContextualQuery_Integration testa queries contextuais com dados reais
func TestContextualQuery_Integration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Criar tabela de teste
	_, err = db.Exec(`
		CREATE TABLE experiences (
			id TEXT PRIMARY KEY,
			title TEXT,
			author_id TEXT,
			state TEXT,
			visibility TEXT,
			created_by TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Inserir dados de teste
	_, err = db.Exec(`
		INSERT INTO experiences (id, title, author_id, state, visibility, created_by) VALUES
		('exp1', 'Public Experience', 'user1', 'PUBLISHED', 'public', 'user1'),
		('exp2', 'Private Experience', 'user1', 'DRAFT', 'private', 'user1'),
		('exp3', 'Other User Experience', 'user2', 'PUBLISHED', 'public', 'user2')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Testar query com diferentes contextos
	queryBuilder := NewQueryBuilder(nil, db)

	// Teste 1: Admin pode ver tudo
	adminQuery := &ContextualQuery{
		ActorID: "admin1",
		Roles:   []string{"admin"},
	}

	sql, args := queryBuilder.BuildEntityQuery("experiences", adminQuery, nil)
	t.Logf("Admin query: %s with args %v", sql, args)

	// Teste 2: Autor pode ver apenas seus próprios itens
	authorQuery := &ContextualQuery{
		ActorID: "user1",
		Roles:   []string{"author"},
	}

	sql, args = queryBuilder.BuildEntityQuery("experiences", authorQuery, nil)
	t.Logf("Author query: %s with args %v", sql, args)

	// Executar query do autor
	rows, err := db.Query(sql, args...)
	if err != nil {
		t.Fatalf("Failed to execute author query: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	if count != 2 { // user1 tem 2 experiências
		t.Errorf("Expected 2 experiences for user1, got %d", count)
	}
}
