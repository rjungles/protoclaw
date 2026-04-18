package agentos

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const ActorIDContextKey contextKey = "actor_id"
const ActorRolesContextKey contextKey = "actor_roles"
const ActorCredentialContextKey contextKey = "actor_credential"

type AuthMiddleware struct {
	actorStore ActorStore
	manifest   interface{}
}

func NewAuthMiddleware(actorStore ActorStore) *AuthMiddleware {
	return &AuthMiddleware{
		actorStore: actorStore,
	}
}

func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actorID := m.authenticate(r)

		ctx := context.WithValue(r.Context(), ActorIDContextKey, actorID)

		if m.actorStore != nil {
			cred, err := m.actorStore.GetByID(actorID)
			if err == nil && cred != nil {
				ctx = context.WithValue(ctx, ActorRolesContextKey, cred.Roles)
				ctx = context.WithValue(ctx, ActorCredentialContextKey, cred)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthMiddleware) authenticate(r *http.Request) string {
	if v := r.Header.Get("X-Actor-ID"); v != "" {
		return v
	}

	if v := r.Header.Get("Authorization"); v != "" {
		if strings.HasPrefix(v, "Bearer ") {
			apiKey := strings.TrimPrefix(v, "Bearer ")
			if m.actorStore != nil {
				cred, err := m.actorStore.GetByAPIKey(apiKey)
				if err == nil && cred != nil {
					return cred.ActorID
				}
			}
		}

		if strings.HasPrefix(v, "ApiKey ") {
			apiKey := strings.TrimPrefix(v, "ApiKey ")
			if m.actorStore != nil {
				cred, err := m.actorStore.GetByAPIKey(apiKey)
				if err == nil && cred != nil {
					return cred.ActorID
				}
			}
		}
	}

	if v := r.Header.Get("X-API-Key"); v != "" {
		if m.actorStore != nil {
			cred, err := m.actorStore.GetByAPIKey(v)
			if err == nil && cred != nil {
				return cred.ActorID
			}
		}
	}

	return "anonymous"
}

func GetActorIDFromContext(ctx context.Context) string {
	if v := ctx.Value(ActorIDContextKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "anonymous"
}

func GetActorRolesFromContext(ctx context.Context) []string {
	if v := ctx.Value(ActorRolesContextKey); v != nil {
		if s, ok := v.([]string); ok {
			return s
		}
	}
	return nil
}

func GetActorCredentialFromContext(ctx context.Context) *ActorCredential {
	if v := ctx.Value(ActorCredentialContextKey); v != nil {
		if c, ok := v.(*ActorCredential); ok {
			return c
		}
	}
	return nil
}

type CORSMiddleware struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
}

func NewCORSMiddleware() *CORSMiddleware {
	return &CORSMiddleware{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Actor-ID", "X-API-Key"},
	}
}

func (m *CORSMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", strings.Join(m.AllowedMethods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(m.AllowedHeaders, ", "))
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type LoggingMiddleware struct{}

func NewLoggingMiddleware() *LoggingMiddleware {
	return &LoggingMiddleware{}
}

func (m *LoggingMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actorID := GetActorIDFromContext(r.Context())
		_ = actorID

		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
