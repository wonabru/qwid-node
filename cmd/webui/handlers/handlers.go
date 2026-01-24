package handlers

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/wonabru/qwid-node/account"
	"github.com/wonabru/qwid-node/blocks"
	"github.com/wonabru/qwid-node/common"
	"github.com/wonabru/qwid-node/logger"
	clientrpc "github.com/wonabru/qwid-node/rpc/client"
	"github.com/wonabru/qwid-node/statistics"
	"github.com/wonabru/qwid-node/transactionsDefinition"
	"github.com/wonabru/qwid-node/wallet"
)

var MainWallet *wallet.Wallet
var NodeIP string
var DelegatedAccount int

type StatsResponse struct {
	Height              int64   `json:"height"`
	HeightMax           int64   `json:"heightMax"`
	TimeInterval        int64   `json:"timeInterval"`
	Transactions        int     `json:"transactions"`
	TransactionsPending int     `json:"transactionsPending"`
	Tps                 float32 `json:"tps"`
	Syncing             bool    `json:"syncing"`
	Difficulty          int32   `json:"difficulty"`
	PriceOracle         float32 `json:"priceOracle"`
	NodeIP              string  `json:"nodeIP"`
	DelegatedAccount    int     `json:"delegatedAccount"`
}

type AccountResponse struct {
	Address          string  `json:"address"`
	Balance          float64 `json:"balance"`
	StakedAmount     float64 `json:"stakedAmount"`
	LockedAmount     float64 `json:"lockedAmount"`
	RewardsAmount    float64 `json:"rewardsAmount"`
	TotalHoldings    float64 `json:"totalHoldings"`
	StakingDetails   []StakingDetail `json:"stakingDetails"`
}

type StakingDetail struct {
	DelegatedAddress string  `json:"delegatedAddress"`
	Staked           float64 `json:"staked"`
	Rewards          float64 `json:"rewards"`
}

type WalletInfoResponse struct {
	Loaded     bool   `json:"loaded"`
	Address    string `json:"address"`
	PubKeyHex  string `json:"pubKeyHex"`
	SigName    string `json:"sigName"`
	SigName2   string `json:"sigName2"`
}

func SignMessage(line []byte) []byte {
	operation := string(line[0:4])
	verificationNeeded := true
	for _, noVerification := range common.ConnectionsWithoutVerification {
		if bytes.Equal([]byte(operation), noVerification) {
			verificationNeeded = false
			break
		}
	}
	if verificationNeeded {
		if MainWallet == nil || (!MainWallet.Check() || !MainWallet.Check2()) {
			logger.GetLogger().Println("wallet not loaded yet")
			return line
		}
		if common.IsPaused() == false {
			line = common.BytesToLenAndBytes(line)
			sign, err := MainWallet.Sign(line, true)
			if err != nil {
				logger.GetLogger().Println(err)
				return line
			}
			line = append(line, sign.GetBytes()...)
		} else {
			line = common.BytesToLenAndBytes(line)
			sign, err := MainWallet.Sign(line, false)
			if err != nil {
				logger.GetLogger().Println(err)
				return line
			}
			line = append(line, sign.GetBytes()...)
		}
	} else {
		line = common.BytesToLenAndBytes(line)
	}
	return line
}

func SetCurrentEncryptions() (string, string, error) {
	clientrpc.InRPC <- SignMessage([]byte("ENCR"))
	var reply []byte
	reply = <-clientrpc.OutRPC
	if bytes.Equal(reply, []byte("Timeout")) {
		return "", "", fmt.Errorf("timeout")
	}
	enc1b, left, err := common.BytesWithLenToBytes(reply)
	if err != nil {
		return "", "", err
	}
	enc2b, _, err := common.BytesWithLenToBytes(left)
	if err != nil {
		return "", "", err
	}
	enc1, err := blocks.FromBytesToEncryptionConfig(enc1b, true)
	if err != nil {
		return "", "", err
	}
	common.SetEncryption(enc1.SigName, enc1.PubKeyLength, enc1.PrivateKeyLength, enc1.SignatureLength, enc1.IsPaused, true)
	enc2, err := blocks.FromBytesToEncryptionConfig(enc2b, false)
	if err != nil {
		return "", "", err
	}
	common.SetEncryption(enc2.SigName, enc2.PubKeyLength, enc2.PrivateKeyLength, enc2.SignatureLength, enc2.IsPaused, false)
	return enc1.SigName, enc2.SigName, nil
}

func GetStats(w http.ResponseWriter, r *http.Request) {
	clientrpc.InRPC <- SignMessage([]byte("STAT"))
	reply := <-clientrpc.OutRPC
	if bytes.Equal(reply, []byte("Timeout")) {
		jsonError(w, "Timeout", http.StatusGatewayTimeout)
		return
	}

	sm := statistics.GetStatsManager()
	st := sm.Stats
	err := common.Unmarshal(reply, common.StatDBPrefix, &st)
	if err != nil {
		jsonError(w, "Failed to unmarshal stats", http.StatusInternalServerError)
		return
	}

	resp := StatsResponse{
		Height:              st.Height,
		HeightMax:           st.HeightMax,
		TimeInterval:        st.TimeInterval,
		Transactions:        st.Transactions,
		TransactionsPending: st.TransactionsPending,
		Tps:                 st.Tps,
		Syncing:             st.Syncing,
		Difficulty:          st.Difficulty,
		PriceOracle:         st.PriceOracle,
		NodeIP:              NodeIP,
		DelegatedAccount:    DelegatedAccount,
	}
	jsonResponse(w, resp)
}

func GetWalletInfo(w http.ResponseWriter, r *http.Request) {
	if MainWallet == nil || !MainWallet.Check() {
		jsonResponse(w, WalletInfoResponse{Loaded: false})
		return
	}

	resp := WalletInfoResponse{
		Loaded:    true,
		Address:   MainWallet.MainAddress.GetHex(),
		PubKeyHex: MainWallet.Account1.PublicKey.GetHex()[:64] + "...",
		SigName:   MainWallet.GetSigName(true),
		SigName2:  MainWallet.GetSigName(false),
	}
	jsonResponse(w, resp)
}

func LoadWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WalletNumber int    `json:"walletNumber"`
		Password     string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WalletNumber < 0 || req.WalletNumber > 255 {
		jsonError(w, "Wallet number should be between 0 and 255", http.StatusBadRequest)
		return
	}

	sigName, sigName2, err := SetCurrentEncryptions()
	if err != nil {
		jsonError(w, "Error retrieving encryption", http.StatusInternalServerError)
		return
	}

	loadedWallet, err := wallet.LoadJSON(uint8(req.WalletNumber), req.Password, sigName, sigName2)
	if err != nil {
		jsonError(w, fmt.Sprintf("Failed to load wallet: %v", err), http.StatusBadRequest)
		return
	}

	MainWallet = loadedWallet

	warnings := []string{}
	if MainWallet.GetSigName(true) != common.SigName() {
		warnings = append(warnings, "Primary encryption has changed. You may need to update wallet.")
	}
	if MainWallet.GetSigName(false) != common.SigName2() {
		warnings = append(warnings, "Secondary encryption has changed. You may need to update wallet.")
	}

	jsonResponse(w, map[string]interface{}{
		"success":  true,
		"address":  MainWallet.MainAddress.GetHex(),
		"warnings": warnings,
	})
}

func ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if MainWallet == nil || !MainWallet.Check() {
		jsonError(w, "Load wallet first", http.StatusBadRequest)
		return
	}

	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := MainWallet.ChangePassword(req.CurrentPassword, req.NewPassword); err != nil {
		jsonError(w, "Wrong current password", http.StatusBadRequest)
		return
	}

	if err := MainWallet.StoreJSON(); err != nil {
		jsonError(w, fmt.Sprintf("Failed to save wallet: %v", err), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]string{"success": "Password changed"})
}

func GetMnemonic(w http.ResponseWriter, r *http.Request) {
	if MainWallet == nil || !MainWallet.Check() {
		jsonError(w, "Load wallet first", http.StatusBadRequest)
		return
	}

	mnemonic1, err1 := MainWallet.GetMnemonicWords(true)
	mnemonic2, err2 := MainWallet.GetMnemonicWords(false)

	resp := map[string]interface{}{
		"primaryMnemonic":   mnemonic1,
		"primaryError":      "",
		"secondaryMnemonic": mnemonic2,
		"secondaryError":    "",
	}
	if err1 != nil {
		resp["primaryError"] = err1.Error()
	}
	if err2 != nil {
		resp["secondaryError"] = err2.Error()
	}

	jsonResponse(w, resp)
}

func GetAccount(w http.ResponseWriter, r *http.Request) {
	if MainWallet == nil || !MainWallet.Check() {
		jsonError(w, "Load wallet first", http.StatusBadRequest)
		return
	}

	inb := append([]byte("ACCT"), MainWallet.MainAddress.GetBytes()...)
	clientrpc.InRPC <- SignMessage(inb)
	re := <-clientrpc.OutRPC
	if bytes.Equal(re, []byte("Timeout")) {
		jsonError(w, "Timeout", http.StatusGatewayTimeout)
		return
	}

	var acc account.Account
	if err := acc.Unmarshal(re); err != nil {
		jsonError(w, "Failed to unmarshal account", http.StatusInternalServerError)
		return
	}

	conf := acc.GetBalanceConfirmedFloat()
	stake := 0.0
	rewards := 0.0
	locks := 0.0
	stakingDetails := []StakingDetail{}

	for i := 1; i < 5; i++ {
		inb = append([]byte("STAK"), MainWallet.MainAddress.GetBytes()...)
		inb = append(inb, byte(i))
		clientrpc.InRPC <- SignMessage(inb)
		re = <-clientrpc.OutRPC
		if bytes.Equal(re, []byte("Timeout")) {
			continue
		}

		var stakeAcc account.StakingAccount
		if err := stakeAcc.Unmarshal(re[:len(re)-8]); err != nil {
			continue
		}

		stakedAmount := account.Int64toFloat64(stakeAcc.StakedBalance)
		rewardsAmount := account.Int64toFloat64(stakeAcc.StakingRewards)
		lockedAmount := account.Int64toFloat64(common.GetInt64FromByte(re[len(re)-8:]))

		stake += stakedAmount
		rewards += rewardsAmount
		locks += lockedAmount

		if stakeAcc.StakedBalance > 0 || stakeAcc.StakingRewards > 0 {
			a := common.Address{}
			a.Init(stakeAcc.DelegatedAccount[:])
			stakingDetails = append(stakingDetails, StakingDetail{
				DelegatedAddress: a.GetHex(),
				Staked:           stakedAmount,
				Rewards:          rewardsAmount,
			})
		}
	}

	resp := AccountResponse{
		Address:        MainWallet.MainAddress.GetHex(),
		Balance:        conf,
		StakedAmount:   stake,
		LockedAmount:   locks,
		RewardsAmount:  rewards,
		TotalHoldings:  conf + stake + rewards,
		StakingDetails: stakingDetails,
	}
	jsonResponse(w, resp)
}

func SendTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if MainWallet == nil || !MainWallet.Check() {
		jsonError(w, "Load wallet first", http.StatusBadRequest)
		return
	}

	var req struct {
		Recipient string  `json:"recipient"`
		Amount    float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	recipientBytes, err := hex.DecodeString(req.Recipient)
	if err != nil {
		jsonError(w, "Invalid recipient address", http.StatusBadRequest)
		return
	}

	recipientAddr, err := common.BytesToAddress(recipientBytes)
	if err != nil {
		jsonError(w, "Invalid recipient address format", http.StatusBadRequest)
		return
	}

	// This is a simplified version - full implementation would need more parameters
	jsonResponse(w, map[string]string{
		"status":  "pending",
		"message": fmt.Sprintf("Transaction to %s for %.8f QWD queued", recipientAddr.GetHex(), req.Amount),
	})
}

func Stake(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "not implemented"})
}

func Unstake(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "not implemented"})
}

func ClaimRewards(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "not implemented"})
}

func GetHistory(w http.ResponseWriter, r *http.Request) {
	if MainWallet == nil || !MainWallet.Check() {
		jsonError(w, "Load wallet first", http.StatusBadRequest)
		return
	}

	// Return empty history for now - can be expanded
	jsonResponse(w, map[string]interface{}{
		"transactions": []interface{}{},
	})
}

func GetDetails(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	if hash == "" {
		jsonError(w, "Hash parameter required", http.StatusBadRequest)
		return
	}

	var b []byte
	var err error
	if len(hash) < 16 {
		height, err := strconv.Atoi(hash)
		if err != nil {
			jsonError(w, "Invalid height format", http.StatusBadRequest)
			return
		}
		b = common.GetByteInt64(int64(height))
	} else {
		b, err = hex.DecodeString(hash)
		if err != nil {
			jsonError(w, "Invalid hash format", http.StatusBadRequest)
			return
		}
	}

	clientrpc.InRPC <- SignMessage(append([]byte("DETS"), b...))
	reply := <-clientrpc.OutRPC
	if len(reply) <= 2 {
		jsonError(w, "Not found", http.StatusNotFound)
		return
	}

	switch string(reply[:2]) {
	case "TX":
		tx := transactionsDefinition.Transaction{}
		tx, _, err := tx.GetFromBytes(reply[2:])
		if err != nil {
			jsonError(w, "Failed to parse transaction", http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]interface{}{
			"type": "transaction",
			"data": tx.GetString(),
		})
	case "AC":
		var acc account.Account
		if err := acc.Unmarshal(reply[2:]); err != nil {
			jsonError(w, "Failed to parse account", http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]interface{}{
			"type":    "account",
			"balance": acc.GetBalanceConfirmedFloat(),
		})
	case "BL":
		bb := blocks.Block{}
		bb, err = bb.GetFromBytes(reply[2:])
		if err != nil {
			jsonError(w, "Failed to parse block", http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]interface{}{
			"type": "block",
			"data": bb.GetString(),
		})
	default:
		jsonResponse(w, map[string]interface{}{
			"type": "unknown",
			"data": string(reply),
		})
	}
}

func GetTokens(w http.ResponseWriter, r *http.Request) {
	clientrpc.InRPC <- SignMessage([]byte("LTKN"))
	reply := <-clientrpc.OutRPC
	if bytes.Equal(reply, []byte("Timeout")) {
		jsonError(w, "Timeout", http.StatusGatewayTimeout)
		return
	}

	var tokens map[string]interface{}
	if err := json.Unmarshal(reply, &tokens); err != nil {
		jsonResponse(w, map[string]interface{}{"tokens": map[string]interface{}{}})
		return
	}
	jsonResponse(w, map[string]interface{}{"tokens": tokens})
}

func GetPools(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]interface{}{"pools": []interface{}{}})
}

func Trade(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "not implemented"})
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
