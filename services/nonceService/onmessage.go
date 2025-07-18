package nonceServices

import (
	"bytes"
	"github.com/okuralabs/okura-node/logger"
	"runtime/debug"

	"github.com/okuralabs/okura-node/account"
	"github.com/okuralabs/okura-node/blocks"
	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/message"
	"github.com/okuralabs/okura-node/oracles"
	"github.com/okuralabs/okura-node/services"
	"github.com/okuralabs/okura-node/services/transactionServices"
	"github.com/okuralabs/okura-node/statistics"
	"github.com/okuralabs/okura-node/tcpip"
	"github.com/okuralabs/okura-node/transactionsDefinition"
	"github.com/okuralabs/okura-node/transactionsPool"
	"github.com/okuralabs/okura-node/voting"
)

func OnMessage(addr [4]byte, m []byte) {
	if common.IsSyncing.Load() {
		return
	}

	h := common.GetHeight()

	logger.GetLogger().Println("New message nonce from:", addr)
	defer func() {
		if r := recover(); r != nil {
			debug.PrintStack()
			logger.GetLogger().Println("recover (nonce Msg)", r)
		}

	}()

	isValid, amsg := message.CheckValidMessage(m)
	if isValid == false {
		logger.GetLogger().Println("nonce msg validation fails")
		tcpip.ReduceAndCheckIfBanIP(addr)
		return
	}
	tcpip.ValidRegisterPeer(addr)
	switch string(amsg.GetHead()) {
	case "nn": // nonce

		//common.NonceMutex.Lock()
		//defer common.NonceMutex.Unlock()
		//fmt.Printf("%v", nonceTransaction)
		//var topic [2]byte
		txn, err := amsg.(message.TransactionsMessage).GetTransactionsFromBytes(common.SigName(), common.SigName2(), common.IsPaused(), common.IsPaused2())
		if err != nil {
			return
		}
		nonceTransaction := map[[2]byte]transactionsDefinition.Transaction{}

		for k, v := range txn {
			nonceTransaction[k] = v[0]
		}
		var transaction transactionsDefinition.Transaction
		for _, v := range nonceTransaction {
			transaction = v
			break
		}

		nonceHeight := transaction.GetHeight()
		// checking if proper height
		if nonceHeight < 1 || nonceHeight != h+1 {
			//logger.GetLogger().Print("nonce height invalid")
			return
		}

		isValid = transaction.Verify(common.SigName(), common.SigName2(), common.IsPaused(), common.IsPaused2())
		if isValid == false {
			logger.GetLogger().Println("nonce signature is invalid")
			tcpip.ReduceAndCheckIfBanIP(addr)
			return
		}
		//register checked Node IP
		tcpip.NodeRegisterPeer(addr)
		txDelAcc := transaction.TxData.Recipient
		n, err := account.IntDelegatedAccountFromAddress(txDelAcc)
		if err != nil {
			return
		}
		//delMy := common.GetDelegatedAccount()
		//if addr != tcpip.MyIP && bytes.Equal(txDelAcc.GetBytes(), delMy.GetBytes()) && addr != [4]byte{0, 0, 0, 0} {
		//	MyIP2 = addr
		//}
		// get oracles from nonce transaction
		optData := transaction.TxData.OptData[8+common.HashLength:]
		_, stakedInDelAcc, _ := account.GetStakedInDelegatedAccount(n)
		stakedInDelAccInt := int64(stakedInDelAcc)

		err = oracles.SavePriceOracle(common.GetInt64FromByte(optData[:8]), nonceHeight, txDelAcc, stakedInDelAccInt)
		if err != nil {
			logger.GetLogger().Println("could not save price oracle", err)
		}
		err = oracles.SaveRandOracle(common.GetInt64FromByte(optData[8:16]), nonceHeight, txDelAcc, stakedInDelAccInt)
		if err != nil {
			logger.GetLogger().Println("could not save rand oracle", err)
		}

		vb, b2, err := common.BytesWithLenToBytes(optData[16:])
		if err != nil {
			logger.GetLogger().Println("could not save voting, parse bytes fails, 1", err)
		}
		err = voting.SaveVotesEncryption1(vb[:], nonceHeight, txDelAcc, stakedInDelAccInt)
		if err != nil {
			logger.GetLogger().Println("could not save voting, 1", err)
		}
		vb, b2, err = common.BytesWithLenToBytes(b2[:])
		if err != nil {
			logger.GetLogger().Println("could not save voting, parse bytes fails, 2", err)
		}
		err = voting.SaveVotesEncryption2(vb[:], nonceHeight, txDelAcc, stakedInDelAccInt)
		if err != nil {
			logger.GetLogger().Println("could not save voting, 2", err)
		}

		mainAddress := transaction.TxParam.Sender

		// checking if enough coins staked
		if _, sumStaked, operationalAcc := account.GetStakedInDelegatedAccount(n); int64(sumStaked) < common.MinStakingForNode || !bytes.Equal(operationalAcc.Address[:], mainAddress.GetBytes()) {
			logger.GetLogger().Println("not enough staked coins to be a node or not valid operational account", sumStaked, common.MinStakingForNode, operationalAcc.Address[:5], mainAddress.GetBytes()[:5])
			tcpip.ReduceAndCheckIfBanIP(addr)
			return
		}

		lastBlock, err := blocks.LoadBlock(h)
		if err != nil {
			logger.GetLogger().Println(err)
			return
		}

		txs := transactionsPool.PoolsTx.PeekTransactions(int(common.MaxTransactionsPerBlock), nonceHeight)
		txsBytes := make([][]byte, len(txs))
		transactionsHashes := []common.Hash{}
		for _, tx := range txs {
			hash := tx.GetHash().GetBytes()
			transactionsHashes = append(transactionsHashes, tx.GetHash())
			txsBytes = append(txsBytes, hash)
		}
		merkleTrie, err := transactionsPool.BuildMerkleTree(h+1, txsBytes, transactionsPool.GlobalMerkleTree.DB)
		defer merkleTrie.Destroy()

		if err != nil {
			logger.GetLogger().Println("cannot build merkleTrie")
			return
		}

		newBlock, err := services.CreateBlockFromNonceMessage([]transactionsDefinition.Transaction{transaction},
			lastBlock,
			merkleTrie,
			transactionsHashes)

		if err != nil {
			logger.GetLogger().Println(err)
			return

		}

		if newBlock.CheckProofOfSynergy() {
			_, _, err := blocks.CheckBlockTransfers(newBlock, lastBlock, false)
			if err == nil {
				services.BroadcastBlock(newBlock)
			} else {
				logger.GetLogger().Println("new block is not valid. Bad transactions included")
			}
		} else {
			//logger.GetLogger().Println("new block is not valid")
		}
		return
	case "rb": //reject block

	case "bl": //block

		common.BlockMutex.Lock()
		defer common.BlockMutex.Unlock()

		lastBlock, err := blocks.LoadBlock(h)
		if err != nil {
			logger.GetLogger().Println(err)
			return
		}
		txnbytes := amsg.GetTransactionsBytes()
		bls := map[[2]byte]blocks.Block{}
		for k, v := range txnbytes {
			if k[0] == byte('N') {

				bls[k], err = bls[k].GetFromBytes(v[0])
				newBlock := bls[k]
				if err != nil {
					logger.GetLogger().Println(err)
					logger.GetLogger().Println("cannot load blocks from bytes")
					tcpip.ReduceAndCheckIfBanIP(addr)
					return
				}

				// Special logging for second block in nonce service
				if newBlock.GetHeader().Height == 1 {
					logger.GetLogger().Printf("=== Processing second block in nonce service ===")
					logger.GetLogger().Printf("Current height: %d", h)
					logger.GetLogger().Printf("Second block hash: %x", newBlock.BlockHash.GetBytes())
					logger.GetLogger().Printf("Second block previous hash: %x", newBlock.GetHeader().PreviousHash.GetBytes())
					logger.GetLogger().Printf("Genesis block hash: %x", lastBlock.BlockHash.GetBytes())
					logger.GetLogger().Printf("Is initial sync: %v", h == 0)
				}

				if newBlock.GetHeader().Height != h+1 {
					logger.GetLogger().Println("block of too short chain")
					return
				}
				merkleTrie, err := blocks.CheckBaseBlock(newBlock, lastBlock, true)
				defer merkleTrie.Destroy()
				if err != nil {
					logger.GetLogger().Println(err)
					tcpip.ReduceAndCheckIfBanIP(addr)
					return
				}
				hashesMissing := blocks.IsAllTransactions(newBlock)
				if len(hashesMissing) > 0 {
					transactionServices.SendGT(addr, hashesMissing, "st")
					return
				}

				err = blocks.CheckBlockAndTransferFunds(&newBlock, lastBlock, merkleTrie, true)
				if err != nil {
					services.ResetAccountsAndBlocksSync(lastBlock.GetHeader().Height)
					logger.GetLogger().Println("check transfer transactions in block fails", err)
					return
				}
				err = newBlock.StoreBlock()
				if err != nil {
					logger.GetLogger().Println(err)
					logger.GetLogger().Println("cannot store block")
					services.ResetAccountsAndBlocksSync(lastBlock.GetHeader().Height)
					return
				}

				logger.GetLogger().Println("New Block success -------------------------------------", h+1)
				err = account.StoreAccounts(newBlock.GetHeader().Height)
				if err != nil {
					logger.GetLogger().Println(err)
				}

				err = account.StoreStakingAccounts(newBlock.GetHeader().Height)
				if err != nil {
					logger.GetLogger().Println(err)
				}
				common.SetHeight(h + 1)
				sm := statistics.GetStatsManager()
				sm.UpdateStatistics(newBlock, lastBlock)
				logger.GetLogger().Println("TPS: ", sm.Stats.Tps)
			}
		}
	default:
	}
}
