package services

import (
	"fmt"
	"github.com/okuralabs/okura-node/blocks"
	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/logger"
	"github.com/okuralabs/okura-node/wallet"
	"golang.org/x/crypto/ssh/terminal"
	"os"
)

func checkBlock(newBlock blocks.Block, lastBlock blocks.Block, checkFinal bool) error {
	logger.GetLogger().Println("checkFinal:", checkFinal)
	merkleTrie, err := blocks.CheckBaseBlock(newBlock, lastBlock)
	defer merkleTrie.Destroy()
	if err != nil {
		logger.GetLogger().Println(err)
		return err
	}
	hashesMissing := blocks.IsAllTransactions(newBlock)
	if len(hashesMissing) > 0 {
		return fmt.Errorf("missing transactions in db %v", hashesMissing)
	}

	err = blocks.CheckBlockAndTransactions(&newBlock, lastBlock, merkleTrie, checkFinal)
	if err != nil {
		logger.GetLogger().Println("check transfer transactions in block fails", err)
		return err
	}
	return nil
}

func checkMainChain() (int64, error) {
	lastBlock, err := blocks.LoadBlock(0)
	if err != nil {
		return -1, err
	}

	height, err := blocks.LastHeightStoredInBlocks()
	if err != nil {
		logger.GetLogger().Println(err)
		return height - 1, err
	}
	logger.GetLogger().Println("blocks.LastHeightStoredInBlocks() height: ", height)
	if height > 1 {
		cw, err := wallet.GetCurrentWallet(height)
		if err == nil {
			wallet.SetActiveWallet(cw)
			bl, err := blocks.LoadBlock(height)
			if err != nil {
				logger.GetLogger().Println(err)
			} else {
				lastBlock, err := blocks.LoadBlock(height - 1)
				if err != nil {
					logger.GetLogger().Println(err)
				} else {
					err = checkBlock(bl, lastBlock, true)
					if err != nil {
						logger.GetLogger().Println(err)

					} else {
						return height, nil
					}
				}
			}
		}
	}
	logger.GetLogger().Println("Last stored index of blocks is ", height)
	if height == 0 {
		logger.GetLogger().Println("Starting blockchain from scratch...")
		return 0, fmt.Errorf("bad chain storage")
	}
	ResetAccountsAndBlocksSync(height - 1)
	for h := int64(1); h < height; h++ {
		bl, err := blocks.LoadBlock(h)
		if err != nil {
			logger.GetLogger().Println(err)

			//err = checkBlock(bl, lastBlock, true)
			return h - 1, err
		}
		cw, err := wallet.GetCurrentWallet(height)
		if err == nil {
			wallet.SetActiveWallet(cw)
			err = checkBlock(bl, lastBlock, h == height-1)
			if err != nil {
				logger.GetLogger().Println(err)
				//err = checkBlock(bl, lastBlock, true)
				logger.GetLogger().Println("Error in block height ", h)
				return h - 1, err
			}
		} else {
			logger.GetLogger().Println("Error get Current Wallet ", h)
			return h - 1, fmt.Errorf("error get current wallet")
		}
		lastBlock = bl
	}
	return height, nil
}

func SetBlockHeightAfterCheck() {
	height, err := checkMainChain()
	if err != nil && height >= 0 {
		logger.GetLogger().Println(err)
		// Get home directory
		homePath, err := os.UserHomeDir()
		if err != nil {
			logger.GetLogger().Fatal("failed to get home directory:", err)
		}
		fmt.Print("Should db with blockchain state should be removed and sync from beginning? Yes/[No]: ")
		answer, err := terminal.ReadPassword(0)
		if string(answer) == "Yes" {
			// remove database related to blockckchain, NOT wallets
			os.RemoveAll(homePath + common.DefaultBlockchainHomePath)
			logger.GetLogger().Fatal("DB files related to chain was removed. run mining once more and sync with other nodes. wrong data stored in db")
		} else {
			logger.GetLogger().Fatal("DB files related to chain was NOT removed. Please run mining once more and sync with other nodes.")
		}
		return
	}
	common.SetHeight(height)

}
