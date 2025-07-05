package wallet

import (
	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/logger"
	"testing"

	"github.com/stretchr/testify/assert"
)

var mainWallet *Wallet
var err error
var password = "a"

func Init() {
	mainWallet, err = GenerateNewWallet(255, password)
	if err != nil {
		return
	}

	mainWallet, err = Load(255, password)
	if err != nil {
		logger.GetLogger().Fatalf("cannot load wallet")
	}
}

func TestGenerateNewWallet(t *testing.T) {
	Init()
	password := "testpassword"
	w, err := GenerateNewWallet(255, password)
	assert.NoError(t, err)
	assert.NotNil(t, w)
	assert.Equal(t, password, w.password)
	assert.NotNil(t, w.passwordBytes)
	assert.NotNil(t, w.Iv)
	assert.NotNil(t, w.secretKey)
	assert.NotNil(t, w.PublicKey)
	assert.NotNil(t, w.Address)
	assert.NotNil(t, w.signer)
}
func TestStoreAndLoadWallet(t *testing.T) {
	Init()
	// Generate a new wallet
	password := "testpassword"
	wallet, err := GenerateNewWallet(255, password)
	assert.NoError(t, err)
	// Put the wallet
	err = wallet.Store(true)
	assert.NoError(t, err)
	// Get the wallet
	loadedWallet, err := Load(255, password)
	if err != nil {
		return
	}
	assert.NoError(t, err)
	// Check if the loaded wallet is the same as the original wallet
	assert.Equal(t, wallet.PublicKey, loadedWallet.PublicKey)
	assert.Equal(t, wallet.Address, loadedWallet.Address)
	assert.Equal(t, wallet.secretKey, loadedWallet.secretKey)
}
func TestStoreAndLoadWalletJSON(t *testing.T) {
	//Init()
	// Generate a new wallet
	password := "xxiii"
	wallet, err := GenerateNewWallet(255, password)
	assert.NoError(t, err)
	// Put the wallet
	err = wallet.StoreJSON(false)
	assert.NoError(t, err)
	// Get the wallet
	loadedWallet, err := LoadJSON(255, password)
	if err != nil {
		return
	}
	assert.NoError(t, err)
	// Check if the loaded wallet is the same as the original wallet
	assert.Equal(t, wallet.PublicKey, loadedWallet.PublicKey)
	assert.Equal(t, wallet.Address, loadedWallet.Address)
	assert.Equal(t, wallet.secretKey, loadedWallet.secretKey)
}
func TestChangePassword(t *testing.T) {
	Init()
	// Generate a new wallet
	password := "testpassword"
	newPassword := "newtestpassword"
	wallet, err := GenerateNewWallet(255, password)
	assert.NoError(t, err)
	// Change the password
	err = wallet.ChangePassword(password, newPassword)
	assert.NoError(t, err)

	loadedWallet, err := Load(255, newPassword)
	if err != nil {
		return
	}
	assert.NoError(t, err)
	// Check if the loaded wallet is the same as the original wallet
	assert.Equal(t, wallet.PublicKey, loadedWallet.PublicKey)
	assert.Equal(t, wallet.Address, loadedWallet.Address)
	assert.Equal(t, wallet.secretKey, loadedWallet.secretKey)
}
func TestSignAndVerify(t *testing.T) {
	// Generate a new wallet
	password := "testpassword"
	wallet, err := GenerateNewWallet(255, password)
	assert.NoError(t, err)
	// Sign a message using the wallet
	message := []byte("Hello, world!")
	signature, err := wallet.Sign(message, true)
	if err != nil {
		logger.GetLogger().Fatal(err)
	}
	// Verify the signature using the wallet's public key
	isVerified := Verify(message, signature.GetBytes(), wallet.PublicKey.GetBytes(), common.SigName(), common.SigName2())
	assert.Equal(t, isVerified, true)
}
