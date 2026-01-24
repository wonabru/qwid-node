package stateDB

import "github.com/wonabru/qwid-node/common"

type Code []byte

type Storage map[common.Hash]common.Hash

type stateObject struct {
	address common.Address
	db      *StateAccount
	code    Code

	originStorage Storage
}
