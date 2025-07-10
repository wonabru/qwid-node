package account

import (
	"bytes"
	"fmt"
	"github.com/okuralabs/okura-node/logger"
	"sync"

	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/database"
)

type AccountsType struct {
	AllAccounts map[[common.AddressLength]byte]Account `json:"all_accounts"`
	Height      int64                                  `json:"height"`
}

var Accounts AccountsType
var AccountsRWMutex sync.RWMutex

func AddTransactionsSender(address [common.AddressLength]byte, hashTxn common.Hash) {
	AccountsRWMutex.Lock()
	defer AccountsRWMutex.Unlock()
	acc := Accounts.AllAccounts[address]
	acc.TransactionsSender = append(acc.TransactionsSender, hashTxn)
	Accounts.AllAccounts[address] = acc
}

func AddTransactionsRecipient(address [common.AddressLength]byte, hashTxn common.Hash) {
	AccountsRWMutex.Lock()
	defer AccountsRWMutex.Unlock()
	acc := Accounts.AllAccounts[address]
	acc.TransactionsRecipient = append(acc.TransactionsRecipient, hashTxn)
	Accounts.AllAccounts[address] = acc
}

// error is not checked one should do the checking before
func SetBalance(address [common.AddressLength]byte, balance int64) {
	AccountsRWMutex.Lock()
	defer AccountsRWMutex.Unlock()
	acc := Accounts.AllAccounts[address]
	acc.Balance = balance
	Accounts.AllAccounts[address] = acc
}

// error is not checked one should do the checking before
func GetBalance(address [common.AddressLength]byte) int64 {
	AccountsRWMutex.RLock()
	defer AccountsRWMutex.RUnlock()
	return Accounts.AllAccounts[address].Balance
}

// Marshal converts AccountsType to a binary format.
func (at AccountsType) Marshal() []byte {
	var buffer bytes.Buffer
	// Number of accounts
	accountCount := len(at.AllAccounts)
	buffer.Write(common.GetByteInt64(int64(accountCount)))

	// Iterate over map and marshal each account
	for address, acc := range at.AllAccounts {
		buffer.Write(address[:])    // Write address
		buffer.Write(acc.Marshal()) // Marshal and write account
	}
	buffer.Write(common.GetByteInt64(at.Height))
	return buffer.Bytes()
}

// Unmarshal decodes AccountsType from a binary format.
func (at *AccountsType) Unmarshal(data []byte) error {
	buffer := bytes.NewBuffer(data)
	// Number of accounts
	accountCount := common.GetInt64FromByte(buffer.Next(8))

	at.AllAccounts = make(map[[common.AddressLength]byte]Account, accountCount)

	// Read each account
	for i := int64(0); i < accountCount; i++ {
		var address [common.AddressLength]byte
		var acc Account

		// Read address
		if n, err := buffer.Read(address[:]); err != nil || n != common.AddressLength {
			return fmt.Errorf("failed to read address: %w", err)
		}

		// Account binary data
		accountData := buffer.Next(common.AddressLength + 17) // Account data length (8 bytes for delay + 1 for multisignNumber + 8 bytes for balance + 20 bytes for address)
		if len(accountData) != common.AddressLength+17 {
			return fmt.Errorf("incorrect account data length: got %d, want 33", len(accountData))
		}

		if err := acc.Unmarshal(accountData); err != nil {
			return fmt.Errorf("failed to unmarshal account: %w", err)
		}

		at.AllAccounts[address] = acc
	}
	at.Height = common.GetInt64FromByte(buffer.Next(8))
	return nil
}

func StoreAccounts(height int64) error {
	if height < 0 {
		height = common.GetHeight()
	}
	AccountsRWMutex.Lock()
	defer AccountsRWMutex.Unlock()
	k := Accounts.Marshal()
	hb := common.GetByteInt64(height)
	prefix := append(common.AccountsDBPrefix[:], hb...)
	err := database.MainDB.Put(prefix, k[:])
	if err != nil {
		logger.GetLogger().Println("cannot store accounts", err)
		return err
	}
	return nil
}

func RemoveAccountsFromDB(height int64) error {
	hb := common.GetByteInt64(height)
	prefix := append(common.AccountsDBPrefix[:], hb...)
	err := database.MainDB.Delete(prefix)
	if err != nil {
		logger.GetLogger().Println("cannot remove account", err)
		return err
	}
	return nil
}

func LoadAccounts(height int64) error {
	var err error
	AccountsRWMutex.Lock()
	defer AccountsRWMutex.Unlock()
	if height < 0 {
		height, err = LastHeightStoredInAccounts()
		if err != nil {
			logger.GetLogger().Println(err)
		}
	}

	hb := common.GetByteInt64(height)
	prefix := append(common.AccountsDBPrefix[:], hb...)
	b, err := database.MainDB.Get(prefix)
	if err != nil || b == nil {
		logger.GetLogger().Println("cannot load accounts", err)
		return err
	}
	err = (&Accounts).Unmarshal(b)
	if err != nil {
		logger.GetLogger().Println("cannot unmarshal accounts")
		return err
	}
	return nil
}

func LastHeightStoredInAccounts() (int64, error) {
	i := int64(0)
	for {
		ib := common.GetByteInt64(i)
		prefix := append(common.AccountsDBPrefix[:], ib...)
		isKey, err := database.MainDB.IsKey(prefix)
		if err != nil {
			return i - 1, err
		}
		if !isKey {
			break
		}
		i++
	}
	return i - 1, nil
}
