package services

import (
	"fmt"
	"os"

	"github.com/okuralabs/okura-node/blocks"
	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/logger"
)

func checkBlock(newBlock blocks.Block, lastBlock blocks.Block, checkFinal bool) error {

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
	if height > 1 {
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
	logger.GetLogger().Println("Last stored index of blocks is ", height)
	if height == 0 {
		logger.GetLogger().Println("Starting blockchain from scratch...")
		return 0, fmt.Errorf("bad chain storage")
	}
	for h := int64(1); h <= height; h++ {
		bl, err := blocks.LoadBlock(h)
		if err != nil {
			logger.GetLogger().Println(err)
			err = checkBlock(bl, lastBlock, true)
			return h - 1, err
		}
		err = checkBlock(bl, lastBlock, h == height)
		if err != nil {
			logger.GetLogger().Println(err)
			err = checkBlock(bl, lastBlock, true)
			logger.GetLogger().Println("Error in block height ", h)
			return h - 1, err
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
		// remove database related to blockckchain, NOT wallets
		os.RemoveAll(homePath + common.DefaultBlockchainHomePath)
		logger.GetLogger().Fatal("DB files related to chain was removed. run mining once more and sync with other nodes. wrong data stored in db")
		return
	}
	common.SetHeight(height)

}
