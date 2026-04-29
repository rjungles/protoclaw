# AgentOS Tools Integration

This document describes the integration between PicoClaw and AgentOS through conversational tools.

## Overview

The AgentOS tools allow the PicoClaw agent to create, manage, and interact with AgentOS systems through natural language conversations. Users can chat with the agent via any configured channel (Telegram, WhatsApp, CLI, etc.) to:

1. Generate system manifests from descriptions
2. Initialize and bootstrap systems
3. Query entity data
4. Manage multiple systems

## Available Tools

### 1. agentos_generate_manifest

Generates an AgentOS system manifest from a natural language description.

**Use when:**
- User says "Create a system for..."
- User describes a business domain (e.g., "car dealership", "e-commerce")
- Need to create a new system manifest

**Example conversation:**
```
User: Create a system for my restaurant
Agent: I'll generate a manifest for your restaurant system...
[Tool generates manifest with Menu, Order, Reservation entities]
Agent: Manifest generated! It includes Menu, Order, and Reservation entities.
```

**Parameters:**
- `description` (required): Natural language description
- `system_name` (required): Name for the system
- `output_path` (optional): Where to save the manifest

### 2. agentos

Executes AgentOS operations (init, bootstrap, serve, status, validate).

**Use when:**
- User wants to initialize a system
- User wants to check system status
- User wants to start/stop system emulation

**Actions:**
- `init`: Initialize system from manifest
- `bootstrap`: Create database schema
- `serve`: Start system emulation
- `status`: List all systems
- `validate`: Validate system health

**Example conversation:**
```
User: Initialize the restaurant system
Agent: [Executes init action]
Agent: System initialized! Database and config created.

User: Bootstrap it
Agent: [Executes bootstrap]
Agent: System bootstrapped! Ready to serve.

User: What's the status?
Agent: [Executes status]
Agent: restaurant (bootstrapped)
```

**Parameters:**
- `action` (required): One of init, bootstrap, serve, status, validate
- `system_name`: Required for most actions
- `manifest_path`: Required for init

### 3. agentos_query

Queries entity data from a running system.

**Use when:**
- User asks "What customers do we have?"
- User wants to see system data
- Need to retrieve entity records

**Example conversation:**
```
User: Show me the vehicles
Agent: [Executes query on Vehicle entity]
Agent: Found 3 vehicles:
  - Vehicle_001: Vehicle 1
  - Vehicle_002: Vehicle 2
  - Vehicle_003: Vehicle 3
```

**Parameters:**
- `system_name` (required): System to query
- `entity` (required): Entity name (e.g., Customer, Vehicle)
- `limit` (optional): Max results (default 10)

## Configuration

### Enable AgentOS Tools

Add to your PicoClaw configuration (config.yaml):

```yaml
tools:
  agentos:
    enabled: true
```

Or via environment variable:
```bash
export PICOCLAW_TOOLS_AGENTOS_ENABLED=true
```

### Set Data Directory

By default, AgentOS systems are stored in `~/.picoclaw/agentos/`. To change:

```bash
export AGENTOS_DATA_DIR=/path/to/agentos/data
```

## Complete Workflow Example

### Via CLI
```bash
# Start interactive mode
picoclaw agent

# Conversation:
You: Create a car dealership system
Agent: Manifest generated with Customer, Vehicle, and Sale entities.

You: Initialize it
Agent: System initialized at ~/.picoclaw/agentos/dealership

You: Bootstrap the system
Agent: Database created. System is ready.

You: Show me the vehicles
Agent: Found sample data:
  - Vehicle_001: Sample Vehicle 1
  - Vehicle_002: Sample Vehicle 2

You: Start the system
Agent: System is now serving at http://localhost:8080/dealership
```

### Via Telegram/WhatsApp
Same conversation flow works through messaging channels. The agent interprets natural language and invokes appropriate tools.

## Tool Implementation Details

### File Structure
```
pkg/tools/
├── agentos.go              # Tool implementations
├── agentos_test.go         # Unit tests
└── agentos_integration_test.go  # Integration tests
```

### Tools Registered

The following tools are automatically registered when AgentOS is enabled:

1. **ExecAgentOSTool** (`agentos`)
   - Wraps AgentOS CLI operations
   - Handles init, bootstrap, serve, status, validate

2. **AgentOSGenerateManifestTool** (`agentos_generate_manifest`)
   - Generates manifests from descriptions
   - Extracts entities from keywords

3. **AgentOSQueryTool** (`agentos_query`)
   - Queries entity data
   - Returns sample data (production: queries database)

### Entity Extraction

The `generate_manifest` tool extracts entities from descriptions using keyword matching:

**Keywords mapped to entities:**
- "customer", "client" → Customer
- "vehicle", "car" → Vehicle
- "order", "sale" → Order
- "product" → Product
- "appointment", "booking" → Appointment
- "employee", "staff" → Employee
- "menu", "reservation", "table" (restaurant domain)

### Registry Storage

Systems are tracked in a simple YAML registry at `~/.picoclaw/agentos/registry.yaml`:

```yaml
dealership:
  manifest: /home/user/.picoclaw/agentos/dealership/system.yaml
  status: bootstrapped
  created: 2026-04-29T10:00:00Z
  updated: 2026-04-29T10:05:00Z

restaurant:
  manifest: /home/user/.picoclaw/agentos/restaurant/system.yaml
  status: initialized
  created: 2026-04-29T11:00:00Z
```

## Testing

### Run Unit Tests
```bash
cd /home/rangel/projetos/picoclaw/protoclaw
go test ./pkg/tools -v -run AgentOS
```

### Run Integration Tests
```bash
go test ./pkg/tools -v -run "TestAgentOSWorkflow|TestAgentOSConversationFlow"
```

### Test Coverage
- Tool initialization
- Parameter validation
- Error handling
- Complete workflows
- Multiple system management

## Extending

### Adding New Actions

To add a new action to the `agentos` tool:

1. Add case in `Execute` switch statement
2. Implement handler function
3. Add test case

Example:
```go
func (t *ExecAgentOSTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
    switch action {
    case "new_action":
        return t.executeNewAction(args, dataDir)
    // ...
    }
}

func (t *ExecAgentOSTool) executeNewAction(args map[string]any, dataDir string) *ToolResult {
    // Implementation
    return UserResult("Success!")
}
```

### Adding New Keywords

To add entity extraction keywords:

```go
func (t *AgentOSGenerateManifestTool) extractEntities(description string) []string {
    keywords := map[string]string{
        "new_keyword": "NewEntity",
        // ...
    }
}
```

## Troubleshooting

### System Not Found
- Check `AGENTOS_DATA_DIR` environment variable
- Verify system was initialized
- Run `agentos status` to list systems

### Manifest Generation Failed
- Ensure description is clear
- Check for write permissions
- Verify output path is valid

### Query Returns No Data
- System must be bootstrapped
- Check entity name matches manifest
- Verify database exists

## Architecture

```
User (Telegram/WhatsApp/CLI)
    ↓
PicoClaw Agent
    ↓
Tool Registry
    ↓
AgentOS Tools (agentos.go)
    ↓
AgentOS Systems
    ↓
Database / Manifests
```

The agent uses the Tool Loop to:
1. Parse user intent
2. Select appropriate tool
3. Execute with parameters
4. Return results to user

## See Also

- `docs/LLM.md` - LLM configuration for systems
- `examples/car-dealership/` - Complete example
- `pkg/agentos/` - AgentOS core implementation
