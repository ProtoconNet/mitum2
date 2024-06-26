package isaacstates

import (
	"context"
	"time"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/network"
	"github.com/ProtoconNet/mitum2/network/quicstream"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/pkg/errors"
)

type (
	StartHandoverYFunc func(context.Context, base.Address, quicstream.ConnInfo) error
	CheckHandoverFunc  func(context.Context, base.Address, quicstream.ConnInfo) error
	AskHandoverFunc    func(context.Context, quicstream.ConnInfo) (
		handoverid string, canMoveConsensus bool, _ error)
	AskHandoverReceivedFunc func(context.Context, base.Address, quicstream.ConnInfo) (
		handoverid string, canMoveConsensus bool, _ error)
	CheckHandoverXFunc func(context.Context) error
)

func (st *States) HandoverXBroker() *HandoverXBroker {
	v, _ := st.handoverXBroker.Value()

	switch {
	case v == nil:
	default:
		if err := v.isCanceled(); err != nil {
			return nil
		}
	}

	return v
}

func (st *States) NewHandoverXBroker(connInfo quicstream.ConnInfo) (handoverid string, _ error) {
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

		handoverid = broker.ID()

		st.Log().Debug().Str("id", handoverid).Msg("handover x broker created")

		return broker, nil
	})

	return handoverid, err
}

func (st *States) CancelHandoverXBroker() error {
	return st.handoverXBroker.Empty(func(broker *HandoverXBroker, isempty bool) error {
		if isempty {
			return nil
		}

		broker.cancel(ErrHandoverCanceled.Errorf("canceled"))

		return nil
	})
}

func (st *States) HandoverYBroker() *HandoverYBroker {
	v, _ := st.handoverYBroker.Value()

	switch {
	case v == nil:
	default:
		if err := v.isCanceled(); err != nil {
			return nil
		}
	}

	return v
}

func (st *States) NewHandoverYBroker(connInfo quicstream.ConnInfo) error {
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

		st.Log().Debug().Msg("handover y broker created")

		return broker, nil
	})

	return err
}

func (st *States) CancelHandoverYBroker() error {
	return st.handoverYBroker.Empty(func(broker *HandoverYBroker, isempty bool) error {
		if isempty {
			return nil
		}

		broker.cancel(ErrHandoverCanceled.Errorf("canceled"))

		return nil
	})
}

func (st *States) cleanHandovers() {
	_ = st.handoverXBroker.Empty(func(_ *HandoverXBroker, isempty bool) error {
		if !isempty {
			st.Log().Debug().Msg("handover x broker cleaned")
		}

		return nil
	})

	_ = st.handoverYBroker.Empty(func(_ *HandoverYBroker, isempty bool) error {
		if !isempty {
			st.Log().Debug().Msg("handover y broker cleaned")
		}

		return nil
	})
}

func NewStartHandoverYFunc(
	local base.Address,
	localci quicstream.ConnInfo,
	isAllowedConsensus func() bool,
	isHandoverStarted func() bool,
	checkX func(context.Context, base.Address, quicstream.ConnInfo) error,
	addSyncSource func(base.Address, quicstream.ConnInfo) error,
	startHandoverY func(quicstream.ConnInfo) error,
) StartHandoverYFunc {
	return func(ctx context.Context, node base.Address, xci quicstream.ConnInfo) error {
		e := util.StringError("check handover y")

		switch {
		case !local.Equal(node):
			return e.Errorf("address not matched")
		case network.EqualConnInfo(localci, xci):
			return e.Errorf("same conn info")
		case isAllowedConsensus():
			return e.Errorf("allowed consensus")
		case isHandoverStarted():
			return e.Errorf("handover already started")
		}

		if err := checkX(ctx, node, xci); err != nil {
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

func NewCheckHandoverFunc(
	local base.Address,
	localci quicstream.ConnInfo,
	isAllowedConsensus func() bool,
	isHandoverStarted func() bool,
	isJoinedMemberlist func() (bool, error),
	currentState func() StateType,
) CheckHandoverFunc {
	return func(_ context.Context, node base.Address, yci quicstream.ConnInfo) error {
		e := util.StringError("check handover x")

		switch {
		case !local.Equal(node):
			return e.Errorf("address not matched")
		case network.EqualConnInfo(localci, yci):
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
			return e.Errorf("x not joined memberlist")
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

func NewAskHandoverReceivedFunc(
	local base.Address,
	localci quicstream.ConnInfo,
	isAllowedConsensus func() bool,
	isHandoverStarted func() bool,
	isJoinedMemberlist func(quicstream.ConnInfo) (bool, error),
	currentState func() StateType,
	setNotAllowConsensus func(),
	startHandoverX func(quicstream.ConnInfo) (handoverid string, _ error),
) AskHandoverReceivedFunc {
	return func(_ context.Context, node base.Address, yci quicstream.ConnInfo) (string, bool, error) {
		e := util.StringError("ask handover")

		switch {
		case !local.Equal(node):
			return "", false, e.Errorf("address not matched")
		case network.EqualConnInfo(localci, yci):
			return "", false, e.Errorf("same conn info")
		case isHandoverStarted():
			return "", false, e.Errorf("handover already started")
		case !isAllowedConsensus():
			return "", true, nil
		}

		switch joined, err := isJoinedMemberlist(yci); {
		case err != nil:
			return "", false, e.Wrap(err)
		case !joined:
			return "", false, e.Errorf("y not joined memberlist")
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

func NewCheckHandoverXFunc(
	isAllowedConsensus func() bool,
	isHandoverStarted func() bool,
	isJoinedMemberlist func() (bool, error),
	currentState func() StateType,
) CheckHandoverXFunc {
	return func(context.Context) error {
		e := util.StringError("check only handover x")

		switch {
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
	joinMemberlist func(context.Context, quicstream.ConnInfo) error,
	sendAsk func(context.Context, base.Address, quicstream.ConnInfo) (string, bool, error),
) AskHandoverFunc {
	return func(ctx context.Context, ci quicstream.ConnInfo) (string, bool, error) {
		e := util.StringError("ask handover to x")

		if err := joinMemberlist(ctx, ci); err != nil {
			return "", false, e.WithMessage(err, "join memberlist")
		}

		<-time.After(time.Second * 6)

		handoverid, canMoveConsensus, err := sendAsk(ctx, local, ci)

		return handoverid, canMoveConsensus, e.WithMessage(err, "ask")
	}
}

func NewHandoverXFinishedFunc(
	leftMemberlist func() error,
	addSyncSource func(base.Address, quicstream.ConnInfo) error,
) func(base.INITVoteproof, base.Address, quicstream.ConnInfo) error {
	return func(_ base.INITVoteproof, node base.Address, yci quicstream.ConnInfo) error {
		e := util.StringError("handover x finished")

		if err := leftMemberlist(); err != nil {
			return e.Wrap(err)
		}

		return e.Wrap(addSyncSource(node, yci))
	}
}

func NewHandoverYFinishedFunc(
	removeSyncSource func(quicstream.ConnInfo) error,
) func(base.INITVoteproof, quicstream.ConnInfo) error {
	return func(_ base.INITVoteproof, xci quicstream.ConnInfo) error {
		if err := removeSyncSource(xci); err != nil {
			return errors.WithMessage(err, "handover y finished")
		}

		return nil
	}
}

func NewHandoverYCanceledFunc(
	leftMemberlist func() error,
	removeSyncSource func(quicstream.ConnInfo) error,
) func(error, quicstream.ConnInfo) {
	return func(_ error, xci quicstream.ConnInfo) {
		_ = leftMemberlist()
		_ = removeSyncSource(xci)
	}
}
