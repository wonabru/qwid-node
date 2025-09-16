package blocks

import (
	"bytes"
	"fmt"
	"github.com/okuralabs/okura-node/account"
	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/logger"
	"github.com/okuralabs/okura-node/transactionsDefinition"
	"github.com/okuralabs/okura-node/transactionsPool"
)

var ZerosHash = make([]byte, common.HashLength)

func CheckStakingTransaction(tx transactionsDefinition.Transaction, sumAmount int64, sumFee int64) bool {
	fee := tx.GasPrice * tx.GasUsage
	amount := tx.TxData.Amount
	address := tx.GetSenderAddress()
	acc, exist := account.GetAccountByAddressBytes(address.GetBytes())
	if !exist || !bytes.Equal(acc.Address[:], address.GetBytes()) {
		logger.GetLogger().Println("no account found in check staking transaction: CheckStakingTransaction")
		return false
	}
	if acc.Balance < fee {
		logger.GetLogger().Println("not enough funds on account to cover fee: CheckStakingTransaction")
		return false
	}
	if acc.Balance < sumFee {
		logger.GetLogger().Println("not enough funds on account to cover sumFee: CheckStakingTransaction")
		return false
	}
	addressRecipient := tx.TxData.Recipient
	var err error
	var n int
	if tx.GetLockedAmount() > 0 {
		n, err = account.IntDelegatedAccountFromAddress(tx.TxData.DelegatedAccountForLocking)
		if n <= 0 || n >= 256 || err != nil {
			fmt.Println("DelegatedAccountForLocking must be a delegated account less than 256: CheckStakingTransaction")
			return false
		}
	} else {
		n, err = account.IntDelegatedAccountFromAddress(addressRecipient)
	}
	if n > 0 && n < 256 {
		if tx.GetLockedAmount() > 0 {

			if amount <= 0 {
				logger.GetLogger().Println("when locking no withdrawals allows: CheckStakingTransaction")
				return false
			}

			if amount < common.MinStakingUser && amount > 0 {
				logger.GetLogger().Println("staking amount has to be larger than ", common.MinStakingUser, ": CheckStakingTransaction")
				return false
			}
			if tx.GetLockedAmount() < 0 {
				logger.GetLogger().Println("locked amount has to be larger or equal than ", 0, ": CheckStakingTransaction")
				return false
			}
			if tx.GetLockedAmount() > amount {
				logger.GetLogger().Println("locked amount has to be less or equal than ", amount, ": CheckStakingTransaction")
				return false
			}
			if tx.GetReleasePerBlock() < 0 {
				logger.GetLogger().Println("release per block has to be larger or equal than ", 0, ": CheckStakingTransaction")
				return false
			}
			if tx.GetReleasePerBlock() > tx.GetLockedAmount() {
				logger.GetLogger().Println("release per block has to be less or equal than ", tx.GetLockedAmount(), ": CheckStakingTransaction")
				return false
			}

		} else {
			accStaking := account.GetStakingAccountByAddressBytes(address.GetBytes(), n)
			if !bytes.Equal(accStaking.DelegatedAccount[:], addressRecipient.GetBytes()) {
				if amount <= 0 {
					logger.GetLogger().Println(n, address.GetHex(), common.Bytes2Hex(accStaking.DelegatedAccount[:]), " != ", common.Bytes2Hex(addressRecipient.GetBytes()))
					logger.GetLogger().Println("no staking account found in check staking transaction", ": CheckStakingTransaction")
					return false
				}

			}
			if amount < common.MinStakingUser && amount > 0 {
				logger.GetLogger().Println("staking amount has to be larger than ", common.MinStakingUser, ": CheckStakingTransaction")
				return false
			}
			if accStaking.StakedBalance+amount < common.MinStakingUser && accStaking.StakedBalance+amount != 0 {
				logger.GetLogger().Println("not enough staked balance. Staking has to be larger than ", common.MinStakingUser, ": CheckStakingTransaction")
				return false
			}
			// check for all transactions together

			if sumAmount < common.MinStakingUser && sumAmount > 0 {
				logger.GetLogger().Println("staking amount has to be larger than ", common.MinStakingUser, ": CheckStakingTransaction")
				return false
			}
			if accStaking.StakedBalance+sumAmount < common.MinStakingUser && accStaking.StakedBalance+sumAmount != 0 {
				logger.GetLogger().Println("not enough staked balance. Staking has to be larger than ", common.MinStakingUser, ": CheckStakingTransaction")
				return false
			}
		}
	}
	if n >= 256 && n < 512 {

		accStaking := account.GetStakingAccountByAddressBytes(address.GetBytes(), n%256)
		if !bytes.Equal(accStaking.Address[:], address.GetBytes()) {
			logger.GetLogger().Println("no staking account found in check staking transaction (rewards)", ": CheckStakingTransaction")
			return false
		}
		if accStaking.StakingRewards+amount < 0 {
			logger.GetLogger().Println("not enough rewards balance. Rewards has to be larger than ", 0, ": CheckStakingTransaction")
			return false
		}
	}
	return true
}

func ProcessMultiSignAndEscrow(tx transactionsDefinition.Transaction) error {

	acc := account.SetAccountByAddressBytes(tx.TxData.Recipient.ByteValue[:])

	// modify escrow parameters
	if tx.TxData.EscrowTransactionsDelay > 0 {
		err := acc.ModifyAccountToEscrow(tx.TxData.EscrowTransactionsDelay)
		if err != nil {
			return err
		}
	}

	// modify multi sign account
	if tx.TxData.MultiSignNumber > 0 {
		accAddreses := make([]common.Address, len(tx.TxData.MultiSignAddresses))
		for i, addr := range tx.TxData.MultiSignAddresses {
			a := &common.Address{}
			err := a.Init(addr[:])
			if err != nil {
				return err
			}
			accAddreses[i] = *a
		}
		err := acc.ModifyAccountToMultiSign(tx.TxData.MultiSignNumber, accAddreses)
		if err != nil {
			return err
		}
	}
	return nil
}

func ProcessTransaction(tx transactionsDefinition.Transaction, height int64) error {
	fee := tx.GasPrice * tx.GasUsage
	amount := tx.TxData.Amount
	operational := len(tx.TxData.OptData) > 0
	address := tx.GetSenderAddress()
	account.AddTransactionsSender(address.ByteValue, tx.GetHash())
	addressRecipient := tx.TxData.Recipient
	account.AddTransactionsRecipient(addressRecipient.ByteValue, tx.GetHash())
	var err error
	var n int
	if tx.GetLockedAmount() > 0 {
		n, err = account.IntDelegatedAccountFromAddress(tx.TxData.DelegatedAccountForLocking)
		if n <= 0 || n >= 256 || err != nil {
			return fmt.Errorf("DelegatedAccountForLocking must be a delegated account less than 256: ProcessTransaction")
		}
	} else {
		n, err = account.IntDelegatedAccountFromAddress(addressRecipient)
	}
	if err == nil { // this is delegated account
		if n > 0 && n < 256 { // this is staking transaction

			if tx.GetLockedAmount() > 0 {
				if amount >= common.MinStakingUser {
					err := account.Stake(addressRecipient.GetBytes(), amount, height, n, operational, tx.GetLockedAmount(), tx.GetReleasePerBlock())
					if err != nil {
						return err
					}
				} else {
					return fmt.Errorf("wrong amount in locking: ProcessTransaction")
				}
				err = AddBalance(address.ByteValue, -fee-amount)
				if err != nil {
					return err
				}
			} else {
				if amount >= common.MinStakingUser {
					err := account.Stake(address.GetBytes(), amount, height, n, operational, 0, 0)
					if err != nil {
						return err
					}
				} else if amount < 0 {
					err := account.Unstake(address.GetBytes(), amount, height, n)
					if err != nil {
						return err
					}

				} else {
					return fmt.Errorf("wrong amount in staking/unstaking: ProcessTransaction")
				}
				err = AddBalance(address.ByteValue, -fee-amount)
				if err != nil {
					return err
				}
			}
			err := ProcessMultiSignAndEscrow(tx)
			if err != nil {
				return err
			}
		}
		if n >= 256 && n < 512 { // this is reward withdrawal transaction

			accStaking := account.GetStakingAccountByAddressBytes(address.GetBytes(), n%256)
			if !bytes.Equal(accStaking.Address[:], address.GetBytes()) {
				return fmt.Errorf("no staking account found in check staking transaction (rewards): ProcessTransaction")
			}
			if amount > 0 {
				logger.GetLogger().Println("not implemented: ProcessTransaction")
				//err := account.Reward(accStaking.Address[:], amount, height, n%256)
				//if err != nil {
				//	return err
				//}
			} else if amount < 0 {
				err := account.WithdrawReward(accStaking.Address[:], amount, height, n%256)
				if err != nil {
					return err
				}
				err = AddBalance(address.ByteValue, -fee-amount)
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("wrong amount in rewarding: ProcessTransaction")
			}
		}
	} else { // this is not delegated account so standard transaction

		senderAcc, exist := account.GetAccountByAddressBytes(address.GetBytes())
		if !exist {
			return fmt.Errorf("no account found")
		}
		if senderAcc.TransactionDelay > 0 && tx.GetHeight()+senderAcc.TransactionDelay > height && bytes.Equal(tx.TxParam.MultiSignTx.GetBytes(), ZerosHash) {
			transactionsPool.PoolTxEscrow.AddTransaction(tx, tx.Hash)

		} else if senderAcc.MultiSignNumber > 0 && bytes.Equal(tx.TxParam.MultiSignTx.GetBytes(), ZerosHash) {
			//TODO MultiSignNumber

			transactionsPool.PoolTxMultiSign.AddTransaction(tx, tx.Hash)
		} else {
			if bytes.Equal(tx.TxParam.MultiSignTx.GetBytes(), ZerosHash) == false {
				transactionsPool.PoolTxMultiSign.AddTransaction(tx, tx.TxParam.MultiSignTx)
			}
			err = AddBalance(address.ByteValue, -amount)
			if err != nil {
				return err
			}

			err = AddBalance(addressRecipient.ByteValue, amount)
			if err != nil {
				return err
			}
		}
		// escrow tx and multisigned should be paid fee upfront
		err = AddBalance(address.ByteValue, -fee)
		if err != nil {
			return err
		}

		err := ProcessMultiSignAndEscrow(tx)
		if err != nil {
			return err
		}
	}
	return nil
}

func ProcessTransactionsMultiSign(tx transactionsDefinition.Transaction, height int64, tree *transactionsPool.MerkleTree) error {

	if bytes.Equal(tx.TxParam.MultiSignTx.GetBytes(), ZerosHash) {
		return nil
	}

	txs := transactionsPool.PoolTxMultiSign.PeekTransactions(common.MaxTransactionInPool, common.GetInt64FromByte(tx.TxParam.MultiSignTx.GetBytes()))

	mainTx := transactionsDefinition.Transaction{}
	for _, t := range txs {
		if bytes.Equal(t.Hash.GetBytes(), tx.TxParam.MultiSignTx.GetBytes()) {
			mainTx = t
			break
		}
	}

	if len(mainTx.GetBytes()) == 0 {
		for _, t := range txs {
			transactionsPool.PoolTxMultiSign.RemoveTransactionByHash(t.Hash.GetBytes())
		}
		return fmt.Errorf("no main transaction in multi signature pool")
	}

	// remove transactions related to main if more than a week in pool
	if height-mainTx.GetHeight() > common.MaxTransactionInMultiSigPool {
		for _, t := range txs {
			transactionsPool.PoolTxMultiSign.RemoveTransactionByHash(t.Hash.GetBytes())
		}
		return fmt.Errorf("no main transaction in multi signature pool")
	}

	acc, exist := account.GetAccountByAddressBytes(mainTx.TxParam.Sender.GetBytes())
	if !exist {
		return fmt.Errorf("no account found: MultiSign")
	}
	if len(txs) < int(acc.MultiSignNumber) {
		logger.GetLogger().Println("not enough signatures for transactions to process ", tx.TxParam.MultiSignTx.GetHex())
		return nil
	}
	numApprovals := 0
	notApprovedYet := acc.MultiSignAddresses[:]
	for _, t := range txs {
		sender := t.TxParam.Sender.ByteValue
		appi := -1
		for i, appr := range notApprovedYet {
			if sender == appr && bytes.Equal(mainTx.TxData.Recipient.GetBytes(), t.TxData.Recipient.GetBytes()) && t.TxData.Amount == 0 {
				numApprovals++
				appi = i
				break
			}
		}
		if appi > 0 {
			notApprovedYet = append(notApprovedYet[:appi], notApprovedYet[appi+1:]...)
		} else if appi == 0 {
			notApprovedYet = notApprovedYet[1:]
		}
	}
	if numApprovals < int(acc.MultiSignNumber) {
		logger.GetLogger().Println("not enough signatures for transactions to process ", tx.TxParam.MultiSignTx.GetHex())
		return nil
	}

	// transaction should be executed

	amount := mainTx.TxData.Amount
	address := mainTx.GetSenderAddress()
	addressRecipient := mainTx.TxData.Recipient
	var err error
	var n int
	if mainTx.GetLockedAmount() > 0 {
		n, err = account.IntDelegatedAccountFromAddress(mainTx.TxData.DelegatedAccountForLocking)
		if n <= 0 || n >= 256 || err != nil {
			return fmt.Errorf("DelegatedAccountForLocking must be a delegated account less than 256: ProcessTransaction")
		}
	} else {
		n, err = account.IntDelegatedAccountFromAddress(addressRecipient)
	}
	if err == nil { // delegated account any transfer should be processed for staking unstaking and reward withdrawal
		return nil
	} else { // this is not delegated account so standard transaction

		if acc.TransactionDelay > 0 && mainTx.GetHeight()+acc.TransactionDelay > height {
			return fmt.Errorf("transaction should not be executed, should be delayed %v", mainTx.Hash.GetHex())
		} else {
			transactionsPool.PoolTxMultiSign.RemoveTransactionByHash(mainTx.Hash.GetBytes())
			err = AddBalance(address.ByteValue, -amount)
			if err != nil {
				// this can happen very rare. Only when escrow is multisign account
				transactionsPool.RemoveBadTransactionByHash(mainTx.Hash.GetBytes(), height, tree)
				return err
			}

			// amount is always >= 0, so no error here will be
			err = AddBalance(addressRecipient.ByteValue, amount)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func ProcessTransactionsEscrow(height int64, tree *transactionsPool.MerkleTree) error {

	txs := transactionsPool.PoolTxEscrow.PeekTransactions(common.MaxTransactionInPool, height)

	for _, tx := range txs {

		amount := tx.TxData.Amount
		address := tx.GetSenderAddress()
		addressRecipient := tx.TxData.Recipient
		var err error
		var n int
		if tx.GetLockedAmount() > 0 {
			n, err = account.IntDelegatedAccountFromAddress(tx.TxData.DelegatedAccountForLocking)
			if n <= 0 || n >= 256 || err != nil {
				return fmt.Errorf("DelegatedAccountForLocking must be a delegated account less than 256: ProcessTransaction")
			}
		} else {
			n, err = account.IntDelegatedAccountFromAddress(addressRecipient)
		}
		if err == nil { // delegated account any transfer should be processed for staking unstaking and reward withdrawal
			return nil
		} else { // this is not delegated account so standard transaction
			senderAcc, exist := account.GetAccountByAddressBytes(address.GetBytes())
			if !exist {
				return fmt.Errorf("no account found: Escrow")
			}
			if senderAcc.TransactionDelay > 0 && tx.GetHeight()+senderAcc.TransactionDelay > height && bytes.Equal(tx.TxParam.MultiSignTx.GetBytes(), ZerosHash) {
				return fmt.Errorf("transaction should not be executed %v", tx.Hash.GetHex())
			} else if senderAcc.MultiSignNumber > 0 && bytes.Equal(tx.TxParam.MultiSignTx.GetBytes(), ZerosHash) {
				if transactionsPool.PoolTxMultiSign.AddTransaction(tx, tx.Hash) {
					transactionsPool.PoolTxEscrow.RemoveTransactionByHash(tx.Hash.GetBytes())
				}
			} else {
				if bytes.Equal(tx.TxParam.MultiSignTx.GetBytes(), ZerosHash) == false {
					transactionsPool.PoolTxMultiSign.AddTransaction(tx, tx.TxParam.MultiSignTx)
				}
				transactionsPool.PoolTxEscrow.RemoveTransactionByHash(tx.Hash.GetBytes())
				err = AddBalance(address.ByteValue, -amount)
				if err != nil {
					// this can happen very rare. Only when escrow is multisign account
					transactionsPool.RemoveBadTransactionByHash(tx.Hash.GetBytes(), height, tree)
					return err
				}

				// amount is always >= 0, so no error here will be
				err = AddBalance(addressRecipient.ByteValue, amount)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
