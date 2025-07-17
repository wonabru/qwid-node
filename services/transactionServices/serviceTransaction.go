package transactionServices

import (
	"bytes"
	"github.com/okuralabs/okura-node/logger"
	"golang.org/x/exp/rand"
	"time"

	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/message"
	"github.com/okuralabs/okura-node/services"
	"github.com/okuralabs/okura-node/tcpip"
	"github.com/okuralabs/okura-node/transactionsDefinition"
	"github.com/okuralabs/okura-node/transactionsPool"
)

func InitTransactionService() {
	services.SendMutexTx.Lock()
	services.SendChanTx = make(chan []byte, 100)

	services.SendMutexTx.Unlock()
	startPublishingTransactionMsg()
	go broadcastTransactionsMsgInLoop(services.SendChanTx)
}

func GenerateTransactionMsg(txs []transactionsDefinition.Transaction, mesgHead []byte, topic [2]byte) (message.TransactionsMessage, error) {

	bm := message.BaseMessage{
		Head:    mesgHead,
		ChainID: common.GetChainID(),
	}
	bb := [][]byte{}
	for _, tx := range txs {
		b := tx.GetBytes()
		bb = append(bb, b)
	}

	n := message.TransactionsMessage{
		BaseMessage:       bm,
		TransactionsBytes: map[[2]byte][][]byte{topic: bb},
	}
	return n, nil
}

func GenerateTransactionMsgGT(txsHashes [][]byte, mesgHead []byte, topic [2]byte) (message.TransactionsMessage, error) {

	bm := message.BaseMessage{
		Head:    mesgHead,
		ChainID: common.GetChainID(),
	}

	n := message.TransactionsMessage{
		BaseMessage:       bm,
		TransactionsBytes: map[[2]byte][][]byte{topic: txsHashes},
	}
	return n, nil
}

func broadcastTransactionsMsgInLoop(chanRecv chan []byte) {

Q:
	for range time.Tick(time.Second) {

		topic := [2]byte{'T', 'T'}

		if SendTransactionMsg(tcpip.MyIP, topic) {
			break
		}

		timeout := time.After(time.Second)

		select {
		case s := <-chanRecv:
			if len(s) == 4 && bytes.Equal(s, []byte("EXIT")) {
				break Q
			}
		case <-timeout:
			// Handle timeout
			//logger.GetLogger().Println("broadcastTransactionsMsgInLoop: Timeout occurred")
			// You can break the loop or return from the function here
			break
		}

	}
}

func SendTransactionMsg(ip [4]byte, topic [2]byte) bool {
	isync := common.IsSyncing.Load()
	if isync == true {
		return true
	}
	txs := transactionsPool.PoolsTx.PeekTransactions(int(common.MaxTransactionsPerBlock), 0)
	n, err := GenerateTransactionMsg(txs, []byte("tx"), topic)
	if err != nil {
		logger.GetLogger().Println(err)
		return false
	}
	if !Send(ip, n.GetBytes()) {
		logger.GetLogger().Println("could not send standard transaction")
		return false
	}
	return true
}

func SendGT(ip [4]byte, txsHashes [][]byte, syncPre string) {
	topic := tcpip.TransactionTopic
	transactionMsg, err := GenerateTransactionMsgGT(txsHashes, []byte(syncPre), topic)
	if err != nil {
		logger.GetLogger().Println("cannot generate transaction msg", err)
	}
	if !Send(ip, transactionMsg.GetBytes()) {
		logger.GetLogger().Println("could not send send transaction in GT message")
	}
}

func Send(addr [4]byte, nb []byte) bool {

	nb = append(addr[:], nb...)
	if services.SendMutexTx.TryLock() {
		defer services.SendMutexTx.Unlock()
		services.SendChanTx <- nb
		return true
	}
	return false
}

func BroadcastTxn(ignoreAddr [4]byte, nb []byte) {
	var ip [4]byte
	var peers = tcpip.GetPeersConnected(tcpip.TransactionTopic)
	num_peers := len(peers)
	for topicip, _ := range peers {
		// trying to send randomly to 1 other nodes
		if rand.Intn(num_peers) >= 1 {
			continue
		}
		copy(ip[:], topicip[2:])
		if !bytes.Equal(ip[:], ignoreAddr[:]) && !bytes.Equal(ip[:], tcpip.MyIP[:]) {
			//logger.GetLogger().Println("send transactions to ", int(ip[0]), int(ip[1]), int(ip[2]), int(ip[3]))
			if !Send(ip, nb) {
				logger.GetLogger().Println("could not broadcast transaction")
			}
		}
	}
}

func startPublishingTransactionMsg() {
	go tcpip.StartNewListener(services.SendChanTx, tcpip.TransactionTopic)
}

func StartSubscribingTransactionMsg(ip [4]byte) {
	recvChan := make(chan []byte, 100) // Increased buffer size
	quit := false
	var ipr [4]byte
	logger.GetLogger().Printf("Starting transaction subscription to peer: %v", ip)

	go tcpip.StartNewConnection(ip, recvChan, tcpip.TransactionTopic)

	logger.GetLogger().Println("Entering transaction message receiving loop for peer:", ip)
	for !services.QUIT.Load() && !quit {
		select {
		case s := <-recvChan:
			if len(s) == 4 && bytes.Equal(s, []byte("EXIT")) {
				logger.GetLogger().Printf("Received EXIT signal for peer %v", ip)
				quit = true
				break
			}
			if len(s) > 4 {
				copy(ipr[:], s[:4])
				OnMessage(ipr, s[4:])
			}
		case <-tcpip.Quit:
			logger.GetLogger().Printf("Received quit signal for peer %v", ip)
			services.QUIT.Store(true)
		default:
			time.Sleep(time.Millisecond * 100) // Reduced sleep time
		}
	}
	logger.GetLogger().Println("Exiting transaction message receiving loop for peer:", ip)
}
