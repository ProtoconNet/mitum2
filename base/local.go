package base

import "github.com/ProtoconNet/mitum2/util"

type LocalParams interface {
	util.IsValider
	NetworkID() NetworkID
	Threshold() Threshold
}
