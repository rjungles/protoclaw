package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/manifest"
)

type Generator struct {
	manifest     *manifest.Manifest
	engine       *policy.Engine
	repo         *Repository
	ruleExecutor *RuleExecutor
}

func NewGenerator(m *manifest.Manifest) (*Generator, error) {
	engine, err := policy.NewEngine(m)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar engine de políticas: %w", err)
	}
	return &Generator{manifest: m, engine: engine, ruleExecutor: NewRuleExecutor(m)}, nil
}

func NewGeneratorWithDB(m *manifest.Manifest, db *sql.DB) (*Generator, error) {
	gen, err := NewGenerator(m)
	if err != nil {
		return nil, err
	}
	gen.repo = NewRepository(db, m)
	return gen, nil
}

func (g *Generator) BuildMux() (*http.ServeMux, error) {
	mux := http.NewServeMux()
	for _, api := range g.manifest.Integrations.APIs {
		for _, ep := range api.Endpoints {
			handler := g.buildHandler(api, ep)
			pattern := strings.TrimSuffix(api.BasePath, "/") + "/" + strings.TrimPrefix(ep.Path, "/")
			methodPattern := strings.ToUpper(ep.Method) + " " + pattern
			mux.HandleFunc(methodPattern, handler)
		}
	}
	mux.HandleFunc("GET /_manifest", g.serveManifest)
	mux.HandleFunc("GET /_health", g.serveHealth)
	mux.HandleFunc("GET /_openapi.json", g.serveOpenAPI)
	mux.HandleFunc("GET /_docs", g.serveDocs)
	return mux, nil
}

func (g *Generator) buildHandler(api manifest.APIConfig, ep manifest.Endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actorID := g.extractActorID(r)
		if len(ep.Permissions) > 0 {
			allowed := false
			for _, perm := range ep.Permissions {
				parts := strings.SplitN(perm, ":", 2)
				if len(parts) != 2 {
					continue
				}
				resource := parts[0]
				action := parts[1]
				ctx := &policy.Context{
					ActorID:    actorID,
					Resource:   resource,
					Action:     action,
					Attributes: map[string]interface{}{},
					Time:       time.Now(),
				}
				if result := g.engine.CheckPermission(ctx); result.Allowed {
					allowed = true
					break
				}
			}
			if !allowed {
				g.writeError(w, http.StatusForbidden, "access denied")
				return
			}
		}

		if g.repo == nil {
			g.writeJSON(w, http.StatusOK, map[string]interface{}{
				"api":      api.Name,
				"endpoint": ep.Path,
				"method":   ep.Method,
				"handler":  ep.Handler,
				"actor":    actorID,
				"message":  "endpoint registered; database not configured",
			})
			return
		}

		handler := g.resolveHandler(ep)
		if handler != nil {
			handler(w, r, actorID, ep)
			return
		}

		g.writeJSON(w, http.StatusOK, map[string]interface{}{
			"api":      api.Name,
			"endpoint": ep.Path,
			"method":   ep.Method,
			"handler":  ep.Handler,
			"actor":    actorID,
			"message":  "handler not implemented",
		})
	}
}

func (g *Generator) resolveHandler(ep manifest.Endpoint) func(w http.ResponseWriter, r *http.Request, actorID string, endpoint manifest.Endpoint) {
	if ep.Handler != "" {
		return g.handlerByName(ep.Handler)
	}
	return g.crudHandler(ep)
}

func (g *Generator) handlerByName(name string) func(w http.ResponseWriter, r *http.Request, actorID string, endpoint manifest.Endpoint) {
	switch name {
	case "list":
		return g.handleList
	case "get":
		return g.handleGet
	case "create":
		return g.handleCreate
	case "update":
		return g.handleUpdate
	case "delete":
		return g.handleDelete
	default:
		return nil
	}
}

func (g *Generator) crudHandler(ep manifest.Endpoint) func(w http.ResponseWriter, r *http.Request, actorID string, endpoint manifest.Endpoint) {
	entityName := g.extractEntityFromPath(ep.Path)
	if entityName == "" {
		return nil
	}

	switch strings.ToUpper(ep.Method) {
	case "GET":
		if strings.Contains(ep.Path, "{id}") {
			return g.handleGet
		}
		return g.handleList
	case "POST":
		return g.handleCreate
	case "PUT", "PATCH":
		return g.handleUpdate
	case "DELETE":
		return g.handleDelete
	default:
		return nil
	}
}

func (g *Generator) extractEntityFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 {
		return toPascalCase(parts[0])
	}
	return ""
}

func (g *Generator) handleList(w http.ResponseWriter, r *http.Request, actorID string, ep manifest.Endpoint) {
	entityName := g.extractEntityFromPath(ep.Path)
	if entityName == "" {
		g.writeError(w, http.StatusBadRequest, "invalid entity")
		return
	}

	limit := 100
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	results, err := g.repo.FindAll(r.Context(), entityName, limit, offset)
	if err != nil {
		g.writeError(w, http.StatusInternalServerError, fmt.Sprintf("query error: %v", err))
		return
	}

	g.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":   results,
		"limit":  limit,
		"offset": offset,
		"count":  len(results),
	})
}

func (g *Generator) handleGet(w http.ResponseWriter, r *http.Request, actorID string, ep manifest.Endpoint) {
	entityName := g.extractEntityFromPath(ep.Path)
	if entityName == "" {
		g.writeError(w, http.StatusBadRequest, "invalid entity")
		return
	}

	id := g.extractIDFromPath(r.URL.Path, ep.Path)
	if id == "" {
		g.writeError(w, http.StatusBadRequest, "missing id")
		return
	}

	result, err := g.repo.FindByID(r.Context(), entityName, id)
	if err == ErrNotFound {
		g.writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		g.writeError(w, http.StatusInternalServerError, fmt.Sprintf("query error: %v", err))
		return
	}

	g.writeJSON(w, http.StatusOK, result)
}

func (g *Generator) handleCreate(w http.ResponseWriter, r *http.Request, actorID string, ep manifest.Endpoint) {
	entityName := g.extractEntityFromPath(ep.Path)
	if entityName == "" {
		g.writeError(w, http.StatusBadRequest, "invalid entity")
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil && err != io.EOF {
		g.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if err := g.ruleExecutor.ExecuteBefore(r.Context(), "create", entityName, data); err != nil {
		g.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := g.repo.Create(r.Context(), entityName, data)
	if err != nil {
		g.writeError(w, http.StatusInternalServerError, fmt.Sprintf("create error: %v", err))
		return
	}

	data["id"] = id

	if err := g.ruleExecutor.ExecuteAfter(r.Context(), "create", entityName, data); err != nil {
		g.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	g.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      id,
		"message": "created successfully",
	})
}

func (g *Generator) handleUpdate(w http.ResponseWriter, r *http.Request, actorID string, ep manifest.Endpoint) {
	entityName := g.extractEntityFromPath(ep.Path)
	if entityName == "" {
		g.writeError(w, http.StatusBadRequest, "invalid entity")
		return
	}

	id := g.extractIDFromPath(r.URL.Path, ep.Path)
	if id == "" {
		g.writeError(w, http.StatusBadRequest, "missing id")
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil && err != io.EOF {
		g.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	data["id"] = id

	if err := g.ruleExecutor.ExecuteBefore(r.Context(), "update", entityName, data); err != nil {
		g.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := g.repo.Update(r.Context(), entityName, id, data); err == ErrNotFound {
		g.writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		g.writeError(w, http.StatusInternalServerError, fmt.Sprintf("update error: %v", err))
		return
	}

	if err := g.ruleExecutor.ExecuteAfter(r.Context(), "update", entityName, data); err != nil {
		g.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	g.writeJSON(w, http.StatusOK, map[string]string{"message": "updated successfully"})
}

func (g *Generator) handleDelete(w http.ResponseWriter, r *http.Request, actorID string, ep manifest.Endpoint) {
	entityName := g.extractEntityFromPath(ep.Path)
	if entityName == "" {
		g.writeError(w, http.StatusBadRequest, "invalid entity")
		return
	}

	id := g.extractIDFromPath(r.URL.Path, ep.Path)
	if id == "" {
		g.writeError(w, http.StatusBadRequest, "missing id")
		return
	}

	data := map[string]interface{}{"id": id}

	if err := g.ruleExecutor.ExecuteBefore(r.Context(), "delete", entityName, data); err != nil {
		g.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := g.repo.Delete(r.Context(), entityName, id); err == ErrNotFound {
		g.writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		g.writeError(w, http.StatusInternalServerError, fmt.Sprintf("delete error: %v", err))
		return
	}

	if err := g.ruleExecutor.ExecuteAfter(r.Context(), "delete", entityName, data); err != nil {
		g.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	g.writeJSON(w, http.StatusOK, map[string]string{"message": "deleted successfully"})
}

func (g *Generator) extractIDFromPath(requestPath, pattern string) string {
	requestParts := strings.Split(strings.Trim(requestPath, "/"), "/")
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")

	for i, p := range patternParts {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") && i < len(requestParts) {
			return requestParts[i]
		}
	}

	return ""
}

func (g *Generator) extractActorID(r *http.Request) string {
	if v := r.Header.Get("X-Actor-ID"); v != "" {
		return v
	}
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return "anonymous"
}

func (g *Generator) serveManifest(w http.ResponseWriter, r *http.Request) {
	data, err := g.manifest.ToJSON()
	if err != nil {
		g.writeError(w, http.StatusInternalServerError, "failed to serialize manifest")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (g *Generator) serveHealth(w http.ResponseWriter, r *http.Request) {
	g.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *Generator) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (g *Generator) writeError(w http.ResponseWriter, status int, message string) {
	g.writeJSON(w, status, map[string]string{"error": message})
}

func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	var result string
	for _, part := range parts {
		if len(part) > 0 {
			result += strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return result
}
