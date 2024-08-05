package isaacstates

import (
	"github.com/ProtoconNet/mitum2/base"
)

type StoppedHandler struct {
	*baseHandler
}

type NewStoppedHandlerType struct {
	*StoppedHandler
}

func NewNewStoppedHandlerType(
	networkID base.NetworkID,
	local base.LocalNode,
) *NewStoppedHandlerType {
	return &NewStoppedHandlerType{
		StoppedHandler: &StoppedHandler{
			baseHandler: newBaseHandlerType(StateStopped, networkID, local),
		},
	}
}

func (h *NewStoppedHandlerType) new() (handler, error) {
	return &StoppedHandler{
		baseHandler: h.baseHandler.new(),
	}, nil
}

func newStoppedSwitchContext(from StateType, err error) baseErrorSwitchContext {
	return newBaseErrorSwitchContext(from, StateStopped, err)
}
