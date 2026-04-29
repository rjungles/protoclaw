package llm

// Router handles request routing to appropriate providers
type Router struct {
	defaultRule  RoutingRule
	intentRules  map[string]RoutingRule
	funcRules    map[string]RoutingRule
}

// NewRouter creates a new router
func NewRouter() *Router {
	return &Router{
		intentRules: make(map[string]RoutingRule),
		funcRules:   make(map[string]RoutingRule),
	}
}

// SetDefaultRule sets the default routing rule
func (r *Router) SetDefaultRule(rule RoutingRule) {
	r.defaultRule = rule
}

// AddIntentRule adds a routing rule for a specific intent
func (r *Router) AddIntentRule(intent string, rule RoutingRule) {
	r.intentRules[intent] = rule
}

// AddFunctionRule adds a routing rule for a specific function
func (r *Router) AddFunctionRule(function string, rule RoutingRule) {
	r.funcRules[function] = rule
}

// Route determines the best provider and model for a request
func (r *Router) Route(req CompletionRequest) RoutingRule {
	// Check function-specific routing first
	if req.Function != "" {
		if rule, ok := r.funcRules[req.Function]; ok {
			return rule
		}
	}

	// Check intent-based routing
	if req.Intent != "" {
		if rule, ok := r.intentRules[req.Intent]; ok {
			return rule
		}
	}

	// Return default
	return r.defaultRule
}
