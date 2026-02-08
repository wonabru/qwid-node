package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

type UserEntry struct {
	PasswordHash string `json:"password_hash"`
	WalletDir    string `json:"wallet_dir"`
	Address      string `json:"address"`
	CreatedAt    string `json:"created_at"`
}

type UserRegistry struct {
	mu       sync.RWMutex
	users    map[string]*UserEntry
	filePath string
}

var Users *UserRegistry

func InitUserRegistry(basePath string) error {
	Users = &UserRegistry{
		users:    make(map[string]*UserEntry),
		filePath: filepath.Join(basePath, "users.json"),
	}
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return err
	}
	return Users.load()
}

func (u *UserRegistry) load() error {
	data, err := os.ReadFile(u.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &u.users)
}

func (u *UserRegistry) save() error {
	data, err := json.MarshalIndent(u.users, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(u.filePath, data, 0600)
}

func (u *UserRegistry) Exists(username string) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	_, ok := u.users[username]
	return ok
}

func (u *UserRegistry) Create(username, password, walletDir, address string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if _, ok := u.users[username]; ok {
		return fmt.Errorf("user already exists")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return err
	}

	u.users[username] = &UserEntry{
		PasswordHash: string(hash),
		WalletDir:    walletDir,
		Address:      address,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	return u.save()
}

func (u *UserRegistry) Authenticate(username, password string) (*UserEntry, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	entry, ok := u.users[username]
	if !ok {
		// Still run bcrypt to prevent timing attacks
		bcrypt.CompareHashAndPassword([]byte("$2a$12$000000000000000000000000000000000000000000000000000000"), []byte(password))
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(entry.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return entry, nil
}

func (u *UserRegistry) GetEntry(username string) *UserEntry {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.users[username]
}

func UserWalletDir(basePath, username string) string {
	return filepath.Join(basePath, "users", username)
}

func bcryptHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
