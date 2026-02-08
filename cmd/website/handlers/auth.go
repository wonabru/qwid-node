package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/wonabru/qwid-node/wallet"
)

// Rate limiting
type rateLimiter struct {
	mu      sync.Mutex
	entries map[string][]time.Time
}

var (
	registerLimiter = &rateLimiter{entries: make(map[string][]time.Time)}
	loginLimiter    = &rateLimiter{entries: make(map[string][]time.Time)}
)

func (rl *rateLimiter) allow(ip string, maxCount int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-window)

	// Clean old entries
	var valid []time.Time
	for _, t := range rl.entries[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	rl.entries[ip] = valid

	if len(valid) >= maxCount {
		return false
	}
	rl.entries[ip] = append(rl.entries[ip], now)
	return true
}

func getClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	return r.RemoteAddr
}

func Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		JsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := getClientIP(r)
	if !registerLimiter.allow(ip, 5, 10*time.Minute) {
		JsonError(w, "Too many registration attempts. Try again later.", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Username) < 3 || len(req.Username) > 32 {
		JsonError(w, "Username must be 3-32 characters", http.StatusBadRequest)
		return
	}
	if len(req.Password) < 6 {
		JsonError(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	// Check for valid characters in username
	for _, c := range req.Username {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			JsonError(w, "Username can only contain letters, numbers, underscores, and hyphens", http.StatusBadRequest)
			return
		}
	}

	if Users.Exists(req.Username) {
		JsonError(w, "Username already taken", http.StatusConflict)
		return
	}

	// Create user wallet directory
	walletDir := UserWalletDir(WebsiteBasePath, req.Username)
	if err := os.MkdirAll(walletDir, 0755); err != nil {
		JsonError(w, "Failed to create wallet directory", http.StatusInternalServerError)
		return
	}

	// Create wallet
	wl := wallet.EmptyWallet(0, SigName, SigName2)
	wl.HomePath = walletDir
	wl.SetPassword(req.Password)
	wl.Iv = wallet.GenerateNewIv()

	acc, err := wallet.GenerateNewAccount(wl, wl.SigName)
	if err != nil {
		JsonError(w, fmt.Sprintf("Failed to generate primary account: %v", err), http.StatusInternalServerError)
		return
	}
	wl.MainAddress = acc.Address
	acc.PublicKey.MainAddress = wl.MainAddress
	wl.Account1 = acc
	copy(wl.Account1.EncryptedSecretKey, acc.EncryptedSecretKey)

	acc, err = wallet.GenerateNewAccount(wl, wl.SigName2)
	if err != nil {
		JsonError(w, fmt.Sprintf("Failed to generate secondary account: %v", err), http.StatusInternalServerError)
		return
	}
	wl.Account2 = acc
	copy(wl.Account2.EncryptedSecretKey, acc.EncryptedSecretKey)

	if err := wl.StoreJSON(); err != nil {
		JsonError(w, fmt.Sprintf("Failed to store wallet: %v", err), http.StatusInternalServerError)
		return
	}

	address := wl.MainAddress.GetHex()

	// Register user
	if err := Users.Create(req.Username, req.Password, walletDir, address); err != nil {
		JsonError(w, fmt.Sprintf("Failed to register user: %v", err), http.StatusInternalServerError)
		return
	}

	JsonResponse(w, map[string]interface{}{
		"success": true,
		"address": address,
		"message": "Account created successfully. Please login.",
	})
}

func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		JsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := getClientIP(r)
	if !loginLimiter.allow(ip, 10, 10*time.Minute) {
		JsonError(w, "Too many login attempts. Try again later.", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	entry, err := Users.Authenticate(req.Username, req.Password)
	if err != nil {
		JsonError(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	// Load user's wallet
	userWallet, err := loadUserWallet(entry.WalletDir, req.Password)
	if err != nil {
		JsonError(w, fmt.Sprintf("Failed to load wallet: %v", err), http.StatusInternalServerError)
		return
	}

	token, err := Sessions.Create(req.Username, userWallet)
	if err != nil {
		JsonError(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	Sessions.SetCookie(w, token)

	JsonResponse(w, map[string]interface{}{
		"success":  true,
		"username": req.Username,
		"address":  entry.Address,
	})
}

func Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		JsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		Sessions.Delete(cookie.Value)
	}
	Sessions.ClearCookie(w)

	JsonResponse(w, map[string]string{"success": "true"})
}

func GetSessionInfo(w http.ResponseWriter, r *http.Request) {
	sess := GetSession(r.Context())
	if sess == nil {
		JsonError(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	entry := Users.GetEntry(sess.Username)
	address := ""
	if entry != nil {
		address = entry.Address
	}

	JsonResponse(w, map[string]interface{}{
		"username": sess.Username,
		"address":  address,
	})
}

func loadUserWallet(walletDir, password string) (*wallet.Wallet, error) {
	// Build the wallet path the same way wallet.LoadJSON does internally,
	// but we need to load from a custom directory.
	// wallet.LoadJSON uses EmptyWallet to determine HomePath, which is based on walletNumber.
	// We'll use LoadJSON but first we need the wallet at the custom path.
	// Since LoadJSON reads from HomePath derived from walletNumber, we need to
	// read the JSON file directly and reconstruct.

	walletFile := walletDir + "/wallet0.json"
	data, err := os.ReadFile(walletFile)
	if err != nil {
		return nil, fmt.Errorf("wallet file not found: %v", err)
	}

	var wl wallet.Wallet
	if err := json.Unmarshal(data, &wl); err != nil {
		return nil, fmt.Errorf("failed to parse wallet: %v", err)
	}

	// We need to fully load the wallet with decrypted keys
	// Temporarily override HomePath, set password, and decrypt
	wl.HomePath = walletDir
	wl.SetPassword(password)

	// Re-load properly using the wallet package's LoadJSON by temporarily
	// creating a symlink approach won't work. Instead, since we have the JSON
	// and password, we use the wallet's built-in decryption.
	// The wallet struct has encrypted keys that need decrypting.
	// We'll call LoadJSON with a custom approach.

	// Actually, let's just write a helper that mimics LoadJSON's decryption
	// but from a custom path. The simplest approach: save a copy at the
	// standard path, load it, then move it back. But that's messy.

	// Better: call the wallet package functions directly.
	// We need to call wl.decrypt on the encrypted secret keys and init the signers.
	// But decrypt is unexported. Let's use a different approach:
	// Temporarily create the wallet at walletNumber=0's standard path, load, restore.

	// Simplest correct approach: use wallet.LoadJSON by ensuring the file
	// exists where it expects. The user's wallet IS at walletDir/wallet0.json.
	// wallet.LoadJSON(0, password, sigName, sigName2) looks at:
	//   homePath + "/.qwid/wallet/0" + "/wallet0.json"
	// But our wallet is at walletDir/wallet0.json.

	// We can create a temporary symlink from the expected path to walletDir.
	// Actually, let's just directly load from the expected path by using a
	// different wallet number that won't conflict. Or better yet, let's
	// look at what LoadJSON actually does and replicate the key parts.

	// LoadJSON calls EmptyWallet(walletNumber) which sets HomePath to
	// homePath + "/.qwid/wallet/" + strconv.Itoa(walletNumber)
	// then reads from filepath.Join(homePath, "wallet"+strconv.Itoa(walletNumber)+".json")
	// Note: HomePath is both the directory AND used to construct the filename.

	// The actual wallet file for user is at: walletDir/wallet0.json
	// LoadJSON expects it at: ~/.qwid/wallet/0/wallet0.json

	// Cleanest approach: symlink walletDir as ~/.qwid/wallet/<tempNum>
	// Or: just copy the file temporarily. Both are hacky.

	// Best approach: read the file, unmarshal, set password, and use the
	// wallet's exported Sign method (which uses the signer that needs init).
	// The signer needs the decrypted secret key. decrypt() is unexported.

	// Actually, the cleanest way is to symlink the user's wallet directory
	// to a temp wallet number path, call LoadJSON, then remove the symlink.

	homePath, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// Use a high wallet number unlikely to conflict (200+)
	// Create symlink from standard path to user's wallet dir
	standardPath := fmt.Sprintf("%s/.qwid/wallet/%d", homePath, 200)

	// Clean up any existing symlink
	os.Remove(standardPath)

	if err := os.Symlink(walletDir, standardPath); err != nil {
		return nil, fmt.Errorf("failed to link wallet: %v", err)
	}
	defer os.Remove(standardPath)

	loadedWallet, err := wallet.LoadJSON(200, password, SigName, SigName2)
	if err != nil {
		return nil, err
	}

	// Restore the correct HomePath
	loadedWallet.HomePath = walletDir

	return loadedWallet, nil
}
