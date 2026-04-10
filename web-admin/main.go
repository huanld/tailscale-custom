package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

//go:embed static/*
var staticFS embed.FS

var (
	headscaleURL string
	apiKey       string
	listenAddr   string
	adminPass    string
)

func main() {
	headscaleURL = getEnv("HEADSCALE_URL", "http://localhost:8080")
	apiKey = getEnv("HEADSCALE_API_KEY", "")
	listenAddr = getEnv("LISTEN_ADDR", ":9080")
	adminPass = getEnv("ADMIN_PASSWORD", "")

	if apiKey == "" {
		log.Fatal("HEADSCALE_API_KEY is required")
	}

	headscaleURL = strings.TrimRight(headscaleURL, "/")

	mux := http.NewServeMux()

	// API routes - proxy to headscale
	mux.HandleFunc("/api/nodes", authMiddleware(handleNodes))
	mux.HandleFunc("/api/nodes/", authMiddleware(handleNodeByID))
	mux.HandleFunc("/api/users", authMiddleware(handleUsers))
	mux.HandleFunc("/api/users/", authMiddleware(handleUserByName))
	mux.HandleFunc("/api/preauthkeys", authMiddleware(handlePreauthKeys))
	mux.HandleFunc("/api/routes", authMiddleware(handleRoutes))
	mux.HandleFunc("/api/routes/", authMiddleware(handleRouteByID))
	mux.HandleFunc("/api/auth", handleAuth)

	// Static files
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	log.Printf("Headscale Web Admin starting on %s", listenAddr)
	log.Printf("Headscale API: %s", headscaleURL)
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Simple auth check
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if adminPass != "" {
			token := r.Header.Get("X-Admin-Token")
			if token != adminPass {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if adminPass == "" || req.Password == adminPass {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "wrong password"})
	}
}

// --- Nodes ---

func handleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		proxyGet(w, "/api/v1/node")
	case http.MethodPost:
		// Register node
		proxyPost(w, r, "/api/v1/node/register")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleNodeByID(w http.ResponseWriter, r *http.Request) {
	// /api/nodes/{id} or /api/nodes/{id}/action
	path := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
	parts := strings.SplitN(path, "/", 2)
	nodeID := parts[0]

	if len(parts) == 2 {
		action := parts[1]
		switch action {
		case "expire":
			proxyPost(w, r, fmt.Sprintf("/api/v1/node/%s/expire", nodeID))
		case "rename":
			proxyPost(w, r, fmt.Sprintf("/api/v1/node/%s/rename", nodeID))
		case "routes":
			proxyGet(w, fmt.Sprintf("/api/v1/node/%s/routes", nodeID))
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		proxyGet(w, fmt.Sprintf("/api/v1/node/%s", nodeID))
	case http.MethodDelete:
		proxyDelete(w, fmt.Sprintf("/api/v1/node/%s", nodeID))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Users ---

func handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		proxyGet(w, "/api/v1/user")
	case http.MethodPost:
		proxyPost(w, r, "/api/v1/user")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleUserByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if r.Method == http.MethodDelete {
		proxyDelete(w, fmt.Sprintf("/api/v1/user/%s", name))
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Pre-auth Keys ---

func handlePreauthKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		user := r.URL.Query().Get("user")
		url := "/api/v1/preauthkey"
		if user != "" {
			url += "?user=" + user
		}
		proxyGet(w, url)
	case http.MethodPost:
		proxyPost(w, r, "/api/v1/preauthkey")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Routes ---

func handleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		proxyGet(w, "/api/v1/routes")
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleRouteByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/routes/")
	parts := strings.SplitN(path, "/", 2)
	routeID := parts[0]

	if len(parts) == 2 {
		action := parts[1]
		switch action {
		case "enable":
			proxyPost(w, r, fmt.Sprintf("/api/v1/routes/%s/enable", routeID))
		case "disable":
			proxyPost(w, r, fmt.Sprintf("/api/v1/routes/%s/disable", routeID))
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
		}
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// --- Proxy helpers ---

var httpClient = &http.Client{Timeout: 15 * time.Second}

func proxyGet(w http.ResponseWriter, path string) {
	url := headscaleURL + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		httpError(w, "failed to create request", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		httpError(w, "headscale API error", err)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func proxyPost(w http.ResponseWriter, r *http.Request, path string) {
	url := headscaleURL + path
	body, _ := io.ReadAll(r.Body)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		httpError(w, "failed to create request", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		httpError(w, "headscale API error", err)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func proxyDelete(w http.ResponseWriter, path string) {
	url := headscaleURL + path
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		httpError(w, "failed to create request", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		httpError(w, "headscale API error", err)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func httpError(w http.ResponseWriter, msg string, err error) {
	log.Printf("ERROR: %s: %v", msg, err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("%s: %v", msg, err)})
}
