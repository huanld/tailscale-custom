package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var staticFS embed.FS

var (
	headscaleURL string
	apiKey       string
	listenAddr   string
	dataDir      string
)

type AppUser struct {
	Username  string `json:"username"`
	PassHash  string `json:"passHash"`
	Salt      string `json:"salt"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

type UserDB struct {
	mu    sync.RWMutex
	Users []AppUser `json:"users"`
	path  string
}

func newUserDB(path string) *UserDB {
	db := &UserDB{path: path}
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &db.Users)
	}
	return db
}

func (db *UserDB) Find(username string) *AppUser {
	db.mu.RLock()
	defer db.mu.RUnlock()
	for i := range db.Users {
		if strings.EqualFold(db.Users[i].Username, username) {
			return &db.Users[i]
		}
	}
	return nil
}

func (db *UserDB) Add(u AppUser) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, ex := range db.Users {
		if strings.EqualFold(ex.Username, u.Username) {
			return fmt.Errorf("user already exists")
		}
	}
	db.Users = append(db.Users, u)
	data, _ := json.MarshalIndent(db.Users, "", "  ")
	return os.WriteFile(db.path, data, 0600)
}

func (db *UserDB) Delete(username string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	for i, u := range db.Users {
		if strings.EqualFold(u.Username, username) {
			db.Users = append(db.Users[:i], db.Users[i+1:]...)
			data, _ := json.MarshalIndent(db.Users, "", "  ")
			return os.WriteFile(db.path, data, 0600)
		}
	}
	return fmt.Errorf("user not found")
}

func (db *UserDB) ChangePassword(username, newPassHash, newSalt string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	for i := range db.Users {
		if strings.EqualFold(db.Users[i].Username, username) {
			db.Users[i].PassHash = newPassHash
			db.Users[i].Salt = newSalt
			data, _ := json.MarshalIndent(db.Users, "", "  ")
			return os.WriteFile(db.path, data, 0600)
		}
	}
	return fmt.Errorf("user not found")
}

func (db *UserDB) List() []AppUser {
	db.mu.RLock()
	defer db.mu.RUnlock()
	out := make([]AppUser, len(db.Users))
	copy(out, db.Users)
	return out
}

func hashPassword(password, salt string) string {
	h := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(h[:])
}

func randomSalt() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func verifyPassword(user *AppUser, password string) bool {
	return hashPassword(password, user.Salt) == user.PassHash
}

type Session struct {
	Username string
	Role     string
	Created  time.Time
}

var (
	userDB   *UserDB
	sessions = struct {
		mu   sync.RWMutex
		data map[string]*Session
	}{data: make(map[string]*Session)}
)

func createSession(username, role string) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	sessions.mu.Lock()
	sessions.data[token] = &Session{Username: username, Role: role, Created: time.Now()}
	sessions.mu.Unlock()
	return token
}

func getSession(token string) *Session {
	sessions.mu.RLock()
	defer sessions.mu.RUnlock()
	s := sessions.data[token]
	if s != nil && time.Since(s.Created) > 24*time.Hour {
		return nil
	}
	return s
}

func deleteSession(token string) {
	sessions.mu.Lock()
	delete(sessions.data, token)
	sessions.mu.Unlock()
}

func main() {
	headscaleURL = getEnv("HEADSCALE_URL", "http://localhost:8080")
	apiKey = getEnv("HEADSCALE_API_KEY", "")
	listenAddr = getEnv("LISTEN_ADDR", ":9080")
	dataDir = getEnv("DATA_DIR", "/data")
	if apiKey == "" {
		log.Fatal("HEADSCALE_API_KEY is required")
	}
	headscaleURL = strings.TrimRight(headscaleURL, "/")
	os.MkdirAll(filepath.Join(dataDir, "downloads"), 0755)
	userDB = newUserDB(filepath.Join(dataDir, "users.json"))
	initAdminUser()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", handleLogin)
	mux.HandleFunc("/api/auth/logout", handleLogout)
	mux.HandleFunc("/api/auth/me", handleMe)
	mux.HandleFunc("/api/auth/password", requireAuth(handleChangePassword))
	mux.HandleFunc("/api/admin/nodes", requireAdmin(handleAdminNodes))
	mux.HandleFunc("/api/admin/nodes/", requireAdmin(handleAdminNodeByID))
	mux.HandleFunc("/api/admin/register", requireAdmin(handleAdminRegister))
	mux.HandleFunc("/api/admin/users", requireAdmin(handleAdminUsers))
	mux.HandleFunc("/api/admin/users/", requireAdmin(handleAdminUserByID))
	mux.HandleFunc("/api/admin/accounts", requireAdmin(handleAdminAccounts))
	mux.HandleFunc("/api/admin/accounts/", requireAdmin(handleAdminAccountByID))
	mux.HandleFunc("/api/admin/preauthkeys", requireAdmin(handleAdminKeys))
	mux.HandleFunc("/api/admin/routes", requireAdmin(handleAdminRoutes))
	mux.HandleFunc("/api/admin/routes/", requireAdmin(handleAdminRouteByID))
	mux.HandleFunc("/api/user/nodes", requireAuth(handleUserNodes))
	mux.HandleFunc("/api/user/register", requireAuth(handleUserRegister))
	mux.HandleFunc("/download/", handleDownload)

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticSub)))
	log.Printf("Headscale Web Admin starting on %s", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func initAdminUser() {
	if userDB.Find("admin") != nil {
		return
	}
	adminPass := getEnv("ADMIN_PASSWORD", "admin123")
	salt := randomSalt()
	userDB.Add(AppUser{
		Username:  "admin",
		PassHash:  hashPassword(adminPass, salt),
		Salt:      salt,
		Role:      "admin",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	log.Println("Created default admin user")
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		s := getSession(token)
		if s == nil {
			jsonResp(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		r.Header.Set("X-Username", s.Username)
		r.Header.Set("X-Role", s.Role)
		next(w, r)
	}
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Role") != "admin" {
			jsonResp(w, http.StatusForbidden, map[string]string{"error": "admin only"})
			return
		}
		next(w, r)
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	req.Username = strings.TrimSpace(req.Username)
	user := userDB.Find(req.Username)
	if user == nil || !verifyPassword(user, req.Password) {
		jsonResp(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	token := createSession(user.Username, user.Role)
	jsonResp(w, http.StatusOK, map[string]interface{}{
		"ok": true, "token": token, "username": user.Username, "role": user.Role,
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	deleteSession(token)
	jsonResp(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	s := getSession(token)
	if s == nil {
		jsonResp(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	jsonResp(w, http.StatusOK, map[string]string{"username": s.Username, "role": s.Role})
}

func handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	username := r.Header.Get("X-Username")
	user := userDB.Find(username)
	if user == nil || !verifyPassword(user, req.OldPassword) {
		jsonResp(w, http.StatusBadRequest, map[string]string{"error": "wrong current password"})
		return
	}
	if len(req.NewPassword) < 4 {
		jsonResp(w, http.StatusBadRequest, map[string]string{"error": "password too short (min 4)"})
		return
	}
	salt := randomSalt()
	userDB.ChangePassword(username, hashPassword(req.NewPassword, salt), salt)
	jsonResp(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		users := userDB.List()
		type safeUser struct {
			Username  string `json:"username"`
			Role      string `json:"role"`
			CreatedAt string `json:"createdAt"`
		}
		out := make([]safeUser, len(users))
		for i, u := range users {
			out[i] = safeUser{Username: u.Username, Role: u.Role, CreatedAt: u.CreatedAt}
		}
		jsonResp(w, http.StatusOK, map[string]interface{}{"accounts": out})
	case http.MethodPost:
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" || req.Password == "" {
			jsonResp(w, http.StatusBadRequest, map[string]string{"error": "username and password required"})
			return
		}
		if len(req.Password) < 4 {
			jsonResp(w, http.StatusBadRequest, map[string]string{"error": "password too short (min 4)"})
			return
		}
		if req.Role == "" {
			req.Role = "user"
		}
		if req.Role != "admin" && req.Role != "user" {
			jsonResp(w, http.StatusBadRequest, map[string]string{"error": "role must be admin or user"})
			return
		}
		salt := randomSalt()
		err := userDB.Add(AppUser{
			Username: req.Username, PassHash: hashPassword(req.Password, salt),
			Salt: salt, Role: req.Role, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			jsonResp(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if req.Role == "user" {
			body, _ := json.Marshal(map[string]string{"name": req.Username})
			hReq, _ := http.NewRequest(http.MethodPost, headscaleURL+"/api/v1/user", bytes.NewReader(body))
			hReq.Header.Set("Authorization", "Bearer "+apiKey)
			hReq.Header.Set("Content-Type", "application/json")
			httpClient.Do(hReq)
		}
		jsonResp(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAdminAccountByID(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/admin/accounts/")
	parts := strings.SplitN(username, "/", 2)
	username = parts[0]
	if len(parts) == 2 && parts[1] == "password" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Password) < 4 {
			jsonResp(w, http.StatusBadRequest, map[string]string{"error": "password too short (min 4)"})
			return
		}
		salt := randomSalt()
		if err := userDB.ChangePassword(username, hashPassword(req.Password, salt), salt); err != nil {
			jsonResp(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		jsonResp(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if r.Method == http.MethodDelete {
		if strings.EqualFold(username, "admin") {
			jsonResp(w, http.StatusBadRequest, map[string]string{"error": "cannot delete admin"})
			return
		}
		// Foreign key check: can't delete account if Headscale user has nodes
		nodeCount, err := getUserNodeCount(username)
		if err != nil {
			httpError(w, "failed to check nodes", err)
			return
		}
		if nodeCount > 0 {
			jsonResp(w, http.StatusConflict, map[string]string{
				"error": fmt.Sprintf("Cannot delete account '%s': has %d active node(s). Delete all nodes first.", username, nodeCount),
			})
			return
		}
		if err := userDB.Delete(username); err != nil {
			jsonResp(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		// Also delete Headscale user
		doHeadscaleRequest(http.MethodDelete, fmt.Sprintf("/api/v1/user/%s", username), nil)
		jsonResp(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func handleAdminNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		proxyGet(w, "/api/v1/node")
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAdminNodeByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/nodes/")
	parts := strings.SplitN(path, "/", 2)
	nodeID := parts[0]
	if len(parts) == 2 {
		switch parts[1] {
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
	if r.Method == http.MethodDelete {
		proxyDelete(w, fmt.Sprintf("/api/v1/node/%s", nodeID))
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAdminRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		User string `json:"user"`
		Key  string `json:"key"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.User == "" || req.Key == "" {
		jsonResp(w, http.StatusBadRequest, map[string]string{"error": "user and key required"})
		return
	}
	url := fmt.Sprintf("%s/api/v1/node/register?user=%s&key=%s", headscaleURL, req.User, req.Key)
	hReq, _ := http.NewRequest(http.MethodPost, url, nil)
	hReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := httpClient.Do(hReq)
	if err != nil {
		httpError(w, "headscale API error", err)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func getUserNodeCount(username string) (int, error) {
	req, _ := http.NewRequest(http.MethodGet, headscaleURL+"/api/v1/node", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var result struct {
		Nodes []struct {
			User struct {
				Name string `json:"name"`
			} `json:"user"`
		} `json:"nodes"`
	}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)
	count := 0
	for _, n := range result.Nodes {
		if strings.EqualFold(n.User.Name, username) {
			count++
		}
	}
	return count, nil
}

func doHeadscaleRequest(method, path string, body []byte) (int, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, headscaleURL+path, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}

func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		proxyGet(w, "/api/v1/user")
	case http.MethodPost:
		var req struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			jsonResp(w, http.StatusBadRequest, map[string]string{"error": "name required"})
			return
		}
		if req.Password != "" && len(req.Password) < 4 {
			jsonResp(w, http.StatusBadRequest, map[string]string{"error": "password too short (min 4)"})
			return
		}
		// Check if login account already exists
		if userDB.Find(req.Name) != nil {
			jsonResp(w, http.StatusConflict, map[string]string{"error": "account already exists"})
			return
		}
		// Create Headscale user
		body, _ := json.Marshal(map[string]string{"name": req.Name})
		status, respBody, err := doHeadscaleRequest(http.MethodPost, "/api/v1/user", body)
		if err != nil {
			httpError(w, "headscale API error", err)
			return
		}
		if status >= 400 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			w.Write(respBody)
			return
		}
		// Also create login account
		if req.Password != "" {
			salt := randomSalt()
			userDB.Add(AppUser{
				Username:  req.Name,
				PassHash:  hashPassword(req.Password, salt),
				Salt:      salt,
				Role:      "user",
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(respBody)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAdminUserByID(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	parts := strings.SplitN(name, "/", 2)
	name = parts[0]

	// Handle /api/admin/users/{name}/password
	if len(parts) == 2 && parts[1] == "password" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Password) < 4 {
			jsonResp(w, http.StatusBadRequest, map[string]string{"error": "password too short (min 4)"})
			return
		}
		user := userDB.Find(name)
		if user == nil {
			jsonResp(w, http.StatusNotFound, map[string]string{"error": "login account not found for this user"})
			return
		}
		salt := randomSalt()
		if err := userDB.ChangePassword(name, hashPassword(req.Password, salt), salt); err != nil {
			jsonResp(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		jsonResp(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if r.Method == http.MethodDelete {
		// Foreign key check: can't delete user with active nodes
		nodeCount, err := getUserNodeCount(name)
		if err != nil {
			httpError(w, "failed to check nodes", err)
			return
		}
		if nodeCount > 0 {
			jsonResp(w, http.StatusConflict, map[string]string{
				"error": fmt.Sprintf("Cannot delete user '%s': has %d active node(s). Delete all nodes first.", name, nodeCount),
			})
			return
		}
		// Delete from Headscale
		status, respBody, err := doHeadscaleRequest(http.MethodDelete, fmt.Sprintf("/api/v1/user/%s", name), nil)
		if err != nil {
			httpError(w, "headscale API error", err)
			return
		}
		if status < 400 {
			// Also delete login account if exists
			userDB.Delete(name)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(respBody)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func handleAdminKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		user := r.URL.Query().Get("user")
		u := "/api/v1/preauthkey"
		if user != "" {
			u += "?user=" + user
		}
		proxyGet(w, u)
	case http.MethodPost:
		proxyPost(w, r, "/api/v1/preauthkey")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAdminRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		proxyGet(w, "/api/v1/routes")
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAdminRouteByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/routes/")
	parts := strings.SplitN(path, "/", 2)
	routeID := parts[0]
	if len(parts) == 2 {
		switch parts[1] {
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

func handleUserNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := r.Header.Get("X-Username")
	u := headscaleURL + "/api/v1/node"
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := httpClient.Do(req)
	if err != nil {
		httpError(w, "headscale API error", err)
		return
	}
	defer resp.Body.Close()
	var result struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)
	var userNodes []json.RawMessage
	for _, raw := range result.Nodes {
		var node struct {
			User struct {
				Name string `json:"name"`
			} `json:"user"`
		}
		json.Unmarshal(raw, &node)
		if strings.EqualFold(node.User.Name, username) {
			userNodes = append(userNodes, raw)
		}
	}
	if userNodes == nil {
		userNodes = []json.RawMessage{}
	}
	jsonResp(w, http.StatusOK, map[string]interface{}{"nodes": userNodes})
}

func handleUserRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := r.Header.Get("X-Username")
	var req struct {
		Key string `json:"key"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Key == "" {
		jsonResp(w, http.StatusBadRequest, map[string]string{"error": "key required"})
		return
	}
	u := fmt.Sprintf("%s/api/v1/node/register?user=%s&key=%s", headscaleURL, username, req.Key)
	hReq, _ := http.NewRequest(http.MethodPost, u, nil)
	hReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := httpClient.Do(hReq)
	if err != nil {
		httpError(w, "headscale API error", err)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(r.URL.Path, "/download/")
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		dir := filepath.Join(dataDir, "downloads")
		entries, _ := os.ReadDir(dir)
		var files []map[string]interface{}
		for _, e := range entries {
			if !e.IsDir() {
				info, _ := e.Info()
				files = append(files, map[string]interface{}{"name": e.Name(), "size": info.Size()})
			}
		}
		if files == nil {
			files = []map[string]interface{}{}
		}
		jsonResp(w, http.StatusOK, map[string]interface{}{"files": files})
		return
	}
	path := filepath.Join(dataDir, "downloads", filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeFile(w, r, path)
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

func proxyGet(w http.ResponseWriter, path string) {
	req, err := http.NewRequest(http.MethodGet, headscaleURL+path, nil)
	if err != nil {
		httpError(w, "request error", err)
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
	body, _ := io.ReadAll(r.Body)
	req, err := http.NewRequest(http.MethodPost, headscaleURL+path, bytes.NewReader(body))
	if err != nil {
		httpError(w, "request error", err)
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
	req, err := http.NewRequest(http.MethodDelete, headscaleURL+path, nil)
	if err != nil {
		httpError(w, "request error", err)
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
	jsonResp(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("%s: %v", msg, err)})
}

func jsonResp(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
