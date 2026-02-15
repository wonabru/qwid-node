package handlers

import (
	"github.com/wonabru/qwid-node/common"
)

func SignMessage(line []byte) []byte {
	line = common.BytesToLenAndBytes(line)
	return line
}
