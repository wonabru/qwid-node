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
				//logger.GetLogger().Println("Processing transaction:", t.Hash.GetHex())
				pk := t.TxData.Pubkey
				//if len(pk.GetBytes()) > 0 {
				//	logger.GetLogger().Println("  Transaction has pubkey, length:", len(pk.GetBytes()))
				//}
				if transactionsPool.PoolsTx.TransactionExists(t.Hash.GetBytes()) {
					//logger.GetLogger().Println("  Transaction already exists in Pool, skipping")
					// Even if already in pool, store the pubkey if present
					if len(pk.GetBytes()) > 0 {
						//logger.GetLogger().Println("  Storing pubkey from existing transaction")
						storePubKeyFromTransaction(pk, t.GetSenderAddress())
					}
					continue
				}
				if transactionsDefinition.CheckFromDBPoolTx(common.TransactionDBPrefix[:], t.Hash.GetBytes()) {
					//logger.GetLogger().Println("  Transaction already exists in DB, skipping")
					// Even if already in DB, store the pubkey if present
					if len(pk.GetBytes()) > 0 {
						//logger.GetLogger().Println("  Storing pubkey from existing transaction in DB")
						storePubKeyFromTransaction(pk, t.GetSenderAddress())
					}
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
						//logger.GetLogger().Println("Storing pubkey from transaction immediately")
						storePubKeyFromTransaction(pk, t.GetSenderAddress())
					}
					// Always broadcast local transactions (from RPC/wallet with addr 0.0.0.0)
					// For remote transactions, only broadcast if not syncing
					isLocalTx := addr == [4]byte{0, 0, 0, 0}
					if isLocalTx { // || !common.IsSyncing.Load() {
						BroadcastTxn(addr, m)
					}
				}
			}
		}
	case "bx":
		// transaction in sync - during sync, skip signature verification because
		// the syncing node may not have sender pubkeys yet (stored during block processing).
		// Block merkle tree guarantees transaction integrity.
		msg := amsg.(message.TransactionsMessage)
		rawTxn := msg.GetTransactionsBytes()

		storedCount := 0
		for _, v := range rawTxn {
			for _, tb := range v {
				tx := transactionsDefinition.Transaction{}
				t, rest, err := tx.GetFromBytes(tb)
				if err != nil || len(rest) > 0 {
					logger.GetLogger().Println("bx: parse error:", err)
					continue
				}
				if transactionsDefinition.CheckFromDBPoolTx(common.TransactionDBPrefix[:], t.Hash.GetBytes()) {
					continue
				}
				if transactionsDefinition.CheckFromDBPoolTx(common.TransactionPoolHashesDBPrefix[:], t.Hash.GetBytes()) {
					continue
				}
				err = t.StoreToDBPoolTx(common.TransactionPoolHashesDBPrefix[:])
				if err != nil {
					logger.GetLogger().Printf("bx: FAILED to store transaction %x: %v", t.Hash.GetBytes()[:8], err)
				} else {
					storedCount++
				}
			}
		}
		logger.GetLogger().Printf("bx: stored %d transactions", storedCount)
	case "st":
		txn := amsg.(message.TransactionsMessage).GetTransactionsBytes()
		for topic, v := range txn {
			txs := []transactionsDefinition.Transaction{}
			for _, hs := range v {
				// First try to load from Pool
				t, err := transactionsDefinition.LoadFromDBPoolTx(common.TransactionPoolHashesDBPrefix[:], hs)
				if err != nil {
					// If not in Pool, try to load from confirmed DB
					t, err = transactionsDefinition.LoadFromDBPoolTx(common.TransactionDBPrefix[:], hs)
					if err != nil {
						logger.GetLogger().Println("cannot load transaction from Pool or DB", err)
						continue
					}
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
		logger.GetLogger().Println("Received bt request from", addr[:], "with", len(txn), "topics")
		for topic, v := range txn {
			logger.GetLogger().Println("  Topic:", topic, "requesting", len(v), "transactions")
			txs := []transactionsDefinition.Transaction{}
			for _, hs := range v {
				logger.GetLogger().Printf("  Looking for tx hash: %x", hs)
				// First try to load from confirmed DB
				t, err := transactionsDefinition.LoadFromDBPoolTx(common.TransactionDBPrefix[:], hs)
				if err != nil {
					// If not in confirmed DB, try to load from Pool
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
			logger.GetLogger().Println("  Found", len(txs), "transactions to send")
			transactionMsg, err := GenerateTransactionMsg(txs, []byte("bx"), topic)
			if err != nil {
				logger.GetLogger().Println("cannot generate transaction msg", err)
			}
			if !Send(addr, transactionMsg.GetBytes()) {
				logger.GetLogger().Println("could not send transaction is sync bt - Send failed")
			} else {
				logger.GetLogger().Println("SENT transaction is sync bt to ", addr[:], "count:", len(txs))
			}
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
		pk.MainAddress = senderAddr
	}
	// Store in DB using the same method as blocks.StorePubKey
	// For primary keys: pk.Address should match senderAddr
	// For secondary keys: pk.MainAddress should match senderAddr
	addressMatch := false
	if pk.Primary {
		addressMatch = bytes.Equal(pk.Address.GetBytes(), senderAddr.GetBytes())
	} else {
		addressMatch = bytes.Equal(pk.MainAddress.GetBytes(), senderAddr.GetBytes())
	}
	if !addressMatch {
		logger.GetLogger().Println("storePubKeyFromTransaction: pubkey address doesn't match sender, skipping")
		logger.GetLogger().Println("  pk.Primary:", pk.Primary)
		logger.GetLogger().Println("  pk.Address:", pk.Address.GetHex())
		logger.GetLogger().Println("  pk.MainAddress:", pk.MainAddress.GetHex())
		logger.GetLogger().Println("  senderAddr:", senderAddr.GetHex())
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
