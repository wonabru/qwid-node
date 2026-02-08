package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/wonabru/qwid-node/cmd/website/handlers"
	"github.com/wonabru/qwid-node/common"
	clientrpc "github.com/wonabru/qwid-node/rpc/client"
	"github.com/wonabru/qwid-node/statistics"
	"github.com/wonabru/qwid-node/tcpip"
	"github.com/wonabru/qwid-node/wallet"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	if len(os.Args) < 5 {
		fmt.Println("Usage: go run cmd/website/main.go <node_ip> <port> <wallet_num> <wallet_password>")
		os.Exit(1)
	}

	ip := os.Args[1]
	port := os.Args[2]
	walletNum, err := strconv.Atoi(os.Args[3])
	if err != nil || walletNum < 0 || walletNum > 255 {
		fmt.Println("Invalid wallet number (0-255)")
		os.Exit(1)
	}
	walletPassword := os.Args[4]

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	statistics.InitStatsManager()
	go clientrpc.ConnectRPC(ip)
	time.Sleep(time.Second)

	ipThis := tcpip.MyIP
	ipStr := net.IPv4(ipThis[0], ipThis[1], ipThis[2], ipThis[3])

	// Get encryption config
	sigName, sigName2 := "", ""
	encDone := make(chan struct{})
	go func() {
		var err error
		sigName, sigName2, err = handlers.SetCurrentEncryptions()
		if err != nil {
			fmt.Println("Warning: error retrieving current encryption:", err)
		}
		close(encDone)
	}()
	select {
	case <-encDone:
	case <-time.After(5 * time.Second):
		fmt.Println("Warning: timeout retrieving encryption config from node, using defaults")
	case <-stop:
		fmt.Println("\nShutting down...")
		os.Exit(0)
	}

	handlers.SigName = sigName
	handlers.SigName2 = sigName2

	// Load node wallet for signing RPC messages
	nodeWallet, err := wallet.LoadJSON(uint8(walletNum), walletPassword, sigName, sigName2)
	if err != nil {
		fmt.Println("Failed to load node wallet:", err)
		os.Exit(1)
	}
	handlers.NodeWallet = nodeWallet
	handlers.NodeIP = ipStr.String()
	handlers.DelegatedAccount = int(common.NumericalDelegatedAccountAddress(common.GetDelegatedAccount()))

	// Initialize user registry
	homePath, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Failed to get home directory:", err)
		os.Exit(1)
	}
	basePath := homePath + "/.qwid/website"
	handlers.WebsiteBasePath = basePath

	if err := handlers.InitUserRegistry(basePath); err != nil {
		fmt.Println("Failed to initialize user registry:", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	// Public routes (no auth required)
	mux.HandleFunc("/api/stats", handlers.CorsMiddleware(handlers.GetStats))
	mux.HandleFunc("/api/details", handlers.CorsMiddleware(handlers.GetDetails))
	mux.HandleFunc("/api/register", handlers.CorsMiddleware(handlers.Register))
	mux.HandleFunc("/api/login", handlers.CorsMiddleware(handlers.Login))

	// Authenticated routes
	mux.HandleFunc("/api/logout", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.Logout)))
	mux.HandleFunc("/api/session", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.GetSessionInfo)))
	mux.HandleFunc("/api/wallet/info", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.GetWalletInfo)))
	mux.HandleFunc("/api/wallet/mnemonic", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.GetMnemonic)))
	mux.HandleFunc("/api/wallet/change-password", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.ChangePassword)))
	mux.HandleFunc("/api/account", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.GetAccount)))
	mux.HandleFunc("/api/send", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.SendTransaction)))
	mux.HandleFunc("/api/history", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.GetHistory)))
	mux.HandleFunc("/api/pending", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.GetPending)))
	mux.HandleFunc("/api/staking/execute", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.ExecuteStaking)))
	mux.HandleFunc("/api/dex/tokens", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.GetTokens)))
	mux.HandleFunc("/api/dex/info", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.GetDexInfo)))
	mux.HandleFunc("/api/dex/trade", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.TradeDex)))
	mux.HandleFunc("/api/dex/execute", handlers.CorsMiddleware(handlers.AuthMiddleware(handlers.ExecuteDex)))

	// Serve static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	fmt.Printf("\n===========================================\n")
	fmt.Printf("  QWID Public Wallet Website\n")
	fmt.Printf("===========================================\n")
	fmt.Printf("  Node IP: %s\n", ipStr.String())
	fmt.Printf("  Node Account: %d\n", handlers.DelegatedAccount)
	fmt.Printf("  Website: http://0.0.0.0:%s\n", port)
	fmt.Printf("  Press Ctrl+C to stop\n")
	fmt.Printf("===========================================\n\n")

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Println("Failed to start server:", err)
			os.Exit(1)
		}
	}()

	<-stop
	fmt.Println("\nShutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Println("Server shutdown error:", err)
	}
	fmt.Println("Server stopped")
}
