package main

import (
	"fmt"
	"os"

	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
)

func main() {
	fmt.Println("=== AgentOS - Teste de Manifestos ===\n")

	// Teste 1: Sistema de Fidelidade para Cafeteria
	fmt.Println("📋 Teste 1: Sistema de Fidelidade para Cafeteria")
	fmt.Println("=" + string(make([]byte, 50)))
	
	testCafeteriaManifest()
	
	fmt.Println()

	// Teste 2: Sistema de Tickets para Estacionamento
	fmt.Println("🅿️  Teste 2: Sistema de Tickets para Estacionamento")
	fmt.Println("=" + string(make([]byte, 50)))
	
	testParkingManifest()
	
	fmt.Println()
	fmt.Println("✅ Todos os testes concluídos com sucesso!")
}

func testCafeteriaManifest() {
	// Carregar manifesto
	manifestPath := "examples/manifests/cafeteria-loyalty.yaml"
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		fmt.Printf("❌ Erro ao carregar manifesto: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Manifesto carregado: %s v%s\n", m.Metadata.Name, m.Metadata.Version)
	fmt.Printf("  Descrição: %s\n", m.Metadata.Description)
	fmt.Printf("  Tags: %v\n", m.Metadata.Tags)

	// Validar estrutura
	parser := &manifest.Parser{}
	err = parser.Validate(m)
	if err != nil {
		fmt.Printf("❌ Erro na validação: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Validação do manifesto: OK")

	// Contar componentes
	fmt.Printf("\n📊 Estatísticas do Sistema:\n")
	fmt.Printf("  • Atores: %d\n", len(m.Actors))
	for _, actor := range m.Actors {
		fmt.Printf("    - %s (%s): %d permissões\n", actor.Name, actor.ID, len(actor.Permissions))
	}

	fmt.Printf("  • Entidades: %d\n", len(m.DataModel.Entities))
	for _, entity := range m.DataModel.Entities {
		fmt.Printf("    - %s: %d campos\n", entity.Name, len(entity.Fields))
	}

	fmt.Printf("  • Regras de Negócio: %d\n", len(m.BusinessRules))
	for _, rule := range m.BusinessRules {
		if rule.Enabled {
			fmt.Printf("    - %s (%s)\n", rule.Name, rule.Trigger.Event)
		}
	}

	fmt.Printf("  • APIs: %d\n", len(m.Integrations.APIs))
	for _, api := range m.Integrations.APIs {
		fmt.Printf("    - %s (%s): %d endpoints\n", api.Name, api.BasePath, len(api.Endpoints))
	}

	fmt.Printf("  • Canais: %d\n", len(m.Integrations.Channels))
	fmt.Printf("  • MCPs: %d\n", len(m.Integrations.MCPs))
	fmt.Printf("  • Webhooks: %d\n", len(m.Integrations.Webhooks))

	// Testar engine de políticas
	fmt.Println("\n🔐 Teste da Engine de Políticas:")
	engine, err := policy.NewEngine(m)
	if err != nil {
		fmt.Printf("❌ Erro ao criar engine: %v\n", err)
		os.Exit(1)
	}

	// Simular verificações de acesso
	testCases := []struct {
		actor    string
		resource string
		action   string
		context  map[string]interface{}
		expected bool
	}{
		{"customer", "loyalty_account", "read", map[string]interface{}{"owner": "self"}, true},
		{"customer", "rewards", "redeem", map[string]interface{}{"owner": "self"}, true},
		{"barista", "transactions", "create", map[string]interface{}{}, true},
		{"barista", "loyalty_account", "add_points", map[string]interface{}{"amount": 50}, true},
		{"barista", "loyalty_account", "add_points", map[string]interface{}{"amount": 150}, false},
		{"manager", "reports", "generate", map[string]interface{}{}, true},
		{"admin", "*", "delete", map[string]interface{}{}, true},
	}

	passed := 0
	failed := 0

	for _, tc := range testCases {
		ctx := &policy.Context{
			ActorID:    tc.actor,
			Resource:   tc.resource,
			Action:     tc.action,
			Attributes: tc.context,
		}
		result := engine.CheckPermission(ctx)
		allowed := result.Allowed

		if !allowed && !tc.expected {
			fmt.Printf("  ✓ (negado conforme esperado) %s:%s:%s -> %v\n", tc.actor, tc.resource, tc.action, allowed)
			passed++
		} else if allowed == tc.expected {
			status := "✓"
			fmt.Printf("  %s %s:%s:%s -> %v\n", status, tc.actor, tc.resource, tc.action, allowed)
			passed++
		} else {
			fmt.Printf("  ❌ %s:%s:%s -> esperado %v, got %v (reason: %s)\n", tc.actor, tc.resource, tc.action, tc.expected, allowed, result.Reason)
			failed++
		}
	}

	fmt.Printf("\n  Resultados: %d passaram, %d falharam\n", passed, failed)

	// Testar validação de regras de negócio
	fmt.Println("\n📜 Regras de Negócio Configuradas:")
	activeRules := 0
	for _, rule := range m.BusinessRules {
		if rule.Enabled {
			activeRules++
			fmt.Printf("  ✓ [%s] %s\n", rule.ID, rule.Name)
			fmt.Printf("    Trigger: %s on %v (before=%v, after=%v)\n", 
				rule.Trigger.Event, rule.Trigger.Entities, rule.Trigger.Before, rule.Trigger.After)
			fmt.Printf("    Condição: %s\n", rule.Condition)
			fmt.Printf("    Ações: %d\n", len(rule.Actions))
		}
	}
	fmt.Printf("  Total: %d regras ativas de %d\n", activeRules, len(m.BusinessRules))

	// Segurança
	fmt.Println("\n🛡️  Configurações de Segurança:")
	fmt.Printf("  • Modelo de Autorização: %s\n", m.Security.Authorization.Model)
	fmt.Printf("  • Default Deny: %v\n", m.Security.Authorization.DefaultDeny)
	fmt.Printf("  • Criptografia em Repouso: %v\n", m.Security.DataProtection.EncryptionAtRest)
	fmt.Printf("  • Criptografia em Trânsito: %v\n", m.Security.DataProtection.EncryptionInTransit)
	fmt.Printf("  • Campos Sensíveis: %d\n", len(m.Security.DataProtection.SensitiveFields))
	for _, sf := range m.Security.DataProtection.SensitiveFields {
		masking := ""
		if sf.Masking {
			masking = fmt.Sprintf(" (mask: %s)", sf.MaskPattern)
		}
		fmt.Printf("    - %s.%s [encrypt=%v, mask=%v]%s\n", sf.Entity, sf.Field, sf.Encryption, sf.Masking, masking)
	}
	fmt.Printf("  • Auditoria: %v (retenção: %d dias)\n", 
		m.Security.Audit.Enabled, m.Security.Audit.RetentionDays)

	// Requisitos não funcionais
	fmt.Println("\n⚡ Requisitos Não Funcionais:")
	fmt.Printf("  • Tempo máximo de resposta: %dms\n", m.NonFunctional.Performance.MaxResponseTimeMs)
	fmt.Printf("  • Usuários concorrentes: %d\n", m.NonFunctional.Performance.MaxConcurrentUsers)
	fmt.Printf("  • Throughput: %d req/s\n", m.NonFunctional.Performance.ThroughputRPS)
	fmt.Printf("  • Disponibilidade: %.2f%%\n", m.NonFunctional.Reliability.AvailabilityPercent)
	fmt.Printf("  • Auto-scaling: %v (%d-%d instâncias)\n", 
		m.NonFunctional.Scalability.AutoScaling,
		m.NonFunctional.Scalability.MinInstances,
		m.NonFunctional.Scalability.MaxInstances)
	fmt.Printf("  • Compliance: %v\n", m.NonFunctional.Compliance.Standards)
}

func testParkingManifest() {
	// Carregar manifesto
	manifestPath := "examples/manifests/parking-ticket.yaml"
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		fmt.Printf("❌ Erro ao carregar manifesto: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Manifesto carregado: %s v%s\n", m.Metadata.Name, m.Metadata.Version)
	fmt.Printf("  Descrição: %s\n", m.Metadata.Description)
	fmt.Printf("  Tags: %v\n", m.Metadata.Tags)

	// Validar estrutura
	parser := &manifest.Parser{}
	err = parser.Validate(m)
	if err != nil {
		fmt.Printf("❌ Erro na validação: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Validação do manifesto: OK")

	// Contar componentes
	fmt.Printf("\n📊 Estatísticas do Sistema:\n")
	fmt.Printf("  • Atores: %d\n", len(m.Actors))
	for _, actor := range m.Actors {
		fmt.Printf("    - %s (%s): %d permissões, roles: %v\n", 
			actor.Name, actor.ID, len(actor.Permissions), actor.Roles)
	}

	fmt.Printf("  • Entidades: %d\n", len(m.DataModel.Entities))
	totalFields := 0
	for _, entity := range m.DataModel.Entities {
		totalFields += len(entity.Fields)
		fmt.Printf("    - %s: %d campos, %d índices\n", 
			entity.Name, len(entity.Fields), len(entity.Indexes))
	}
	fmt.Printf("  • Total de campos: %d\n", totalFields)
	fmt.Printf("  • Relacionamentos: %d\n", len(m.DataModel.Relations))

	fmt.Printf("  • Regras de Negócio: %d\n", len(m.BusinessRules))
	activeRules := 0
	for _, rule := range m.BusinessRules {
		if rule.Enabled {
			activeRules++
			fmt.Printf("    ✓ %s [%s]\n", rule.Name, rule.ID)
		}
	}

	fmt.Printf("  • APIs: %d\n", len(m.Integrations.APIs))
	for _, api := range m.Integrations.APIs {
		fmt.Printf("    - %s (%s)\n", api.Name, api.BasePath)
		for _, ep := range api.Endpoints {
			fmt.Printf("      • %s %s - %s\n", ep.Method, ep.Path, ep.Description)
		}
	}

	fmt.Printf("  • Canais: %d\n", len(m.Integrations.Channels))
	fmt.Printf("  • MCPs: %d\n", len(m.Integrations.MCPs))
	for _, mcpc := range m.Integrations.MCPs {
		fmt.Printf("    - %s (%s via %s)\n", mcpc.Name, mcpc.Server, mcpc.Transport)
		fmt.Printf("      Tools: %v\n", mcpc.Tools)
	}
	fmt.Printf("  • Webhooks: %d\n", len(m.Integrations.Webhooks))

	// Testar engine de políticas
	fmt.Println("\n🔐 Teste da Engine de Políticas:")
	engine, err := policy.NewEngine(m)
	if err != nil {
		fmt.Printf("❌ Erro ao criar engine: %v\n", err)
		os.Exit(1)
	}

	// Simular verificações de acesso específicas do estacionamento
	testCases := []struct {
		actor    string
		resource string
		action   string
		context  map[string]interface{}
		expected bool
		desc     string
	}{
		{"driver", "own_tickets", "pay", map[string]interface{}{"owner": "self"}, true, "Cliente paga seu ticket"},
		{"driver", "tickets", "read", map[string]interface{}{"owner": "other"}, false, "Cliente não vê tickets de outros"},
		{"attendant", "tickets", "create", map[string]interface{}{"shift_active": true}, true, "Atendente cria ticket com turno ativo"},
		{"attendant", "tickets", "create", map[string]interface{}{"shift_active": false}, false, "Atendente não cria ticket com turno inativo"},
		{"attendant", "payments", "process", map[string]interface{}{"amount": 30.0}, true, "Atendente processa pagamento até R$500"},
		{"attendant", "payments", "process", map[string]interface{}{"amount": 600.0}, false, "Atendente não processa pagamento > R$500"},
		{"attendant", "gates", "open", map[string]interface{}{}, true, "Atendente abre portão"},
		{"supervisor", "discounts", "apply", map[string]interface{}{"discount_percent": 30}, true, "Supervisor aplica desconto até 50%"},
		{"supervisor", "discounts", "apply", map[string]interface{}{"discount_percent": 60}, false, "Supervisor não aplica desconto > 50%"},
		{"supervisor", "payments", "refund", map[string]interface{}{"amount": 1500}, true, "Supervisor reembolsa até R$2000"},
		{"supervisor", "payments", "refund", map[string]interface{}{"amount": 2500}, false, "Supervisor não reembolsa > R$2000"},
		{"manager", "rates", "create", map[string]interface{}{}, true, "Gerente cria tarifas"},
		{"manager", "staff", "delete", map[string]interface{}{}, true, "Gerente remove funcionário"},
		{"admin", "*", "execute", map[string]interface{}{}, true, "Admin tem acesso total"},
	}

	passed := 0
	failed := 0

	for _, tc := range testCases {
		ctx := &policy.Context{
			ActorID:    tc.actor,
			Resource:   tc.resource,
			Action:     tc.action,
			Attributes: tc.context,
		}
		result := engine.CheckPermission(ctx)
		allowed := result.Allowed

		if !allowed && !tc.expected {
			fmt.Printf("  ✓ (negado) %s\n", tc.desc)
			passed++
		} else if allowed == tc.expected {
			status := "✓"
			fmt.Printf("  %s %s\n", status, tc.desc)
			passed++
		} else {
			fmt.Printf("  ❌ %s -> esperado %v, got %v (reason: %s)\n", tc.desc, tc.expected, allowed, result.Reason)
			failed++
		}
	}

	fmt.Printf("\n  Resultados: %d passaram, %d falharam\n", passed, failed)

	// Detalhar regras de negócio críticas
	fmt.Println("\n📜 Regras de Negócio Críticas:")
	criticalRules := []string{
		"rule_calculate_parking_fee",
		"rule_validate_entry",
		"rule_authorize_exit",
		"rule_process_payment",
	}

	for _, ruleID := range criticalRules {
		for _, rule := range m.BusinessRules {
			if rule.ID == ruleID {
				fmt.Printf("  🔹 %s\n", rule.Name)
				fmt.Printf("     Trigger: %s on %v\n", rule.Trigger.Event, rule.Trigger.Entities)
				fmt.Printf("     Tipo: before=%v, after=%v\n", rule.Trigger.Before, rule.Trigger.After)
				fmt.Printf("     Ações: %d\n", len(rule.Actions))
				for i, action := range rule.Actions {
					fmt.Printf("       %d. [%s] %s\n", i+1, action.Type, action.Target)
				}
				break
			}
		}
	}

	// Segurança e conformidade
	fmt.Println("\n🛡️  Segurança e Conformidade:")
	fmt.Printf("  • Modelo: %s (default deny: %v)\n", 
		m.Security.Authorization.Model, m.Security.Authorization.DefaultDeny)
	fmt.Printf("  • Métodos de Autenticação: %v\n", m.Security.Authentication.Methods)
	fmt.Printf("  • Timeout de Sessão: %d minutos\n", m.Security.Authentication.SessionTimeout)
	fmt.Printf("  • Política de Senhas:\n")
	pp := m.Security.Authentication.PasswordPolicy
	fmt.Printf("      - Comprimento mínimo: %d\n", pp.MinLength)
	fmt.Printf("      - Maiúsculas: %v, Minúsculas: %v, Números: %v, Especiais: %v\n",
		pp.RequireUppercase, pp.RequireLowercase, pp.RequireNumbers, pp.RequireSpecial)
	
	fmt.Printf("  • Proteção de Dados:\n")
	fmt.Printf("      - Encrypt at rest: %v\n", m.Security.DataProtection.EncryptionAtRest)
	fmt.Printf("      - Encrypt in transit: %v\n", m.Security.DataProtection.EncryptionInTransit)
	fmt.Printf("      - Campos sensíveis: %d\n", len(m.Security.DataProtection.SensitiveFields))
	
	fmt.Printf("  • Auditoria:\n")
	fmt.Printf("      - Habilitada: %v\n", m.Security.Audit.Enabled)
	fmt.Printf("      - Nível: %s\n", m.Security.Audit.LogLevel)
	fmt.Printf("      - Eventos monitorados: %d\n", len(m.Security.Audit.IncludeEvents))
	fmt.Printf("      - Retenção: %d dias\n", m.Security.Audit.RetentionDays)
	
	fmt.Printf("  • Conformidade: %v\n", m.NonFunctional.Compliance.Standards)
	fmt.Printf("      - Residência de dados: %s\n", m.NonFunctional.Compliance.Region)

	// Integrações de hardware
	fmt.Println("\n🔌 Integrações de Hardware e Pagamento:")
	for _, mcpc := range m.Integrations.MCPs {
		fmt.Printf("  • %s\n", mcpc.Name)
		fmt.Printf("    Servidor: %s\n", mcpc.Server)
		fmt.Printf("    Transporte: %s\n", mcpc.Transport)
		fmt.Printf("    Tools disponíveis:\n")
		for _, tool := range mcpc.Tools {
			fmt.Printf("      - %s\n", tool)
		}
	}

	// SLA e Performance
	fmt.Println("\n⚡ SLA e Performance:")
	fmt.Printf("  • Tempo máximo de resposta: %dms\n", m.NonFunctional.Performance.MaxResponseTimeMs)
	fmt.Printf("  • Usuários concorrentes máximos: %d\n", m.NonFunctional.Performance.MaxConcurrentUsers)
	fmt.Printf("  • Throughput: %d requisições/segundo\n", m.NonFunctional.Performance.ThroughputRPS)
	fmt.Printf("  • Disponibilidade: %.2f%% (%.1f horas de downtime/mês)\n", 
		m.NonFunctional.Reliability.AvailabilityPercent,
		(100-m.NonFunctional.Reliability.AvailabilityPercent)*730/100)
	fmt.Printf("  • MTTR: %d minutos\n", m.NonFunctional.Reliability.MTTRMinutes)
	fmt.Printf("  • MTBF: %d horas (%.1f dias)\n", 
		m.NonFunctional.Reliability.MTBFHours, 
		float64(m.NonFunctional.Reliability.MTBFHours)/24)
}
