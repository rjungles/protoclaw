# LLM Integration Documentation

PicoClaw AgentOS provides comprehensive LLM (Large Language Model) integration with support for multiple providers, intelligent routing, and hot-reload configuration.

## Table of Contents

- [Overview](#overview)
- [Supported Providers](#supported-providers)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [Usage in Go Code](#usage-in-go-code)
- [Routing Rules](#routing-rules)
- [Agent Integration](#agent-integration)
- [Environment Variables](#environment-variables)
- [Troubleshooting](#troubleshooting)

## Overview

The LLM system in AgentOS provides:

- **Multi-Provider Support**: OpenAI, Anthropic, Google, Stepfun, NVIDIA, and OpenAI-Compatible APIs
- **Provider Chains**: Automatic fallback between providers
- **Intelligent Routing**: Route requests based on function, intent, cost, or A/B testing
- **Hot-Reload**: Configuration changes applied without restart
- **Agent Integration**: LLM-powered agents with memory and capabilities
- **Per-System Configuration**: Each system has its own LLM configuration

## Supported Providers

### Native Providers

| Provider | Type | Description |
|----------|------|-------------|
| OpenAI | `openai` | GPT-4, GPT-4o, GPT-4o-mini, etc. |
| Anthropic | `anthropic` | Claude 3 Opus, Sonnet, Haiku |
| Google | `google` | Gemini models via Vertex AI |
| Stepfun | `stepfun` | Stepfun API |
| NVIDIA | `nvidia` | NVIDIA NIM models |

### OpenAI-Compatible Providers

Use type `compatible` for these providers:

| Provider | Base URL |
|----------|----------|
| Groq | `https://api.groq.com/openai/v1` |
| Together AI | `https://api.together.xyz/v1` |
| Fireworks | `https://api.fireworks.ai/inference/v1` |
| OpenRouter | `https://openrouter.ai/api/v1` |
| LM Studio | `http://localhost:1234/v1` |
| Ollama | `http://localhost:11434/v1` |

## Configuration

Configuration is stored in `{data_dir}/{system}/config/llm.yaml`.

### Basic Configuration

```yaml
version: "1.0"
system: "my-system"

settings:
  hot_reload: true
  reload_interval: 5
  provider_chain:
    timeout: 30
    max_retries: 2
    fallback: true
  default_routing:
    provider: "openai"
    model: "gpt-4o-mini"
    timeout: 30
    max_tokens: 4096
    temperature: 0.7

providers:
  - name: "openai"
    type: "openai"
    enabled: true
    priority: 1
    models:
      - id: "gpt-4o"
        name: "GPT-4o"
        max_tokens: 128000
      - id: "gpt-4o-mini"
        name: "GPT-4o Mini"
        max_tokens: 128000
    config:
      base_url: "https://api.openai.com/v1"
    costs:
      input_per_1k: 0.005
      output_per_1k: 0.015

  - name: "groq"
    type: "compatible"
    enabled: true
    priority: 2
    models:
      - id: "mixtral-8x7b-32768"
        name: "Mixtral 8x7B"
        max_tokens: 32768
    config:
      base_url: "https://api.groq.com/openai/v1"

agents:
  chat-agent:
    provider: "openai"
    model: "gpt-4o-mini"
    temperature: 0.7
    max_tokens: 2000
    system_prompt: "You are a helpful assistant."
    capabilities:
      - text_generation
      - code

routing:
  functions:
    code-review:
      provider: "anthropic"
      model: "claude-3-sonnet-20240229"
      temperature: 0.3
    summarize:
      provider: "groq"
      model: "mixtral-8x7b-32768"
  intents:
    coding:
      provider: "openai"
      model: "gpt-4o"
  cost_based:
    enabled: true
  ab_testing:
    enabled: false

defaults:
  temperature: 0.7
  max_tokens: 2000
  timeout: 30s
  retry:
    max_attempts: 3
    backoff: exponential

env_file: ".env"
```

### Configuration Fields

#### Settings

| Field | Description | Default |
|-------|-------------|---------|
| `hot_reload` | Enable configuration hot-reload | `true` |
| `reload_interval` | Seconds between config checks | `5` |
| `provider_chain.timeout` | Provider timeout in seconds | `30` |
| `provider_chain.max_retries` | Max retries per provider | `2` |
| `provider_chain.fallback` | Enable provider fallback | `true` |

#### Providers

| Field | Description | Required |
|-------|-------------|----------|
| `name` | Provider identifier | Yes |
| `type` | Provider type (`openai`, `anthropic`, `google`, `stepfun`, `nvidia`, `compatible`) | Yes |
| `enabled` | Whether provider is active | Yes |
| `priority` | Priority in fallback chain | No |
| `models` | List of available models | Yes |
| `config.base_url` | API base URL | For `compatible` |
| `costs` | Pricing per 1K tokens | No |

#### Routing Rules

| Rule Type | Description |
|-----------|-------------|
| `functions` | Route by business function name |
| `intents` | Route by intent classification |
| `cost_based` | Use cheapest available provider |
| `ab_testing` | Split traffic between variants |

## CLI Commands

### Initialize LLM Configuration

```bash
# Create default config for the default system
agentos llm init

# Create config for a specific system
agentos llm init --system my-system
```

### Validate Configuration

```bash
# Validate default system
agentos llm validate

# Validate specific system
agentos llm validate --system my-system
```

### Check Status

```bash
# Show status for all systems
agentos llm status

# Show status for specific system
agentos llm status --system my-system
```

### Manage Providers

```bash
# Add a new provider
agentos llm provider add my-provider \
  --type compatible \
  --url https://api.example.com/v1 \
  --model gpt-4
```

## Usage in Go Code

### Basic Usage

```go
package main

import (
    "context"
    "log"
    
    "github.com/sipeed/picoclaw/pkg/agentos/llm"
)

func main() {
    // Create service
    service, err := llm.NewService("/path/to/llm.yaml")
    if err != nil {
        log.Fatal(err)
    }
    defer service.Shutdown()
    
    // Execute a function
    resp, err := service.ExecuteFunction(context.Background(), llm.FunctionRequest{
        Function: "summarize",
        Input:    "Long text to summarize...",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    log.Println("Output:", resp.Output)
    log.Println("Model:", resp.Model)
    log.Println("Tokens:", resp.Usage.TotalTokens)
}
```

### Using Agents

```go
// Create an agent
agent, err := llm.NewLLMAgent("chat-agent", service)
if err != nil {
    log.Fatal(err)
}

// Chat with the agent
response, err := agent.Chat(context.Background(), "Hello!")
if err != nil {
    log.Fatal(err)
}

// Chat with context
response, err = agent.ChatWithContext(ctx, "Explain this", map[string]interface{}{
    "style": "technical",
    "format": "markdown",
})

// Use specialized methods
summary, _ := agent.Summarize(ctx, longText, 100)
classification, _ := agent.Classify(ctx, text, []string{"urgent", "normal", "low"})
translation, _ := agent.Translate(ctx, text, "Spanish")
analysis, _ := agent.Analyze(ctx, content, map[string]interface{}{
    "sentiment": "string",
    "topics":    []string{},
})
```

### Registering Custom Function Handlers

```go
service.RegisterFunctionFunc("my-function", func(ctx context.Context, req llm.FunctionRequest) (*llm.FunctionResponse, error) {
    // Custom implementation
    return &llm.FunctionResponse{
        Output:   "Custom result",
        Function: req.Function,
        Model:    "custom",
    }, nil
})
```

### Registering Custom Agent Handlers

```go
service.RegisterAgentFunc("my-agent", func(ctx context.Context, input string, context map[string]interface{}) (*llm.AgentResponse, error) {
    // Custom implementation
    return &llm.AgentResponse{
        Response: "Custom response",
        Agent:    "my-agent",
    }, nil
})
```

## Routing Rules

### Function-Based Routing

Route requests to specific providers based on business function:

```yaml
routing:
  functions:
    code-review:
      provider: "anthropic"
      model: "claude-3-opus-20240229"
      temperature: 0.3
    summarize:
      provider: "groq"
      model: "mixtral-8x7b-32768"
```

Usage in code:

```go
resp, _ := service.ExecuteFunction(ctx, llm.FunctionRequest{
    Function: "code-review",
    Input:    code,
})
```

### Intent-Based Routing

Route based on intent classification:

```yaml
routing:
  intents:
    coding:
      provider: "openai"
      model: "gpt-4o"
    support:
      provider: "anthropic"
      model: "claude-3-sonnet-20240229"
```

Usage in code:

```go
resp, _ := service.GetManager().Complete(ctx, llm.CompletionRequest{
    Intent:  "coding",
    Messages: []llm.Message{{Role: "user", Content: prompt}},
})
```

### Cost-Based Routing

Automatically use the cheapest available provider:

```yaml
routing:
  cost_based:
    enabled: true
    strategy: "cheapest"  # or "best_value"
```

### A/B Testing

Split traffic between models for testing:

```yaml
routing:
  ab_testing:
    enabled: true
    current_variant: "a"  # Change to "b" to switch
    variant_a:
      provider: "openai"
      model: "gpt-4o-mini"
    variant_b:
      provider: "anthropic"
      model: "claude-3-haiku-20240307"
```

## Agent Integration

### Agent Configuration

```yaml
agents:
  support-agent:
    provider: "openai"
    model: "gpt-4o-mini"
    temperature: 0.7
    max_tokens: 2000
    system_prompt: |
      You are a helpful customer support agent.
      Be polite, concise, and professional.
    capabilities:
      - chat
      - classification
      - sentiment_analysis
    provider_chain:
      - provider: "openai"
        model: "gpt-4o-mini"
        timeout: 30
      - provider: "groq"
        model: "mixtral-8x7b-32768"
        timeout: 20
```

### Agent Capabilities

The following capabilities are supported:

- `chat` - General conversation
- `text_generation` - Generate text content
- `code` - Code-related tasks
- `analysis` - Data analysis
- `classification` - Classify content
- `summarization` - Summarize text
- `translation` - Translate between languages
- `question_answering` - Answer questions
- `sentiment_analysis` - Analyze sentiment

### Agent Memory

Agents maintain conversation memory:

```go
// Clear memory
agent.ClearMemory()

// Set max memory size (in message pairs)
agent.SetMaxMemory(20)

// Get current memory
memory := agent.GetMemory()
for _, msg := range memory {
    fmt.Printf("%s: %s\n", msg.Role, msg.Content)
}
```

## Environment Variables

Create a `.env` file in `{data_dir}/{system}/config/`:

```bash
# OpenAI
OPENAI_API_KEY=sk-...

# Anthropic
ANTHROPIC_API_KEY=sk-ant-...

# Google
GOOGLE_API_KEY=...

# Groq
GROQ_API_KEY=gsk_...

# Together AI
TOGETHER_API_KEY=...

# Fireworks
FIREWORKS_API_KEY=...

# OpenRouter
OPENROUTER_API_KEY=...

# NVIDIA
NVIDIA_API_KEY=nvapi-...

# Stepfun
STEPFUN_API_KEY=...
```

Or use provider-specific env vars:

```bash
MY_PROVIDER_API_KEY=...
```

## Troubleshooting

### Configuration Not Found

```
Error: configuração LLM não encontrada
```

**Solution**: Run `agentos llm init` to create the configuration.

### Provider Not Configured

```
Error: API key not found for provider openai
```

**Solution**: 
1. Check `.env` file exists
2. Verify API key is set: `OPENAI_API_KEY=sk-...`
3. Restart the service after updating env vars

### Invalid Configuration

```
Error: config validation failed
```

**Solution**: Run `agentos llm validate` to see detailed errors.

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| Hot-reload not working | File watcher limit | Increase `fs.inotify.max_user_watches` |
| Provider timeout | Network issues | Increase `settings.provider_chain.timeout` |
| No providers available | All disabled | Enable at least one provider in config |
| Rate limit exceeded | Too many requests | Add fallback providers or increase timeouts |

### Debug Mode

Enable debug logging:

```go
// Set environment variable
os.Setenv("LLM_DEBUG", "1")
```

### Health Check

```bash
# Check all systems
agentos llm status

# Check specific system
agentos llm status --system my-system

# Test provider connectivity
curl -H "Authorization: Bearer $OPENAI_API_KEY" \
  https://api.openai.com/v1/models
```

## Advanced Topics

### Custom Provider Implementation

```go
type MyProvider struct {
    *llm.BaseProvider
    client *http.Client
}

func (p *MyProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
    // Implementation
}

func (p *MyProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamResponse, error) {
    // Implementation
}

func (p *MyProvider) Embed(ctx context.Context, req llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
    // Implementation
}

// Register
func RegisterMyProvider(factory *llm.ProviderFactory) {
    factory.Register("my-provider", func(name string, config *llm.ProviderConfig) (llm.Provider, error) {
        return NewMyProvider(name, config)
    })
}
```

### Embedding Support

```go
resp, err := provider.Embed(ctx, llm.EmbeddingRequest{
    Model: "text-embedding-ada-002",
    Input: []string{"Hello world", "Goodbye world"},
})

for i, embedding := range resp.Embeddings {
    fmt.Printf("Embedding %d: %v...\n", i, embedding[:5])
}
```

### Streaming Responses

```go
stream, err := provider.Stream(ctx, llm.CompletionRequest{
    Model: "gpt-4",
    Messages: []llm.Message{{Role: "user", Content: prompt}},
})
if err != nil {
    log.Fatal(err)
}

for chunk := range stream {
    if chunk.Done {
        break
    }
    fmt.Print(chunk.Content)
}
```

## API Reference

See GoDoc for complete API reference:

- `github.com/sipeed/picoclaw/pkg/agentos/llm` - Core types and manager
- `github.com/sipeed/picoclaw/pkg/agentos/llm/providers` - Provider implementations

## License

Same as PicoClaw project.
