package isaacstates

import (
	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
)

type StoppedHandler struct {
	*baseHandler
}

type NewStoppedHandlerType struct {
	*StoppedHandler
}

func NewNewStoppedHandlerType(
	local base.LocalNode,
	params *isaac.LocalParams,
) *NewStoppedHandlerType {
	return &NewStoppedHandlerType{
		StoppedHandler: &StoppedHandler{
			baseHandler: newBaseHandler(StateStopped, local, params),
		},
	}
}

func (h *NewStoppedHandlerType) new() (handler, error) {
	return &StoppedHandler{
		baseHandler: h.baseHandler.new(),
	}, nil
}

func newStoppedSwitchContext(from StateType, err error) baseErrorSwitchContext {
	return newBaseErrorSwitchContext(StateStopped, err, switchContextOKFuncCheckFrom(from))
}
