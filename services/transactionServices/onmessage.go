package transactionServices

import (
	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/logger"
	"github.com/okuralabs/okura-node/message"
	"github.com/okuralabs/okura-node/tcpip"
	"github.com/okuralabs/okura-node/transactionsDefinition"
	"github.com/okuralabs/okura-node/transactionsPool"
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
		if common.IsSyncing.Load() {
			return
		}
		msg := amsg.(message.TransactionsMessage)
		txn, err := msg.GetTransactionsFromBytes(common.SigName(), common.SigName2(), common.IsPaused(), common.IsPaused2())
		if err != nil {
			return
		}
		logger.GetLogger().Println("get tx from ", addr[:])
		if transactionsPool.PoolsTx.NumberOfTransactions() > common.MaxTransactionInPool {
			logger.GetLogger().Println("no more transactions can be accepted to the pool")
			return
		}
		// need to check transactions
		for _, v := range txn {
			for _, t := range v {
				if transactionsPool.PoolsTx.TransactionExists(t.Hash.GetBytes()) {
					logger.GetLogger().Println("transaction just exists in Pool")
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
					if !common.IsSyncing.Load() {
						//maybe we should not broadcast automatically transactions. Third party should care about it
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
					logger.GetLogger().Println("transaction just exists in Pool")
					continue
				}
				if transactionsDefinition.CheckFromDBPoolTx(common.TransactionDBPrefix[:], t.Hash.GetBytes()) {
					logger.GetLogger().Println("transaction just exists in DB")
					continue
				}

				isAdded := transactionsPool.PoolsTx.AddTransaction(t, t.Hash)
				if isAdded {
					logger.GetLogger().Println("transactions added to pool bx")
					err := t.StoreToDBPoolTx(common.TransactionDBPrefix[:])
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
				t, err := transactionsDefinition.LoadFromDBPoolTx(common.TransactionDBPrefix[:], hs)
				if err != nil {
					logger.GetLogger().Println("cannot load transaction from DB", err)
					continue
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
