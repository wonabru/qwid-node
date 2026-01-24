package tcpip

import (
	"bytes"
	"context"
	"github.com/wonabru/qwid-node/common"
	"github.com/wonabru/qwid-node/logger"
	"sync"
	"time"
)

var bannedIP map[[4]byte]int64
var bannedIPMutex sync.RWMutex
var whiteListIPs map[[4]byte]bool

func init() {
	bannedIP = map[[4]byte]int64{}
	whiteListIPs = map[[4]byte]bool{}
}

func AddWhiteListIPs(ip [4]byte) {
	whiteListIPs[ip] = true
}

func IsIPBanned(ip [4]byte) bool {
	bannedIPMutex.RLock()
	defer bannedIPMutex.RUnlock()
	for wip, _ := range whiteListIPs {
		if bytes.Equal(wip[:], ip[:]) {
			return false
		}
	}
	if hbanned, ok := bannedIP[ip]; ok {
		if hbanned > common.GetCurrentTimeStampInSecond() {
			return true
		}
	}
	return false
}

func BanIP(ip [4]byte) {
	// internal IP should not be banned || bytes.Equal(ip[:2], InternalIP[:2])
	for wip, _ := range whiteListIPs {
		if bytes.Equal(wip[:], ip[:]) {
			return
		}
	}
	bannedIPMutex.Lock()
	logger.GetLogger().Println("BANNING ", ip)
	bannedIP[ip] = common.GetCurrentTimeStampInSecond() + common.BannedTimeSeconds
	bannedIPMutex.Unlock()
	if PeersMutex.TryLock() {
		defer PeersMutex.Unlock()
		if _, ok := validPeersConnected[ip]; ok {
			delete(validPeersConnected, ip)
		}
		if _, ok := nodePeersConnected[ip]; ok {
			delete(nodePeersConnected, ip)
		}
		tcpConns := tcpConnections[NonceTopic]
		tcpConn, ok := tcpConns[ip]
		if ok {
			CloseAndRemoveConnection(tcpConn)
			return
		}
		tcpConns = tcpConnections[TransactionTopic]
		tcpConn, ok = tcpConns[ip]
		if ok {
			CloseAndRemoveConnection(tcpConn)
			return
		}
		tcpConns = tcpConnections[SyncTopic]
		tcpConn, ok = tcpConns[ip]
		if ok {
			CloseAndRemoveConnection(tcpConn)
			return
		}
	}
}

func ReduceAndCheckIfBanIP(ip [4]byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	PeersMutex.Lock()
	defer PeersMutex.Unlock()
	select {
	case <-ctx.Done():
		// Handle timeout
		logger.GetLogger().Println("ReduceAndCheckIfBanIP: timeout in sending")

	default:
		if _, ok := validPeersConnected[ip]; ok {
			ReduceTrustRegisterPeer(ip)
		}
		if _, ok := validPeersConnected[ip]; !ok {
			logger.GetLogger().Println("not trusted ip", ip)
			BanIP(ip)
		}
	}
}
