package wallet

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/wonabru/qwid-node/logger"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wonabru/bip39"
	"io"
	"sync"

	"github.com/wonabru/qwid-node/common"
	"github.com/wonabru/qwid-node/crypto/oqs"
	"golang.org/x/crypto/sha3"
)

var globalMutex sync.RWMutex

type Account struct {
	secretKey          common.PrivKey
	EncryptedSecretKey []byte         `json:"encrypted_secret_key"`
	PublicKey          common.PubKey  `json:"public_key"`
	Address            common.Address `json:"address"`
	signer             oqs.Signature
}

// Wallet Structure map of Height and wallet which was change
type Wallet struct {
	password      string
	passwordBytes []byte
	Iv            []byte             `json:"iv"`
	HomePath      string             `json:"home_path"`
	WalletNumber  uint8              `json:"wallet_number"`
	MainAddress   common.Address     `json:"main_address"`
	SigName       string             `json:"sig_name"`
	SigName2      string             `json:"sig_name_2"`
	Account1      Account            `json:"account_1"`
	Account2      Account            `json:"account_2"`
	Accounts      map[string]Account `json:"accounts"`
}

var activeWallet *Wallet

type AnyWallet interface {
	GetWallet() Wallet
}

func InitActiveWallet(walletNumber uint8, password string, sigName, sigName2 string) {
	var err error
	w, err := LoadJSON(walletNumber, password, sigName, sigName2)
	activeWallet = w
	if err != nil {
		logger.GetLogger().Println("wrong password", err)
		os.Exit(1)
	}
	if activeWallet == nil {
		logger.GetLogger().Println("failed to load wallet")
		os.Exit(1)
	}
}

func GetCurrentWallet(sigName, sigName2 string) (*Wallet, error) {
	aw := GetActiveWallet()
	var err error
	w, err := LoadJSON(aw.WalletNumber, aw.password, sigName, sigName2)
	currentWallet := w
	if err != nil {
		logger.GetLogger().Println("wrong password: GetCurrentWallet")
		return nil, err
	}
	if currentWallet == nil {
		logger.GetLogger().Println("failed to load wallet: GetCurrentWallet")
		return nil, fmt.Errorf("failed to load wallet: GetCurrentWallet")
	}
	return currentWallet, nil
}

func (w *Wallet) SetPassword(password string) {
	(*w).password = password
	(*w).passwordBytes = passwordToByte(password)
}

func GetActiveWallet() *Wallet {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	return activeWallet
}

func SetActiveWallet(w *Wallet) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	activeWallet = w
}

func (w *Wallet) ShowInfo() string {

	s := fmt.Sprintln("Length of public key:", w.Account1.PublicKey.GetLength())
	s += fmt.Sprintln("Beginning of public key:", w.Account1.PublicKey.GetHex()[:10])
	s += fmt.Sprintln("Address:", w.Account1.Address.GetHex())
	s += fmt.Sprintln("Length of private key:", w.GetSecretKey().GetLength())
	s += fmt.Sprintln("Length of public key 2:", w.Account2.PublicKey.GetLength())
	s += fmt.Sprintln("Beginning of public key 2:", w.Account2.PublicKey.GetHex()[:10])
	s += fmt.Sprintln("Address 2:", w.Account2.Address.GetHex())
	s += fmt.Sprintln("Length of private key 2:", w.GetSecretKey2().GetLength())
	s += fmt.Sprintln("MainAddress:", w.MainAddress.GetHex())
	s += fmt.Sprintln("Wallet location", w.HomePath)
	s += fmt.Sprintln("Wallet Number", w.WalletNumber)
	fmt.Println(s)
	return s
}

func passwordToByte(password string) []byte {
	sh := make([]byte, 32)
	sha3.ShakeSum256(sh, []byte(password))
	return sh
}

func (w *Wallet) GetSigName(primary bool) string {
	if primary {
		return w.SigName
	} else {
		return w.SigName2
	}
}

func EmptyWallet(walletNumber uint8, sigName, sigName2 string) Wallet {
	homePath, err := os.UserHomeDir()
	if err != nil {
		logger.GetLogger().Fatal(err)
	}

	return Wallet{
		password:      "",
		passwordBytes: nil,
		Iv:            nil,
		Account1:      EmptyAccount(),
		Account2:      EmptyAccount(),
		SigName:       sigName,
		SigName2:      sigName2,
		Accounts:      map[string]Account{},
		MainAddress:   common.Address{},
		HomePath:      homePath + common.DefaultWalletHomePath + strconv.Itoa(int(walletNumber)),
		WalletNumber:  walletNumber,
	}
}

func EmptyAccount() Account {
	return Account{
		secretKey:          common.PrivKey{},
		EncryptedSecretKey: nil,
		PublicKey:          common.PubKey{},
		Address:            common.Address{},
		signer:             oqs.Signature{},
	}
}

func GenerateNewAccount(w Wallet, sigName string) (Account, error) {
	if len(w.password) < 1 {
		return Account{}, fmt.Errorf("password cannot be empty")
	}

	var signer oqs.Signature

	// ignore potential errors everywhere
	err := signer.Init(sigName, nil)
	if err != nil {
		return Account{}, err
	}
	pubKey, err := signer.GenerateKeyPair()
	if err != nil {
		return Account{}, err
	}

	acc := EmptyAccount()
	err = acc.PublicKey.Init(pubKey, w.MainAddress)
	if err != nil {
		return Account{}, err
	}
	acc.Address = acc.PublicKey.GetAddress()

	err = acc.secretKey.Init(signer.ExportSecretKey(), acc.Address, false)
	if err != nil {
		return Account{}, err
	}
	acc.signer = signer

	se, err := w.encrypt(acc.secretKey.GetBytes())
	if err != nil {
		logger.GetLogger().Println(err)
		return Account{}, err
	}
	acc.EncryptedSecretKey = make([]byte, len(se))
	copy(acc.EncryptedSecretKey, se)

	return acc, nil
}

func (w *Wallet) AddNewEncryptionToActiveWallet(sigName string, primary bool) error {

	if len(w.password) < 1 {
		return fmt.Errorf("password cannot be empty")
	}

	var signer oqs.Signature
	ew := EmptyWallet(w.WalletNumber, sigName, sigName)
	// ignore potential errors everywhere
	err := signer.Init(sigName, nil)
	if err != nil {
		return err
	}
	pubKey, err := signer.GenerateKeyPair()
	if err != nil {
		return err
	}
	mainAddress, err := common.PubKeyToAddress(pubKey, primary)
	if err != nil {
		return err
	}
	if primary {
		err = w.Account1.PublicKey.Init(pubKey, mainAddress)
		if err != nil {
			return err
		}
		(*w).Account1.Address = w.Account1.PublicKey.GetAddress()
		err = w.Account1.secretKey.Init(signer.ExportSecretKey(), w.Account1.Address, true)
		if err != nil {
			return err
		}
		(*w).Account1.signer = signer
		(*w).HomePath = ew.HomePath
	} else {
		err = w.Account2.PublicKey.Init(pubKey, mainAddress)
		if err != nil {
			return err
		}
		(*w).Account2.Address = w.Account2.PublicKey.GetAddress()
		err = w.Account2.secretKey.Init(signer.ExportSecretKey(), w.Account2.Address, false)
		if err != nil {
			return err
		}
		(*w).Account2.signer = signer
	}

	logger.GetLogger().Println(signer.Details())
	return nil
}

func GenerateNewIv() []byte {
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}
	return iv
}

func (w *Wallet) encrypt(v []byte) ([]byte, error) {
	cb, err := aes.NewCipher(w.passwordBytes)
	if err != nil {
		logger.GetLogger().Println("Can not create AES function")
		return []byte{}, err
	}
	v = append([]byte("validationTag"), v...)
	ciphertext := make([]byte, aes.BlockSize+len(v))
	stream := cipher.NewCTR(cb, w.Iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], v)
	return ciphertext, nil
}

func (w *Wallet) decrypt(v []byte) ([]byte, error) {
	cb, err := aes.NewCipher(w.passwordBytes)
	if err != nil {
		logger.GetLogger().Println("Can not create AES function")
		return []byte{}, err
	}

	plaintext := make([]byte, aes.BlockSize+len(v))
	stream := cipher.NewCTR(cb, w.Iv)
	stream.XORKeyStream(plaintext, v[aes.BlockSize:])
	if !bytes.Equal(plaintext[:len(common.ValidationTag)], []byte(common.ValidationTag)) {
		return nil, fmt.Errorf("wrong password")
	}
	return plaintext[len(common.ValidationTag):], nil
}

func (w *Wallet) GetMnemonicWords(primary bool) (string, error) {
	var secret []byte
	var secretLength int
	if primary {
		secret = w.GetSecretKey().GetBytes()
		secretLength = w.GetSecretKey().GetLength()
	} else {
		secret = w.GetSecretKey2().GetBytes()
		secretLength = w.GetSecretKey2().GetLength()
	}
	if secret == nil {
		return "", fmt.Errorf("you need load wallet first")
	}

	if secretLength > 64 {
		return "", fmt.Errorf("secret must be less than 64 bytes")
	}
	if secretLength < 64 {
		logger.GetLogger().Println("not all mnemonic words are important. secret is less than 64 bytes")
		secretTmp := make([]byte, 64)
		copy(secretTmp, secret)
		secret = secretTmp[:]
	}
	mnemonic, _ := bip39.NewMnemonic(secret)

	secretKey, _ := bip39.MnemonicToByteArray(mnemonic)
	if !bytes.Equal(secretKey[:secretLength], secret[:secretLength]) {
		logger.GetLogger().Println("Can not restore secret key from mnemonic")
		return "", fmt.Errorf("can not restore secret key from mnemonic")
	}
	return mnemonic, nil
}

func (w *Wallet) RestoreSecretKeyFromMnemonic(mnemonic string, primary bool) error {
	secretKey, err := bip39.MnemonicToByteArray(mnemonic)
	if err != nil {
		logger.GetLogger().Println("Can not restore secret key")
		return err
	}
	var signer oqs.Signature
	if primary {
		//if len(secretKey) < common.PrivateKeyLength() {
		//	return fmt.Errorf("not enough bytes for primary encryption private key")
		//}
		err = w.Account1.secretKey.Init(secretKey[:], w.Account1.Address, true)
		if err != nil {
			return err
		}

		err = signer.Init(common.SigName(), w.Account1.secretKey.GetBytes())
		if err != nil {
			return err
		}
		(*w).Account1.signer = signer
	} else {
		//if len(secretKey) < common.PrivateKeyLength2() {
		//	return fmt.Errorf("not enough bytes for secondary encryption private key")
		//}
		err = w.Account2.secretKey.Init(secretKey[:], w.Account2.Address, false)
		if err != nil {
			return err
		}
		err = signer.Init(common.SigName2(), w.Account2.secretKey.GetBytes())
		if err != nil {
			return err
		}
		(*w).Account2.signer = signer
	}

	return nil
}

func (w *Wallet) StoreJSON() error {
	if w.GetSecretKey().GetBytes() == nil {
		return fmt.Errorf("you need load wallet first")
	}

	// Create the wallet file path
	walletFile := filepath.Join(w.HomePath, "wallet"+strconv.Itoa(int(w.WalletNumber)))
	logger.GetLogger().Println("walletFile:", walletFile+".json")

	if _, ok := w.Accounts[w.SigName]; !ok {
		logger.GetLogger().Println("not properly structured wallet. Now OK")
		w.Accounts[w.SigName] = w.Account1
		copy(w.Accounts[w.SigName].EncryptedSecretKey[:], w.Account1.EncryptedSecretKey[:])
	}
	if _, ok := w.Accounts[w.SigName2]; !ok {
		logger.GetLogger().Println("not properly structured wallet. Now OK")
		w.Accounts[w.SigName2] = w.Account2
		copy(w.Accounts[w.SigName2].EncryptedSecretKey[:], w.Account2.EncryptedSecretKey[:])
	}

	se, err := w.encrypt(w.Account1.secretKey.GetBytes())
	if err != nil {
		logger.GetLogger().Println(err)
		return err
	}

	w.Account1.EncryptedSecretKey = make([]byte, len(se))
	copy(w.Account1.EncryptedSecretKey, se)

	se, err = w.encrypt(w.Account2.secretKey.GetBytes())
	if err != nil {
		logger.GetLogger().Println(err)
		return err
	}

	w.Account2.EncryptedSecretKey = make([]byte, len(se))
	copy(w.Account2.EncryptedSecretKey, se)

	// Marshal the wallet to JSON
	wm, err := json.MarshalIndent(&w, "", "    ")
	if err != nil {
		logger.GetLogger().Println(err)
		return err
	}
	// Create wallet directory if it doesn't exist
	if err := os.MkdirAll(w.HomePath, 0755); err != nil {
		return err
	}

	// Write the wallet to the JSON file
	if err := os.WriteFile(walletFile+".json", wm, 0600); err != nil {
		return err
	}

	return nil
}

// LoadJSON if height >= 0 current wallet will be replaced by latest but not larger than height
func LoadJSON(walletNumber uint8, password string, sigName, sigName2 string) (*Wallet, error) {
	if len(password) == 0 {
		return nil, fmt.Errorf("password cannot be empty")
	}

	ew := EmptyWallet(walletNumber, sigName, sigName2)

	homePath := ew.HomePath

	// Load wallet JSON file
	walletFile := filepath.Join(homePath, "wallet"+strconv.Itoa(int(walletNumber))+".json")
	data, err := os.ReadFile(walletFile)
	if err != nil {
		return nil, err
	}
	var w Wallet
	// Unmarshal JSON data into wallet struct
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}

	if w.SigName != sigName {
		w.SigName = sigName
		if a, ok := w.Accounts[sigName]; ok {
			w.Account1 = a
			copy(w.Account1.EncryptedSecretKey[:], a.EncryptedSecretKey[:])
		} else {
			acc, err := GenerateNewAccount(w, sigName)
			if err != nil {
				return nil, err
			}

			w.Account1 = acc
			copy(w.Account1.EncryptedSecretKey[:], acc.EncryptedSecretKey[:])
		}
	}
	if w.SigName2 != sigName2 {
		w.SigName2 = sigName2
		if a, ok := w.Accounts[sigName2]; ok {
			w.Account2 = a
			copy(w.Account2.EncryptedSecretKey[:], a.EncryptedSecretKey[:])
		} else {
			acc, err := GenerateNewAccount(w, sigName2)
			if err != nil {
				return nil, err
			}
			w.Account2 = acc
			copy(w.Account2.EncryptedSecretKey[:], acc.EncryptedSecretKey[:])
		}
	}

	w.SetPassword(password)
	ds, err := w.decrypt(w.Account1.EncryptedSecretKey)
	if err != nil {
		logger.GetLogger().Println(err)
		return nil, err
	}
	var signer oqs.Signature
	err = signer.Init(w.SigName, ds)
	if err != nil {
		return nil, err
	}
	ds = ds[:signer.Details().LengthSecretKey]
	err = signer.Init(w.SigName, ds)
	if err != nil {
		return nil, err
	}
	w.Account1.signer = signer
	// maybe one should limit amount of bytes to pass here
	cnz := CountNonZeroBytes(ds)
	logger.GetLogger().Println("cnz:", cnz)

	err = w.Account1.secretKey.Init(ds, w.Account1.Address, true)
	if err != nil {
		return nil, err
	}

	ds, err = w.decrypt(w.Account2.EncryptedSecretKey)
	if err != nil {
		logger.GetLogger().Println(err)
		return nil, err
	}
	var signer2 oqs.Signature
	err = signer2.Init(w.SigName2, ds)
	if err != nil {
		return nil, err
	}
	ds = ds[:signer2.Details().LengthSecretKey]
	err = signer2.Init(w.SigName2, ds)
	if err != nil {
		return nil, err
	}
	w.Account2.signer = signer2
	cnz = CountNonZeroBytes(ds)
	logger.GetLogger().Println("cnz:", cnz)
	err = w.Account2.secretKey.Init(ds, w.Account2.Address, false)
	if err != nil {
		return nil, err
	}

	w.MainAddress.Primary = true
	w.Account1.Address.Primary = true
	w.Account2.Address.Primary = false
	w.Account1.PublicKey.Address.Primary = true
	w.Account2.PublicKey.Address.Primary = false
	w.Account1.PublicKey.Primary = true
	w.Account2.PublicKey.Primary = false

	// Ensure MainAddress equals Account1.Address (they should be the same)
	// If MainAddress is empty or doesn't match, set it from Account1.Address
	zeroAddr := make([]byte, common.AddressLength)
	if bytes.Equal(w.MainAddress.GetBytes(), zeroAddr) {
		w.MainAddress = w.Account1.Address
		logger.GetLogger().Println("MainAddress was empty, set from Account1.Address:", w.MainAddress.GetHex())
	} else if !bytes.Equal(w.MainAddress.GetBytes(), w.Account1.Address.GetBytes()) {
		logger.GetLogger().Println("WARNING: MainAddress differs from Account1.Address!")
		logger.GetLogger().Println("MainAddress:", w.MainAddress.GetHex())
		logger.GetLogger().Println("Account1.Address:", w.Account1.Address.GetHex())
		// Use Account1.Address as the canonical address
		w.MainAddress = w.Account1.Address
	}

	// Ensure MainAddress is set on PublicKeys (may be empty from older wallet JSON)
	w.Account1.PublicKey.MainAddress = w.MainAddress
	w.Account2.PublicKey.MainAddress = w.MainAddress

	w.Account1.secretKey.Address.Primary = true
	w.Account2.secretKey.Address.Primary = false
	w.Account1.secretKey.Primary = true
	w.Account2.secretKey.Primary = false

	w.HomePath = homePath
	w.StoreJSON()
	//logger.GetLogger().Println("PubKey:", w.Account1.PublicKey.GetHex())
	//logger.GetLogger().Println("PubKey2:", w.Account2.PublicKey.GetHex())
	logger.GetLogger().Println("MainAddress:", w.MainAddress.GetHex())
	return &w, err
}

func (w *Wallet) ChangePassword(password, newPassword string) error {
	if w.passwordBytes == nil {
		return fmt.Errorf("you need load wallet first")
	}
	if !bytes.Equal(passwordToByte(password), w.passwordBytes) {
		return fmt.Errorf("current password is not valid")
	}

	globalMutex.Lock()
	defer globalMutex.Unlock()

	w2 := Wallet{
		password:      newPassword,
		passwordBytes: passwordToByte(newPassword),
		Iv:            w.Iv,
		HomePath:      w.HomePath,
		WalletNumber:  w.WalletNumber,
		MainAddress:   w.MainAddress,
		SigName:       w.SigName,
		SigName2:      w.SigName2,
		Account1:      w.Account1,
		Account2:      w.Account2,
		Accounts:      w.Accounts,
	}

	for k, v := range w.Accounts {
		ds, err := w.decrypt(v.EncryptedSecretKey)
		if err != nil {
			logger.GetLogger().Println(err)
			return err
		}
		se, err := w2.encrypt(ds)
		if err != nil {
			logger.GetLogger().Println(err)
			return err
		}
		copy(w2.Accounts[k].EncryptedSecretKey, se)
	}
	err := w2.StoreJSON()
	if err != nil {
		logger.GetLogger().Println("Can not store new wallet")
		return err
	}
	_, err = LoadJSON(w2.WalletNumber, newPassword, w2.SigName, w2.SigName2)
	if err != nil {
		return err
	}
	return nil
}

func (w *Wallet) Sign(data []byte, primary bool) (*common.Signature, error) {
	if len(data) > 0 {
		if primary {
			signature, err := w.Account1.signer.Sign(data)
			if err != nil {
				return nil, err
			}
			signature = append([]byte{0}, signature...)
			sig := &common.Signature{}
			err = sig.Init(signature, w.MainAddress)
			if err != nil {
				return nil, err
			}
			return sig, nil
		} else {
			signature2, err := w.Account2.signer.Sign(data)
			if err != nil {
				return nil, err
			}
			signature2 = append([]byte{1}, signature2...)
			sig := &common.Signature{}
			err = sig.Init(signature2, w.MainAddress)
			if err != nil {
				return nil, err
			}
			return sig, nil
		}
	}
	return nil, fmt.Errorf("input data are empty")
}

func Verify(msg []byte, sig []byte, pubkey []byte, sigName, sigName2 string, isPaused, isPaused2 bool) bool {
	var verifier oqs.Signature
	var err error
	primary := sig[0] == 0
	sig = sig[1:]
	//logger.GetLogger().Println("Primary:", primary)
	if primary && !isPaused {
		//logger.GetLogger().Println("Primary sign")
		err = verifier.Init(sigName, nil)
		if err != nil {
			logger.GetLogger().Println("verifier:", err)
			return false
		}
		if verifier.Details().LengthPublicKey == len(pubkey) {
			isVerified, err := verifier.Verify(msg, sig, pubkey)

			if err != nil {
				logger.GetLogger().Println(err)
				return false
			}
			if !isVerified {
				logger.GetLogger().Println("msg:", string(msg[:5]), "sig:", string(sig[:5]), "pubkey:", string(pubkey[:5]))
			}
			return isVerified
		}
		logger.GetLogger().Println("verifier.Details().LengthPublicKey:", verifier.Details().LengthPublicKey, "len(pubkey):", len(pubkey))
	}
	if !primary && !isPaused2 {
		//logger.GetLogger().Println("Secondary sign")
		err = verifier.Init(sigName2, nil)
		if err != nil {
			logger.GetLogger().Println("verifier:", err)
			return false
		}
		if verifier.Details().LengthPublicKey == len(pubkey) {
			isVerified, err := verifier.Verify(msg, sig, pubkey)
			if err != nil {
				logger.GetLogger().Println(err)
				return false
			}
			if !isVerified {
				logger.GetLogger().Println("msg:", string(msg[:5]), "sig:", string(sig[:5]), "pubkey:", string(pubkey[:5]))
			}
			return isVerified
		}
		logger.GetLogger().Println("verifier.Details().LengthPublicKey:", verifier.Details().LengthPublicKey, "len(pubkey):", len(pubkey))
	}
	//logger.GetLogger().Println(primary, isPaused, isPaused2)
	return false
}

func (w *Wallet) GetSecretKey() common.PrivKey {
	if w == nil {
		return common.PrivKey{}
	}
	return w.Account1.secretKey
}

func (w *Wallet) Check() bool {
	if (w != nil) && len(w.passwordBytes) > 0 && (len(w.GetSecretKey().GetBytes()) == w.GetSecretKey().GetLength()) {
		return true
	}
	return false
}

func (w *Wallet) GetSecretKey2() common.PrivKey {
	if w == nil {
		return common.PrivKey{}
	}
	return w.Account2.secretKey
}

func (w *Wallet) Check2() bool {
	if (w != nil) && len(w.passwordBytes) > 0 && (len(w.GetSecretKey2().GetBytes()) == w.GetSecretKey2().GetLength()) {
		return true
	}
	return false
}
