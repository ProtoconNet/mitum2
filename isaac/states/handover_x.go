package isaacstates

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/logging"
)

var (
	errHandoverCanceled = util.NewMError("handover canceled")
	errHandoverIgnore   = util.NewMError("ignore")
	errHandoverReset    = util.NewMError("wrong")
)

var (
	defaultHandoverXMinChallengeCount uint64 = 2
	defaultHandoverXReadyEnd          uint64 = 3
)

type HandoverXBrokerArgs struct {
	SendFunc          func(context.Context, interface{}) error
	CheckIsReady      func() (bool, error)
	WhenCanceled      func(error)
	Local             base.Node
	NetworkID         base.NetworkID
	MinChallengeCount uint64
	ReadyEnd          uint64
}

func NewHandoverXBrokerArgs(local base.Node, networkID base.NetworkID) *HandoverXBrokerArgs {
	return &HandoverXBrokerArgs{
		Local:             local,
		NetworkID:         networkID,
		MinChallengeCount: defaultHandoverXMinChallengeCount,
		SendFunc: func(context.Context, interface{}) error {
			return util.ErrNotImplemented.Errorf("SendFunc")
		},
		CheckIsReady: func() (bool, error) { return false, util.ErrNotImplemented.Errorf("CheckIsReady") },
		WhenCanceled: func(error) {},
		ReadyEnd:     defaultHandoverXReadyEnd,
	}
}

// HandoverXBroker handles handover processes of consensus node.
type HandoverXBroker struct {
	lastVoteproof   base.Voteproof
	lastReceived    interface{}
	cancelByMessage func()
	cancel          func(error)
	stop            func()
	*logging.Logging
	ctxFunc               func() context.Context
	args                  *HandoverXBrokerArgs
	successcount          *util.Locked[uint64]
	id                    string
	previousReadyHandover base.StagePoint
	readyEnd              uint64
	challengecount        uint64
	lastchallengecount    uint64
}

func NewHandoverXBroker(ctx context.Context, args *HandoverXBrokerArgs) *HandoverXBroker {
	hctx, cancel := context.WithCancel(ctx)

	id := util.ULID().String()

	var cancelOnce sync.Once

	h := &HandoverXBroker{
		Logging: logging.NewLogging(func(lctx zerolog.Context) zerolog.Context {
			return lctx.Str("module", "handover-x-broker").Str("id", id)
		}),
		args:         args,
		id:           id,
		ctxFunc:      func() context.Context { return hctx },
		successcount: util.EmptyLocked[uint64](),
	}

	h.cancel = func(err error) {
		cancelOnce.Do(func() {
			defer h.Log().Debug().Err(err).Msg("canceled")

			_ = args.SendFunc(ctx, newHandoverMessageCancel(id))

			cancel()

			args.WhenCanceled(err)
		})
	}

	h.cancelByMessage = func() {
		cancelOnce.Do(func() {
			defer h.Log().Debug().Msg("canceled by message")

			cancel()

			args.WhenCanceled(errHandoverCanceled.Errorf("canceled by message"))
		})
	}

	h.stop = func() {
		cancelOnce.Do(func() {
			defer h.Log().Debug().Msg("stopped")

			cancel()
		})
	}

	return h
}

func (h *HandoverXBroker) ID() string {
	return h.id
}

func (h *HandoverXBroker) isCanceled() error {
	if err := h.ctxFunc().Err(); err != nil {
		return errHandoverCanceled.Wrap(err)
	}

	return nil
}

func (h *HandoverXBroker) isReady() (uint64, bool) {
	count, _ := h.successcount.Value()

	return count, h.checkIsReady(count)
}

func (h *HandoverXBroker) checkIsReady(count uint64) bool {
	return count >= h.args.MinChallengeCount
}

func (h *HandoverXBroker) isFinished(vp base.Voteproof) (isFinished bool, _ error) {
	if err := h.isCanceled(); err != nil {
		return false, err
	}

	if vp == nil {
		return false, nil
	}

	_ = h.successcount.Get(func(uint64, bool) error {
		if h.readyEnd < 1 {
			return nil
		}

		if _, ok := vp.(base.INITVoteproof); !ok {
			return nil
		}

		switch count, ok := h.isReady(); {
		case !ok:
			return nil
		default:
			isFinished = count >= h.readyEnd

			return nil
		}
	})

	return isFinished, nil
}

func (h *HandoverXBroker) finish(ivp base.INITVoteproof) error {
	if err := h.isCanceled(); err != nil {
		return err
	}

	defer h.stop()

	hc := newHandoverMessageFinish(h.id, ivp)
	if err := h.args.SendFunc(h.ctxFunc(), hc); err != nil {
		return errHandoverCanceled.Wrap(err)
	}

	h.Log().Debug().Interface("message", hc).Msg("sent HandoverMessageFinish")

	return nil
}

// sendVoteproof sends voteproof to Y; If ready to finish, sendVoteproof will
// finish broker process.
func (h *HandoverXBroker) sendVoteproof(ctx context.Context, vp base.Voteproof) (isFinished bool, _ error) {
	if err := h.isCanceled(); err != nil {
		return false, err
	}

	if ivp, ok := vp.(base.INITVoteproof); ok {
		switch ok, err := h.isFinished(ivp); {
		case err != nil:
			return false, err
		case ok:
			h.Log().Debug().Interface("init_voteproof", ivp).Msg("finished")

			if err := h.finish(ivp); err != nil {
				return false, err
			}

			return true, nil
		}
	}

	switch err := h.sendVoteproofErr(ctx, vp); {
	case err == nil,
		errors.Is(err, errHandoverIgnore):
		return false, nil
	default:
		h.cancel(err)

		return false, errHandoverCanceled.Wrap(err)
	}
}

func (h *HandoverXBroker) sendVoteproofErr(ctx context.Context, vp base.Voteproof) error {
	_ = h.successcount.Get(func(uint64, bool) error {
		h.lastVoteproof = vp
		h.challengecount++

		return nil
	})

	hc := newHandoverMessageData(h.id, vp)
	if err := h.args.SendFunc(ctx, hc); err != nil {
		return errHandoverIgnore.Wrap(err)
	}

	return nil
}

func (h *HandoverXBroker) sendData(ctx context.Context, data interface{}) error {
	if err := h.isCanceled(); err != nil {
		return err
	}

	hc := newHandoverMessageData(h.id, data)
	if err := h.args.SendFunc(ctx, hc); err != nil {
		return errHandoverIgnore.Wrap(err)
	}

	return nil
}

func (h *HandoverXBroker) receive(i interface{}) error {
	_, err := h.successcount.Set(func(before uint64, _ bool) (uint64, error) {
		var after, beforeReadyEnd uint64
		{
			beforeReadyEnd = h.readyEnd
		}

		var err error

		switch after, err = h.receiveInternal(i, before); {
		case err == nil:
		case errors.Is(err, errHandoverIgnore):
			after = before

			err = nil
		case errors.Is(err, errHandoverReset):
			h.readyEnd = 0

			err = nil
		case errors.Is(err, errHandoverCanceled):
		default:
			h.cancel(err)

			err = errHandoverCanceled.Wrap(err)
		}

		h.Log().Debug().
			Interface("data", i).
			Err(err).
			Uint64("min_challenge_count", h.args.MinChallengeCount).
			Dict("before", zerolog.Dict().
				Uint64("challenge_count", before).
				Uint64("ready_end", beforeReadyEnd),
			).
			Dict("after", zerolog.Dict().
				Uint64("challenge_count", after).
				Uint64("ready_end", h.readyEnd),
			).
			Msg("received")

		return after, err
	})

	return err
}

func (h *HandoverXBroker) receiveInternal(i interface{}, successcount uint64) (uint64, error) {
	if err := h.isCanceled(); err != nil {
		return 0, err
	}

	if id, ok := i.(HandoverMessage); ok {
		if h.id != id.HandoverID() {
			return 0, errors.Errorf("id not matched")
		}
	}

	if iv, ok := i.(util.IsValider); ok {
		if err := iv.IsValid(h.args.NetworkID); err != nil {
			return 0, err
		}
	}

	if _, ok := i.(HandoverMessageCancel); ok {
		h.cancelByMessage()

		return 0, errHandoverCanceled.Errorf("canceled by message")
	}

	switch t := i.(type) {
	case HandoverMessageChallengeStagePoint,
		HandoverMessageChallengeBlockMap:
		return h.receiveChallenge(t, successcount)
	case HandoverMessageReady:
		return successcount, h.receiveHandoverReady(t, successcount)
	default:
		return 0, errHandoverIgnore.Errorf("Y sent unknown message, %T", i)
	}
}

func (h *HandoverXBroker) receiveChallenge(i interface{}, successcount uint64) (uint64, error) {
	var err error

	switch t := i.(type) {
	case HandoverMessageChallengeStagePoint:
		err = h.receiveStagePoint(t)
	case HandoverMessageChallengeBlockMap:
		err = h.receiveBlockMap(t)
	default:
		return 0, errHandoverIgnore.Errorf("Y sent unknown message, %T", i)
	}

	if err != nil {
		return 0, err
	}

	after := successcount + 1

	if h.lastchallengecount != h.challengecount-1 {
		after = 1
	}

	h.lastchallengecount = h.challengecount

	return after, nil
}

func (h *HandoverXBroker) receiveStagePoint(i HandoverMessageChallengeStagePoint) error {
	point := i.Point()

	if err := func() error {
		if h.lastReceived == nil {
			return nil
		}

		if prev, ok := h.lastReceived.(base.StagePoint); ok && point.Compare(prev) < 1 {
			return errHandoverIgnore.Errorf("old stagepoint from y")
		}

		return nil
	}(); err != nil {
		return err
	}

	h.lastReceived = point

	if h.lastVoteproof == nil {
		return errors.Errorf("no last init voteproof for blockmap from y")
	}

	switch t := h.lastVoteproof.(type) {
	case base.INITVoteproof:
		if !t.Point().Equal(point) {
			return errHandoverReset.Errorf("stagepoint not match for init voteproof")
		}
	case base.ACCEPTVoteproof:
		if t.Result() == base.VoteResultMajority {
			return errHandoverReset.Errorf("unexpected stagepoint for majority accept voteproof")
		}

		if !t.Point().Equal(point) {
			return errHandoverReset.Errorf("stagepoint not match for accept voteproof")
		}
	default:
		return errors.Errorf("unknown voteproof, %T", t)
	}

	return nil
}

func (h *HandoverXBroker) receiveBlockMap(i HandoverMessageChallengeBlockMap) error {
	m := i.BlockMap()

	switch {
	case !m.Node().Equal(h.args.Local.Address()):
		return errors.Errorf("invalid blockmap from y; not signed by local; wrong address")
	case !m.Signer().Equal(h.args.Local.Publickey()):
		return errors.Errorf("invalid blockmap from y; not signed by local; different key")
	}

	if err := func() error {
		if h.lastReceived == nil {
			return nil
		}

		prevbm, ok := h.lastReceived.(base.BlockMap)
		if !ok {
			return nil
		}

		if m.Manifest().Height() <= prevbm.Manifest().Height() {
			return errHandoverIgnore.Errorf("old blockmap from y")
		}

		return nil
	}(); err != nil {
		return err
	}

	h.lastReceived = m

	if h.lastVoteproof == nil {
		return errors.Errorf("no last accept voteproof for blockmap from y")
	}

	switch avp, ok := h.lastVoteproof.(base.ACCEPTVoteproof); {
	case !ok:
		return errHandoverReset.Errorf("last not accept voteproof")
	case avp.Result() != base.VoteResultMajority:
		return errHandoverReset.Errorf("last not majority accept voteproof, but blockmap from y")
	case !avp.BallotMajority().NewBlock().Equal(m.Manifest().Hash()):
		return errHandoverReset.Errorf("manifest hash not match")
	default:
		return nil
	}
}

func (h *HandoverXBroker) receiveHandoverReady(hc HandoverMessageReady, successcount uint64) error {
	err := h.receiveHandoverReadyOK(hc)

	var isReady bool
	if err == nil && h.checkIsReady(successcount) {
		isReady, err = h.args.CheckIsReady()
	}

	if serr := h.args.SendFunc(
		h.ctxFunc(),
		newHandoverMessageReadyResponse(h.id, hc.Point(), isReady, err),
	); serr != nil {
		h.readyEnd = 0

		if err != nil {
			return util.JoinErrors(err, serr)
		}

		return errHandoverIgnore.Wrap(serr)
	}

	switch {
	case err != nil:
		h.readyEnd = 0

		return err
	case !isReady, h.readyEnd > 0:
	default:
		h.readyEnd = successcount + h.args.ReadyEnd
	}

	return nil
}

func (h *HandoverXBroker) receiveHandoverReadyOK(hc HandoverMessageReady) error {
	if err := hc.IsValid(nil); err != nil {
		return errors.WithMessage(err, "invalid HandoverMessageReady")
	}

	switch vp := h.lastVoteproof; {
	case vp == nil:
		return errors.Errorf("no last voteproof, but HandoverMessageReady from y")
	case hc.Point().Compare(vp.Point()) > 0:
		return errHandoverReset.Errorf("higher HandoverMessageReady point with last voteproof")
	}

	switch prev := h.previousReadyHandover; {
	case prev.IsZero():
	case hc.Point().Compare(prev) <= 0:
		return errHandoverReset.Errorf(
			"HandoverMessageReady point should be higher than previous")
	}

	h.previousReadyHandover = hc.Point()

	return nil
}