package wallet

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/okuralabs/okura-node/logger"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wonabru/bip39"
	"io"
	"sync"

	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/crypto/oqs"
	"golang.org/x/crypto/sha3"
)

var globalMutex sync.RWMutex

type Wallet struct {
	password            string
	passwordBytes       []byte
	Iv                  []byte `json:"iv"`
	secretKey           common.PrivKey
	secretKey2          common.PrivKey
	EncryptedSecretKey  []byte         `json:"encrypted_secret_key"`
	EncryptedSecretKey2 []byte         `json:"encrypted_secret_key2"`
	PublicKey           common.PubKey  `json:"public_key"`
	PublicKey2          common.PubKey  `json:"public_key2"`
	Address             common.Address `json:"address"`
	Address2            common.Address `json:"address2"`
	MainAddress         common.Address `json:"main_address"`
	signer              oqs.Signature
	signer2             oqs.Signature
	HomePath            string `json:"home_path"`
	WalletNumber        uint8  `json:"wallet_number"`
}

// Structure map of Height and wallet which was chaange
type GeneralWallet struct {
	WalletChain   map[int64]Wallet `json:"wallet_chain"`
	CurrentWallet Wallet           `json:"current_wallet"`
}

var activeWallet *Wallet

type AnyWallet interface {
	GetWallet() Wallet
}

func InitActiveWallet(walletNumber uint8, password string, height int64) {
	var err error
	gw, err := LoadJSON(walletNumber, password, height)
	activeWallet = &gw.CurrentWallet
	if err != nil {
		logger.GetLogger().Println("wrong password")
		os.Exit(1)
	}
	if activeWallet == nil {
		logger.GetLogger().Println("failed to load wallet")
		os.Exit(1)
	}
}

func GetCurrentWallet(height int64) (*Wallet, error) {
	aw := GetActiveWallet()
	var err error
	gw, err := LoadJSON(aw.WalletNumber, aw.password, height)
	currentWallet := &gw.CurrentWallet
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

	s := fmt.Sprintln("Length of public key:", w.PublicKey.GetLength())
	s += fmt.Sprintln("Beginning of public key:", w.PublicKey.GetHex()[:10])
	s += fmt.Sprintln("Address:", w.Address.GetHex())
	s += fmt.Sprintln("Length of private key:", w.GetSecretKey().GetLength())
	s += fmt.Sprintln("Length of public key 2:", w.PublicKey2.GetLength())
	s += fmt.Sprintln("Beginning of public key 2:", w.PublicKey2.GetHex()[:10])
	s += fmt.Sprintln("Address 2:", w.Address2.GetHex())
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
		return w.signer.Details().Name
	} else {
		return w.signer2.Details().Name
	}
}

func EmptyWallet(walletNumber uint8, sigName, sigName2 string) *Wallet {
	homePath, err := os.UserHomeDir()
	if err != nil {
		logger.GetLogger().Fatal(err)
	}
	return &Wallet{
		password:      "",
		passwordBytes: nil,
		Iv:            nil,
		secretKey:     common.PrivKey{},
		secretKey2:    common.PrivKey{},
		PublicKey:     common.PubKey{},
		PublicKey2:    common.PubKey{},
		Address:       common.Address{},
		Address2:      common.Address{},
		MainAddress:   common.Address{},
		signer:        oqs.Signature{},
		signer2:       oqs.Signature{},
		HomePath:      homePath + common.DefaultWalletHomePath + strconv.Itoa(int(walletNumber)),
		WalletNumber:  walletNumber,
	}
}

func EmptyGeneralWallet(walletNumber uint8, sigName, sigName2 string) *GeneralWallet {
	w := EmptyWallet(walletNumber, sigName, sigName2)
	return &GeneralWallet{
		map[int64]Wallet{},
		*w,
	}
}

func GenerateNewWallet(walletNumber uint8, password string) (*GeneralWallet, error) {
	if len(password) < 1 {
		return nil, fmt.Errorf("password cannot be empty")
	}
	w := EmptyGeneralWallet(walletNumber, common.SigName(), common.SigName2())
	w.CurrentWallet.SetPassword(password)
	(*w).CurrentWallet.Iv = generateNewIv()
	var signer oqs.Signature
	var signer2 oqs.Signature
	//defer signer.Clean()

	// ignore potential errors everywhere
	err := signer.Init(common.SigName(), nil)
	if err != nil {
		return nil, err
	}
	pubKey, err := signer.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	mainAddress, err := common.PubKeyToAddress(pubKey, true)
	if err != nil {
		return nil, err
	}
	err = w.CurrentWallet.PublicKey.Init(pubKey, mainAddress)
	if err != nil {
		return nil, err
	}
	(*w).CurrentWallet.Address = w.CurrentWallet.PublicKey.GetAddress()
	(*w).CurrentWallet.MainAddress = (*w).CurrentWallet.Address
	err = w.CurrentWallet.secretKey.Init(signer.ExportSecretKey(), w.CurrentWallet.Address)
	if err != nil {
		return nil, err
	}
	(*w).CurrentWallet.signer = signer

	err = signer2.Init(common.SigName2(), nil)
	if err != nil {
		return nil, err
	}
	pubKey2, err := signer2.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	err = w.CurrentWallet.PublicKey2.Init(pubKey2, mainAddress)
	if err != nil {
		return nil, err
	}
	(*w).CurrentWallet.Address2 = w.CurrentWallet.PublicKey2.GetAddress()
	err = w.CurrentWallet.secretKey2.Init(signer2.ExportSecretKey(), w.CurrentWallet.Address2)
	if err != nil {
		return nil, err
	}
	se, err := (*w).CurrentWallet.encrypt(w.CurrentWallet.secretKey.GetBytes())
	if err != nil {
		logger.GetLogger().Println(err)
		return nil, err
	}
	(*w).CurrentWallet.EncryptedSecretKey = make([]byte, len(se))
	copy((*w).CurrentWallet.EncryptedSecretKey, se)

	se, err = w.CurrentWallet.encrypt(w.CurrentWallet.secretKey2.GetBytes())
	if err != nil {
		logger.GetLogger().Println(err)
		return nil, err
	}

	(*w).CurrentWallet.EncryptedSecretKey2 = make([]byte, len(se))
	copy((*w).CurrentWallet.EncryptedSecretKey2, se)

	(*w).CurrentWallet.signer2 = signer2
	(*w).WalletChain[0] = (*w).CurrentWallet
	copy((*w).WalletChain[0].EncryptedSecretKey[:], (*w).CurrentWallet.EncryptedSecretKey[:])
	copy((*w).WalletChain[0].EncryptedSecretKey2[:], (*w).CurrentWallet.EncryptedSecretKey2[:])
	return w, nil
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
		err = w.PublicKey.Init(pubKey, mainAddress)
		if err != nil {
			return err
		}
		(*w).Address = w.PublicKey.GetAddress()
		err = w.secretKey.Init(signer.ExportSecretKey(), w.Address)
		if err != nil {
			return err
		}
		(*w).signer = signer
		(*w).HomePath = ew.HomePath
	} else {
		err = w.PublicKey2.Init(pubKey, mainAddress)
		if err != nil {
			return err
		}
		(*w).Address2 = w.PublicKey2.GetAddress()
		err = w.secretKey2.Init(signer.ExportSecretKey(), w.Address2)
		if err != nil {
			return err
		}
		(*w).signer2 = signer
	}

	logger.GetLogger().Println(signer.Details())
	return nil
}

func generateNewIv() []byte {
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
		if len(secretKey) < common.PrivateKeyLength() {
			return fmt.Errorf("not enough bytes for primary encryption private key")
		}
		err = w.secretKey.Init(secretKey[:common.PrivateKeyLength()], w.Address)
		if err != nil {
			return err
		}

		err = signer.Init(common.SigName(), w.secretKey.GetBytes())
		if err != nil {
			return err
		}
		(*w).signer = signer
	} else {
		if len(secretKey) < common.PrivateKeyLength2() {
			return fmt.Errorf("not enough bytes for secondary encryption private key")
		}
		err = w.secretKey2.Init(secretKey[:common.PrivateKeyLength2()], w.Address2)
		if err != nil {
			return err
		}
		err = signer.Init(common.SigName2(), w.secretKey2.GetBytes())
		if err != nil {
			return err
		}
		(*w).signer2 = signer
	}

	return nil
}

func (w *Wallet) StoreJSON(heightWhenChanged int64) error {
	if w.GetSecretKey().GetBytes() == nil {
		return fmt.Errorf("you need load wallet first")
	}
	gw := EmptyGeneralWallet(w.WalletNumber, w.GetSigName(true), w.GetSigName(false))
	gw.WalletChain[heightWhenChanged] = *w
	copy((*gw).WalletChain[heightWhenChanged].EncryptedSecretKey, (*w).EncryptedSecretKey)
	copy((*gw).WalletChain[heightWhenChanged].EncryptedSecretKey2, (*w).EncryptedSecretKey2)
	gw.CurrentWallet = *w
	copy((*gw).CurrentWallet.EncryptedSecretKey, (*w).EncryptedSecretKey)
	copy((*gw).CurrentWallet.EncryptedSecretKey2, (*w).EncryptedSecretKey2)
	err := gw.StoreJSON(heightWhenChanged)
	if err != nil {
		logger.GetLogger().Println(err)
		return err
	}
	return nil
}
func (w *GeneralWallet) StoreJSON(heightWhenChanged int64) error {
	if w.CurrentWallet.GetSecretKey().GetBytes() == nil {
		return fmt.Errorf("you need load wallet first")
	}

	// Create the wallet file path
	walletFile := filepath.Join(w.CurrentWallet.HomePath, "wallet"+strconv.Itoa(int(w.CurrentWallet.WalletNumber)))
	logger.GetLogger().Println("walletFile:", walletFile+".json")

	if heightWhenChanged >= 0 {
		w.WalletChain[heightWhenChanged] = w.CurrentWallet
	}
	se, err := w.CurrentWallet.encrypt(w.CurrentWallet.secretKey.GetBytes())
	if err != nil {
		logger.GetLogger().Println(err)
		return err
	}

	w2 := w
	(*w2).CurrentWallet.EncryptedSecretKey = make([]byte, len(se))
	copy((*w2).CurrentWallet.EncryptedSecretKey, se)

	se, err = w.CurrentWallet.encrypt(w2.CurrentWallet.secretKey2.GetBytes())
	if err != nil {
		logger.GetLogger().Println(err)
		return err
	}

	(*w2).CurrentWallet.EncryptedSecretKey2 = make([]byte, len(se))
	copy((*w2).CurrentWallet.EncryptedSecretKey2, se)

	// Marshal the wallet to JSON
	wm, err := json.MarshalIndent(&w, "", "    ")
	if err != nil {
		logger.GetLogger().Println(err)
		return err
	}
	// Create wallet directory if it doesn't exist
	if err := os.MkdirAll(w.CurrentWallet.HomePath, 0755); err != nil {
		return err
	}

	// Write the wallet to the JSON file
	if err := os.WriteFile(walletFile+".json", wm, 0600); err != nil {
		return err
	}

	return nil
}

// LoadJSON if height >= 0 current wallet will be replaced by latest but not larger than height
func LoadJSON(walletNumber uint8, password string, height int64) (*GeneralWallet, error) {
	if len(password) == 0 {
		return nil, fmt.Errorf("password cannot be empty")
	}

	w := EmptyGeneralWallet(walletNumber, common.SigName(), common.SigName2())
	if w == nil {
		return nil, fmt.Errorf("failed to create empty wallet")
	}

	homePath := w.CurrentWallet.HomePath

	// Load wallet JSON file
	walletFile := filepath.Join(homePath, "wallet"+strconv.Itoa(int(w.CurrentWallet.WalletNumber))+".json")
	data, err := os.ReadFile(walletFile)
	if err != nil {
		return nil, err
	}
	// Unmarshal JSON data into wallet struct
	if err := json.Unmarshal(data, w); err != nil {
		return nil, err
	}
	cw := EmptyWallet(walletNumber, common.SigName(), common.SigName2())
	if height >= 0 {
		maxHeight := int64(0)
		for k, v := range w.WalletChain {
			if height >= k && k >= maxHeight {
				cw = &v
				maxHeight = k
			}
		}
		w.CurrentWallet = *cw
		copy(w.CurrentWallet.EncryptedSecretKey[:], cw.EncryptedSecretKey[:])
		copy(w.CurrentWallet.EncryptedSecretKey2[:], cw.EncryptedSecretKey2[:])
		copy(w.CurrentWallet.Iv[:], cw.Iv[:])
	}

	w.CurrentWallet.SetPassword(password)
	ds, err := w.CurrentWallet.decrypt(w.CurrentWallet.EncryptedSecretKey)
	if err != nil {
		logger.GetLogger().Println(err)
		return nil, err
	}
	err = w.CurrentWallet.secretKey.Init(ds[:common.PrivateKeyLength()], w.CurrentWallet.Address)
	if err != nil {
		return nil, err
	}
	var signer oqs.Signature
	err = signer.Init(common.SigName(), w.CurrentWallet.secretKey.GetBytes())
	if err != nil {
		return nil, err
	}
	(*w).CurrentWallet.signer = signer

	// Unmarshal JSON data into second wallet struct
	w2 := *w
	w.CurrentWallet.MainAddress.Primary = true
	w.CurrentWallet.Address.Primary = true
	w.CurrentWallet.Address2.Primary = false
	w.CurrentWallet.PublicKey.Address.Primary = true
	w.CurrentWallet.PublicKey2.Address.Primary = false
	w.CurrentWallet.PublicKey.Primary = true
	w.CurrentWallet.PublicKey2.Primary = false
	w.CurrentWallet.PublicKey.MainAddress.Primary = true
	w.CurrentWallet.PublicKey2.MainAddress.Primary = true

	w.CurrentWallet.secretKey.Address.Primary = true
	w.CurrentWallet.secretKey2.Address.Primary = false
	w.CurrentWallet.secretKey.Primary = true
	w.CurrentWallet.secretKey2.Primary = false

	w2.CurrentWallet.SetPassword(password)
	ds, err = w2.CurrentWallet.decrypt(w.CurrentWallet.EncryptedSecretKey2)
	if err != nil {
		logger.GetLogger().Println(err)
		return nil, err
	}
	err = w.CurrentWallet.secretKey2.Init(ds[:common.PrivateKeyLength2()], w.CurrentWallet.Address2)
	if err != nil {
		return nil, err
	}
	var signer2 oqs.Signature
	err = signer2.Init(common.SigName2(), w.CurrentWallet.secretKey2.GetBytes())
	if err != nil {
		return nil, err
	}
	(*w).CurrentWallet.signer2 = signer2
	(*w).CurrentWallet.HomePath = homePath

	logger.GetLogger().Println("PubKey:", w.CurrentWallet.PublicKey.GetHex())
	logger.GetLogger().Println("PubKey2:", w.CurrentWallet.PublicKey2.GetHex())
	logger.GetLogger().Println("MainAddress:", w.CurrentWallet.MainAddress.GetHex())
	return w, err
}

func (w *GeneralWallet) ChangePassword(password, newPassword string) error {
	if w.CurrentWallet.passwordBytes == nil {
		return fmt.Errorf("you need load wallet first")
	}
	if !bytes.Equal(passwordToByte(password), w.CurrentWallet.passwordBytes) {
		return fmt.Errorf("current password is not valid")
	}

	globalMutex.Lock()
	defer globalMutex.Unlock()

	w2 := Wallet{
		password:      newPassword,
		passwordBytes: passwordToByte(newPassword),
		Iv:            w.CurrentWallet.Iv,
		secretKey:     w.CurrentWallet.secretKey,
		PublicKey:     w.CurrentWallet.PublicKey,
		Address:       w.CurrentWallet.Address,
		signer:        w.CurrentWallet.signer,
		secretKey2:    w.CurrentWallet.secretKey2,
		PublicKey2:    w.CurrentWallet.PublicKey2,
		Address2:      w.CurrentWallet.Address2,
		signer2:       w.CurrentWallet.signer2,
		MainAddress:   w.CurrentWallet.MainAddress,
		HomePath:      w.CurrentWallet.HomePath,
		WalletNumber:  w.CurrentWallet.WalletNumber,
	}
	ew := EmptyGeneralWallet(w.CurrentWallet.WalletNumber, w.CurrentWallet.GetSigName(true), w.CurrentWallet.GetSigName(false))
	for k, v := range w.WalletChain {
		ww := Wallet{
			password:      newPassword,
			passwordBytes: passwordToByte(newPassword),
			Iv:            v.Iv,
			secretKey:     v.secretKey,
			PublicKey:     v.PublicKey,
			Address:       v.Address,
			signer:        v.signer,
			secretKey2:    v.secretKey2,
			PublicKey2:    v.PublicKey2,
			Address2:      v.Address2,
			signer2:       v.signer2,
			MainAddress:   v.MainAddress,
			HomePath:      v.HomePath,
			WalletNumber:  v.WalletNumber,
		}
		ew.WalletChain[k] = ww
	}
	ew.CurrentWallet = w2
	err := ew.StoreJSON(-1)
	if err != nil {
		logger.GetLogger().Println("Can not store new wallet")
		return err
	}
	_, err = LoadJSON(ew.CurrentWallet.WalletNumber, newPassword, -1)
	if err != nil {
		return err
	}
	return nil
}

func (w *Wallet) Sign(data []byte, primary bool) (*common.Signature, error) {
	if len(data) > 0 {
		if primary {
			signature, err := w.signer.Sign(data)
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
			signature2, err := w.signer2.Sign(data)
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

func Verify(msg []byte, sig []byte, pubkey []byte) bool {
	var verifier oqs.Signature
	var err error
	primary := sig[0] == 0
	sig = sig[1:]
	logger.GetLogger().Println("Primary:", primary)
	if primary && !common.IsPaused() {
		logger.GetLogger().Println("Primary sign")
		err = verifier.Init(common.SigName(), nil)
		if err != nil {
			logger.GetLogger().Println("verifier:", err)
			return false
		}
		if verifier.Details().LengthPublicKey == len(pubkey) {
			isVerified, err := verifier.Verify(msg, sig, pubkey)
			logger.GetLogger().Println("isVerified:", isVerified)
			if err != nil {
				logger.GetLogger().Println(err)
				return false
			}
			if !isVerified {
				logger.GetLogger().Println("msg:", msg, "sig:", sig, "pubkey:", pubkey)
			}
			return isVerified
		}
		logger.GetLogger().Println("verifier.Details().LengthPublicKey:", verifier.Details().LengthPublicKey, "len(pubkey):", len(pubkey))
	}
	if !primary && !common.IsPaused2() {
		logger.GetLogger().Println("Secondary sign")
		err = verifier.Init(common.SigName2(), nil)
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
			return isVerified
		}
		logger.GetLogger().Println("verifier.Details().LengthPublicKey:", verifier.Details().LengthPublicKey, "len(pubkey):", len(pubkey))
	}
	return false
}

func (w *Wallet) GetSecretKey() common.PrivKey {
	if w == nil {
		return common.PrivKey{}
	}
	return w.secretKey
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
	return w.secretKey2
}

func (w *Wallet) Check2() bool {
	if (w != nil) && len(w.passwordBytes) > 0 && (len(w.GetSecretKey2().GetBytes()) == w.GetSecretKey2().GetLength()) {
		return true
	}
	return false
}
