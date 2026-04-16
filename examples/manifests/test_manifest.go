package main

import (
	"fmt"
	"os"

	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/manifest"
)

func main() {
	fmt.Println("=== Teste do Sistema de Manifesto e Políticas ===\n")

	// 1. Carregar e parsear o manifesto
	fmt.Println("1. Carregando manifesto de exemplo...")
	m, err := manifest.ParseFile("examples/manifests/task-management.yaml")
	if err != nil {
		fmt.Printf("Erro ao carregar manifesto: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   ✓ Manifesto carregado: %s v%s\n", m.Metadata.Name, m.Metadata.Version)
	fmt.Printf("   ✓ Descrição: %s\n", m.Metadata.Description)

	// 2. Validar o manifesto
	fmt.Println("\n2. Validando manifesto...")
	parser := &manifest.Parser{}
	err = parser.Validate(m)
	if err != nil {
		fmt.Printf("   ✗ Erro de validação: %v\n", err)
		os.Exit(1)
	}
	warnings := parser.GetWarnings()
	if len(warnings) > 0 {
		fmt.Printf("   ⚠ Avisos: %v\n", warnings)
	} else {
		fmt.Println("   ✓ Manifesto válido")
	}

	// 3. Imprimir resumo do manifesto
	fmt.Println("\n3. Resumo do manifesto:")
	fmt.Printf("   • Atores definidos: %d\n", len(m.Actors))
	for _, actor := range m.Actors {
		fmt.Printf("     - %s (%s): %d permissões\n", actor.Name, actor.ID, len(actor.Permissions))
	}

	fmt.Printf("   • Entidades no modelo de dados: %d\n", len(m.DataModel.Entities))
	for _, entity := range m.DataModel.Entities {
		fmt.Printf("     - %s: %d campos\n", entity.Name, len(entity.Fields))
	}

	fmt.Printf("   • Regras de negócio: %d\n", len(m.BusinessRules))
	for _, rule := range m.BusinessRules {
		status := "✓"
		if !rule.Enabled {
			status = "✗"
		}
		fmt.Printf("     %s %s: %s\n", status, rule.ID, rule.Name)
	}

	fmt.Printf("   • APIs configuradas: %d\n", len(m.Integrations.APIs))
	for _, api := range m.Integrations.APIs {
		fmt.Printf("     - %s (%s): %d endpoints\n", api.Name, api.BasePath, len(api.Endpoints))
	}

	fmt.Printf("   • Canais configurados: %d\n", len(m.Integrations.Channels))
	for _, ch := range m.Integrations.Channels {
		status := "ativo"
		if !ch.Enabled {
			status = "inativo"
		}
		fmt.Printf("     - %s (%s): %s\n", ch.Name, ch.Type, status)
	}

	// 4. Criar engine de políticas
	fmt.Println("\n4. Inicializando engine de políticas...")
	engine, err := policy.NewEngine(m)
	if err != nil {
		fmt.Printf("   ✗ Erro ao criar engine: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("   ✓ Engine de políticas inicializada")

	// 5. Validar configurações de segurança
	fmt.Println("\n5. Validando configurações de segurança...")
	err = policy.ValidateManifest(m)
	if err != nil {
		fmt.Printf("   ✗ Erro na validação de segurança: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("   ✓ Configurações de segurança válidas")

	// 6. Testar verificações de permissão
	fmt.Println("\n6. Testando verificações de permissão:")

	// Teste 1: Admin acessando tudo
	ctx := &policy.Context{
		ActorID:  "admin",
		Resource: "projects",
		Action:   "delete",
	}
	result := engine.CheckPermission(ctx)
	fmt.Printf("   • Admin pode deletar projetos? %v (%s)\n", result.Allowed, result.Reason)

	// Teste 2: Member lendo projetos
	ctx = &policy.Context{
		ActorID:  "member",
		Resource: "projects",
		Action:   "read",
	}
	result = engine.CheckPermission(ctx)
	fmt.Printf("   • Member pode ler projetos? %v (%s)\n", result.Allowed, result.Reason)

	// Teste 3: Member deletando projetos (deveria ser negado)
	ctx = &policy.Context{
		ActorID:  "member",
		Resource: "projects",
		Action:   "delete",
	}
	result = engine.CheckPermission(ctx)
	fmt.Printf("   • Member pode deletar projetos? %v (%s)\n", result.Allowed, result.Reason)

	// Teste 4: Viewer lendo tarefas
	ctx = &policy.Context{
		ActorID:  "viewer",
		Resource: "tasks",
		Action:   "read",
	}
	result = engine.CheckPermission(ctx)
	fmt.Printf("   • Viewer pode ler tarefas? %v (%s)\n", result.Allowed, result.Reason)

	// Teste 5: Manager com herança de papéis
	roles := engine.GetAllRoles("manager")
	fmt.Printf("   • Papéis do Manager (com herança): %v\n", roles)

	// 7. Listar recursos
	fmt.Println("\n7. Recursos protegidos:")
	resources := engine.ListResources()
	for _, res := range resources {
		fmt.Printf("   - %s\n", res)
	}

	// 8. Serialização
	fmt.Println("\n8. Testando serialização:")
	yamlData, err := m.ToYAML()
	if err != nil {
		fmt.Printf("   ✗ Erro ao serializar YAML: %v\n", err)
	} else {
		fmt.Printf("   ✓ YAML gerado: %d bytes\n", len(yamlData))
	}

	jsonData, err := m.ToJSON()
	if err != nil {
		fmt.Printf("   ✗ Erro ao serializar JSON: %v\n", err)
	} else {
		fmt.Printf("   ✓ JSON gerado: %d bytes\n", len(jsonData))
	}

	fmt.Println("\n=== Todos os testes concluídos com sucesso! ===")
}
