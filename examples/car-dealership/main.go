// AutoPrime Concessionária - Exemplo de aplicação com LLM
// Este exemplo demonstra como usar o sistema LLM em uma aplicação real

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/sipeed/picoclaw/pkg/agentos/llm"
	"github.com/sipeed/picoclaw/pkg/agentos/llm/providers"
)

// ChatSession gerencia uma sessão de chat
// com histórico de mensagens para o agente

func main() {
	// Carrega o diretório do exemplo
	exampleDir := getExampleDir()

	// Configuração do LLM
	configPath := filepath.Join(exampleDir, "llm.yaml")

	fmt.Println("=== AutoPrime Concessionária - Chatbot Demo ===")
	fmt.Println()

	// Cria o serviço LLM
	service, err := llm.NewService(configPath)
	if err != nil {
		log.Printf("Aviso: Usando modo de demonstração simulada: %v\n", err)
		fmt.Println()
	} else {
		defer service.Shutdown()
	}

	// Demonstra as capacidades do chatbot (simulado)
	demonstrateChatbot()

	// Demonstra análise de sentimento
	demonstrateSentimentAnalysis()

	// Demonstra geração de descrição
	demonstrateVehicleDescription()

	// Demonstra roteamento inteligente
	demonstrateIntelligentRouting()

		fmt.Println("=== Demonstração concluída ===")
	fmt.Println("Para executar com providers reais, configure as chaves API no arquivo .env")
}

// getExampleDir retorna o diretório deste exemplo
func getExampleDir() string {
	// Em produção, isso seria obtido de configuração
	// Aqui usamos o diretório atual
	if dir := os.Getenv("EXAMPLE_DIR"); dir != "" {
		return dir
	}
	// Tenta descobrir o diretório
	if _, err := os.Stat("llm.yaml"); err == nil {
		return "."
	}
	// Fallback
	return "/home/rangel/projetos/picoclaw/protoclaw/examples/car-dealership"
}

// createMockService cria um serviço simulado para demonstração
func createMockService() *llm.Service {
	// Cria um serviço com configuração mínima
	// Note: Em produção, use NewService com uma configuração válida
	service := &llm.Service{}
	// Inicializa o mapa de agentes para evitar panic
	// Isso é apenas para demonstração
	return service
}

// registerCustomAgents registra agentes especializados
func registerCustomAgents(service *llm.Service) {
	// Registra agente de qualificação de leads
	service.RegisterAgentFunc("qualificador-leads", func(ctx context.Context, input string, context map[string]interface{}) (*llm.AgentResponse, error) {
		// Simula análise de lead
		return &llm.AgentResponse{
			Response: "Lead qualificado! O cliente demonstrou interesse em SUVs e tem orçamento acima de R$ 100.000.",
			Agent:    "qualificador-leads",
			Actions: []llm.AgentAction{
				{
					Type:   "notify_sales",
					Target: "sales-team",
					Payload: map[string]interface{}{
						"priority":   "high",
						"vehicle":    "SUV",
						"budget_min": 100000,
					},
				},
			},
		}, nil
	})

	// Registra agente de negociação
	service.RegisterAgentFunc("negociador", func(ctx context.Context, input string, context map[string]interface{}) (*llm.AgentResponse, error) {
		return &llm.AgentResponse{
			Response: "Com base no histórico, posso oferecer 5% de desconto para fechamento nesta semana.",
			Agent:    "negociador",
			Actions: []llm.AgentAction{
				{
					Type:   "generate_proposal",
					Target: "proposal-system",
					Payload: map[string]interface{}{
						"discount": 0.05,
						"valid_until": "2024-01-20",
					},
				},
			},
		}, nil
	})

	fmt.Println("✓ Agentes personalizados registrados")
}

// demonstrateChatbot demonstra o chatbot de atendimento
func demonstrateChatbot() {
		fmt.Println("--- Chatbot de Atendimento ---")
	fmt.Println("Agente: Olá! Bem-vindo à AutoPrime Concessionária. Como posso ajudá-lo hoje?")
	fmt.Println()

	// Simula interações
	interactions := []struct {
		user      string
		assistant string
	}{
		{
			user:      "Olá, gostaria de ver carros disponíveis",
			assistant: "Olá! Temos diversos veículos disponíveis. Qual categoria te interessa mais? Temos hatches, sedans, SUVs, pickups e vans. Além disso, trabalhamos com veículos 0km e seminovos. Qual é o seu orçamento aproximado?",
		},
		{
			user:      "Estou procurando um SUV até 150 mil",
			assistant: "Perfeito! Temos excelentes opções de SUVs nessa faixa de preço. 😊\n\nTemos modelos como:\n- SUV compactos a partir de R$ 95.000\n- SUVs médios a partir de R$ 120.000\n- Opções 0km e seminovos com baixa quilometragem\n\nPosso agendar uma visita para você conhecer pessoalmente? Temos um showroom muito completo e você poderá fazer test-drive nos modelos de interesse.",
		},
		{
			user:      "Quais são as formas de pagamento?",
			assistant: "Oferecemos diversas opções de pagamento para facilitar sua aquisição:\n\n💰 À vista com desconto especial\n💳 Financiamento em até 60x\n🔄 Consórcio AutoPrime (taxa administrativa: 12%)\n🚗 Troca com avaliação justa do seu usado\n\nPara financiamento, trabalhamos com as principais instituições e taxas a partir de 1,29% ao mês. Você prefere fazer uma simulação personalizada agora ou gostaria de conhecer os veículos primeiro?",
		},
	}

	for _, interaction := range interactions {
		fmt.Printf("Cliente: %s\n\n", interaction.user)
		fmt.Printf("Atendente: %s\n\n", interaction.assistant)
	}
}

// demonstrateSentimentAnalysis demonstra análise de sentimento
func demonstrateSentimentAnalysis() {
		fmt.Println()
	fmt.Println("--- Análise de Sentimento ---")

	mensagens := []struct {
		text     string
		expected string
	}{
		{
			text:     "Ótimo atendimento! Estou muito satisfeito com a concessionária.",
			expected: "positive",
		},
		{
			text:     "Gostaria de informações sobre o Honda Civic 2023",
			expected: "neutral",
		},
		{
			text:     "Estou muito insatisfeito com o atraso na entrega! Péssimo serviço!",
			expected: "negative",
		},
	}

	for _, m := range mensagens {
		fmt.Printf("Mensagem: \"%s\"\n", m.text)
		fmt.Printf("Sentimento esperado: %s\n", m.expected)
		
		// Simula análise
		fmt.Printf("Resultado: Análise detecta sentimento %s com 85%% de confiança\n", m.expected)
		fmt.Printf("  - Requer atenção humana: %v\n", m.expected == "negative")
		fmt.Printf("  - Requer follow-up: %v\n", m.expected == "positive")
		fmt.Println()
	}
}

// demonstrateVehicleDescription demonstra geração de descrições
func demonstrateVehicleDescription() {
		fmt.Println()
	fmt.Println("--- Geração de Descrição de Veículo ---")

	veiculos := []map[string]interface{}{
		{
			"brand": "Toyota",
			"model": "Corolla",
			"year":  2023,
			"specs": map[string]string{
				"engine":      "2.0 Flex",
				"power":       "177 cv",
				"transmission": "CVT",
				"features":    "Teto solar panorâmico, Bancos em couro, Central multimídia",
			},
		},
		{
			"brand": "Honda",
			"model": "CR-V",
			"year":  2024,
			"specs": map[string]string{
				"engine":      "1.5 Turbo",
				"power":       "190 cv",
				"transmission": "CVT",
				"features":    "Teto solar, Sistema Honda Sensing, Couro, Rodas 19",
			},
		},
	}

	for _, v := range veiculos {
		brand := v["brand"].(string)
		model := v["model"].(string)
		year := v["year"].(int)
	
		fmt.Printf("Veículo: %s %s %d\n", brand, model, year)
		fmt.Println("Descrição gerada:")
		fmt.Println()
		
		// Simula descrição gerada
		descricoes := map[string]string{
			"Toyota Corolla": `🚗 **Toyota Corolla 2023 - Excelência em Cada Detalhe**

Descubra o sedã mais vendido do Brasil! Com motor 2.0 Flex de 177 cv e transmissão CVT de última geração, o Corolla oferece performance e economia excepcionais.

✨ **Diferenciais:**
- Teto solar panorâmico para momentos inesquecíveis
- Interior premium em couro
- Central multimídia com Apple CarPlay/Android Auto

**Por que escolher este Corolla?**
✅ Procedência garantida AutoPrime
✅ Revisões incluídas por 2 anos
✅ Garantia estendida disponível

👉 Agende seu test-drive e sinta a diferença!`,
			"Honda CR-V": `🌟 **Honda CR-V 2024 - O SUV que Você Merece**

Eleve sua experiência de direção com a nova geração CR-V! Motor 1.5 Turbo de 190 cv combinado com a confiabilidade Honda que você conhece.

🎯 **Destaques:**
- Tecnologia Honda Sensing completa
- Acabamento premium em couro
- Rodas aro 19 esportivas
- Teto solar para aventuras

**Benefícios Exclusivos AutoPrime:**
🛡️ Seguro com 20% de desconto
🔧 Revisões programadas incluídas
📱 Suporte 24h via app

⚡ Unidades limitadas! Não perca essa oportunidade.`,
		}
		
		key := fmt.Sprintf("%s %s", brand, model)
		if desc, ok := descricoes[key]; ok {
			fmt.Println(desc)
		}
		fmt.Println()
	}
}

// demonstrateIntelligentRouting demonstra o roteamento inteligente
func demonstrateIntelligentRouting() {
		fmt.Println()
	fmt.Println("--- Roteamento Inteligente ---")

	// Mostra como diferentes funções usam diferentes providers
	roteamentos := []struct {
		funcao    string
		provider  string
		modelo    string
		motivo    string
	}{
		{
			funcao:   "chatbotResponse",
			provider: "openai",
			modelo:   "gpt-4o",
			motivo:   "Atendimento de alta qualidade",
		},
		{
			funcao:   "analyzeCustomerSentiment",
			provider: "anthropic",
			modelo:   "claude-3-sonnet",
			motivo:   "Análise precisa de sentimento",
		},
		{
			funcao:   "technicalQuery",
			provider: "groq",
			modelo:   "llama3-70b",
			motivo:   "Resposta rápida para consultas técnicas",
		},
		{
			funcao:   "financingQuery",
			provider: "openai",
			modelo:   "gpt-4o-mini",
			motivo:   "Econômico para consultas financeiras",
		},
	}

	fmt.Println("Função → Provider → Modelo")
	fmt.Println("-" + string(make([]byte, 50)))
	for _, r := range roteamentos {
		fmt.Printf("%-25s → %-10s → %-20s (%s)\n", r.funcao, r.provider, r.modelo, r.motivo)
	}

	fmt.Println()
	fmt.Println("✓ Roteamento por custo: Habilitado")
	fmt.Println("✓ Provider chain: OpenAI → Groq → Anthropic")
	fmt.Println("✓ Fallback automático: Ativado")
	fmt.Println("✓ Hot-reload: 5 segundos")
}

// registerAllProviders registra todos os providers disponíveis
func registerAllProviders() {
	factory := llm.NewProviderFactory()
	providers.Init(factory)
	
	fmt.Println("Providers registrados:")
	fmt.Println("  - openai (OpenAI)")
	fmt.Println("  - anthropic (Anthropic)")
	fmt.Println("  - groq (Groq - OpenAI Compatible)")
	fmt.Println("  - google (Google Vertex AI)")
	fmt.Println("  - nvidia (NVIDIA NIM)")
	fmt.Println("  - stepfun (Stepfun)")
	fmt.Println("  - compatible (Generic OpenAI-Compatible)")
}

// init inicializa o sistema
func init() {
	// Registra providers
	registerAllProviders()
}
