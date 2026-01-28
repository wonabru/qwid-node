package nonceServices

import (
	"bytes"
	"github.com/wonabru/qwid-node/blocks"
	"github.com/wonabru/qwid-node/common"
	"github.com/wonabru/qwid-node/logger"
	"github.com/wonabru/qwid-node/message"
	"github.com/wonabru/qwid-node/pubkeys"
	"github.com/wonabru/qwid-node/services"
	"github.com/wonabru/qwid-node/tcpip"
	"github.com/wonabru/qwid-node/transactionsDefinition"
	"github.com/wonabru/qwid-node/voting"
	"github.com/wonabru/qwid-node/wallet"
	"golang.org/x/exp/rand"
	"sync"
	"time"
)

var LastRepliedIP [4]byte
var EncryptionOptData []byte
var encryptionMutex sync.Mutex

func init() {
	ResetToDefaultEncryptionOptData()
}

func ResetToDefaultEncryptionOptData() {
	encryptionMutex.Lock()
	defer encryptionMutex.Unlock()
	// Encryption1 and Encryption2 when changed than needs to add bytes
	encryption1 := common.BytesToLenAndBytes([]byte{})
	encryption2 := common.BytesToLenAndBytes([]byte{})
	EncryptionOptData = append(encryption1, encryption2...)
}

func SetEncryptionData(ne1 []byte, ne2 []byte) {
	encryptionMutex.Lock()
	defer encryptionMutex.Unlock()
	// Encryption1 and Encryption2 when changed than needs to add bytes
	encryption1 := common.BytesToLenAndBytes(ne1)
	encryption2 := common.BytesToLenAndBytes(ne2)
	EncryptionOptData = append(encryption1, encryption2...)
}

func InitChannelVoting(voteChan chan []byte) {
	quit := false
	for !quit {
		select {
		case s := <-voteChan:
			primary := true
			if s[0] != 0 {
				primary = false
			}
			err := blocks.SetEncryptionFromBytes(s[1:], primary)
			if err != nil {
				logger.GetLogger().Println(err)
				voteChan <- []byte(err.Error())
			} else {

				if err != nil {
					voteChan <- []byte(err.Error())
				} else {
					voteChan <- []byte("Set new encryption was successful")
				}
			}

		case <-tcpip.Quit:
			quit = true
		default:
			// Optional: Add a small sleep to prevent busy-waiting
			time.Sleep(time.Millisecond)
		}
	}
}

func InitNonceService() {
	services.SendMutexNonce.Lock()
	services.SendChanNonce = make(chan []byte, 10)

	services.SendChanSelfNonce = make(chan []byte, 10)
	services.SendMutexNonce.Unlock()
	startPublishingNonceMsg()
	time.Sleep(time.Second)
	go sendNonceMsgInLoop()
	go InitChannelVoting(blocks.VoteChannel)
}

func generateNonceMsg(topic [2]byte) (message.TransactionsMessage, error) {
	h := common.GetHeight()
	w := wallet.GetActiveWallet()
	primary := common.GetNodeSignPrimary(h)
	sender := wallet.GetActiveWallet().MainAddress

	var nonceTransaction transactionsDefinition.Transaction
	tp := transactionsDefinition.TxParam{
		ChainID:     common.GetChainID(),
		Sender:      sender,
		SendingTime: common.GetCurrentTimeStampInSecond(),
		Nonce:       0,
	}
	lastBlockHash, err := blocks.LoadHashOfBlock(h)
	if err != nil {
		lastBlockHash = common.EmptyHash().GetBytes()
	}
	optData := common.GetByteInt64(h)
	optData = append(optData, lastBlockHash...)

	//TODO Price oracle currently is random: 0.9 - 1.1 KURA/USD
	priceOracle := int64(rand.Intn(10000000) - 5000000 + 100000000)
	randOracle := rand.Int63()
	optData = append(optData, common.GetByteInt64(priceOracle)...)
	optData = append(optData, common.GetByteInt64(randOracle)...)

	voting.VotesEncryptionMutex.Lock()
	if voting.AfterReset {
		ResetToDefaultEncryptionOptData()
		voting.AfterReset = false
	}
	voting.VotesEncryptionMutex.Unlock()

	optData = append(optData, EncryptionOptData...)

	pubkey := common.PubKey{}
	if primary == false {
		pktrie, err := pubkeys.LoadTreeWithoutAddresses(sender)
		if err != nil {
			return message.TransactionsMessage{}, err
		}
		isAddr := pktrie.IsAddressInTree(w.Account2.Address)
		if !isAddr {
			logger.GetLogger().Println("no address2 in blockchain")
			pubkey = w.Account2.PublicKey
		}
	}

	dataTx := transactionsDefinition.TxData{
		Recipient: common.GetDelegatedAccount(), // will be delegated account temporary
		Amount:    0,
		OptData:   optData[:],
		Pubkey:    pubkey,
	}
	nonceTransaction = transactionsDefinition.Transaction{
		TxData:    dataTx,
		TxParam:   tp,
		Hash:      common.Hash{},
		Signature: common.Signature{},
		Height:    h + 1,
		GasPrice:  0,
		GasUsage:  0,
	}

	err = (&nonceTransaction).CalcHashAndSet()
	if err != nil {
		return message.TransactionsMessage{}, err
	}

	err = (&nonceTransaction).Sign(w, primary)
	if err != nil {
		return message.TransactionsMessage{}, err
	}

	bm := message.BaseMessage{
		Head:    []byte("nn"),
		ChainID: common.GetChainID(),
	}
	bb := nonceTransaction.GetBytes()
	n := message.TransactionsMessage{
		BaseMessage:       bm,
		TransactionsBytes: map[[2]byte][][]byte{topic: {bb}},
	}

	return n, nil
}

func sendNonceMsgInLoopSelf(chanRecv chan []byte) {
	var topic = [2]byte{'S', 'S'}
Q:
	for range time.Tick(time.Second) {
		sendNonceMsg(tcpip.MyIP, topic)
		timeout := time.After(time.Second)

		select {
		case s := <-chanRecv:
			if len(s) == 4 && bytes.Equal(s, []byte("EXIT")) {
				break Q
			}
		case <-timeout:
			// Handle timeout
			//logger.GetLogger().Println("sendNonceMsgInLoopSelf: Timeout occurred")
			// You can break the loop or return from the function here
			break
		}
	}
}

func sendNonceMsg(ip [4]byte, topic [2]byte) {
	h := common.GetHeight()
	if h < common.CurrentHeightOfNetwork {
		return
	}
	//isync := common.IsSyncing.Load()
	//if isync == true {
	//	return
	//}
	n, err := generateNonceMsg(topic)
	if err != nil {
		logger.GetLogger().Println(err)
		return
	}
	if !Send(ip, n.GetBytes()) {
		logger.GetLogger().Println("could not send nonce message")
	}
}

func Send(addr [4]byte, nb []byte) bool {
	nb = append(addr[:], nb...)
	if services.SendMutexNonce.TryLock() {
		defer services.SendMutexNonce.Unlock()
		services.SendChanNonce <- nb
		return true
	}
	return false
}

func sendNonceMsgInLoop() {
	for range time.Tick(time.Second * 5) {
		var topic = [2]byte{'N', 'N'}
		sendNonceMsg([4]byte{0, 0, 0, 0}, topic)
	}
}

func startPublishingNonceMsg() {
	go tcpip.StartNewListener(tcpip.NonceTopic)
	go tcpip.LoopSend(services.SendChanNonce, tcpip.NonceTopic)
	go tcpip.StartNewListener(tcpip.SelfNonceTopic)
	go tcpip.LoopSend(services.SendChanSelfNonce, tcpip.SelfNonceTopic)
}

func StartSubscribingNonceMsg(ip [4]byte) {
	recvChan := make(chan []byte, 10) // Use a buffered channel
	quit := false
	var ipr [4]byte
	go tcpip.StartNewConnection(ip, recvChan, tcpip.NonceTopic)
	for !services.QUIT.Load() && !quit {
		select {
		case s := <-recvChan:
			if len(s) == 4 && bytes.Equal(s, []byte("EXIT")) {
				quit = true
				break
			}
			if len(s) > 4 {
				copy(ipr[:], s[:4])
				OnMessage(ipr, s[4:])
				//send reply to valid nonce message from other nodes
				if ipr != tcpip.MyIP {
					if LastRepliedIP != ipr {
						sendReply(ipr)
					} else {
						LastRepliedIP = [4]byte{0, 0, 0, 0}
					}
				}
			}
		case <-tcpip.Quit:
			services.QUIT.Store(true)
		default:
			// Optional: Add a small sleep to prevent busy-waiting
			time.Sleep(time.Millisecond)
		}
	}
}

func sendReply(addr [4]byte) {
	LastRepliedIP = addr
	var topic = [2]byte{'N', 'N'}
	n, err := generateNonceMsg(topic)
	if err != nil {
		logger.GetLogger().Println(err)
		return
	}
	if Send(addr, n.GetBytes()) {
		logger.GetLogger().Println("send reply to node ", addr, " my ip ", tcpip.MyIP)
	}
}

func StartSubscribingNonceMsgSelf() {
	recvChanSelf := make(chan []byte, 10) // Use a buffered channel
	recvChanExit := make(chan []byte, 10) // Use a buffered channel
	quit := false
	var ip [4]byte
	go tcpip.StartNewConnection(tcpip.MyIP, recvChanSelf, tcpip.SelfNonceTopic)
	go sendNonceMsgInLoopSelf(recvChanExit)
	for !services.QUIT.Load() && !quit {
		select {
		case s := <-recvChanSelf:
			if len(s) == 4 && bytes.Equal(s, []byte("EXIT")) {
				recvChanExit <- s
				quit = true
				break
			}
			if len(s) > 4 {
				copy(ip[:], s[:4])
				OnMessage(ip, s[4:])
			}
		case <-tcpip.Quit:
			services.QUIT.Store(true)
		default:

			// Optional: Add a small sleep to prevent busy-waiting
			time.Sleep(time.Millisecond)
		}
	}
}
