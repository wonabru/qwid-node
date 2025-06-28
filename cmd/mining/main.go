package main

import (
	"fmt"
	"github.com/okuralabs/okura-node/blocks"
	"github.com/okuralabs/okura-node/services"
	"github.com/okuralabs/okura-node/statistics"
	"github.com/okuralabs/okura-node/transactionsPool"
	"strconv"
	"strings"

	"github.com/okuralabs/okura-node/logger"
	"github.com/okuralabs/okura-node/wallet"
	"golang.org/x/crypto/ssh/terminal"

	_ "net/http/pprof"
	"os"
	"time"

	"github.com/okuralabs/okura-node/account"
	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/genesis"
	serverrpc "github.com/okuralabs/okura-node/rpc/server"
	nonceService "github.com/okuralabs/okura-node/services/nonceService"
	syncServices "github.com/okuralabs/okura-node/services/syncService"
	"github.com/okuralabs/okura-node/services/transactionServices"
	"github.com/okuralabs/okura-node/tcpip"
)

func main() {
	var err error
	logger.InitLogger()
	defer logger.CloseLogger()
	// Now you can use log functions as usual
	logger.GetLogger().Println("Application started")

	fmt.Print("Enter password: ")
	password, err := terminal.ReadPassword(0)
	if err != nil {
		logger.GetLogger().Fatal(err)
	}

	// Initialize wallet
	logger.GetLogger().Println("Initializing wallet...")
	wallet.InitActiveWallet(0, string(password))

	// Initialize genesis block
	logger.GetLogger().Println("Initializing genesis block for setting init params...")
	genesis.InitGenesis(false)

	// Load accounts
	logger.GetLogger().Println("Loading accounts...")
	err = account.LoadAccounts(-1)
	if err != nil {
		addrbytes := [common.AddressLength]byte{}
		copy(addrbytes[:], wallet.GetActiveWallet().Address.GetBytes())
		// Initialize accounts
		a := account.Account{
			Balance: 0,
			Address: addrbytes,
		}
		allAccounts := map[[20]byte]account.Account{}
		allAccounts[addrbytes] = a
		account.Accounts = account.AccountsType{AllAccounts: allAccounts}
		err = account.StoreAccounts(0)
		if err != nil {
			logger.GetLogger().Fatal("Failed to store accounts:", err)
		}

		// Initialize DEX accounts
		logger.GetLogger().Println("Initializing DEX accounts...")
		allDexAccounts := map[[20]byte]account.DexAccount{}
		account.DexAccounts = account.DexAccountsType{AllDexAccounts: allDexAccounts}
		err = account.StoreDexAccounts(0)
		if err != nil {
			logger.GetLogger().Fatal("Failed to store DEX accounts:", err)
		}

		// Initialize staking accounts
		logger.GetLogger().Println("Setting up staking accounts...")
		for i := 1; i < 256; i++ {
			del := common.GetDelegatedAccountAddress(int16(i))
			delbytes := [common.AddressLength]byte{}
			copy(delbytes[:], del.GetBytes())
			sa := account.StakingAccount{
				StakedBalance:    0,
				StakingRewards:   0,
				DelegatedAccount: delbytes,
				StakingDetails:   nil,
			}
			allStakingAccounts := map[[20]byte]account.StakingAccount{}
			allStakingAccounts[addrbytes] = sa
			account.StakingAccounts[i] = account.StakingAccountsType{AllStakingAccounts: allStakingAccounts}
		}
		err = account.StoreStakingAccounts(0)
		if err != nil {
			logger.GetLogger().Fatal("Failed to store staking accounts:", err)
		}
	}

	// Load accounts
	logger.GetLogger().Println("Loading accounts...")
	err = account.LoadAccounts(-1)
	if err != nil {
		logger.GetLogger().Fatal("Failed to load accounts:", err)
	}
	defer func() {
		common.IsSyncing.Store(true)
		logger.GetLogger().Println("Storing accounts...")
		account.StoreAccounts(-1)
	}()

	// Load DEX accounts
	logger.GetLogger().Println("Loading DEX accounts...")
	err = account.LoadDexAccounts(-1)
	if err != nil {
		logger.GetLogger().Fatal("Failed to load DEX accounts:", err)
	}
	defer func() {
		common.IsSyncing.Store(true)
		logger.GetLogger().Println("Storing DEX accounts...")
		account.StoreDexAccounts(-1)
	}()

	// Load staking accounts
	logger.GetLogger().Println("Loading staking accounts...")
	err = account.LoadStakingAccounts(-1)
	if err != nil {
		logger.GetLogger().Fatal("Failed to load staking accounts:", err)
	}
	defer func() {
		common.IsSyncing.Store(true)
		logger.GetLogger().Println("Storing staking accounts...")
		account.StoreStakingAccounts(-1)
	}()

	// Initialize state database
	logger.GetLogger().Println("Initializing state database...")
	blocks.InitStateDB()

	// Initialize transaction pool and merkle tree
	logger.GetLogger().Println("Initializing transaction pool and merkle tree...")
	transactionsPool.InitPermanentTrie()
	defer transactionsPool.GlobalMerkleTree.Destroy()

	// Initialize statistics
	statistics.InitStatsManager()

	//Load Main Blockchain
	services.SetBlockHeightAfterCheck()

	if common.GetHeight() < 0 {
		// Initialize genesis block
		logger.GetLogger().Println("Initializing genesis block with processing transactions...")
		genesis.InitGenesis(true)
	}

	// Initialize services
	logger.GetLogger().Println("Initializing transaction service...")
	transactionServices.InitTransactionService()

	logger.GetLogger().Println("Initializing sync service...")
	syncServices.InitSyncService()

	logger.GetLogger().Println("Starting RPC server...")
	go serverrpc.ListenRPC()

	logger.GetLogger().Println("Initializing nonce service...")
	nonceService.InitNonceService()
	go nonceService.StartSubscribingNonceMsgSelf()
	go nonceService.StartSubscribingNonceMsg(tcpip.MyIP)

	logger.GetLogger().Println("Starting transaction and sync message subscriptions...")
	go transactionServices.StartSubscribingTransactionMsg(tcpip.MyIP)
	go syncServices.StartSubscribingSyncMsg(tcpip.MyIP)

	time.Sleep(time.Second)

	if len(os.Args) > 1 {
		logger.GetLogger().Println("Processing command line arguments...")
		ips := strings.Split(os.Args[1], ".")
		if len(ips) != 4 {
			logger.GetLogger().Println("Invalid IP address format")
			return
		}
		var ip [4]byte
		for i := 0; i < 4; i++ {
			num, err := strconv.Atoi(ips[i])
			if err != nil {
				logger.GetLogger().Println("Invalid IP address segment:", ips[i])
				return
			}
			ip[i] = byte(num)
		}

		logger.GetLogger().Println("Connecting to peer:", ip)
		go nonceService.StartSubscribingNonceMsg(ip)
		go syncServices.StartSubscribingSyncMsg(ip)
		go transactionServices.StartSubscribingTransactionMsg(ip)
	}

	time.Sleep(time.Second)

	logger.GetLogger().Println("Starting peer discovery...")
	chanPeer := make(chan []byte)
	go tcpip.LookUpForNewPeersToConnect(chanPeer)
	topic := [2]byte{}
	ip := [4]byte{}

	logger.GetLogger().Println("Entering main loop...")
QF:
	for {
		select {

		case topicip := <-chanPeer:
			copy(topic[:], topicip[:2])
			copy(ip[:], topicip[2:])
			logger.GetLogger().Printf("Received peer message - Topic: %s, IP: %v", string(topic[:]), ip)

			if topic[0] == 'T' {
				logger.GetLogger().Println("Starting transaction subscription for peer:", ip)
				go transactionServices.StartSubscribingTransactionMsg(ip)
			}
			if topic[0] == 'N' {
				logger.GetLogger().Println("Starting nonce subscription for peer:", ip)
				go nonceService.StartSubscribingNonceMsg(ip)
			}
			if topic[0] == 'S' {
				logger.GetLogger().Println("Starting self nonce subscription")
				go nonceService.StartSubscribingNonceMsgSelf()
			}
			if topic[0] == 'B' {
				logger.GetLogger().Println("Starting sync subscription for peer:", ip)
				go syncServices.StartSubscribingSyncMsg(ip)
			}

		case <-tcpip.Quit:
			logger.GetLogger().Println("Received quit signal, shutting down...")
			break QF
		}
	}

}
