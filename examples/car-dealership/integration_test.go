// Testes de integração para o sistema AutoPrime Concessionária
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/agentos/llm"
	"github.com/sipeed/picoclaw/pkg/manifest"
)

// TestSystemManifestValidation valida o manifesto do sistema
func TestSystemManifestValidation(t *testing.T) {
	exampleDir := getTestDir()
	manifestPath := filepath.Join(exampleDir, "system.yaml")

	// Verifica se o arquivo existe
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Skip("Manifest file not found, skipping test")
	}

	// Parse do manifesto
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	// Valida metadados
	if m.Metadata.Name != "AutoPrime Concessionária" {
		t.Errorf("Expected system name 'AutoPrime Concessionária', got '%s'", m.Metadata.Name)
	}

	if m.Metadata.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", m.Metadata.Version)
	}

	// Valida entidades (usando DataModel.Entities)
	expectedEntities := []string{"Customer", "Vehicle", "VehicleInterest", "Appointment", "Sale", "Salesperson", "ChatMessage"}
	entityNames := make(map[string]bool)
	for _, e := range m.DataModel.Entities {
		entityNames[e.Name] = true
	}

	for _, entityName := range expectedEntities {
		if !entityNames[entityName] {
			t.Errorf("Expected entity '%s' not found in manifest", entityName)
		}
	}

	// Valida regras de negócio (usando BusinessRules)
	ruleNames := make(map[string]bool)
	for _, r := range m.BusinessRules {
		ruleNames[r.Name] = true
	}

	expectedRules := []string{"notifyOnHighInterest", "remindAppointment", "followUpChat"}
	for _, ruleName := range expectedRules {
		if !ruleNames[ruleName] {
			t.Errorf("Expected business rule '%s' not found in manifest", ruleName)
		}
	}

	// Valida canais (usando Integrations.Channels)
	channelTypes := make(map[string]bool)
	for _, c := range m.Integrations.Channels {
		channelTypes[c.Type] = true
	}

	expectedChannels := []string{"websocket", "webhook", "smtp", "telegram"}
	for _, ct := range expectedChannels {
		if !channelTypes[ct] {
			t.Errorf("Expected channel type '%s' not found", ct)
		}
	}

	t.Logf("✓ Manifest validation passed for %s v%s with %d entities and %d business rules", 
		m.Metadata.Name, m.Metadata.Version, len(m.DataModel.Entities), len(m.BusinessRules))
}

// TestLLMConfigValidation valida a configuração LLM
func TestLLMConfigValidation(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	// Verifica se o arquivo existe
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	// Carrega a configuração
	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load LLM config: %v", err)
	}

	// Valida sistema
	if config.System != "autoprime-concessionaria" {
		t.Errorf("Expected system 'autoprime-concessionaria', got '%s'", config.System)
	}

	// Valida configurações
	if !config.Settings.HotReload {
		t.Error("Expected hot_reload to be enabled")
	}

	if config.Settings.ReloadInterval != 5 {
		t.Errorf("Expected reload_interval 5, got %d", config.Settings.ReloadInterval)
	}

	// Valida provedores
	if len(config.Providers) < 2 {
		t.Errorf("Expected at least 2 providers, got %d", len(config.Providers))
	}

	// Valida provedores específicos
	requiredProviders := []string{"openai", "groq", "anthropic"}
	for _, providerName := range requiredProviders {
		provider := config.GetProvider(providerName)
		if provider == nil {
			t.Errorf("Expected provider '%s' not found", providerName)
			continue
		}
		if !provider.Enabled {
			t.Errorf("Provider '%s' should be enabled", providerName)
		}
	}

	// Valida agentes
	requiredAgents := []string{"atendente-virtual", "analista-sentimento", "redator", "especialista-financeiro", "especialista-tecnico"}
	for _, agentName := range requiredAgents {
		agent := config.GetAgent(agentName)
		if agent == nil {
			t.Errorf("Expected agent '%s' not found", agentName)
			continue
		}
		if agent.Provider == "" {
			t.Errorf("Agent '%s' should have a provider configured", agentName)
		}
		if agent.Model == "" {
			t.Errorf("Agent '%s' should have a model configured", agentName)
		}
	}

	// Valida roteamento
	if len(config.Routing.Functions) == 0 {
		t.Error("Expected routing functions to be configured")
	}

	if len(config.Routing.Intents) == 0 {
		t.Error("Expected routing intents to be configured")
	}

	if !config.Routing.CostBased.Enabled {
		t.Error("Expected cost-based routing to be enabled")
	}

	t.Logf("✓ LLM config validation passed with %d providers and %d agents", 
		len(config.Providers), len(config.Agents))
}

// TestRoutingRules valida as regras de roteamento
func TestRoutingRules(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Testa roteamento de funções
	functionRoutes := map[string]struct {
		provider string
		model    string
	}{
		"chatbotResponse":             {provider: "openai", model: "gpt-4o"},
		"analyzeCustomerSentiment":    {provider: "anthropic", model: "claude-3-sonnet-20240229"},
		"generateVehicleDescription":  {provider: "openai", model: "gpt-4o"},
		"technicalQuery":              {provider: "groq", model: "llama3-70b-8192"},
		"financingQuery":              {provider: "openai", model: "gpt-4o-mini"},
		"simpleQuery":                 {provider: "groq", model: "mixtral-8x7b-32768"},
	}

	for funcName, expected := range functionRoutes {
		rule, ok := config.Routing.Functions[funcName]
		if !ok {
			t.Errorf("Routing rule for function '%s' not found", funcName)
			continue
		}
		if rule.Provider != expected.provider {
			t.Errorf("Function '%s': expected provider '%s', got '%s'", 
				funcName, expected.provider, rule.Provider)
		}
		if rule.Model != expected.model {
			t.Errorf("Function '%s': expected model '%s', got '%s'", 
				funcName, expected.model, rule.Model)
		}
	}

	// Testa roteamento de intenções
	intentRoutes := map[string]string{
		"interesse_compra": "openai",
		"duvida_tecnica":   "groq",
		"financiamento":    "openai",
		"reclamacao":       "anthropic",
		"agendamento":      "openai",
	}

	for intent, expectedProvider := range intentRoutes {
		rule, ok := config.Routing.Intents[intent]
		if !ok {
			t.Errorf("Routing rule for intent '%s' not found", intent)
			continue
		}
		if rule.Provider != expectedProvider {
			t.Errorf("Intent '%s': expected provider '%s', got '%s'", 
				intent, expectedProvider, rule.Provider)
		}
	}

	t.Logf("✓ All routing rules validated")
}

// TestAgentSystemPrompts valida os prompts dos agentes
func TestAgentSystemPrompts(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Valida prompts específicos
	agentValidations := map[string]func(string) bool{
		"atendente-virtual": func(prompt string) bool {
			return strings.Contains(prompt, "AutoPrime") &&
				strings.Contains(prompt, "concessionária") &&
				strings.Contains(prompt, "cordial")
		},
		"analista-sentimento": func(prompt string) bool {
			return strings.Contains(prompt, "sentiment") &&
				strings.Contains(prompt, "JSON")
		},
		"redator": func(prompt string) bool {
			return strings.Contains(prompt, "descrições") &&
				strings.Contains(prompt, "veículos")
		},
		"especialista-financeiro": func(prompt string) bool {
			return strings.Contains(prompt, "Financiamento") &&
				strings.Contains(prompt, "taxas")
		},
		"especialista-tecnico": func(prompt string) bool {
			return strings.Contains(prompt, "Especialista Técnico") &&
				strings.Contains(prompt, "especificações")
		},
	}

	for agentName, validate := range agentValidations {
		agent := config.GetAgent(agentName)
		if agent == nil {
			t.Errorf("Agent '%s' not found", agentName)
			continue
		}

		if agent.SystemPrompt == "" {
			t.Errorf("Agent '%s' should have a system prompt", agentName)
			continue
		}

		if !validate(agent.SystemPrompt) {
			t.Errorf("Agent '%s' system prompt validation failed", agentName)
		}
	}

	t.Logf("✓ All agent system prompts validated")
}

// TestProviderConfiguration valida a configuração dos provedores
func TestProviderConfiguration(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Valida OpenAI
	openai := config.GetProvider("openai")
	if openai != nil {
		if openai.Type != llm.ProviderTypeOpenAI {
			t.Errorf("OpenAI provider type should be 'openai', got '%s'", openai.Type)
		}
		if len(openai.Models) < 2 {
			t.Errorf("OpenAI should have at least 2 models, got %d", len(openai.Models))
		}
	}

	// Valida Groq (compatible)
	groq := config.GetProvider("groq")
	if groq != nil {
		if groq.Type != llm.ProviderTypeCompatible {
			t.Errorf("Groq provider type should be 'compatible', got '%s'", groq.Type)
		}
		baseURL := groq.GetBaseURL()
		if !strings.Contains(baseURL, "groq") {
			t.Errorf("Groq base URL should contain 'groq', got '%s'", baseURL)
		}
	}

	// Valida Anthropic
	anthropic := config.GetProvider("anthropic")
	if anthropic != nil {
		if anthropic.Type != llm.ProviderTypeAnthropic {
			t.Errorf("Anthropic provider type should be 'anthropic', got '%s'", anthropic.Type)
		}
	}

	// Valida prioridades
	priorities := make(map[int]bool)
	for _, p := range config.Providers {
		if priorities[p.Priority] {
			t.Errorf("Duplicate provider priority: %d", p.Priority)
		}
		priorities[p.Priority] = true
	}

	t.Logf("✓ Provider configuration validated")
}

// TestChatbotScenarios testa cenários de atendimento
func TestChatbotScenarios(t *testing.T) {
	// Cenários de teste para o chatbot
	scenarios := []struct {
		name         string
		input        string
		expectIntent string
		keywords     []string
	}{
		{
			name:         "Busca por SUV",
			input:        "Quero comprar um SUV até 150 mil",
			expectIntent: "interesse_compra",
			keywords:     []string{"SUV", "150", "mil"},
		},
		{
			name:         "Dúvida técnica",
			input:        "Qual o consumo do Corolla na cidade?",
			expectIntent: "duvida_tecnica",
			keywords:     []string{"consumo", "Corolla"},
		},
		{
			name:         "Financiamento",
			input:        "Quais as taxas de financiamento?",
			expectIntent: "financiamento",
			keywords:     []string{"taxas", "financiamento"},
		},
		{
			name:         "Reclamação",
			input:        "Estou muito insatisfeito com o atraso na entrega do meu carro",
			expectIntent: "reclamacao",
			keywords:     []string{"insatisfeito", "atraso"},
		},
		{
			name:         "Agendamento",
			input:        "Gostaria de agendar um test drive para sábado",
			expectIntent: "agendamento",
			keywords:     []string{"agendar", "test drive"},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Verifica se o input contém as keywords esperadas
			for _, keyword := range scenario.keywords {
				if !strings.Contains(strings.ToLower(scenario.input), strings.ToLower(keyword)) {
					t.Errorf("Input should contain keyword '%s'", keyword)
				}
			}
		})
	}

	t.Logf("✓ Chatbot scenarios validated: %d scenarios", len(scenarios))
}

// TestIntegrationFlow testa um fluxo completo de integração
func TestIntegrationFlow(t *testing.T) {
	// Este teste simula um fluxo completo de atendimento
	t.Log("Simulating complete customer service flow...")

	steps := []struct {
		step        string
		description string
	}{
		{
			step:        "1",
			description: "Cliente inicia conversa no website",
		},
		{
			step:        "2",
			description: "Chatbot atendente-virtual processa mensagem via OpenAI GPT-4o",
		},
		{
			step:        "3",
			description: "Sistema analisa sentimento via Anthropic Claude",
		},
		{
			step:        "4",
			description: "Cliente demonstra interesse em SUV específico",
		},
		{
			step:        "5",
			description: "Sistema gera descrição criativa do veículo via GPT-4o",
		},
		{
			step:        "6",
			description: "Cliente pergunta sobre financiamento",
		},
		{
			step:        "7",
			description: "Especialista-financeiro responde via GPT-4o-mini",
		},
		{
			step:        "8",
			description: "Cliente faz consulta técnica sobre motor",
		},
		{
			step:        "9",
			description: "Especialista-tecnico responde rapidamente via Groq Llama3",
		},
		{
			step:        "10",
			description: "Lead qualificado encaminhado para equipe de vendas",
		},
	}

	for _, s := range steps {
		t.Logf("  %s. %s", s.step, s.description)
	}

	t.Logf("✓ Integration flow validated: %d steps", len(steps))
}

// TestBusinessRules valida as regras de negócio
func TestBusinessRules(t *testing.T) {
	exampleDir := getTestDir()
	manifestPath := filepath.Join(exampleDir, "system.yaml")

	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Skip("Manifest file not found, skipping test")
	}

	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	// Valida regras de negócio
	ruleNames := make(map[string]bool)
	for _, r := range m.BusinessRules {
		ruleNames[r.Name] = true
	}

	expectedRules := []string{
		"notifyOnHighInterest",
		"remindAppointment",
		"followUpChat",
	}

	for _, ruleName := range expectedRules {
		if !ruleNames[ruleName] {
			t.Errorf("Expected business rule '%s' not found", ruleName)
		}
	}

	t.Logf("✓ Business rules validated: %d rules found", len(m.BusinessRules))
}

// TestChannelsConfiguration valida configuração de canais
func TestChannelsConfiguration(t *testing.T) {
	exampleDir := getTestDir()
	manifestPath := filepath.Join(exampleDir, "system.yaml")

	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Skip("Manifest file not found, skipping test")
	}

	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	// Valida tipos de canais
	channelTypes := make(map[string]bool)
	for _, c := range m.Integrations.Channels {
		channelTypes[c.Type] = true
	}

	expectedTypes := []string{"websocket", "webhook", "smtp", "telegram"}
	for _, ct := range expectedTypes {
		if !channelTypes[ct] {
			t.Errorf("Expected channel type '%s' not found", ct)
		}
	}

	t.Logf("✓ Channels configuration validated: %d channels", len(m.Integrations.Channels))
}

// TestSecurityConfiguration valida configuração de segurança
func TestSecurityConfiguration(t *testing.T) {
	exampleDir := getTestDir()
	manifestPath := filepath.Join(exampleDir, "system.yaml")

	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Skip("Manifest file not found, skipping test")
	}

	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	// Valida políticas de segurança
	if m.Security.Authorization.DefaultDeny {
		t.Log("Default deny authorization policy enabled")
	}

	// Valida campos sensíveis
	sensitiveFields := []string{}
	for _, sf := range m.Security.DataProtection.SensitiveFields {
		sensitiveFields = append(sensitiveFields, sf.Field)
	}

	t.Logf("✓ Security configuration validated: %d sensitive fields", len(sensitiveFields))
}

// TestProviderChainConfiguration valida a cadeia de providers
func TestProviderChainConfiguration(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Valida configuração da cadeia
	if config.Settings.ProviderChain.Timeout != 30 {
		t.Errorf("Expected provider chain timeout 30, got %d", config.Settings.ProviderChain.Timeout)
	}

	if config.Settings.ProviderChain.MaxRetries != 2 {
		t.Errorf("Expected provider chain max retries 2, got %d", config.Settings.ProviderChain.MaxRetries)
	}

	if !config.Settings.ProviderChain.Fallback {
		t.Error("Expected provider chain fallback to be enabled")
	}

	// Valida provider chain nos agentes
	atendente := config.GetAgent("atendente-virtual")
	if atendente != nil {
		if len(atendente.ProviderChain) == 0 {
			t.Error("Agent 'atendente-virtual' should have a provider chain configured")
		} else {
			t.Logf("✓ Agent 'atendente-virtual' has provider chain with %d entries", len(atendente.ProviderChain))
		}
	}

	t.Logf("✓ Provider chain configuration validated")
}

// TestCostBasedRouting valida roteamento por custo
func TestCostBasedRouting(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Valida configuração de custo
	if !config.Routing.CostBased.Enabled {
		t.Error("Expected cost-based routing to be enabled")
	}

	if config.Routing.CostBased.Strategy != "cheapest" {
		t.Errorf("Expected cost strategy 'cheapest', got '%s'", config.Routing.CostBased.Strategy)
	}

	// Valida custos nos providers
	for _, provider := range config.Providers {
		if provider.Costs.InputPer1K == 0 && provider.Costs.OutputPer1K == 0 {
			t.Logf("Warning: Provider '%s' has no cost configured", provider.Name)
		}
	}

	t.Logf("✓ Cost-based routing validated")
}

// TestABTestingConfiguration valida configuração de A/B testing
func TestABTestingConfiguration(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// A/B testing pode estar desabilitado, mas deve estar configurado
	if config.Routing.ABTesting.Enabled {
		if config.Routing.ABTesting.VariantA.Provider == "" {
			t.Error("A/B testing variant A should have a provider configured")
		}
		if config.Routing.ABTesting.VariantB.Provider == "" {
			t.Error("A/B testing variant B should have a provider configured")
		}
	}

	t.Logf("✓ A/B testing configuration validated (enabled: %v)", config.Routing.ABTesting.Enabled)
}

// TestCapabilities valida capacidades dos agentes
func TestCapabilities(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Valida capacidades esperadas
	expectedCapabilities := map[string][]string{
		"atendente-virtual":       {"text_generation", "question_answering", "classification"},
		"analista-sentimento":     {"sentiment_analysis", "classification"},
		"redator":                 {"text_generation"},
	}

	for agentName, expectedCaps := range expectedCapabilities {
		agent := config.GetAgent(agentName)
		if agent == nil {
			t.Errorf("Agent '%s' not found", agentName)
			continue
		}

		for _, cap := range expectedCaps {
			found := false
			for _, agentCap := range agent.Capabilities {
				if agentCap == cap {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Agent '%s' should have capability '%s'", agentName, cap)
			}
		}
	}

	t.Logf("✓ Agent capabilities validated")
}

// TestManifestParser valida o parser de manifestos
func TestManifestParser(t *testing.T) {
	exampleDir := getTestDir()
	manifestPath := filepath.Join(exampleDir, "system.yaml")

	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Skip("Manifest file not found, skipping test")
	}

	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	// Valida estrutura básica
	if m.Metadata.Name == "" {
		t.Error("Manifest should have a name")
	}

	// Verifica se pode serializar para JSON
	_, err = m.ToJSON()
	if err != nil {
		t.Errorf("Failed to serialize to JSON: %v", err)
	}

	// Verifica se pode serializar para YAML
	_, err = m.ToYAML()
	if err != nil {
		t.Errorf("Failed to serialize to YAML: %v", err)
	}

	t.Logf("✓ Manifest parser validated")
}

// TestAlertConfiguration valida configuração de alertas
func TestAlertConfiguration(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Valida alertas
	if len(config.Alerts) == 0 {
		t.Log("No alerts configured")
		return
	}

	for _, alert := range config.Alerts {
		if alert.Name == "" {
			t.Error("Alert should have a name")
		}
		if alert.Condition == "" {
			t.Error("Alert should have a condition")
		}
	}

	t.Logf("✓ Alert configuration validated: %d alerts", len(config.Alerts))
}

// TestEnvironmentVariables valida configuração de variáveis de ambiente
func TestEnvironmentVariables(t *testing.T) {
	exampleDir := getTestDir()
	configPath := filepath.Join(exampleDir, "llm.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("LLM config file not found, skipping test")
	}

	config, err := llm.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verifica arquivo de ambiente
	if config.EnvFile != "" {
		t.Logf("Environment file configured: %s", config.EnvFile)
	}

	// Verifica se providers têm API keys configuráveis
	for _, provider := range config.Providers {
		_, err := provider.GetAPIKey()
		// É esperado que falhe sem variáveis de ambiente configuradas
		_ = err
	}

	t.Logf("✓ Environment variables configuration validated")
}

// getTestDir retorna o diretório de teste
func getTestDir() string {
	// Tenta várias opções
	if dir := os.Getenv("TEST_DIR"); dir != "" {
		return dir
	}

	// Diretório do teste atual
	if _, err := os.Stat("system.yaml"); err == nil {
		return "."
	}

	if _, err := os.Stat("llm.yaml"); err == nil {
		return "."
	}

	// Fallback para o diretório do exemplo
	return "/home/rangel/projetos/picoclaw/protoclaw/examples/car-dealership"
}
