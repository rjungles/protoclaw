package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest representa a definição completa do sistema baseado em agentes
type Manifest struct {
	// Metadados do sistema
	Metadata Metadata `yaml:"metadata" json:"metadata"`

	// Definição de atores e seus papéis
	Actors []Actor `yaml:"actors" json:"actors"`

	// Modelo de dados e entidades
	DataModel DataModel `yaml:"data_model" json:"data_model"`

	// Regras de negócio
	BusinessRules []BusinessRule `yaml:"business_rules" json:"business_rules"`

	// Integrações (APIs, MCPs, canais)
	Integrations Integrations `yaml:"integrations" json:"integrations"`

	// Políticas de segurança e acesso
	Security SecurityPolicy `yaml:"security" json:"security"`

	// Configurações não funcionais
	NonFunctional NonFunctional `yaml:"non_functional" json:"non_functional"`
}

// Metadata contém informações básicas do sistema
type Metadata struct {
	Name        string            `yaml:"name" json:"name"`
	Version     string            `yaml:"version" json:"version"`
	Description string            `yaml:"description" json:"description"`
	Author      string            `yaml:"author" json:"author"`
	CreatedAt   string            `yaml:"created_at" json:"created_at"`
	Tags        []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// Actor define um tipo de usuário ou entidade do sistema
type Actor struct {
	ID          string            `yaml:"id" json:"id"`
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Roles       []string          `yaml:"roles" json:"roles"`
	Permissions []Permission      `yaml:"permissions" json:"permissions"`
	Attributes  map[string]string `yaml:"attributes,omitempty" json:"attributes,omitempty"`
}

// Permission define uma permissão específica
type Permission struct {
	Resource string   `yaml:"resource" json:"resource"`
	Actions  []string `yaml:"actions" json:"actions"` // read, write, delete, execute
	Condition string  `yaml:"condition,omitempty" json:"condition,omitempty"`
}

// DataModel define a estrutura de dados do sistema
type DataModel struct {
	Entities []Entity `yaml:"entities" json:"entities"`
	Relations []Relation `yaml:"relations,omitempty" json:"relations,omitempty"`
}

// Entity representa uma entidade/tabela no modelo de dados
type Entity struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Fields      []Field           `yaml:"fields" json:"fields"`
	Indexes     []Index           `yaml:"indexes,omitempty" json:"indexes,omitempty"`
	Constraints []Constraint      `yaml:"constraints,omitempty" json:"constraints,omitempty"`
	Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// Field define um campo da entidade
type Field struct {
	Name        string      `yaml:"name" json:"name"`
	Type        string      `yaml:"type" json:"type"` // string, int, float, bool, datetime, json, reference
	Required    bool        `yaml:"required" json:"required"`
	Unique      bool        `yaml:"unique" json:"unique"`
	MaxLength   *int        `yaml:"max_length,omitempty" json:"max_length,omitempty"`
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Reference   *Reference  `yaml:"reference,omitempty" json:"reference,omitempty"`
}

// Reference define uma relação com outra entidade
type Reference struct {
	Entity string `yaml:"entity" json:"entity"`
	Field  string `yaml:"field" json:"field"`
	OnDelete string `yaml:"on_delete,omitempty" json:"on_delete,omitempty"` // cascade, set_null, restrict
}

// Index define um índice na entidade
type Index struct {
	Name    string   `yaml:"name" json:"name"`
	Fields  []string `yaml:"fields" json:"fields"`
	Unique  bool     `yaml:"unique" json:"unique"`
}

// Constraint define uma restrição de integridade
type Constraint struct {
	Name       string   `yaml:"name" json:"name"`
	Type       string   `yaml:"type" json:"type"` // unique, check, foreign_key
	Fields     []string `yaml:"fields" json:"fields"`
	Expression string   `yaml:"expression,omitempty" json:"expression,omitempty"`
}

// Relation define relacionamentos entre entidades
type Relation struct {
	Name     string `yaml:"name" json:"name"`
	From     string `yaml:"from" json:"from"`
	To       string `yaml:"to" json:"to"`
	Type     string `yaml:"type" json:"type"` // one_to_one, one_to_many, many_to_many
	Through  string `yaml:"through,omitempty" json:"through,omitempty"`
}

// BusinessRule define uma regra de negócio
type BusinessRule struct {
	ID          string            `yaml:"id" json:"id"`
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Trigger     Trigger           `yaml:"trigger" json:"trigger"`
	Condition   string            `yaml:"condition,omitempty" json:"condition,omitempty"`
	Actions     []RuleAction      `yaml:"actions" json:"actions"`
	Priority    int               `yaml:"priority,omitempty" json:"priority,omitempty"`
	Enabled     bool              `yaml:"enabled" json:"enabled"`
	Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// Trigger define quando uma regra é acionada
type Trigger struct {
	Event     string   `yaml:"event" json:"event"` // create, update, delete, read
	Entities  []string `yaml:"entities" json:"entities"`
	Before    bool     `yaml:"before" json:"before"`
	After     bool     `yaml:"after" json:"after"`
}

// RuleAction define uma ação executada por uma regra
type RuleAction struct {
	Type       string                 `yaml:"type" json:"type"` // validate, transform, notify, execute, reject
	Target     string                 `yaml:"target,omitempty" json:"target,omitempty"`
	Parameters map[string]interface{} `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Script     string                 `yaml:"script,omitempty" json:"script,omitempty"`
}

// Integrations define as integrações do sistema
type Integrations struct {
	APIs       []APIConfig       `yaml:"apis,omitempty" json:"apis,omitempty"`
	MCPs       []MCPConfig       `yaml:"mcps,omitempty" json:"mcps,omitempty"`
	Channels   []ChannelConfig   `yaml:"channels,omitempty" json:"channels,omitempty"`
	Webhooks   []WebhookConfig   `yaml:"webhooks,omitempty" json:"webhooks,omitempty"`
}

// APIConfig define uma API exposta pelo sistema
type APIConfig struct {
	Name        string            `yaml:"name" json:"name"`
	BasePath    string            `yaml:"base_path" json:"base_path"`
	Version     string            `yaml:"version" json:"version"`
	Endpoints   []Endpoint        `yaml:"endpoints" json:"endpoints"`
	Auth        AuthConfig        `yaml:"auth,omitempty" json:"auth,omitempty"`
	RateLimit   *RateLimitConfig  `yaml:"rate_limit,omitempty" json:"rate_limit,omitempty"`
	CORS        *CORSConfig       `yaml:"cors,omitempty" json:"cors,omitempty"`
}

// Endpoint define um endpoint de API
type Endpoint struct {
	Path        string            `yaml:"path" json:"path"`
	Method      string            `yaml:"method" json:"method"` // GET, POST, PUT, DELETE, PATCH
	Description string            `yaml:"description" json:"description"`
	Handler     string            `yaml:"handler" json:"handler"` // referência a skill ou função
	Input       *SchemaRef        `yaml:"input,omitempty" json:"input,omitempty"`
	Output      *SchemaRef        `yaml:"output,omitempty" json:"output,omitempty"`
	Permissions []string          `yaml:"permissions,omitempty" json:"permissions,omitempty"`
}

// SchemaRef referencia um schema do DataModel
type SchemaRef struct {
	Entity string   `yaml:"entity" json:"entity"`
	Fields []string `yaml:"fields,omitempty" json:"fields,omitempty"`
}

// MCPConfig define uma integração MCP
type MCPConfig struct {
	Name        string            `yaml:"name" json:"name"`
	Server      string            `yaml:"server" json:"server"`
	Transport   string            `yaml:"transport" json:"transport"` // stdio, sse, websocket
	Config      map[string]string `yaml:"config,omitempty" json:"config,omitempty"`
	Tools       []string          `yaml:"tools,omitempty" json:"tools,omitempty"`
	Resources   []string          `yaml:"resources,omitempty" json:"resources,omitempty"`
}

// ChannelConfig define um canal de comunicação
type ChannelConfig struct {
	Type     string            `yaml:"type" json:"type"` // telegram, discord, slack, web, irc
	Name     string            `yaml:"name" json:"name"`
	Config   map[string]string `yaml:"config" json:"config"`
	Agents   []string          `yaml:"agents,omitempty" json:"agents,omitempty"`
	Enabled  bool              `yaml:"enabled" json:"enabled"`
}

// WebhookConfig define um webhook
type WebhookConfig struct {
	Name        string            `yaml:"name" json:"name"`
	URL         string            `yaml:"url" json:"url"`
	Method      string            `yaml:"method" json:"method"`
	Events      []string          `yaml:"events" json:"events"`
	Headers     map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Secret      string            `yaml:"secret,omitempty" json:"secret,omitempty"`
	RetryConfig *RetryConfig      `yaml:"retry,omitempty" json:"retry,omitempty"`
}

// AuthConfig define configuração de autenticação
type AuthConfig struct {
	Type     string            `yaml:"type" json:"type"` // jwt, api_key, oauth2, basic
	Config   map[string]string `yaml:"config,omitempty" json:"config,omitempty"`
}

// RateLimitConfig define limites de taxa
type RateLimitConfig struct {
	RequestsPerSecond int `yaml:"requests_per_second" json:"requests_per_second"`
	BurstSize         int `yaml:"burst_size" json:"burst_size"`
}

// CORSConfig define políticas CORS
type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins" json:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods" json:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers" json:"allowed_headers"`
}

// RetryConfig define política de retry
type RetryConfig struct {
	MaxRetries int `yaml:"max_retries" json:"max_retries"`
	DelayMs    int `yaml:"delay_ms" json:"delay_ms"`
}

// SecurityPolicy define políticas de segurança
type SecurityPolicy struct {
	Authentication AuthenticationPolicy `yaml:"authentication" json:"authentication"`
	Authorization  AuthorizationPolicy  `yaml:"authorization" json:"authorization"`
	DataProtection DataProtectionPolicy `yaml:"data_protection" json:"data_protection"`
	Audit          AuditPolicy          `yaml:"audit" json:"audit"`
}

// AuthenticationPolicy define políticas de autenticação
type AuthenticationPolicy struct {
	Methods          []string `yaml:"methods" json:"methods"`
	SessionTimeout   int      `yaml:"session_timeout_minutes" json:"session_timeout_minutes"`
	MFARequired      bool     `yaml:"mfa_required" json:"mfa_required"`
	PasswordPolicy   *PasswordPolicy `yaml:"password_policy,omitempty" json:"password_policy,omitempty"`
}

// PasswordPolicy define política de senhas
type PasswordPolicy struct {
	MinLength       int  `yaml:"min_length" json:"min_length"`
	RequireUppercase bool `yaml:"require_uppercase" json:"require_uppercase"`
	RequireLowercase bool `yaml:"require_lowercase" json:"require_lowercase"`
	RequireNumbers  bool `yaml:"require_numbers" json:"require_numbers"`
	RequireSpecial  bool `yaml:"require_special" json:"require_special"`
}

// AuthorizationPolicy define políticas de autorização
type AuthorizationPolicy struct {
	Model             string            `yaml:"model" json:"model"` // rbac, abac, acl
	DefaultDeny       bool              `yaml:"default_deny" json:"default_deny"`
	RoleHierarchy     []RoleHierarchy   `yaml:"role_hierarchy,omitempty" json:"role_hierarchy,omitempty"`
	ContextConditions []ContextCondition `yaml:"context_conditions,omitempty" json:"context_conditions,omitempty"`
}

// RoleHierarchy define hierarquia de papéis
type RoleHierarchy struct {
	Role     string   `yaml:"role" json:"role"`
	Inherits []string `yaml:"inherits" json:"inherits"`
}

// ContextCondition define condições contextuais para acesso
type ContextCondition struct {
	Name       string            `yaml:"name" json:"name"`
	Expression string            `yaml:"expression" json:"expression"`
	Message    string            `yaml:"message" json:"message"`
}

// DataProtectionPolicy define políticas de proteção de dados
type DataProtectionPolicy struct {
	EncryptionAtRest    bool                `yaml:"encryption_at_rest" json:"encryption_at_rest"`
	EncryptionInTransit bool                `yaml:"encryption_in_transit" json:"encryption_in_transit"`
	SensitiveFields     []SensitiveField    `yaml:"sensitive_fields,omitempty" json:"sensitive_fields,omitempty"`
	DataRetention       *DataRetention      `yaml:"data_retention,omitempty" json:"data_retention,omitempty"`
}

// SensitiveField define um campo sensível
type SensitiveField struct {
	Entity      string `yaml:"entity" json:"entity"`
	Field       string `yaml:"field" json:"field"`
	Encryption  bool   `yaml:"encryption" json:"encryption"`
	Masking     bool   `yaml:"masking" json:"masking"`
	MaskPattern string `yaml:"mask_pattern,omitempty" json:"mask_pattern,omitempty"`
}

// DataRetention define política de retenção de dados
type DataRetention struct {
	DefaultDays int                `yaml:"default_days" json:"default_days"`
	Rules       []RetentionRule    `yaml:"rules,omitempty" json:"rules,omitempty"`
}

// RetentionRule define regra de retenção específica
type RetentionRule struct {
	Entity string `yaml:"entity" json:"entity"`
	Days   int    `yaml:"days" json:"days"`
	Action string `yaml:"action" json:"action"` // delete, archive, anonymize
}

// AuditPolicy define políticas de auditoria
type AuditPolicy struct {
	Enabled           bool              `yaml:"enabled" json:"enabled"`
	LogLevel          string            `yaml:"log_level" json:"log_level"` // debug, info, warn, error
	IncludeEvents     []string          `yaml:"include_events,omitempty" json:"include_events,omitempty"`
	ExcludeEvents     []string          `yaml:"exclude_events,omitempty" json:"exclude_events,omitempty"`
	RetentionDays     int               `yaml:"retention_days" json:"retention_days"`
	StorageBackend    string            `yaml:"storage_backend" json:"storage_backend"` // file, database, external
}

// NonFunctional define requisitos não funcionais
type NonFunctional struct {
	Performance PerformanceRequirements `yaml:"performance" json:"performance"`
	Reliability ReliabilityRequirements `yaml:"reliability" json:"reliability"`
	Scalability ScalabilityRequirements `yaml:"scalability" json:"scalability"`
	Compliance  ComplianceRequirements  `yaml:"compliance" json:"compliance"`
}

// PerformanceRequirements define requisitos de performance
type PerformanceRequirements struct {
	MaxResponseTimeMs int `yaml:"max_response_time_ms" json:"max_response_time_ms"`
	MaxConcurrentUsers int `yaml:"max_concurrent_users" json:"max_concurrent_users"`
	ThroughputRPS     int `yaml:"throughput_rps" json:"throughput_rps"`
}

// ReliabilityRequirements define requisitos de confiabilidade
type ReliabilityRequirements struct {
	AvailabilityPercent float64 `yaml:"availability_percent" json:"availability_percent"`
	MTTRMinutes         int     `yaml:"mttr_minutes" json:"mttr_minutes"`
	MTBFHours           int     `yaml:"mtbf_hours" json:"mtbf_hours"`
}

// ScalabilityRequirements define requisitos de escalabilidade
type ScalabilityRequirements struct {
	AutoScaling     bool     `yaml:"auto_scaling" json:"auto_scaling"`
	MinInstances    int      `yaml:"min_instances" json:"min_instances"`
	MaxInstances    int      `yaml:"max_instances" json:"max_instances"`
	ScalingMetrics  []string `yaml:"scaling_metrics" json:"scaling_metrics"`
}

// ComplianceRequirements define requisitos de conformidade
type ComplianceRequirements struct {
	Standards []string `yaml:"standards" json:"standards"` // GDPR, HIPAA, SOC2, etc.
	Region    string   `yaml:"region" json:"region"`
}

// Parser gerencia o parsing e validação de manifests
type Parser struct {
	validationErrors []error
	warnings         []string
}

// ParseFile carrega e parseia um arquivo de manifesto
func ParseFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		return ParseYAML(data)
	case ".json":
		return ParseJSON(data)
	default:
		// Tenta YAML primeiro, depois JSON
		manifest, err := ParseYAML(data)
		if err != nil {
			return ParseJSON(data)
		}
		return manifest, nil
	}
}

// ParseYAML parseia dados YAML
func ParseYAML(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse YAML manifest: %w", err)
	}
	return &manifest, nil
}

// ParseJSON parseia dados JSON
func ParseJSON(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse JSON manifest: %w", err)
	}
	return &manifest, nil
}

// Validate valida o manifesto
func (p *Parser) Validate(manifest *Manifest) error {
	p.validationErrors = []error{}
	p.warnings = []string{}

	// Valida metadata
	p.validateMetadata(&manifest.Metadata)

	// Valida atores
	p.validateActors(manifest.Actors)

	// Valida modelo de dados
	p.validateDataModel(&manifest.DataModel)

	// Valida regras de negócio
	p.validateBusinessRules(manifest.BusinessRules)

	// Valida integrações
	p.validateIntegrations(&manifest.Integrations)

	// Valida segurança
	p.validateSecurity(&manifest.Security)

	if len(p.validationErrors) > 0 {
		// Combina todos os erros em uma única mensagem
		errMsg := ""
		for i, err := range p.validationErrors {
			if i > 0 {
				errMsg += "; "
			}
			errMsg += err.Error()
		}
		return errors.New(errMsg)
	}

	return nil
}

func (p *Parser) validateMetadata(m *Metadata) {
	if m.Name == "" {
		p.validationErrors = append(p.validationErrors, errors.New("metadata.name is required"))
	}
	if m.Version == "" {
		p.validationErrors = append(p.validationErrors, errors.New("metadata.version is required"))
	}
}

func (p *Parser) validateActors(actors []Actor) {
	if len(actors) == 0 {
		p.warnings = append(p.warnings, "no actors defined")
		return
	}

	seenIDs := make(map[string]bool)
	for _, actor := range actors {
		if actor.ID == "" {
			p.validationErrors = append(p.validationErrors, errors.New("actor.id is required"))
			continue
		}
		if seenIDs[actor.ID] {
			p.validationErrors = append(p.validationErrors, fmt.Errorf("duplicate actor id: %s", actor.ID))
		}
		seenIDs[actor.ID] = true

		if actor.Name == "" {
			p.validationErrors = append(p.validationErrors, fmt.Errorf("actor.name is required for id: %s", actor.ID))
		}
	}
}

func (p *Parser) validateDataModel(dm *DataModel) {
	if len(dm.Entities) == 0 {
		p.warnings = append(p.warnings, "no entities defined in data model")
		return
	}

	seenEntities := make(map[string]bool)
	for _, entity := range dm.Entities {
		if entity.Name == "" {
			p.validationErrors = append(p.validationErrors, errors.New("entity.name is required"))
			continue
		}
		if seenEntities[entity.Name] {
			p.validationErrors = append(p.validationErrors, fmt.Errorf("duplicate entity name: %s", entity.Name))
		}
		seenEntities[entity.Name] = true

		if len(entity.Fields) == 0 {
			p.validationErrors = append(p.validationErrors, fmt.Errorf("entity %s must have at least one field", entity.Name))
		}

		seenFields := make(map[string]bool)
		for _, field := range entity.Fields {
			if field.Name == "" {
				p.validationErrors = append(p.validationErrors, fmt.Errorf("field name is required in entity %s", entity.Name))
				continue
			}
			if seenFields[field.Name] {
				p.validationErrors = append(p.validationErrors, fmt.Errorf("duplicate field %s in entity %s", field.Name, entity.Name))
			}
			seenFields[field.Name] = true

			if field.Type == "" {
				p.validationErrors = append(p.validationErrors, fmt.Errorf("field %s in entity %s must have a type", field.Name, entity.Name))
			}
		}
	}
}

func (p *Parser) validateBusinessRules(rules []BusinessRule) {
	seenIDs := make(map[string]bool)
	for _, rule := range rules {
		if rule.ID == "" {
			p.validationErrors = append(p.validationErrors, errors.New("business rule id is required"))
			continue
		}
		if seenIDs[rule.ID] {
			p.validationErrors = append(p.validationErrors, fmt.Errorf("duplicate business rule id: %s", rule.ID))
		}
		seenIDs[rule.ID] = true

		if rule.Name == "" {
			p.validationErrors = append(p.validationErrors, fmt.Errorf("business rule name is required for id: %s", rule.ID))
		}

		if len(rule.Trigger.Entities) == 0 {
			p.validationErrors = append(p.validationErrors, fmt.Errorf("business rule %s must have trigger entities", rule.ID))
		}

		if rule.Trigger.Event == "" {
			p.validationErrors = append(p.validationErrors, fmt.Errorf("business rule %s must have a trigger event", rule.ID))
		}
	}
}

func (p *Parser) validateIntegrations(i *Integrations) {
	// Validação básica - pode ser expandida
	for _, api := range i.APIs {
		if api.Name == "" {
			p.validationErrors = append(p.validationErrors, errors.New("API name is required"))
		}
		if api.BasePath == "" {
			p.validationErrors = append(p.validationErrors, fmt.Errorf("API %s base_path is required", api.Name))
		}
	}
}

func (p *Parser) validateSecurity(s *SecurityPolicy) {
	// Validação básica - pode ser expandida
	if s.Authorization.DefaultDeny && len(s.Authorization.RoleHierarchy) == 0 {
		p.warnings = append(p.warnings, "default_deny is true but no role hierarchy defined")
	}
}

// GetWarnings retorna avisos da validação
func (p *Parser) GetWarnings() []string {
	return p.warnings
}

// GetErrors retorna erros da validação
func (p *Parser) GetErrors() []error {
	return p.validationErrors
}

// ToYAML serializa o manifesto para YAML
func (m *Manifest) ToYAML() ([]byte, error) {
	return yaml.Marshal(m)
}

// ToJSON serializa o manifesto para JSON
func (m *Manifest) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}
