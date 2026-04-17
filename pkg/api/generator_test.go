package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

func loadTestManifest(t *testing.T, name string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.ParseFile("../manifest/testdata/" + name)
	if err != nil {
		m, err = manifest.ParseFile("../../examples/manifests/" + name)
		if err != nil {
			t.Fatalf("failed to load manifest %s: %v", name, err)
		}
	}
	return m
}

func TestGenerator_BuildMux_Basic(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")
	gen, err := NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	mux, err := gen.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/_health")
	if err != nil {
		t.Fatalf("GET /_health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /_health: want 200, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp, err = http.Get(srv.URL + "/_manifest")
	if err != nil {
		t.Fatalf("GET /_manifest: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /_manifest: want 200, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestGenerator_BuildMux_Endpoints(t *testing.T) {
	m := loadTestManifest(t, "parking-ticket.yaml")
	gen, err := NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	mux, err := gen.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, api := range m.Integrations.APIs {
		for _, ep := range api.Endpoints {
			pattern := strings.TrimSuffix(api.BasePath, "/") + "/" + strings.TrimPrefix(ep.Path, "/")
			url := srv.URL + pattern

			var req *http.Request
			switch strings.ToUpper(ep.Method) {
			case "GET":
				req, _ = http.NewRequest(http.MethodGet, url, nil)
			case "POST":
				req, _ = http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
			case "PUT":
				req, _ = http.NewRequest(http.MethodPut, url, strings.NewReader("{}"))
			case "DELETE":
				req, _ = http.NewRequest(http.MethodDelete, url, nil)
			default:
				req, _ = http.NewRequest(http.MethodGet, url, nil)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("%s %s: request error: %v", ep.Method, pattern, err)
				continue
			}
			_ = resp.Body.Close()

			if resp.StatusCode == http.StatusMethodNotAllowed {
				t.Errorf("%s %s: method not allowed (pattern may not be registered)", ep.Method, pattern)
			}
		}
	}
}

func TestGenerator_PermissionCheck(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")
	gen, err := NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	mux, err := gen.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, api := range m.Integrations.APIs {
		for _, ep := range api.Endpoints {
			if len(ep.Permissions) == 0 {
				continue
			}
			pattern := strings.TrimSuffix(api.BasePath, "/") + "/" + strings.TrimPrefix(ep.Path, "/")
			url := srv.URL + pattern

			var req *http.Request
			switch strings.ToUpper(ep.Method) {
			case "GET":
				req, _ = http.NewRequest(http.MethodGet, url, nil)
			case "POST":
				req, _ = http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
			default:
				req, _ = http.NewRequest(http.MethodGet, url, nil)
			}

			req.Header.Set("X-Actor-ID", "unknown_actor")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("%s %s: request error: %v", ep.Method, pattern, err)
				continue
			}
			_ = resp.Body.Close()

			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s %s: want 403 for unknown actor, got %d", ep.Method, pattern, resp.StatusCode)
			}
		}
	}
}
