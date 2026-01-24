package account

import (
	"github.com/wonabru/qwid-node/common"
	"math"
)

func getRemainingSupply(supply int64) int64 {
	return common.MaxTotalSupply - supply
}

func GetReward(supply int64) int64 {
	cr := common.RewardRatio * float64(getRemainingSupply(supply))
	cr = math.Round(cr)
	return int64(cr)
}
