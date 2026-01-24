package tcpip

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	"github.com/wonabru/qwid-node/common"
	"github.com/wonabru/qwid-node/logger"
)

var ChanPeer = make(chan []byte)

func StartNewListener(topic [2]byte) {

	conn, err := Listen([4]byte{0, 0, 0, 0}, Ports[topic])
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	defer func() {
		PeersMutex.Lock()
		defer PeersMutex.Unlock()
		for _, tcpConn := range tcpConnections[topic] {
			tcpConn.Close()
		}
	}()
	for {
		select {
		case <-Quit:
			logger.GetLogger().Println("Should exit StartNewListener")
		default:
			_, err := Accept(topic, conn)
			if err != nil {
				logger.GetLogger().Println(err)
				continue
			}
		}
	}
}

func LoopSend(sendChan <-chan []byte, topic [2]byte) {
	var ipr [4]byte
	for {
		select {
		case s := <-sendChan:
			if len(s) > 4 {
				copy(ipr[:], s[:4])
			} else {
				logger.GetLogger().Println("wrong message", topic)
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)

			PeersMutex.Lock()
			select {
			case <-ctx.Done():
				// Handle timeout
				PeersMutex.Unlock()
				logger.GetLogger().Println("timeout in sending")
				cancel()
				continue
			default:

				if bytes.Equal(ipr[:], []byte{0, 0, 0, 0}) {

					tmpConn := tcpConnections[topic]
					for k, tcpConn0 := range tmpConn {
						//if _, ok := validPeersConnected[k]; ok {
						//	ReduceTrustRegisterPeer(k)
						//}
						if _, ok := validPeersConnected[k]; !ok {
							logger.GetLogger().Println("when send to all, ignore connection", k)
							//CloseAndRemoveConnection(tcpConn0)
						} else if !bytes.Equal(k[:], MyIP[:]) {
							//logger.GetLogger().Println("send to ipr", k)
							err := Send(tcpConn0, s[4:])
							if err != nil {
								logger.GetLogger().Println("error in sending to all ", err)
								deletedIP := CloseAndRemoveConnection(tcpConn0)
								if len(deletedIP) >= 1 {
									ChanPeer <- deletedIP[0]
								}
							}
						}
					}
				} else {
					tcpConns := tcpConnections[topic]
					tcpConn, ok := tcpConns[ipr]

					//if _, ok2 := validPeersConnected[ipr]; ok2 {
					//	ReduceTrustRegisterPeer(ipr)
					//}
					if _, ok2 := validPeersConnected[ipr]; !ok2 {
						logger.GetLogger().Println("ignore when send to ", ipr)
						//CloseAndRemoveConnection(tcpConn)
					} else if ok {
						//logger.GetLogger().Println("send to ip", ipr)
						err := Send(tcpConn, s[4:])
						if err != nil {
							logger.GetLogger().Println("error in sending to ", ipr, err)
							deletedIP := CloseAndRemoveConnection(tcpConn)
							if len(deletedIP) >= 1 {
								ChanPeer <- deletedIP[0]
							}
						}
					} else {
						//fmt.Println("no connection to given ip", ipr, topic)
						//BanIP(ipr, topic)
					}

				}
				PeersMutex.Unlock()
				cancel()
			}
		//case b := <-waitChan:
		//	if bytes.Equal(b, topic[:]) {
		//		time.Sleep(time.Millisecond * 10)
		//	}
		case <-Quit:
			logger.GetLogger().Println("Should exit LoopSend")
		default:
		}
	}
}

func StartNewConnection(ip [4]byte, receiveChan chan []byte, topic [2]byte) {
	ipport := fmt.Sprintf("%d.%d.%d.%d:%d", ip[0], ip[1], ip[2], ip[3], Ports[topic])
	if bytes.Equal(ip[:], []byte{127, 0, 0, 1}) {
		ipport = fmt.Sprintf(":%d", Ports[topic])
	}

	logger.GetLogger().Printf("Attempting to connect to %s for topic %v", ipport, topic)

	tcpAddr, err := net.ResolveTCPAddr("tcp", ipport)
	if err != nil {
		logger.GetLogger().Printf("Failed to resolve TCP address for %s: %v", ipport, err)
		return
	}

	var tcpConn *net.TCPConn
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		tcpConn, err = net.DialTCP("tcp", nil, tcpAddr)
		if err == nil {
			break
		}
		logger.GetLogger().Printf("Connection attempt %d to %s failed: %v", i+1, ipport, err)

		time.Sleep(time.Second * 2)
		PeersMutex.Lock()
		ReduceTrustRegisterPeer(ip)
		trust, ok := validPeersConnected[ip]
		PeersMutex.Unlock()
		if ok && trust <= 0 {
			BanIP(ip)
		} else if i == maxRetries-1 {
			BanIP(ip)
		}

	}

	if err != nil {
		logger.GetLogger().Printf("Failed to establish connection to %s after %d attempts: %v", ipport, maxRetries, err)
		return
	}

	if topic == TransactionTopic {
		logger.GetLogger().Printf("Successfully connected for TRANSACTIONS TOPIC with %v", ip)
	}

	reconnectionTries := 0
	resetNumber := 0

	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger().Printf("Recovered from panic in connection to %v: %v", ip, r)
			receiveChan <- []byte("EXIT")
			PeersMutex.Lock()
			deletedIP := CloseAndRemoveConnection(tcpConn)
			PeersMutex.Unlock()
			if len(deletedIP) >= 1 {
				ChanPeer <- deletedIP[0]
			}
		}
	}()

	logger.GetLogger().Printf("Starting message processing loop for connection to %v", ip)

	rTopic := map[[2]byte][]byte{}

	for {
		resetNumber++
		if resetNumber%100 == 0 {
			reconnectionTries = 0
		}

		select {
		case <-Quit:
			logger.GetLogger().Printf("Received quit signal for connection to %v", ip)
			PeersMutex.Lock()
			deletedIP := CloseAndRemoveConnection(tcpConn)
			PeersMutex.Unlock()
			if len(deletedIP) >= 1 {
				ChanPeer <- deletedIP[0]
			}
			return
		default:
			r := Receive(topic, tcpConn)
			if r == nil {
				continue
			}
			if bytes.Equal(r, []byte("<-ERR->")) {
				if reconnectionTries > common.ConnectionMaxTries {
					logger.GetLogger().Println("error in read. Closing connection", ip, string(r))
					tcpConn.Close()
					tcpConn, err = net.DialTCP("tcp", nil, tcpAddr)
					if err != nil {
						logger.GetLogger().Printf("Connection attempt %d to %s failed: %v", ipport, err.Error())
					}
					reconnectionTries = 0
					continue
				}
				reconnectionTries++
				time.Sleep(time.Millisecond * 10)
				continue
			}
			if bytes.Equal(r, []byte("<-CLS->")) {

				logger.GetLogger().Println("Closing connection", ip, r)
				PeersMutex.Lock()
				receiveChan <- []byte("EXIT")
				deletedIP := CloseAndRemoveConnection(tcpConn)
				PeersMutex.Unlock()
				if len(deletedIP) >= 1 {
					ChanPeer <- deletedIP[0]
				}
				return

			}
			//if bytes.Equal(r, []byte("WAIT")) {
			//	waitChan <- topic[:]
			//	continue
			//}

			rt, ok := rTopic[topic]
			if ok {
				r = append(rt, r...)
			}
			if !bytes.Equal(r[len(r)-7:], []byte("<-END->")) {
				rTopic[topic] = r
			} else {
				rTopic[topic] = []byte{}
			}

			if int32(len(r)) > common.MaxMessageSizeBytes {
				logger.GetLogger().Println("error: too long message received: ", len(r))
				PeersMutex.Lock()
				ReduceTrustRegisterPeer(ip)
				PeersMutex.Unlock()
				rTopic[topic] = []byte{}
				if trust, ok := validPeersConnected[ip]; ok && trust <= 0 {
					BanIP(ip)
					receiveChan <- []byte("EXIT")
					return
				}
				continue
			}
			if bytes.Equal(r[len(r)-7:], []byte("<-END->")) {
				if len(r) > 4 {
					if bytes.Equal(r[:4], common.MessageInitialization[:]) {
						receiveChan <- append(ip[:], r[4:]...)
					} else {
						logger.GetLogger().Println("wrong MessageInitialization", r[:4], "should be", common.MessageInitialization[:])
						PeersMutex.Lock()
						ReduceTrustRegisterPeer(ip)
						PeersMutex.Unlock()
						if trust, ok := validPeersConnected[ip]; ok && trust <= 0 {
							BanIP(ip)
							receiveChan <- []byte("EXIT")
							return
						}
					}
				}
			}
		}
	}
}

func CloseAndRemoveConnection(tcpConn *net.TCPConn) [][]byte {
	if tcpConn == nil {
		return [][]byte{}
	}

	topicipBytes := [6]byte{}
	// Method 2: Compare TCP addresses directly (more robust)
	remoteAddr := tcpConn.RemoteAddr().(*net.TCPAddr)
	localAddr := tcpConn.LocalAddr().(*net.TCPAddr)
	deletedIP := [][]byte{}
	// Find and remove the connection
	for topic, connections := range tcpConnections {
		for peerIP, conn := range connections {
			rTCPAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
			lTCPAddr, ok2 := conn.LocalAddr().(*net.TCPAddr)
			// Using direct TCP address comparison for more robust matching
			if ok && ok2 {
				if (rTCPAddr.IP.Equal(remoteAddr.IP) || lTCPAddr.IP.Equal(remoteAddr.IP)) && (rTCPAddr.Port == localAddr.Port || lTCPAddr.Port == localAddr.Port || rTCPAddr.Port == remoteAddr.Port || lTCPAddr.Port == remoteAddr.Port) {
					fmt.Printf("Closing connection for topic %v, peer %v (IP: %v, Port: %d)\n",
						topic, peerIP, remoteAddr.IP, remoteAddr.Port)

					deletedIP = append(deletedIP, append(topic[:], peerIP[:]...))
					fmt.Println("Closing connection (send)", topic, peerIP)
					tcpConnections[topic][peerIP].Close()
					copy(topicipBytes[:], append(topic[:], peerIP[:]...))
					delete(tcpConnections[topic], peerIP)
					delete(peersConnected, topicipBytes)
					delete(oldPeers, topicipBytes)
				}
			}
		}
	}
	return deletedIP
}
