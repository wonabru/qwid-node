package transactionServices

import (
	"bytes"
	"encoding/json"

	"github.com/wonabru/qwid-node/common"
	"github.com/wonabru/qwid-node/database"
	"github.com/wonabru/qwid-node/logger"
	"github.com/wonabru/qwid-node/message"
	"github.com/wonabru/qwid-node/tcpip"
	"github.com/wonabru/qwid-node/transactionsDefinition"
	"github.com/wonabru/qwid-node/transactionsPool"
)

func OnMessage(addr [4]byte, m []byte) {

	//logger.GetLogger().Println("New message nonce from:", addr)

	defer func() {
		if r := recover(); r != nil {
			//debug.PrintStack()
			logger.GetLogger().Println("recover (nonce Msg)", r)
		}

	}()

	isValid, amsg := message.CheckValidMessage(m)
	if isValid == false {
		logger.GetLogger().Println("transaction msg validation fails")
		tcpip.ReduceAndCheckIfBanIP(addr)
		return
	}

	switch string(amsg.GetHead()) {
	case "tx":

		msg := amsg.(message.TransactionsMessage)
		txn, err := msg.GetTransactionsFromBytes(common.SigName(), common.SigName2(), common.IsPaused(), common.IsPaused2())
		if err != nil {
			return
		}
		//logger.GetLogger().Println("get tx from ", addr[:])
		if transactionsPool.PoolsTx.NumberOfTransactions() > common.MaxTransactionInPool {
			logger.GetLogger().Println("no more transactions can be accepted to the pool")
			return
		}
		// need to check transactions
		for _, v := range txn {
			for _, t := range v {
				if transactionsPool.PoolsTx.TransactionExists(t.Hash.GetBytes()) {
					//logger.GetLogger().Println("transaction just exists in Pool")
					continue
				}
				if transactionsDefinition.CheckFromDBPoolTx(common.TransactionDBPrefix[:], t.Hash.GetBytes()) {
					logger.GetLogger().Println("transaction just exists in DB")
					continue
				}

				isAdded := transactionsPool.PoolsTx.AddTransaction(t, t.Hash)
				if isAdded {
					err := t.StoreToDBPoolTx(common.TransactionPoolHashesDBPrefix[:])
					if err != nil {
						transactionsPool.PoolsTx.RemoveTransactionByHash(t.Hash.GetBytes())
						err := transactionsDefinition.RemoveTransactionFromDBbyHash(common.TransactionPoolHashesDBPrefix[:], t.Hash.GetBytes())
						if err != nil {
							logger.GetLogger().Println(err)
						}
						logger.GetLogger().Println(err)
						continue
					}
					// Store pubkey immediately so it's available for nonce verification
					pk := t.TxData.Pubkey
					if len(pk.GetBytes()) > 0 {
						logger.GetLogger().Println("Storing pubkey from transaction immediately")
						storePubKeyFromTransaction(pk, t.GetSenderAddress())
					}
					// Always broadcast local transactions (from RPC/wallet with addr 0.0.0.0)
					// For remote transactions, only broadcast if not syncing
					isLocalTx := addr == [4]byte{0, 0, 0, 0}
					if isLocalTx || !common.IsSyncing.Load() {
						BroadcastTxn(addr, m)
					}
				}
			}
		}
	case "bx":
		// transaction in sync
		msg := amsg.(message.TransactionsMessage)
		txn, err := msg.GetTransactionsFromBytes(common.SigName(), common.SigName2(), common.IsPaused(), common.IsPaused2())
		if err != nil {
			return
		}
		logger.GetLogger().Println("get bx from ", addr[:])
		// need to check transactions
		for _, v := range txn {
			for _, t := range v {
				if transactionsPool.PoolsTx.TransactionExists(t.Hash.GetBytes()) {
					//logger.GetLogger().Println("transaction just exists in Pool. bx")
					continue
				}

				isAdded := transactionsPool.PoolsTx.AddTransaction(t, t.Hash)
				if isAdded {
					//logger.GetLogger().Println("transactions added to pool bx")
					err := t.StoreToDBPoolTx(common.TransactionPoolHashesDBPrefix[:])
					if err != nil {
						transactionsPool.PoolsTx.RemoveTransactionByHash(t.Hash.GetBytes())
						err := transactionsDefinition.RemoveTransactionFromDBbyHash(common.TransactionDBPrefix[:], t.Hash.GetBytes())
						if err != nil {
							logger.GetLogger().Println(err)
						}
						logger.GetLogger().Println(err)
					}
				}
			}
		}
	case "st":
		txn := amsg.(message.TransactionsMessage).GetTransactionsBytes()
		for topic, v := range txn {
			txs := []transactionsDefinition.Transaction{}
			for _, hs := range v {
				t, err := transactionsDefinition.LoadFromDBPoolTx(common.TransactionPoolHashesDBPrefix[:], hs)
				if err != nil {
					logger.GetLogger().Println("cannot load transaction from pool", err)

					transactionsDefinition.RemoveTransactionFromDBbyHash(common.TransactionPoolHashesDBPrefix[:], hs)
					continue
				}
				if len(t.GetBytes()) > 0 {
					txs = append(txs, t)
				}
			}
			transactionMsg, err := GenerateTransactionMsg(txs, []byte("tx"), topic)
			if err != nil {
				logger.GetLogger().Println("cannot generate transaction msg", err)
			}
			if !Send(addr, transactionMsg.GetBytes()) {
				logger.GetLogger().Println("could not send transaction in sync")
			}
			logger.GetLogger().Println("SENT transaction is sync st to ", addr[:])
		}
	case "bt":
		txn := amsg.(message.TransactionsMessage).GetTransactionsBytes()
		for topic, v := range txn {
			txs := []transactionsDefinition.Transaction{}
			for _, hs := range v {
				// First try to load from confirmed DB
				t, err := transactionsDefinition.LoadFromDBPoolTx(common.TransactionDBPrefix[:], hs)
				if err != nil {
					// If not in DB, try to load from Pool
					t, err = transactionsDefinition.LoadFromDBPoolTx(common.TransactionPoolHashesDBPrefix[:], hs)
					if err != nil {
						logger.GetLogger().Println("cannot load transaction from DB or Pool", err)
						continue
					}
				}
				if len(t.GetBytes()) > 0 {
					txs = append(txs, t)
				}
			}
			transactionMsg, err := GenerateTransactionMsg(txs, []byte("bx"), topic)
			if err != nil {
				logger.GetLogger().Println("cannot generate transaction msg", err)
			}
			if !Send(addr, transactionMsg.GetBytes()) {
				logger.GetLogger().Println("could not send transaction is sync bt")
			}
			logger.GetLogger().Println("SENT transaction is sync bt to ", addr[:])
		}
	default:
	}
}

// storePubKeyFromTransaction stores the pubkey from a transaction immediately
// so it's available for nonce verification before the block is processed
func storePubKeyFromTransaction(pk common.PubKey, senderAddr common.Address) {
	zeroBytes := make([]byte, common.AddressLength)
	// Derive address from pubkey bytes if not set
	if bytes.Equal(pk.Address.GetBytes(), zeroBytes) {
		derivedAddr, err := common.PubKeyToAddress(pk.GetBytes(), pk.Primary)
		if err != nil {
			logger.GetLogger().Println("storePubKeyFromTransaction: cannot derive address:", err)
			return
		}
		pk.Address = derivedAddr
	}
	// Set MainAddress if not set
	if bytes.Equal(pk.MainAddress.GetBytes(), zeroBytes) {
		pk.MainAddress = pk.Address
	}
	// Store in DB using the same method as blocks.StorePubKey
	if !bytes.Equal(pk.Address.GetBytes(), senderAddr.GetBytes()) {
		logger.GetLogger().Println("storePubKeyFromTransaction: pubkey address doesn't match sender, skipping")
		return
	}
	// Store pubkey marshal
	pkm, err := json.Marshal(pk)
	if err != nil {
		logger.GetLogger().Println("storePubKeyFromTransaction: marshal error:", err)
		return
	}
	err = database.MainDB.Put(append(common.PubKeyMarshalDBPrefix[:], pk.Address.GetBytes()...), pkm)
	if err != nil {
		logger.GetLogger().Println("storePubKeyFromTransaction: DB put error:", err)
		return
	}
	logger.GetLogger().Println("storePubKeyFromTransaction: stored pubkey for", pk.Address.GetHex())
}
