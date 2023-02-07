package isaac

import (
	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util/hint"
)

type OperationProcessHandler interface {
	Hints() []hint.Hint
	PreProcess(operation base.Operation) (bool, error)
	Process(operation base.Operation) ([]base.State, error)
}
