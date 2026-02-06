package transactionsPool

import (
	"fmt"

	"github.com/wonabru/qwid-node/common"
	"github.com/wonabru/qwid-node/logger"
	"github.com/wonabru/qwid-node/transactionsDefinition"
)

func RemoveBadTransactionByHash(hash []byte, height int64, tree *MerkleTree) error {
	PoolsTx.RemoveTransactionByHash(hash)
	PoolTxEscrow.RemoveTransactionByHash(hash)
	PoolTxMultiSign.RemoveTransactionByHash(hash)

	// Try to load transaction from pool or confirmed DB before removing
	tx, err := transactionsDefinition.LoadFromDBPoolTx(common.TransactionPoolHashesDBPrefix[:], hash)
	if err != nil {
		tx, err = transactionsDefinition.LoadFromDBPoolTx(common.TransactionDBPrefix[:], hash)
	}

	// Store to bad transaction DB so other nodes can still sync it
	if err == nil && len(tx.GetBytes()) > 0 {
		err = tx.StoreToDBPoolTx(common.BadTransactionDBPrefix[:])
		if err != nil {
			logger.GetLogger().Println("failed to store bad transaction:", err)
		}
	}

	err = transactionsDefinition.RemoveTransactionFromDBbyHash(common.TransactionPoolHashesDBPrefix[:], hash)
	if err != nil {
		logger.GetLogger().Println(err)
	}
	// NOTE: Do NOT delete from confirmed DB (TransactionDBPrefix) - other nodes need these
	// transactions for sync. Only remove from pool DB.
	err = transactionsDefinition.RemoveTransactionFromDBbyHash(common.TransactionDBPrefix[:], hash)
	if err != nil {
		logger.GetLogger().Println(err)
	}
	err = CheckTransactionInDBAndInMarkleTrie(hash, tree)
	if err == nil {
		logger.GetLogger().Println("transaction is in trie")
	}
	err = RemoveMerkleTrieFromDB(height)
	if err != nil {
		logger.GetLogger().Println(err)
	}
	PoolsTx.BanTransactionByHash(hash)
	PoolTxEscrow.BanTransactionByHash(hash)
	PoolTxMultiSign.BanTransactionByHash(hash)
	return nil
}

func CheckTransactionInDBAndInMarkleTrie(hash []byte, tree *MerkleTree) error {
	if transactionsDefinition.CheckFromDBPoolTx(common.TransactionDBPrefix[:], hash) {
		dbTx, err := transactionsDefinition.LoadFromDBPoolTx(common.TransactionDBPrefix[:], hash)
		if err != nil {
			//TODO
			//transactionsDefinition.RemoveTransactionFromDBbyHash(common.TransactionDBPrefix[:], hash)
			return err
		}
		h := dbTx.Height

		txHeight, err := FindTransactionInBlocks(hash, h)
		if err != nil {
			if !tree.IsTxHashInTree(hash) {
				return nil
			}
			return err
		}

		if txHeight <= 0 {
			logger.GetLogger().Println("transaction not in merkle tree. removing transaction: checkTransactionInDBAndInMarkleTrie")
		} else {
			return fmt.Errorf("transaction was previously added in chain: checkTransactionInDBAndInMarkleTrie")
		}
	}
	return nil
}
