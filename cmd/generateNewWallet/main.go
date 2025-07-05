package main

import (
	"fmt"
	"github.com/okuralabs/okura-node/logger"
	"github.com/okuralabs/okura-node/wallet"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"os/user"
	"strconv"
)

func main() {
	// Get the current user
	currentUser, err := user.Current()
	if err != nil {
		fmt.Println("Error getting current user:", err)
		return
	}

	fmt.Println("Current user:", currentUser.Username)
	var input string
	fmt.Print("Enter wallet number (0-255): ")
	_, err = fmt.Scanln(&input)
	if err != nil {
		fmt.Println("Error reading input:", err)
		return
	}
	walletNumber, err := strconv.Atoi(input)
	if (err != nil) || (0 > walletNumber) || (walletNumber > 255) {
		logger.GetLogger().Fatalf("wallet number should be integer from 0 to 255. Not ", walletNumber)
	}
	fmt.Print("Enter password: ")

	password, err := terminal.ReadPassword(0)
	if err != nil {
		logger.GetLogger().Fatal(err)
	}
	w, err := wallet.GenerateNewWallet(uint8(walletNumber), string(password))
	if err != nil {
		logger.GetLogger().Printf("Can not create wallet. Error %v", err)
	}
	folderPath := w.HomePath
	err = os.MkdirAll(w.HomePath, 0755)
	if err != nil {
		logger.GetLogger().Fatal(err)
	}
	fileInfo, err := os.Stat(folderPath)
	if err != nil {
		fmt.Println("Error getting folder info:", err)
		return
	}
	// Get the folder permissions
	permissions := fileInfo.Mode().Perm()
	fmt.Printf("Folder permissions: %v\n", permissions)
	// Check if the current user has read, write, and execute permissions
	hasReadPermission := permissions&0400 != 0
	hasWritePermission := permissions&0200 != 0
	hasExecutePermission := permissions&0100 != 0
	fmt.Printf("Read permission: %v\n", hasReadPermission)
	fmt.Printf("Write permission: %v\n", hasWritePermission)
	fmt.Printf("Execute permission: %v\n", hasExecutePermission)

	gw := wallet.EmptyGeneralWallet(w.WalletNumber, w.GetSigName(true), w.GetSigName(false))
	gw.WalletChain[0] = *w
	gw.CurrentWallet = *w
	err = gw.StoreJSON(0)
	if err != nil {
		logger.GetLogger().Println(err)
		return
	}
}
