package base

import (
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/hint"
)

var StateFixedtreeHint = hint.MustNewHint("state-fixedtree-v0.0.1")

type State interface {
	util.Hasher // NOTE <key> + <value HashByte> + <height>
	util.IsValider
	Key() string
	Value() StateValue
	Height() Height          // NOTE manifest height
	Previous() util.Hash     // NOTE previous state hash
	Operations() []util.Hash // NOTE operation fact hash
}

type StateValue interface {
	util.HashByter
	util.IsValider
}

type StateMergeValue interface {
	StateValue
	Key() string
	Value() StateValue
	Merger(Height, State) StateValueMerger
}

type StateValueMerger interface {
	Key() string
	Merge(value StateValue, operationfact util.Hash) error
	CloseValue() (newState State, _ error)
	Close() error // NOTE can go to sync.Pool
}

type GetStateFunc func(key string) (State, bool, error)

func IsEqualState(a, b State) bool {
	switch {
	case a == nil || b == nil:
		return false
	case !a.Hash().Equal(b.Hash()):
		return false
	case a.Key() != b.Key():
		return false
	case a.Height() != b.Height():
		return false
	case !IsEqualStateValue(a.Value(), b.Value()):
		return false
	case len(a.Operations()) != len(b.Operations()):
		return false
	default:
		ao := a.Operations()
		bo := b.Operations()

		for i := range ao {
			if !ao[i].Equal(bo[i]) {
				return false
			}
		}

		return true
	}
}

func IsEqualStateValue(a, b StateValue) bool {
	return util.IsEqualHashByter(a, b)
}
