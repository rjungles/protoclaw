# Atualização do Comando `serve` - Modo Multi-Sistema

## Problema
O comando `serve` original não permitia a execução simultânea de múltiplos sistemas AgentOS. Quando existiam vários sistemas registrados, o usuário precisava usar `multi-serve` explicitamente.

## Solução
O comando `serve` foi atualizado para detectar automaticamente quando há múltiplos sistemas e iniciar o servidor multi-sistema nesses casos.

## Mudanças Realizadas

### 1. Flag `--single` Adicionada
```bash
picoclaw agentos serve --single --system cafeteria
```nForça o modo de sistema único mesmo quando existem múltiplos sistemas.

### 2. Comportamento Inteligente Padrão

O comando `serve` agora funciona assim:

| Cenário | Comportamento |
|---------|---------------|
| Um sistema registrado | Inicia servidor single normalmente |
| Múltiplos sistemas + sem `--system` | **Inicia multi-serve automaticamente** |
| Múltiplos sistemas + `--system <name>` | Inicia apenas o sistema especificado |
| Múltiplos sistemas + `--single` | Força modo single, usa sistema padrão |

### 3. Exemplos de Uso

```bash
# Inicia todos os sistemas (quando múltiplos existem)
picoclaw agentos serve

# Serve um sistema específico
picoclaw agentos serve --system cafeteria

# Força modo single mesmo com múltiplos sistemas
picoclaw agentos serve --single --system cafeteria

# Especifica host e porta
picoclaw agentos serve --host 0.0.0.0 --port 3000
```

## Implementação

A função `runMultiServe()` foi extraída do `multi-serve` para permitir reutilização pelo comando `serve`.

Quando `serve` detecta múltiplos sistemas sem um sistema específico solicitado:
1. Exibe mensagem informativa: "Detectado N sistemas. Iniciando em modo multi-sistema..."
2. Chama `runMultiServe()` internamente
3. Todos os sistemas são disponibilizados em URLs prefixadas (ex: `/cafeteria/api/v1/...`)

## Benefícios

1. **UX Simplificada**: Usuários não precisam saber a diferença entre `serve` e `multi-serve`
2. **Descoberta Automática**: O sistema detecta a melhor opção automaticamente
3. **Flexibilidade**: Ainda é possível forçar comportamentos específicos com flags
4. **Compatibilidade**: Comportamento anterior preservado quando explicitamente solicitado
