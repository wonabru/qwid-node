package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/wonabru/qwid-node/cmd/webui/handlers"
	"github.com/wonabru/qwid-node/common"
	clientrpc "github.com/wonabru/qwid-node/rpc/client"
	"github.com/wonabru/qwid-node/statistics"
	"github.com/wonabru/qwid-node/tcpip"
	"github.com/wonabru/qwid-node/wallet"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	var ip string
	port := "8080"

	if len(os.Args) > 1 {
		ip = os.Args[1]
	} else {
		ip = "127.0.0.1"
	}
	if len(os.Args) > 2 {
		port = os.Args[2]
	}

	statistics.InitStatsManager()
	go clientrpc.ConnectRPC(ip)
	time.Sleep(time.Second)

	ipThis := tcpip.MyIP
	ipStr := net.IPv4(ipThis[0], ipThis[1], ipThis[2], ipThis[3])

	sigName, sigName2, err := handlers.SetCurrentEncryptions()
	if err != nil {
		fmt.Println("Warning: error retrieving current encryption:", err)
	}

	w := wallet.EmptyWallet(0, sigName, sigName2)
	handlers.MainWallet = &w
	handlers.NodeIP = ipStr.String()
	handlers.DelegatedAccount = int(common.NumericalDelegatedAccountAddress(common.GetDelegatedAccount()))

	// Setup routes
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/stats", corsMiddleware(handlers.GetStats))
	mux.HandleFunc("/api/wallet/load", corsMiddleware(handlers.LoadWallet))
	mux.HandleFunc("/api/wallet/info", corsMiddleware(handlers.GetWalletInfo))
	mux.HandleFunc("/api/wallet/change-password", corsMiddleware(handlers.ChangePassword))
	mux.HandleFunc("/api/wallet/mnemonic", corsMiddleware(handlers.GetMnemonic))
	mux.HandleFunc("/api/account", corsMiddleware(handlers.GetAccount))
	mux.HandleFunc("/api/send", corsMiddleware(handlers.SendTransaction))
	mux.HandleFunc("/api/staking/stake", corsMiddleware(handlers.Stake))
	mux.HandleFunc("/api/staking/unstake", corsMiddleware(handlers.Unstake))
	mux.HandleFunc("/api/staking/claim", corsMiddleware(handlers.ClaimRewards))
	mux.HandleFunc("/api/history", corsMiddleware(handlers.GetHistory))
	mux.HandleFunc("/api/details", corsMiddleware(handlers.GetDetails))
	mux.HandleFunc("/api/dex/tokens", corsMiddleware(handlers.GetTokens))
	mux.HandleFunc("/api/dex/pools", corsMiddleware(handlers.GetPools))
	mux.HandleFunc("/api/dex/trade", corsMiddleware(handlers.Trade))

	// Serve static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	fmt.Printf("\n===========================================\n")
	fmt.Printf("  QWID Wallet Web UI\n")
	fmt.Printf("===========================================\n")
	fmt.Printf("  Node IP: %s\n", ipStr.String())
	fmt.Printf("  Node Account: %d\n", handlers.DelegatedAccount)
	fmt.Printf("  Web UI: http://localhost:%s\n", port)
	fmt.Printf("===========================================\n\n")

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Println("Failed to start server:", err)
		os.Exit(1)
	}
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
