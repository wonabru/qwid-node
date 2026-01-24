package transactionsDefinition

import (
	"bytes"
	"fmt"
	"github.com/wonabru/qwid-node/logger"
	"strconv"

	"github.com/wonabru/qwid-node/account"
	"github.com/wonabru/qwid-node/common"
	"github.com/wonabru/qwid-node/database"
	"github.com/wonabru/qwid-node/pubkeys"
	"github.com/wonabru/qwid-node/wallet"
)

type Transaction struct {
	TxData          TxData           `json:"tx_data"`
	TxParam         TxParam          `json:"tx_param"`
	Hash            common.Hash      `json:"hash"`
	Signature       common.Signature `json:"signature"`
	Height          int64            `json:"height"`
	GasPrice        int64            `json:"gas_price"`
	GasUsage        int64            `json:"gas_usage"`
	OutputLogs      []byte           `json:"outputLogs,omitempty"`
	ContractAddress common.Address   `json:"contractAddress,omitempty"`
}

func (mt *Transaction) GetData() TxData {
	return mt.TxData
}

func (mt *Transaction) GetParam() TxParam {
	return mt.TxParam
}

func (mt *Transaction) GasUsageEstimate() int64 {
	gas := len(mt.TxData.OptData) * 100
	gas += len(mt.TxData.Pubkey.GetBytes()) * 100
	return int64(gas) + 30000
}

func (mt *Transaction) GetGasUsage() int64 {
	return 2100
}

func (mt *Transaction) GetSignature() common.Signature {
	return mt.Signature
}

func (mt *Transaction) GetHeight() int64 {
	return mt.Height
}

func (mt *Transaction) GetHash() common.Hash {
	return mt.Hash
}

func (tx *Transaction) GetString() string {
	t := "Common parameters:\n" + tx.TxParam.GetString() + "\n"
	t += "Data:\n" + tx.TxData.GetString() + "\n"
	t += "Block Height: " + strconv.FormatInt(tx.Height, 10) + "\n"
	t += "Gas Price: " + strconv.FormatInt(tx.GasPrice, 10) + "\n"
	t += "Gas Usage: " + strconv.FormatInt(tx.GasUsage, 10) + "\n"
	t += "Hash: " + tx.Hash.GetHex() + "\n"
	t += "Signature: " + tx.Signature.GetHex() + "\n"
	t += "Contract Address: " + tx.ContractAddress.GetHex() + "\n"
	t += "Contract Logs:\n" + string(tx.OutputLogs) + "\n"
	return t
}

func (tx *Transaction) GetSenderAddress() common.Address {
	return tx.TxParam.Sender
}

func (tx *Transaction) GetFromBytes(b []byte) (Transaction, []byte, error) {

	if len(b) < 76+common.SignatureLength(true)+1 && len(b) < 76+common.SignatureLength2(true)+1 {
		return Transaction{}, nil, fmt.Errorf("Not enough bytes for transaction unmarshal len bytes %v", len(b))
	}
	tp := TxParam{}
	tp, b, err := tp.GetFromBytes(b)
	if err != nil {
		return Transaction{}, nil, err
	}
	td := TxData{}
	adata, b, err := td.GetFromBytes(b)
	if err != nil {
		return Transaction{}, nil, err
	}
	at := Transaction{
		TxData:    adata,
		TxParam:   tp,
		Hash:      common.Hash{},
		Signature: common.Signature{},
		Height:    common.GetInt64FromByte(b[:8]),
		GasPrice:  common.GetInt64FromByte(b[8:16]),
		GasUsage:  common.GetInt64FromByte(b[16:24]),
	}
	at.Hash = common.GetHashFromBytes(b[24:56])
	vb, leftb, err := common.BytesWithLenToBytes(b[56:])
	if err != nil {
		return Transaction{}, nil, err
	}
	signature, err := common.GetSignatureFromBytes(vb, tp.Sender)
	if err != nil {
		return Transaction{}, nil, err
	}
	at.Signature = signature
	err = at.ContractAddress.Init(leftb[:20])
	if err != nil {
		return Transaction{}, nil, err
	}
	toBytes, leftb2, err := common.BytesWithLenToBytes(leftb[20:])
	if err != nil {
		return Transaction{}, nil, err
	}
	at.OutputLogs = toBytes[:]
	return at, leftb2, nil
}

func (mt *Transaction) GetGasPrice() int64 {
	return mt.GasPrice
}

func (tx *Transaction) GetBytesWithoutSignature(withHash bool) []byte {

	b := tx.TxParam.GetBytes()
	bd, err := tx.TxData.GetBytes()
	if err != nil {
		return nil
	}
	b = append(b, bd...)
	b = append(b, common.GetByteInt64(tx.Height)...)
	b = append(b, common.GetByteInt64(tx.GasPrice)...)
	b = append(b, common.GetByteInt64(tx.GasUsage)...)
	if withHash {
		b = append(b, tx.GetHash().GetBytes()...)
	}
	return b
}

func (mt *Transaction) CalcHashAndSet() error {
	b := mt.GetBytesWithoutSignature(false)
	hash, err := common.CalcHashFromBytes(b)
	if err != nil {
		return err
	}
	mt.Hash = hash
	return nil
}

func (mt *Transaction) GetBytes() []byte {
	if mt != nil {
		b := mt.GetBytesWithoutSignature(true)
		sb := common.BytesToLenAndBytes(mt.GetSignature().GetBytes())
		b = append(b, sb...)
		b = append(b, mt.ContractAddress.GetBytes()...)
		olb := common.BytesToLenAndBytes(mt.OutputLogs)
		b = append(b, olb...)

		return b
	}
	return nil
}

func (mt *Transaction) StoreToDBPoolTx(prefix []byte) error {
	prefix = append(prefix, mt.GetHash().GetBytes()...)
	bt := mt.GetBytes()
	if len(bt) == 0 {
		return fmt.Errorf("transaction has no body. storing fails: StoreToDBPoolTx")
	}
	err := database.MainDB.Put(prefix, bt)
	if err != nil {
		return err
	}
	return nil
}

func (mt *Transaction) RemoveFromDBPoolTx(prefix []byte) error {
	prefix = append(prefix, mt.GetHash().GetBytes()...)
	err := database.MainDB.Delete(prefix)
	if err != nil {
		return err
	}
	return nil
}

func RemoveTransactionFromDBbyHash(prefix []byte, hash []byte) error {
	prefix = append(prefix, hash...)
	err := database.MainDB.Delete(prefix)
	if err != nil {
		return err
	}
	return nil
}

func LoadFromDBPoolTx(prefix []byte, hashTransaction []byte) (Transaction, error) {
	prefix2 := append(prefix, hashTransaction...)
	bt, err := database.MainDB.Get(prefix2)
	if err != nil {
		return Transaction{}, err
	}
	if len(bt) == 0 {
		err = database.MainDB.Delete(prefix2)
		if err != nil {
			return Transaction{}, err
		}
		return Transaction{}, fmt.Errorf("in database transaction has no bytes stored: %v", hashTransaction)
		//logger.GetLogger().Println("in database transaction has no bytes stored")
	}
	mt := &Transaction{}
	at, restb, err := mt.GetFromBytes(bt)
	if err != nil {
		return Transaction{}, err
	}
	if len(restb) > 0 {
		logger.GetLogger().Println("len(restb)", len(restb))
	}
	return at, nil
}

func CheckFromDBPoolTx(prefix []byte, hashTransaction []byte) bool {
	prefix = append(prefix, hashTransaction...)
	isKey, err := database.MainDB.IsKey(prefix)
	if err != nil {
		return false
	}
	return isKey
}

// Verify - checking if hash is correct and signature
func (tx *Transaction) Verify(sigName, sigName2 string, isPausedTmp, isPaused2Tmp bool) bool {
	recipientAddress := tx.TxData.Recipient
	n, err := account.IntDelegatedAccountFromAddress(recipientAddress)
	if tx.GetData().Amount < 0 && err != nil && n < 512 {
		logger.GetLogger().Println("transaction amount has to be larger or equal 0")
		return false
	}
	if tx.GetLockedAmount() > 0 {
		n, err := account.IntDelegatedAccountFromAddress(tx.GetDelegatedAccountForLocking())

		if n < 0 || n > 256 || err != nil {
			logger.GetLogger().Println("transaction locking must have delegated account properly set")
			return false
		}
		if tx.GetLockedAmount() < 0 {
			logger.GetLogger().Println("transaction locked amount has to be larger or equal 0")
			return false
		}
		if tx.GetLockedAmount() > tx.GetData().Amount {
			logger.GetLogger().Println("transaction locked amount has to be less or equal amount")
			return false
		}
		if tx.GetReleasePerBlock() < 0 {
			logger.GetLogger().Println("transaction release amount per block has to be larger or equal 0")
			return false
		}
		if tx.GetReleasePerBlock() > tx.GetLockedAmount() {
			logger.GetLogger().Println("transaction release amount per block has to be less or equal locked amount")
			return false
		}
	}

	canAccountBeModified := account.CanBeModifiedAccount(tx.TxData.Recipient.GetBytes())

	if canAccountBeModified == false && (tx.TxData.EscrowTransactionsDelay > 0 || tx.TxData.MultiSignNumber > 0) {
		logger.GetLogger().Println("Account cannot be modified")
		return false
	}

	//escrow check
	if tx.TxData.EscrowTransactionsDelay > 0 {
		if tx.TxData.EscrowTransactionsDelay > common.MaxTransactionDelay {
			logger.GetLogger().Println("transaction delay has to be less than ", common.MaxTransactionDelay)
			return false
		}
	} else if tx.TxData.EscrowTransactionsDelay < 0 {
		logger.GetLogger().Println("transaction delay must be larger than 0")
		return false
	}

	// multisign check
	if tx.TxData.MultiSignNumber > 0 {
		if int(tx.TxData.MultiSignNumber) > len(tx.TxData.MultiSignAddresses) {
			logger.GetLogger().Println("number of signatures cannot overflow number of defined addresses in multi sign account")
			return false
		}
	}
	b := tx.GetHash().GetBytes()
	err = tx.CalcHashAndSet()
	if err != nil {
		return false
	}
	if !bytes.Equal(b, tx.GetHash().GetBytes()) {
		logger.GetLogger().Println("check transaction hash fails")
		return false
	}
	signature := tx.GetSignature()
	primary := signature.GetBytes()[0] == 0

	pk := tx.TxData.GetPubKey()
	pkb := pk.GetBytes()
	if len(pkb) == 0 {
		pkp, err := pubkeys.LoadPubKeyWithPrimary(tx.GetSenderAddress(), primary)
		if err != nil {
			logger.GetLogger().Println("cannot load sender pubkey from DB:", err)
			return false
		}
		pkb = pkp.GetBytes()
	} else {
		// If pubkey is included in transaction, verify it matches the sender address
		senderAddr := tx.GetSenderAddress()
		pkAddr, err := common.PubKeyToAddress(pkb, primary)
		if err != nil {
			logger.GetLogger().Println("cannot derive address from pubkey in transaction:", err)
			return false
		}
		if !bytes.Equal(pkAddr.GetBytes(), senderAddr.GetBytes()) {
			logger.GetLogger().Println("pubkey in transaction does not match sender address:", pkAddr.GetHex(), "!=", senderAddr.GetHex())
			return false
		}
	}
	//logger.GetLogger().Println(sigName, sigName2, isPausedTmp, isPaused2Tmp)
	return wallet.Verify(b, signature.GetBytes(), pkb, sigName, sigName2, isPausedTmp, isPaused2Tmp)
}

func (tx *Transaction) Sign(w *wallet.Wallet, primary bool) error {
	b := tx.GetHash()
	sign, err := w.Sign(b.GetBytes(), primary)
	if err != nil {
		return err
	}
	tx.Signature = *sign
	return nil
}

func EmptyTransaction() Transaction {
	tx := Transaction{
		TxData: TxData{
			Recipient: common.EmptyAddress(),
			Amount:    0,
			OptData:   []byte{},
		},
		TxParam: TxParam{
			ChainID:     0,
			Sender:      common.EmptyAddress(),
			SendingTime: 0,
			Nonce:       0,
		},
		Hash:      common.EmptyHash(),
		Signature: common.Signature{},
		Height:    0,
		GasPrice:  0,
		GasUsage:  0,
	}
	err := tx.CalcHashAndSet()
	if err != nil {
		logger.GetLogger().Println("empty transaction calc hash fails")
	}
	tx.Signature = common.EmptySignature()
	return tx
}
