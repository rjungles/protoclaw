package providers

import "github.com/sipeed/picoclaw/pkg/agentos/llm"

// Init registers all LLM providers with the factory
func Init(factory *llm.ProviderFactory) {
	// Native providers
	RegisterOpenAI(factory)
	RegisterAnthropic(factory)
	RegisterGoogle(factory)
	RegisterNVIDIA(factory)
	RegisterStepfun(factory)

	// OpenAI-Compatible providers (Groq, Together AI, Fireworks, OpenRouter, etc.)
	RegisterCompatible(factory)
}
