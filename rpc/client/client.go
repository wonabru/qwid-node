package clientrpc

import (
	"github.com/wonabru/qwid-node/logger"
	"github.com/wonabru/qwid-node/tcpip"
	"net/rpc"
	"strconv"
	"sync"
	"time"
)

const (
	retryInterval = 5 * time.Second
	bufferSize    = 1024 * 1024
)

var InRPC = make(chan []byte)
var OutRPC = make(chan []byte)
var muRPC = sync.Mutex{}

func ConnectRPC(ip string) {
	address := ip + ":" + strconv.Itoa(tcpip.Ports[tcpip.RPCTopic])
	var client *rpc.Client
	var err error
	for {
		client, err = rpc.Dial("tcp", address)
		if err == nil {
			break
		}
		logger.GetLogger().Printf("Failed to connect to RPC server at %s: %v. Retrying in %v...", address, err, retryInterval)
		time.Sleep(retryInterval)
	}

	for {
		select {
		case line := <-InRPC:
			muRPC.Lock()
			reply := make([]byte, bufferSize)
			err = client.Call("Listener.Send", line, &reply)
			if err != nil {
				logger.GetLogger().Printf("RPC call failed: %v. Reconnecting...", err)
				for {
					client, err = rpc.Dial("tcp", address)
					if err == nil {
						break
					}
					logger.GetLogger().Printf("Failed to reconnect to RPC server at %s: %v. Retrying in %v...", address, err, retryInterval)
					time.Sleep(retryInterval)
				}
			} else {
				OutRPC <- reply
			}
			muRPC.Unlock()
		default:
			time.Sleep(time.Millisecond * 100)
		}
	}
}
