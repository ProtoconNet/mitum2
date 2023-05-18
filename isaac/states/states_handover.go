package isaacstates

import (
	"context"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/network/quicstream"
	"github.com/spikeekips/mitum/util"
)

type (
	StartHandoverYFunc func(context.Context, base.Address, quicstream.UDPConnInfo) error
	CheckHandoverXFunc func(context.Context, base.Address, quicstream.UDPConnInfo) error
	AskHandoverFunc    func(context.Context, quicstream.UDPConnInfo) (
		handoverid string, canMoveConsensus bool, _ error)
	AskHandoverReceivedFunc func(context.Context, base.Address, quicstream.UDPConnInfo) (
		handoverid string, canMoveConsensus bool, _ error)
)

func (st *States) HandoverXBroker() *HandoverXBroker {
	v, _ := st.handoverXBroker.Value()

	return v
}

func (st *States) NewHandoverXBroker(connInfo quicstream.UDPConnInfo) error {
	_, err := st.handoverXBroker.Set(func(_ *HandoverXBroker, isempty bool) (*HandoverXBroker, error) {
		switch {
		case !isempty:
			return nil, errors.Errorf("already under handover x")
		case !st.AllowedConsensus():
			return nil, errors.Errorf("not allowed consensus")
		case st.HandoverYBroker() != nil:
			return nil, errors.Errorf("under handover y")
		}

		broker, err := st.args.NewHandoverXBroker(context.Background(), connInfo)
		if err != nil {
			st.Log().Error().Err(err).Msg("failed new handover x broker")

			return nil, err
		}

		if err := broker.patchStates(st); err != nil {
			return nil, err
		}

		return broker, nil
	})

	return err
}

func (st *States) HandoverYBroker() *HandoverYBroker {
	v, _ := st.handoverYBroker.Value()

	return v
}

func (st *States) NewHandoverYBroker(connInfo quicstream.UDPConnInfo) error {
	_, err := st.handoverYBroker.Set(func(_ *HandoverYBroker, isempty bool) (*HandoverYBroker, error) {
		switch {
		case !isempty:
			return nil, errors.Errorf("already under handover y")
		case st.AllowedConsensus():
			return nil, errors.Errorf("allowed consensus")
		case st.HandoverXBroker() != nil:
			return nil, errors.Errorf("under handover x")
		}

		broker, err := st.args.NewHandoverYBroker(context.Background(), connInfo)
		if err != nil {
			st.Log().Error().Err(err).Msg("failed new handover y broker")

			return nil, err
		}

		if err := broker.patchStates(st); err != nil {
			return nil, err
		}

		return broker, nil
	})

	return err
}

func (st *States) cleanHandover() {
	_ = st.handoverXBroker.Empty(func(i *HandoverXBroker, isempty bool) error {
		if !isempty {
			st.Log().Debug().Msg("handover x broker canceled")
		}

		return nil
	})

	_ = st.handoverYBroker.Empty(func(i *HandoverYBroker, isempty bool) error {
		if !isempty {
			st.Log().Debug().Msg("handover y broker canceled")
		}

		return nil
	})
}

func NewStartHandoverYFunc(
	local base.Address,
	localci quicstream.UDPConnInfo,
	isAllowedConsensus func() bool,
	isHandoverStarted func() bool,
	checkX func(base.Address, quicstream.UDPConnInfo) error,
	addSyncSource func(base.Address, quicstream.UDPConnInfo) error,
	startHandoverY func(quicstream.UDPConnInfo) error,
) StartHandoverYFunc {
	return func(ctx context.Context, node base.Address, xci quicstream.UDPConnInfo) error {
		e := util.StringError("check handover y")

		switch {
		case !local.Equal(node):
			return e.Errorf("address not matched")
		case localci.Addr().String() == xci.Addr().String():
			return e.Errorf("same conn info")
		case isAllowedConsensus():
			return e.Errorf("allowed consensus")
		case isHandoverStarted():
			return e.Errorf("handover already started")
		}

		if err := checkX(node, xci); err != nil {
			return e.WithMessage(err, "check x")
		}

		if err := addSyncSource(node, xci); err != nil {
			return e.WithMessage(err, "add sync source")
		}

		if err := startHandoverY(xci); err != nil {
			return e.WithMessage(err, "start handover y")
		}

		return nil
	}
}

func NewCheckHandoverXFunc(
	local base.Address,
	localci quicstream.UDPConnInfo,
	isAllowedConsensus func() bool,
	isHandoverStarted func() bool,
	isJoinedMemberlist func() (bool, error),
	currentState func() StateType,
) CheckHandoverXFunc {
	return func(ctx context.Context, node base.Address, yci quicstream.UDPConnInfo) error {
		e := util.StringError("check handover x")

		switch {
		case !local.Equal(node):
			return e.Errorf("address not matched")
		case localci.Addr().String() == yci.Addr().String():
			return e.Errorf("same conn info")
		case !isAllowedConsensus():
			return e.Errorf("not allowed consensus")
		case isHandoverStarted():
			return e.Errorf("handover already started")
		}

		switch joined, err := isJoinedMemberlist(); {
		case err != nil:
			return e.Wrap(err)
		case !joined:
			return e.Errorf("not joined memberlist")
		}

		switch currentState() {
		case StateSyncing, StateConsensus, StateJoining, StateBooting:
			return nil
		case StateHandover:
			return e.Errorf("x is under handover x")
		default:
			return e.Errorf("not valid state")
		}
	}
}

func NewAskHandoverFunc(
	local base.Address,
	joinMemberlist func(context.Context) error,
	sendAsk func(context.Context, quicstream.UDPConnInfo) (string, bool, error),
) AskHandoverFunc {
	return func(ctx context.Context, ci quicstream.UDPConnInfo) (string, bool, error) {
		e := util.StringError("ask handover to x")

		if err := joinMemberlist(ctx); err != nil {
			return "", false, e.WithMessage(err, "join memberlist")
		}

		handoverid, canMoveConsensus, err := sendAsk(ctx, ci)

		return handoverid, canMoveConsensus, e.WithMessage(err, "ask")
	}
}

func NewAskHandoverReceivedFunc(
	local base.Address,
	localci quicstream.UDPConnInfo,
	isAllowedConsensus func() bool,
	isJoinedMemberlist func(quicstream.UDPConnInfo) (bool, error),
	currentState func() StateType,
	setNotAllowConsensus func(),
	startHandoverX func(quicstream.UDPConnInfo) (handoverid string, _ error),
) AskHandoverReceivedFunc {
	return func(ctx context.Context, node base.Address, yci quicstream.UDPConnInfo) (string, bool, error) {
		e := util.StringError("ask handover")

		switch {
		case !local.Equal(node):
			return "", false, e.Errorf("address not matched")
		case localci.Addr().String() == yci.Addr().String():
			return "", false, e.Errorf("same conn info")
		case !isAllowedConsensus():
			return "", true, nil
		}

		switch joined, err := isJoinedMemberlist(yci); {
		case err != nil:
			return "", false, e.Wrap(err)
		case !joined:
			return "", false, e.Errorf("not joined memberlist")
		}

		switch currentState() {
		case StateConsensus, StateJoining, StateBooting:
		default:
			setNotAllowConsensus()

			return "", true, nil
		}

		id, err := startHandoverX(yci)

		return id, false, err
	}
}

func NewHandoverXFinishedFunc(
	leftMemberlist func() error,
) func(base.INITVoteproof) error {
	return func(base.INITVoteproof) error {
		return util.StringError("handover x finished").
			WithMessage(leftMemberlist(), "left memberlist")
	}
}

func NewHandoverYFinishedFunc(
	leftMemberlist func() error,
	removeSyncSource func() error,
) func(base.INITVoteproof) error {
	return func(base.INITVoteproof) error {
		lch := make(chan error)
		rch := make(chan error)

		go func() { lch <- leftMemberlist() }()
		go func() { rch <- removeSyncSource() }()

		return util.StringError("handover y finished").Wrap(
			util.JoinErrors(<-lch, <-rch),
		)
	}
}

func NewHandoverYCanceledFunc(
	leftMemberlist func() error,
	removeSyncSource func() error,
) func(base.INITVoteproof) error {
	return func(base.INITVoteproof) error {
		lch := make(chan error)
		rch := make(chan error)

		go func() { lch <- leftMemberlist() }()
		go func() { rch <- removeSyncSource() }()

		return util.StringError("handover y canceled").Wrap(
			util.JoinErrors(<-lch, <-rch),
		)
	}
}
