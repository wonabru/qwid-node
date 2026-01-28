package transactionServices

import (
	"bytes"
	"time"

	"github.com/wonabru/qwid-node/common"
	"github.com/wonabru/qwid-node/logger"
	"github.com/wonabru/qwid-node/message"
	"github.com/wonabru/qwid-node/services"
	"github.com/wonabru/qwid-node/tcpip"
	"github.com/wonabru/qwid-node/transactionsDefinition"
	"github.com/wonabru/qwid-node/transactionsPool"
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

		// Broadcast pending transactions to all connected peers
		// if !common.IsSyncing.Load() {
		txs := transactionsPool.PoolsTx.PeekTransactions(int(common.MaxTransactionsPerBlock), 0)
		if len(txs) > 0 {
			topic := [2]byte{'T', 'T'}
			n, err := GenerateTransactionMsg(txs, []byte("tx"), topic)
			if err == nil {
				// Send to all connected peers
				peers := tcpip.GetPeersConnected(tcpip.TransactionTopic)
				for topicip := range peers {
					var ip [4]byte
					copy(ip[:], topicip[2:])
					if !bytes.Equal(ip[:], tcpip.MyIP[:]) {
						Send(ip, n.GetBytes())
					}
				}
			}
			// }
		}

		timeout := time.After(time.Second)

		select {
		case s := <-chanRecv:
			if len(s) == 4 && bytes.Equal(s, []byte("EXIT")) {
				logger.GetLogger().Println("broadcastTransactionsMsgInLoop: EXIT")
				break Q
			}
		case <-timeout:
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
	if num_peers == 0 {
		return
	}
	for topicip := range peers {
		// Send to all peers to ensure transactions reach mining nodes
		// Previously was randomly selecting ~1 peer which caused transactions to not propagate properly
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
	go tcpip.StartNewListener(tcpip.TransactionTopic)
	go tcpip.LoopSend(services.SendChanTx, tcpip.TransactionTopic)
}

func StartSubscribingTransactionMsg(ip [4]byte) {
	recvChan := make(chan []byte, 100) // Increased buffer size
	quit := false
	var ipr [4]byte
	go tcpip.StartNewConnection(ip, recvChan, tcpip.TransactionTopic)
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
