package manifest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile é uma função auxiliar para testes
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func TestParseYAML(t *testing.T) {
	yamlData := `
metadata:
  name: "TestSystem"
  version: "1.0.0"
  description: "Sistema de teste"
  author: "Test Author"

actors:
  - id: "admin"
    name: "Administrador"
    description: "Usuário administrador do sistema"
    roles: ["admin", "user"]
    permissions:
      - resource: "users"
        actions: ["read", "write", "delete"]
      - resource: "orders"
        actions: ["read", "write"]

data_model:
  entities:
    - name: "User"
      description: "Usuário do sistema"
      fields:
        - name: "id"
          type: "string"
          required: true
          unique: true
        - name: "email"
          type: "string"
          required: true
          unique: true
        - name: "name"
          type: "string"
          required: true
        - name: "created_at"
          type: "datetime"
          required: true
      indexes:
        - name: "idx_email"
          fields: ["email"]
          unique: true

business_rules:
  - id: "BR001"
    name: "Validar Email Único"
    description: "Garante que emails sejam únicos"
    trigger:
      event: "create"
      entities: ["User"]
      before: true
    condition: "email not exists"
    actions:
      - type: "validate"
        target: "email"
    enabled: true
    priority: 1

security:
  authentication:
    methods: ["jwt", "api_key"]
    session_timeout_minutes: 60
    mfa_required: false
  authorization:
    model: "rbac"
    default_deny: true
  audit:
    enabled: true
    log_level: "info"
    retention_days: 90
`

	manifest, err := ParseYAML([]byte(yamlData))
	require.NoError(t, err)
	require.NotNil(t, manifest)

	assert.Equal(t, "TestSystem", manifest.Metadata.Name)
	assert.Equal(t, "1.0.0", manifest.Metadata.Version)
	assert.Len(t, manifest.Actors, 1)
	assert.Equal(t, "admin", manifest.Actors[0].ID)
	assert.Len(t, manifest.DataModel.Entities, 1)
	assert.Equal(t, "User", manifest.DataModel.Entities[0].Name)
	assert.Len(t, manifest.BusinessRules, 1)
	assert.Equal(t, "BR001", manifest.BusinessRules[0].ID)
}

func TestParseJSON(t *testing.T) {
	jsonData := `{
  "metadata": {
    "name": "JSONSystem",
    "version": "2.0.0",
    "description": "Sistema JSON"
  },
  "actors": [
    {
      "id": "user",
      "name": "Usuário Comum",
      "roles": ["user"],
      "permissions": [
        {
          "resource": "orders",
          "actions": ["read", "write"]
        }
      ]
    }
  ],
  "data_model": {
    "entities": [
      {
        "name": "Order",
        "fields": [
          {"name": "id", "type": "int", "required": true},
          {"name": "total", "type": "float", "required": true}
        ]
      }
    ]
  },
  "security": {
    "authentication": {
      "methods": ["jwt"],
      "session_timeout_minutes": 30
    },
    "authorization": {
      "model": "rbac",
      "default_deny": true
    },
    "audit": {
      "enabled": true,
      "log_level": "warn",
      "retention_days": 30
    }
  }
}`

	manifest, err := ParseJSON([]byte(jsonData))
	require.NoError(t, err)
	require.NotNil(t, manifest)

	assert.Equal(t, "JSONSystem", manifest.Metadata.Name)
	assert.Equal(t, "2.0.0", manifest.Metadata.Version)
	assert.Len(t, manifest.Actors, 1)
	assert.Equal(t, "user", manifest.Actors[0].ID)
}

func TestParseFile(t *testing.T) {
	// Cria arquivo YAML temporário para teste
	tmpDir := t.TempDir()
	yamlPath := tmpDir + "/test_manifest.yaml"
	
	yamlContent := `
metadata:
  name: "FileTest"
  version: "1.0.0"
actors:
  - id: "test"
    name: "Test Actor"
    roles: ["test"]
data_model:
  entities:
    - name: "TestEntity"
      fields:
        - name: "id"
          type: "string"
          required: true
business_rules:
  - id: "BR001"
    name: "Test Rule"
    trigger:
      event: "create"
      entities: ["TestEntity"]
    actions: []
    enabled: true
security:
  authentication:
    methods: ["jwt"]
    session_timeout_minutes: 60
  authorization:
    model: "rbac"
    default_deny: false
  audit:
    enabled: false
    log_level: "info"
    retention_days: 30
`
	err := writeFile(yamlPath, []byte(yamlContent))
	require.NoError(t, err)

	manifest, err := ParseFile(yamlPath)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	assert.Equal(t, "FileTest", manifest.Metadata.Name)
}

func TestValidate_Metadata(t *testing.T) {
	parser := &Parser{}
	
	// Teste com metadata inválido
	manifest := &Manifest{
		Metadata: Metadata{},
	}
	
	err := parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metadata.name is required")
	
	// Teste com metadata válido
	manifest.Metadata.Name = "ValidName"
	manifest.Metadata.Version = "1.0.0"
	
	err = parser.Validate(manifest)
	assert.NoError(t, err)
}

func TestValidate_Actors(t *testing.T) {
	parser := &Parser{}
	
	// Teste com ator sem ID
	manifest := &Manifest{
		Metadata: Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []Actor{
			{ID: "", Name: "Invalid"},
		},
		DataModel: DataModel{Entities: []Entity{
			{Name: "Test", Fields: []Field{{Name: "id", Type: "string"}}},
		}},
	}
	
	err := parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "actor.id is required")
	
	// Teste com IDs duplicados
	manifest.Actors = []Actor{
		{ID: "dup", Name: "First"},
		{ID: "dup", Name: "Second"},
	}
	
	err = parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate actor id: dup")
}

func TestValidate_DataModel(t *testing.T) {
	parser := &Parser{}
	
	// Teste com entidade sem nome
	manifest := &Manifest{
		Metadata: Metadata{Name: "Test", Version: "1.0.0"},
		DataModel: DataModel{
			Entities: []Entity{
				{Name: "", Fields: []Field{{Name: "id", Type: "string"}}},
			},
		},
	}
	
	err := parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "entity.name is required")
	
	// Teste com campos duplicados
	manifest.DataModel.Entities[0].Name = "TestEntity"
	manifest.DataModel.Entities[0].Fields = []Field{
		{Name: "id", Type: "string"},
		{Name: "id", Type: "int"},
	}
	
	err = parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate field id in entity TestEntity")
	
	// Teste com campo sem tipo
	manifest.DataModel.Entities[0].Fields = []Field{
		{Name: "id", Type: ""},
	}
	
	err = parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must have a type")
}

func TestValidate_BusinessRules(t *testing.T) {
	parser := &Parser{}
	
	// Teste com regra sem ID
	manifest := &Manifest{
		Metadata: Metadata{Name: "Test", Version: "1.0.0"},
		BusinessRules: []BusinessRule{
			{ID: "", Name: "Invalid"},
		},
	}
	
	err := parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "business rule id is required")
	
	// Teste com regra sem trigger entities
	manifest.BusinessRules[0].ID = "BR001"
	manifest.BusinessRules[0].Trigger = Trigger{
		Event: "create",
	}
	
	err = parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must have trigger entities")
	
	// Teste com regra sem evento
	manifest.BusinessRules[0].Trigger.Entities = []string{"User"}
	manifest.BusinessRules[0].Trigger.Event = ""
	
	err = parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must have a trigger event")
}

func TestValidate_Integrations(t *testing.T) {
	parser := &Parser{}
	
	// Teste com API sem nome
	manifest := &Manifest{
		Metadata: Metadata{Name: "Test", Version: "1.0.0"},
		Integrations: Integrations{
			APIs: []APIConfig{
				{Name: "", BasePath: "/api"},
			},
		},
	}
	
	err := parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API name is required")
	
	// Teste com API sem base_path
	manifest.Integrations.APIs[0].Name = "TestAPI"
	manifest.Integrations.APIs[0].BasePath = ""
	
	err = parser.Validate(manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "base_path is required")
}

func TestToYAML(t *testing.T) {
	manifest := &Manifest{
		Metadata: Metadata{
			Name:    "TestYAML",
			Version: "1.0.0",
		},
		Actors: []Actor{
			{ID: "admin", Name: "Admin", Roles: []string{"admin"}},
		},
		DataModel: DataModel{
			Entities: []Entity{
				{
					Name: "User",
					Fields: []Field{
						{Name: "id", Type: "string", Required: true},
					},
				},
			},
		},
		BusinessRules: []BusinessRule{
			{
				ID: "BR001",
				Name: "Test",
				Trigger: Trigger{Event: "create", Entities: []string{"User"}},
				Actions: []RuleAction{},
				Enabled: true,
			},
		},
		Security: SecurityPolicy{
			Authentication: AuthenticationPolicy{Methods: []string{"jwt"}, SessionTimeout: 60},
			Authorization: AuthorizationPolicy{Model: "rbac", DefaultDeny: true},
			Audit: AuditPolicy{Enabled: true, LogLevel: "info", RetentionDays: 30},
		},
	}
	
	data, err := manifest.ToYAML()
	require.NoError(t, err)
	require.NotEmpty(t, data)
	
	// Parseia novamente para verificar consistência
	parsed, err := ParseYAML(data)
	require.NoError(t, err)
	assert.Equal(t, manifest.Metadata.Name, parsed.Metadata.Name)
}

func TestToJSON(t *testing.T) {
	manifest := &Manifest{
		Metadata: Metadata{
			Name:    "TestJSON",
			Version: "2.0.0",
		},
		Actors: []Actor{
			{ID: "user", Name: "User", Roles: []string{"user"}},
		},
		DataModel: DataModel{
			Entities: []Entity{
				{
					Name: "Product",
					Fields: []Field{
						{Name: "id", Type: "int", Required: true},
						{Name: "price", Type: "float"},
					},
				},
			},
		},
		BusinessRules: []BusinessRule{
			{
				ID: "BR002",
				Name: "Validate Price",
				Trigger: Trigger{Event: "create", Entities: []string{"Product"}},
				Actions: []RuleAction{},
				Enabled: true,
			},
		},
		Security: SecurityPolicy{
			Authentication: AuthenticationPolicy{Methods: []string{"api_key"}, SessionTimeout: 30},
			Authorization: AuthorizationPolicy{Model: "abac", DefaultDeny: false},
			Audit: AuditPolicy{Enabled: false, LogLevel: "debug", RetentionDays: 60},
		},
	}
	
	data, err := manifest.ToJSON()
	require.NoError(t, err)
	require.NotEmpty(t, data)
	
	// Parseia novamente para verificar consistência
	parsed, err := ParseJSON(data)
	require.NoError(t, err)
	assert.Equal(t, manifest.Metadata.Name, parsed.Metadata.Name)
	assert.Equal(t, manifest.Metadata.Version, parsed.Metadata.Version)
}

func TestValidationWarnings(t *testing.T) {
	parser := &Parser{}
	
	// Manifesto válido mas com avisos
	manifest := &Manifest{
		Metadata: Metadata{Name: "Test", Version: "1.0.0"},
		// Sem atores - deve gerar warning
		DataModel: DataModel{
			Entities: []Entity{
				{Name: "Test", Fields: []Field{{Name: "id", Type: "string"}}},
			},
		},
	}
	
	err := parser.Validate(manifest)
	assert.NoError(t, err) // Sem erros, apenas warnings
	
	warnings := parser.GetWarnings()
	assert.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "no actors defined")
}

func TestComplexManifest(t *testing.T) {
	yamlData := `
metadata:
  name: "ECommerceSystem"
  version: "1.0.0"
  description: "Sistema completo de e-commerce"
  author: "Team"
  tags: ["ecommerce", "retail"]
  labels:
    environment: "production"
    region: "us-east-1"

actors:
  - id: "customer"
    name: "Cliente"
    description: "Cliente da loja"
    roles: ["customer"]
    permissions:
      - resource: "products"
        actions: ["read"]
      - resource: "orders"
        actions: ["read", "write"]
        condition: "owner == self"
  
  - id: "seller"
    name: "Vendedor"
    description: "Vendedor da plataforma"
    roles: ["seller", "customer"]
    permissions:
      - resource: "products"
        actions: ["read", "write", "delete"]
      - resource: "orders"
        actions: ["read", "update"]

data_model:
  entities:
    - name: "Customer"
      description: "Cliente do sistema"
      fields:
        - name: "id"
          type: "string"
          required: true
          unique: true
        - name: "email"
          type: "string"
          required: true
          unique: true
        - name: "name"
          type: "string"
          required: true
        - name: "created_at"
          type: "datetime"
          required: true
      indexes:
        - name: "idx_customer_email"
          fields: ["email"]
          unique: true
    
    - name: "Product"
      description: "Produto da loja"
      fields:
        - name: "id"
          type: "string"
          required: true
          unique: true
        - name: "name"
          type: "string"
          required: true
        - name: "price"
          type: "float"
          required: true
        - name: "stock"
          type: "int"
          required: true
          default: 0
        - name: "seller_id"
          type: "reference"
          required: true
          reference:
            entity: "Seller"
            field: "id"
            on_delete: "cascade"
    
    - name: "Order"
      description: "Pedido de compra"
      fields:
        - name: "id"
          type: "string"
          required: true
          unique: true
        - name: "customer_id"
          type: "reference"
          required: true
          reference:
            entity: "Customer"
            field: "id"
        - name: "total"
          type: "float"
          required: true
        - name: "status"
          type: "string"
          required: true
          default: "pending"
        - name: "created_at"
          type: "datetime"
          required: true
      constraints:
        - name: "chk_total_positive"
          type: "check"
          expression: "total > 0"

  relations:
    - name: "customer_orders"
      from: "Customer"
      to: "Order"
      type: "one_to_many"
    
    - name: "seller_products"
      from: "Seller"
      to: "Product"
      type: "one_to_many"

business_rules:
  - id: "BR001"
    name: "Validar Estoque"
    description: "Impede venda de produto sem estoque"
    trigger:
      event: "create"
      entities: ["Order"]
      before: true
    condition: "product.stock >= order.quantity"
    actions:
      - type: "validate"
        target: "stock"
    enabled: true
    priority: 1
  
  - id: "BR002"
    name: "Calcular Total"
    description: "Calcula automaticamente o total do pedido"
    trigger:
      event: "create"
      entities: ["Order"]
      after: true
    actions:
      - type: "transform"
        target: "total"
        script: "sum(items.price * items.quantity)"
    enabled: true
    priority: 2
  
  - id: "BR003"
    name: "Notificar Vendedor"
    description: "Notifica vendedor sobre novo pedido"
    trigger:
      event: "create"
      entities: ["Order"]
      after: true
    actions:
      - type: "notify"
        target: "seller"
        parameters:
          channel: "email"
          template: "new_order"
    enabled: true
    priority: 3

integrations:
  apis:
    - name: "Public API"
      base_path: "/api/v1"
      version: "1.0.0"
      endpoints:
        - path: "/products"
          method: "GET"
          description: "Lista produtos"
          handler: "list_products"
        - path: "/orders"
          method: "POST"
          description: "Cria pedido"
          handler: "create_order"
          permissions: ["customer", "seller"]
      auth:
        type: "jwt"
      rate_limit:
        requests_per_second: 100
        burst_size: 200
      cors:
        allowed_origins: ["https://example.com"]
        allowed_methods: ["GET", "POST", "PUT", "DELETE"]
        allowed_headers: ["Authorization", "Content-Type"]
  
  mcps:
    - name: "Payment Gateway"
      server: "payment-mcp"
      transport: "stdio"
      config:
        endpoint: "https://payment.gateway.com"
      tools: ["process_payment", "refund"]
  
  channels:
    - type: "web"
      name: "Web Portal"
      config:
        port: "8080"
        host: "0.0.0.0"
      enabled: true
    
    - type: "telegram"
      name: "Telegram Bot"
      config:
        token: "${TELEGRAM_TOKEN}"
      enabled: false

security:
  authentication:
    methods: ["jwt", "api_key"]
    session_timeout_minutes: 60
    mfa_required: false
    password_policy:
      min_length: 8
      require_uppercase: true
      require_lowercase: true
      require_numbers: true
      require_special: false
  
  authorization:
    model: "rbac"
    default_deny: true
    role_hierarchy:
      - role: "admin"
        inherits: ["seller", "customer"]
      - role: "seller"
        inherits: ["customer"]
    context_conditions:
      - name: "business_hours"
        expression: "hour >= 8 && hour <= 18"
        message: "Acesso permitido apenas em horário comercial"
  
  data_protection:
    encryption_at_rest: true
    encryption_in_transit: true
    sensitive_fields:
      - entity: "Customer"
        field: "email"
        encryption: true
        masking: true
        mask_pattern: "***@***.***"
    data_retention:
      default_days: 365
      rules:
        - entity: "Order"
          days: 1825
          action: "archive"
  
  audit:
    enabled: true
    log_level: "info"
    include_events: ["create", "update", "delete"]
    exclude_events: ["read"]
    retention_days: 90
    storage_backend: "database"

non_functional:
  performance:
    max_response_time_ms: 200
    max_concurrent_users: 1000
    throughput_rps: 500
  
  reliability:
    availability_percent: 99.9
    mttr_minutes: 30
    mtbf_hours: 720
  
  scalability:
    auto_scaling: true
    min_instances: 2
    max_instances: 10
    scaling_metrics: ["cpu", "memory", "requests"]
  
  compliance:
    standards: ["GDPR", "LGPD"]
    region: "south-america-east1"
`

	manifest, err := ParseYAML([]byte(yamlData))
	require.NoError(t, err)
	require.NotNil(t, manifest)

	parser := &Parser{}
	err = parser.Validate(manifest)
	assert.NoError(t, err)

	// Verificações específicas
	assert.Equal(t, "ECommerceSystem", manifest.Metadata.Name)
	assert.Len(t, manifest.Actors, 2)
	assert.Len(t, manifest.DataModel.Entities, 3)
	assert.Len(t, manifest.BusinessRules, 3)
	assert.Len(t, manifest.Integrations.APIs, 1)
	assert.Len(t, manifest.Integrations.MCPs, 1)
	assert.True(t, manifest.Security.DataProtection.EncryptionAtRest)
	assert.Equal(t, 99.9, manifest.NonFunctional.Reliability.AvailabilityPercent)
}
