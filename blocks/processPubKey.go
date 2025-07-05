package blocks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/okuralabs/okura-node/common"
	"github.com/okuralabs/okura-node/database"
	"github.com/okuralabs/okura-node/logger"
	"github.com/okuralabs/okura-node/pubkeys"
	"github.com/okuralabs/okura-node/transactionsDefinition"
	"github.com/okuralabs/okura-node/wallet"
)

func StoreAddress(mainAddress common.Address, pk common.PubKey) error {
	index, err := pubkeys.FindAddressForMainAddress(mainAddress, pk.Address)
	if err != nil {
		return err
	}
	if index >= 0 {
		return fmt.Errorf("address just stored before")
	}
	err = pubkeys.AddPubKeyToAddress(pk, mainAddress)
	if err != nil {
		return err
	}
	return nil
}

func AddNewPubKeyToActiveWallet(sigName string, primary bool) error {
	w := wallet.GetActiveWallet()
	makeBackup := false
	if w.GetSigName(primary) != sigName {
		makeBackup = true
		w.HomePathOld = w.HomePath
		err := w.AddNewEncryptionToActiveWallet(sigName, primary)
		if err != nil {
			return err
		}
	}
	if primary {
		err := StorePubKey(w.PublicKey)
		if err != nil {
			return err
		}
		err = StorePubKeyInPatriciaTrie(w.PublicKey)
		if err != nil {
			return err
		}
	} else {
		err := StorePubKey(w.PublicKey2)
		if err != nil {
			return err
		}
		err = StorePubKeyInPatriciaTrie(w.PublicKey2)
		if err != nil {
			return err
		}
	}
	err := w.StoreJSON(makeBackup)
	if err != nil {
		return err
	}
	return nil
}

//func GetMainAddress(a common.Address) (common.Address, error) {
//	pk, err := memDatabase.LoadPubKey(a.GetBytes())
//	if err != nil {
//		return common.Address{}, err
//	}
//	return pk.MainAddress, nil
//}

func StorePubKey(pk common.PubKey) error {
	a, err := common.PubKeyToAddress(pk.GetBytes(), pk.Primary)
	if err != nil {
		return err
	}
	if !bytes.Equal(a.GetBytes(), pk.Address.GetBytes()) {
		return fmt.Errorf("address is different in pubkey and recovered from bytes")
	}
	//err = memDatabase.MainDB.Put(append(common.PubKeyDBPrefix[:], a.GetBytes()...), pk.GetBytes())
	//if err != nil {
	//	return err
	//}
	pkm, err := json.Marshal(pk)
	if err != nil {
		return err
	}
	err = database.MainDB.Put(append(common.PubKeyMarshalDBPrefix[:], a.GetBytes()...), pkm)
	return err
}

func StorePubKeyInPatriciaTrie(pk common.PubKey) error {
	addresses, err := pubkeys.LoadAddresses(pk.MainAddress)
	if err != nil {
		if err.Error() != "key not found" {
			return err
		}
		logger.GetLogger().Println("key not found")
		addresses = []common.Address{}
	}
	if len(addresses) == 0 {
		mainAddress, err2 := pubkeys.CreateAddressFromFirstPubKey(pk)
		if err2 != nil {
			return err2
		}
		if !bytes.Equal(pk.MainAddress.GetBytes(), mainAddress.GetBytes()) {
			return fmt.Errorf("error with creation of address from first pub key %v != %v", pk.MainAddress.GetHex(), mainAddress.GetHex())
		}
		return nil
	}
	exist := false
	for _, a := range addresses {
		if bytes.Equal(a.GetBytes(), pk.Address.GetBytes()) {
			exist = true
			break
		}
	}
	if exist {
		//logger.GetLogger().Println(" address from pub key is just stored in mainaddress of patricia trie")
		return nil
	}

	address, err := common.PubKeyToAddress(pk.GetBytes(), pk.Primary)
	if err != nil {
		return err
	}
	addresses = append(addresses, address)
	tree, err := pubkeys.BuildMerkleTree(pk.MainAddress, addresses, pubkeys.GlobalMerkleTree.DB)
	if err != nil {
		return err
	}
	for _, a := range addresses {
		if !tree.IsAddressInTree(a) {
			return fmt.Errorf("pubkey patricia trie fails to add pubkey")
		}
	}
	err = tree.StoreTree(pk.MainAddress)
	if err != nil {
		return err
	}

	return nil
}

// LoadPubKey : a - address in bytes of pubkey
//func LoadPubKey(a []byte, mainAddress common.Address) (pk *common.PubKey, err error) {
//	pkb, err := memDatabase.MainDB.Get(append(common.PubKeyDBPrefix[:], a...))
//	if err != nil {
//		return &common.PubKey{}, err
//	}
//	err = pk.Init(pkb, mainAddress)
//	if err != nil {
//		return &common.PubKey{}, err
//	}
//	return pk, nil
//}

// ProcessBlockPubKey : store pubkey on each transaction
func ProcessBlockPubKey(block Block) error {
	for _, txh := range block.TransactionsHashes {
		t, err := transactionsDefinition.LoadFromDBPoolTx(common.TransactionPoolHashesDBPrefix[:], txh.GetBytes())
		if err != nil {
			//TODO
			//transactionsDefinition.RemoveTransactionFromDBbyHash(common.TransactionPoolHashesDBPrefix[:], txh.GetBytes())
			return err
		}
		pk := t.TxData.Pubkey
		zeroBytes := make([]byte, common.AddressLength)
		if bytes.Equal(pk.MainAddress.GetBytes(), zeroBytes) {
			return nil
		}
		err = StorePubKey(pk)
		if err != nil {
			return err
		}
		err = StorePubKeyInPatriciaTrie(pk)
		if err != nil {
			return err
		}
	}
	return nil
}
