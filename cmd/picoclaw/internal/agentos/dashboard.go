package agentos

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// DashboardHandler handles the admin dashboard web interface
type DashboardHandler struct {
	server *MultiSystemServer
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(server *MultiSystemServer) *DashboardHandler {
	return &DashboardHandler{server: server}
}

// ServeHTTP implements http.Handler
func (d *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin")
	if path == "" || path == "/" {
		d.serveDashboard(w, r)
		return
	}

	switch {
	case strings.HasPrefix(path, "/api/systems"):
		d.serveAPISystems(w, r)
	case strings.HasPrefix(path, "/api/metrics"):
		d.serveAPIMetrics(w, r)
	case strings.HasPrefix(path, "/api/keys/"):
		d.serveAPIKeys(w, r)
	default:
		d.serveDashboard(w, r)
	}
}

// serveDashboard renders the HTML dashboard
func (d *DashboardHandler) serveDashboard(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="pt-BR">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AgentOS Multi-System Dashboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f7fa;
            color: #333;
            line-height: 1.6;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 2rem;
            text-align: center;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        .header h1 { font-size: 2rem; margin-bottom: 0.5rem; }
        .header p { opacity: 0.9; }
        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 2rem;
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 1.5rem;
            margin-bottom: 2rem;
        }
        .stat-card {
            background: white;
            border-radius: 12px;
            padding: 1.5rem;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            transition: transform 0.2s;
        }
        .stat-card:hover { transform: translateY(-2px); }
        .stat-card h3 {
            font-size: 0.875rem;
            color: #666;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 0.5rem;
        }
        .stat-card .value {
            font-size: 2rem;
            font-weight: bold;
            color: #667eea;
        }
        .systems-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(350px, 1fr));
            gap: 1.5rem;
        }
        .system-card {
            background: white;
            border-radius: 12px;
            padding: 1.5rem;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            border-left: 4px solid #667eea;
        }
        .system-card h2 {
            font-size: 1.25rem;
            margin-bottom: 0.5rem;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        .system-card .status {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            background: #22c55e;
        }
        .system-card .version {
            font-size: 0.875rem;
            color: #666;
            margin-bottom: 1rem;
        }
        .system-card .description {
            color: #555;
            margin-bottom: 1rem;
            font-size: 0.9rem;
        }
        .metrics {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 1rem;
            margin-bottom: 1rem;
        }
        .metric {
            text-align: center;
            padding: 0.75rem;
            background: #f8fafc;
            border-radius: 8px;
        }
        .metric .number {
            font-size: 1.5rem;
            font-weight: bold;
            color: #667eea;
        }
        .metric .label {
            font-size: 0.75rem;
            color: #666;
            text-transform: uppercase;
        }
        .endpoints {
            background: #f8fafc;
            border-radius: 8px;
            padding: 1rem;
            font-family: monospace;
            font-size: 0.8rem;
        }
        .endpoints a {
            color: #667eea;
            text-decoration: none;
            display: block;
            margin: 0.25rem 0;
        }
        .endpoints a:hover { text-decoration: underline; }
        .footer {
            text-align: center;
            padding: 2rem;
            color: #666;
            font-size: 0.875rem;
        }
        .refresh-btn {
            background: #667eea;
            color: white;
            border: none;
            padding: 0.5rem 1rem;
            border-radius: 6px;
            cursor: pointer;
            font-size: 0.875rem;
            margin-bottom: 1rem;
        }
        .refresh-btn:hover { background: #5a67d8; }
        .api-key-section {
            background: #fef3c7;
            border: 1px solid #f59e0b;
            border-radius: 8px;
            padding: 1rem;
            margin-top: 1rem;
        }
        .api-key-section h4 {
            margin-bottom: 0.5rem;
            color: #92400e;
        }
        .key-display {
            font-family: monospace;
            background: white;
            padding: 0.5rem;
            border-radius: 4px;
            word-break: break-all;
        }
        @media (max-width: 768px) {
            .container { padding: 1rem; }
            .header h1 { font-size: 1.5rem; }
            .systems-grid { grid-template-columns: 1fr; }
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>AgentOS Multi-System Dashboard</h1>
        <p>Gerenciamento de múltiplos sistemas</p>
    </div>
    
    <div class="container">
        <div style="text-align: center; margin-bottom: 1rem;">
            <button class="refresh-btn" onclick="location.reload()">Atualizar</button>
            <span style="color: #666;">Última atualização: ` + time.Now().Format("15:04:05") + `</span>
        </div>
        
        <div class="stats-grid">
            <div class="stat-card">
                <h3>Sistemas Ativos</h3>
                <div class="value" id="system-count">0</div>
            </div>
            <div class="stat-card">
                <h3>Total de Operações</h3>
                <div class="value" id="operation-count">0</div>
            </div>
            <div class="stat-card">
                <h3>Total de Entidades</h3>
                <div class="value" id="entity-count">0</div>
            </div>
            <div class="stat-card">
                <h3>Total de Atores</h3>
                <div class="value" id="actor-count">0</div>
            </div>
        </div>
        
        <div class="systems-grid" id="systems-grid">
            <!-- Systems will be loaded here -->
        </div>
        
        <div class="footer">
            <p>AgentOS Dashboard | Auto-generated</p>
        </div>
    </div>
    
    <script>
        const systems = ` + d.getSystemsJSON() + `;
        
        document.getElementById('system-count').textContent = systems.length;
        
        let totalOps = 0, totalEntities = 0, totalActors = 0;
        
        const grid = document.getElementById('systems-grid');
        
        systems.forEach(function(sys) {
            totalOps += sys.operations;
            totalEntities += sys.entities;
            totalActors += sys.actors;
            
            const card = document.createElement('div');
            card.className = 'system-card';
            card.innerHTML = '<h2><span class="status"></span> ' + sys.name + '</h2>' +
                '<div class="version">' + sys.api_name + ' v' + sys.version + '</div>' +
                '<div class="description">' + sys.description + '</div>' +
                '<div class="metrics">' +
                    '<div class="metric">' +
                        '<div class="number">' + sys.operations + '</div>' +
                        '<div class="label">Operações</div>' +
                    '</div>' +
                    '<div class="metric">' +
                        '<div class="number">' + sys.entities + '</div>' +
                        '<div class="label">Entidades</div>' +
                    '</div>' +
                    '<div class="metric">' +
                        '<div class="number">' + sys.actors + '</div>' +
                        '<div class="label">Atores</div>' +
                    '</div>' +
                '</div>' +
                '<div class="endpoints">' +
                    '<strong>Endpoints:</strong><br>' +
                    '<a href="' + sys.prefix + '/_health" target="_blank">Health</a>' +
                    '<a href="' + sys.prefix + '/_system/info" target="_blank">Info</a>' +
                    '<a href="' + sys.prefix + '/_system/actors" target="_blank">Atores</a>' +
                '</div>';
            grid.appendChild(card);
        });
        
        document.getElementById('operation-count').textContent = totalOps;
        document.getElementById('entity-count').textContent = totalEntities;
        document.getElementById('actor-count').textContent = totalActors;
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// serveAPISystems returns systems data as JSON
func (d *DashboardHandler) serveAPISystems(w http.ResponseWriter, r *http.Request) {
	systems := make([]map[string]interface{}, 0, len(d.server.Systems))
	for name, sys := range d.server.Systems {
		systems = append(systems, map[string]interface{}{
			"name":        name,
			"prefix":      sys.Prefix,
			"api_name":    sys.Instance.Manifest.Metadata.Name,
			"version":     sys.Instance.Manifest.Metadata.Version,
			"description": sys.Instance.Manifest.Metadata.Description,
			"operations":  len(sys.Instance.Catalog.ListAll()),
			"entities":    len(sys.Instance.Manifest.DataModel.Entities),
			"actors":      len(sys.Instance.Manifest.Actors),
			"database":    filepath.Base(sys.System.DBConnection),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"systems": systems,
		"count":   len(systems),
	})
}

// serveAPIMetrics returns metrics data
func (d *DashboardHandler) serveAPIMetrics(w http.ResponseWriter, r *http.Request) {
	totalOps := 0
	totalEntities := 0
	totalActors := 0

	for _, sys := range d.server.Systems {
		totalOps += len(sys.Instance.Catalog.ListAll())
		totalEntities += len(sys.Instance.Manifest.DataModel.Entities)
		totalActors += len(sys.Instance.Manifest.Actors)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"systems_count":    len(d.server.Systems),
		"total_operations": totalOps,
		"total_entities":   totalEntities,
		"total_actors":     totalActors,
		"auth_enabled":     d.server.globalAuth.Enabled,
	})
}

// serveAPIKeys handles API key management
func (d *DashboardHandler) serveAPIKeys(w http.ResponseWriter, r *http.Request) {
	// Extract system name from path: /api/keys/{system}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, `{"error":"system not specified"}`, http.StatusBadRequest)
		return
	}

	systemName := parts[3]
	sys, ok := d.server.Systems[systemName]
	if !ok {
		http.Error(w, `{"error":"system not found"}`, http.StatusNotFound)
		return
	}

	switch r.Method {
	case "GET":
		// Return number of keys (not the keys themselves for security)
		sys.apiKeysMu.RLock()
		keyCount := len(sys.apiKeys)
		sys.apiKeysMu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"system":    systemName,
			"key_count": keyCount,
		})

	case "POST":
		// Generate new key
		newKey := generateAPIKey()
		sys.apiKeysMu.Lock()
		sys.apiKeys[newKey] = true
		sys.apiKeysMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"system": systemName,
			"key":    newKey,
			"action": "created",
		})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// getSystemsJSON returns systems data as JSON string for embedding in HTML
func (d *DashboardHandler) getSystemsJSON() string {
	systems := make([]map[string]interface{}, 0, len(d.server.Systems))
	for name, sys := range d.server.Systems {
		systems = append(systems, map[string]interface{}{
			"name":        name,
			"prefix":      sys.Prefix,
			"api_name":    sys.Instance.Manifest.Metadata.Name,
			"version":     sys.Instance.Manifest.Metadata.Version,
			"description": sys.Instance.Manifest.Metadata.Description,
			"operations":  len(sys.Instance.Catalog.ListAll()),
			"entities":    len(sys.Instance.Manifest.DataModel.Entities),
			"actors":      len(sys.Instance.Manifest.Actors),
		})
	}

	data, _ := json.Marshal(systems)
	return string(data)
}
