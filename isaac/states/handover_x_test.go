package isaacstates

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/valuehash"
	"github.com/stretchr/testify/suite"
)

type testHandoverXBroker struct {
	baseTestHandoverBroker
}

func (t *testHandoverXBroker) TestNew() {
	args := t.xargs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := NewHandoverXBroker(ctx, args)

	t.Run("isCanceled", func() {
		t.NoError(broker.isCanceled())
	})

	t.Run("isReady", func() {
		count, isReady := broker.isReady()
		t.Equal(uint64(0), count)
		t.False(isReady)
	})

	t.Run("isFinished", func() {
		isFinished, err := broker.isFinished(nil)
		t.NoError(err)
		t.False(isFinished)
	})

	t.Run("canceled by context; isCanceled", func() {
		cancel()

		err := broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})

	t.Run("cancel(); isCanceled", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		t.NoError(broker.isCanceled())

		broker.cancel(nil)

		err := broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})
}

func (t *testHandoverXBroker) setReady(broker *HandoverXBroker) {
	_, isReady := broker.isReady()
	if isReady {
		return
	}

	end := 2 + defaultHandoverXReadyEnd
	broker.readyEnd = end

	broker.successcount.SetValue(2 + end)
}

func (t *testHandoverXBroker) TestSendVoteproof() {
	args := t.xargs()
	args.SendFunc = func(context.Context, interface{}) error { return nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := NewHandoverXBroker(ctx, args)

	point := base.RawPoint(33, 44)

	t.Run("send voteproof", func() {
		avp, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(ctx, avp)
		t.NoError(err)
		t.False(isFinished)

		isFinished, err = broker.sendVoteproof(ctx, ivp)
		t.NoError(err)
		t.False(isFinished)
	})

	t.Run("send accept voteproof and finish", func() {
		point = point.NextHeight()

		t.setReady(broker)

		avp, _ := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(ctx, avp)
		t.NoError(err)
		t.False(isFinished)

		t.NoError(broker.isCanceled())
	})

	t.Run("send init voteproof and finish", func() {
		point = point.NextHeight()

		t.setReady(broker)

		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(ctx, ivp)
		t.NoError(err)
		t.True(isFinished)

		err = broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})

	t.Run("send init voteproof again", func() {
		point = point.NextHeight()

		t.setReady(broker)

		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		_, err := broker.sendVoteproof(ctx, ivp)
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})
}

func (t *testHandoverXBroker) TestReceiveStagePoint() {
	args := t.xargs()

	broker := NewHandoverXBroker(context.Background(), args)

	point := base.RawPoint(33, 44)

	t.Run("receive; no previous voteproof", func() {
		h := newHandoverMessageChallengeStagePoint(broker.ID(), base.NewStagePoint(point, base.StageINIT))
		t.Error(broker.receive(h))

		t.Equal(uint64(0), broker.successcount.MustValue())

		err := broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})

	broker = NewHandoverXBroker(context.Background(), args)

	t.Run("receive", func() {
		point = point.NextHeight()

		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		h := newHandoverMessageChallengeStagePoint(broker.ID(), ivp.Point())
		t.NoError(broker.receive(h))
		t.Equal(uint64(1), broker.successcount.MustValue())
	})

	t.Run("receive lower; ignored", func() {
		h := newHandoverMessageChallengeStagePoint(broker.ID(), base.NewStagePoint(point, base.StageINIT))
		t.NoError(broker.receive(h))

		t.Equal(uint64(1), broker.successcount.MustValue())
	})

	t.Run("receive next point", func() {
		args.MinChallengeCount = 2

		point = point.NextHeight()

		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		h := newHandoverMessageChallengeStagePoint(broker.ID(), ivp.Point())
		t.NoError(broker.receive(h))
		t.Equal(uint64(2), broker.successcount.MustValue())

		count, isReady := broker.isReady()
		t.Equal(broker.successcount.MustValue(), count)
		t.True(isReady)
	})

	t.Run("invalid; cancel", func() {
		h := newHandoverMessageChallengeStagePoint(broker.ID(), base.ZeroStagePoint)
		t.Error(broker.receive(h))

		err := broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})
}

func (t *testHandoverXBroker) TestReceiveBlockMap() {
	point := base.NewStagePoint(base.RawPoint(33, 44), base.StageINIT)

	t.Run("receive; wrong address", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		hm := newHandoverMessageChallengeBlockMap(broker.ID(), point, t.newBlockMap(point.Height(), base.RandomLocalNode(), nil))
		err := broker.receive(hm)
		t.Error(err)
		t.ErrorContains(err, "wrong address")

		t.Equal(uint64(0), broker.successcount.MustValue())

		err = broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})

	t.Run("receive; wrong publickey", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		local := base.NewBaseLocalNode(base.DummyNodeHint, base.NewMPrivatekey(), t.Local.Address())

		hm := newHandoverMessageChallengeBlockMap(broker.ID(), point, t.newBlockMap(point.Height(), local, nil))
		err := broker.receive(hm)
		t.Error(err)
		t.ErrorContains(err, "different key")

		t.Equal(uint64(0), broker.successcount.MustValue())

		err = broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})

	t.Run("receive without no previous voteproof", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		hm := newHandoverMessageChallengeBlockMap(broker.ID(), point, t.newBlockMap(point.Height(), t.Local, nil))
		t.Error(broker.receive(hm))

		t.Equal(uint64(0), broker.successcount.MustValue())

		err := broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})

	t.Run("receive; last not accept voteproof", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		hm := newHandoverMessageChallengeBlockMap(broker.ID(), point, t.newBlockMap(point.Height(), t.Local, nil))

		_, ivp := t.VoteproofsPair(point.PrevHeight(), point.Point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		t.NoError(broker.receive(hm))
		t.Equal(uint64(0), broker.successcount.MustValue())

		t.NoError(broker.isCanceled())
	})

	t.Run("receive; last not majority accept voteproof and stagepoint", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		avp, _ := t.VoteproofsPair(point.Point, point.NextHeight(), nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		avp.SetMajority(nil)

		hc := newHandoverMessageChallengeStagePoint(broker.ID(), avp.Point())

		isFinished, err := broker.sendVoteproof(context.Background(), avp)
		t.NoError(err)
		t.False(isFinished)

		t.NoError(broker.receive(hc))
		t.Equal(uint64(1), broker.successcount.MustValue())

		t.NoError(broker.isCanceled())
	})

	t.Run("receive; last not majority accept voteproof", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		bm := t.newBlockMap(point.Height(), t.Local, nil)
		hm := newHandoverMessageChallengeBlockMap(broker.ID(), point, bm)

		avp, _ := t.VoteproofsPair(point.Point, point.NextHeight(), bm.Manifest().Hash(), nil, nil, []base.LocalNode{base.RandomLocalNode()})
		avp.SetMajority(nil)

		isFinished, err := broker.sendVoteproof(context.Background(), avp)
		t.NoError(err)
		t.False(isFinished)

		t.NoError(broker.receive(hm))
		t.Equal(uint64(0), broker.successcount.MustValue())

		t.NoError(broker.isCanceled())
	})

	t.Run("receive; last accept voteproof, but different manifest hash", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		bm := t.newBlockMap(point.Height(), t.Local, nil)
		hm := newHandoverMessageChallengeBlockMap(broker.ID(), point, bm)

		avp, _ := t.VoteproofsPair(point.Point, point.NextHeight(), nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})

		isFinished, err := broker.sendVoteproof(context.Background(), avp)
		t.NoError(err)
		t.False(isFinished)

		t.NoError(broker.receive(hm))
		t.Equal(uint64(0), broker.successcount.MustValue())

		t.NoError(broker.isCanceled())
	})

	t.Run("receive", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		bm := t.newBlockMap(point.Height(), t.Local, nil)
		hm := newHandoverMessageChallengeBlockMap(broker.ID(), point, bm)

		avp, _ := t.VoteproofsPair(point.Point, point.NextHeight(), bm.Manifest().Hash(), nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), avp)
		t.NoError(err)
		t.False(isFinished)

		t.NoError(broker.receive(hm))
		t.Equal(uint64(1), broker.successcount.MustValue())

		t.Run("receive same", func() {
			t.NoError(broker.receive(hm))
			t.Equal(uint64(1), broker.successcount.MustValue())

			t.NoError(broker.isCanceled())
		})
	})

	t.Run("invalid; cancel", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		bm := t.newBlockMap(point.Height(), t.Local, nil)
		bm.M = nil

		hm := newHandoverMessageChallengeBlockMap(broker.ID(), point, bm)

		t.Error(broker.receive(hm))

		t.Equal(uint64(0), broker.successcount.MustValue())

		err := broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})
}

func (t *testHandoverXBroker) TestReceiveHandoverMessageReady() {
	point := base.RawPoint(33, 44)

	t.Run("wrong ID", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		h := newHandoverMessageReady(util.UUID().String(), base.NewStagePoint(point, base.StageINIT))
		t.Error(broker.receive(h))

		err := broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})

	t.Run("before no previous voteproof", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		h := newHandoverMessageReady(broker.ID(), base.NewStagePoint(point, base.StageINIT))
		t.Error(broker.receive(h))

		err := broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})

	t.Run("not higher previous HandoverMessageReady", func() {
		args := t.xargs()

		broker := NewHandoverXBroker(context.Background(), args)

		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		hc := newHandoverMessageChallengeStagePoint(broker.ID(), ivp.Point())
		t.NoError(broker.receive(hc))

		hr := newHandoverMessageReady(broker.ID(), base.NewStagePoint(point, base.StageINIT))
		t.NoError(broker.receive(hr))

		t.Equal(uint64(1), broker.successcount.MustValue())

		t.T().Log("send again")
		t.NoError(broker.receive(hr))
		t.Equal(uint64(1), broker.successcount.MustValue())
	})

	t.Run("not ready", func() {
		args := t.xargs()

		sendch := make(chan HandoverMessageReadyResponse, 1)
		args.SendFunc = func(_ context.Context, i interface{}) error {
			if h, ok := i.(HandoverMessageReadyResponse); ok {
				sendch <- h
			}

			return nil
		}

		broker := NewHandoverXBroker(context.Background(), args)
		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		hc := newHandoverMessageChallengeStagePoint(broker.ID(), ivp.Point())
		t.NoError(broker.receive(hc))
		t.Equal(uint64(1), broker.successcount.MustValue())

		count, isReady := broker.isReady()
		t.Equal(broker.successcount.MustValue(), count)
		t.False(isReady)

		h := newHandoverMessageReady(broker.ID(), base.NewStagePoint(point, base.StageINIT))
		t.NoError(broker.receive(h))

		select {
		case <-time.After(time.Second * 2):
			t.NoError(errors.Errorf("failed to wait HandoverMessageFinish"))
		case h := <-sendch:
			t.Equal(broker.ID(), h.HandoverID())
			t.False(h.OK())
			t.Nil(h.Err())

			t.NoError(broker.isCanceled())
		}
	})

	t.Run("ready", func() {
		args := t.xargs()
		args.MinChallengeCount = 1

		sendch := make(chan HandoverMessageReadyResponse, 1)
		args.SendFunc = func(_ context.Context, i interface{}) error {
			if h, ok := i.(HandoverMessageReadyResponse); ok {
				sendch <- h
			}

			return nil
		}

		broker := NewHandoverXBroker(context.Background(), args)
		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		hc := newHandoverMessageChallengeStagePoint(broker.ID(), ivp.Point())
		t.NoError(broker.receive(hc))
		t.Equal(uint64(1), broker.successcount.MustValue())

		count, isReady := broker.isReady()
		t.Equal(broker.successcount.MustValue(), count)
		t.True(isReady)

		h := newHandoverMessageReady(broker.ID(), base.NewStagePoint(point, base.StageINIT))
		t.NoError(broker.receive(h))

		select {
		case <-time.After(time.Second * 2):
			t.NoError(errors.Errorf("failed to wait HandoverMessageFinish"))
		case h := <-sendch:
			t.Equal(broker.ID(), h.HandoverID())
			t.True(h.OK())
			t.Nil(h.Err())

			t.NoError(broker.isCanceled())
		}
	})

	t.Run("ready, but check ready is not ok", func() {
		args := t.xargs()
		args.MinChallengeCount = 1
		args.CheckIsReady = func() (bool, error) {
			return false, nil
		}

		sendch := make(chan HandoverMessageReadyResponse, 1)
		args.SendFunc = func(_ context.Context, i interface{}) error {
			if h, ok := i.(HandoverMessageReadyResponse); ok {
				sendch <- h
			}

			return nil
		}

		broker := NewHandoverXBroker(context.Background(), args)
		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		hc := newHandoverMessageChallengeStagePoint(broker.ID(), ivp.Point())
		t.NoError(broker.receive(hc))
		t.Equal(uint64(1), broker.successcount.MustValue())

		count, isReady := broker.isReady()
		t.Equal(broker.successcount.MustValue(), count)
		t.True(isReady)

		h := newHandoverMessageReady(broker.ID(), base.NewStagePoint(point, base.StageINIT))
		t.NoError(broker.receive(h))

		select {
		case <-time.After(time.Second * 1):
			t.NoError(errors.Errorf("failed to wait HandoverMessageReadyResponse"))
		case rhc := <-sendch:
			t.False(rhc.OK())
			t.Nil(rhc.Err())
		}
	})

	t.Run("ready, but check ready error", func() {
		args := t.xargs()
		args.MinChallengeCount = 1
		args.CheckIsReady = func() (bool, error) {
			return false, errors.Errorf("hahaha")
		}

		sendch := make(chan HandoverMessageReadyResponse, 1)
		args.SendFunc = func(_ context.Context, i interface{}) error {
			if h, ok := i.(HandoverMessageReadyResponse); ok {
				sendch <- h
			}

			return nil
		}

		broker := NewHandoverXBroker(context.Background(), args)
		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		hc := newHandoverMessageChallengeStagePoint(broker.ID(), ivp.Point())
		t.NoError(broker.receive(hc))
		t.Equal(uint64(1), broker.successcount.MustValue())

		count, isReady := broker.isReady()
		t.Equal(broker.successcount.MustValue(), count)
		t.True(isReady)

		h := newHandoverMessageReady(broker.ID(), base.NewStagePoint(point, base.StageINIT))
		err = broker.receive(h)
		t.True(errors.Is(err, errHandoverCanceled))
		t.ErrorContains(err, "hahaha")

		select {
		case <-time.After(time.Second * 1):
			t.NoError(errors.Errorf("failed to wait HandoverMessageReadyResponse"))
		case rhc := <-sendch:
			t.False(rhc.OK())
			t.NotNil(rhc.Err())
			t.ErrorContains(rhc.Err(), "hahaha")
		}
	})
}

func (t *testHandoverXBroker) TestFinish() {
	t.Run("finish with nil voteproof", func() {
		args := t.xargs()

		sendch := make(chan HandoverMessageFinish, 1)
		args.SendFunc = func(_ context.Context, i interface{}) error {
			if h, ok := i.(HandoverMessageFinish); ok {
				sendch <- h
			}

			return nil
		}

		broker := NewHandoverXBroker(context.Background(), args)
		t.NoError(broker.finish(nil))

		select {
		case <-time.After(time.Second * 2):
			t.NoError(errors.Errorf("failed to wait HandoverMessageFinish"))
		case h := <-sendch:
			t.Equal(broker.ID(), h.HandoverID())
			t.Nil(h.INITVoteproof())

			err := broker.isCanceled()
			t.Error(err)
			t.True(errors.Is(err, errHandoverCanceled))
		}
	})

	t.Run("finish; but failed to send", func() {
		args := t.xargs()

		args.SendFunc = func(_ context.Context, i interface{}) error {
			return errors.Errorf("failed to send")
		}

		broker := NewHandoverXBroker(context.Background(), args)
		err := broker.finish(nil)
		t.Error(err)
		t.ErrorContains(err, "failed to send")
		t.True(errors.Is(err, errHandoverCanceled))

		err = broker.isCanceled()
		t.Error(err)
		t.True(errors.Is(err, errHandoverCanceled))
	})
}

func (t *testHandoverXBroker) TestReceiveSerialChallenge() {
	point := base.RawPoint(33, 44)

	t.Run("ok", func() {
		args := t.xargs()
		broker := NewHandoverXBroker(context.Background(), args)

		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		h := newHandoverMessageChallengeStagePoint(broker.ID(), ivp.Point())
		t.NoError(broker.receive(h))
		t.Equal(uint64(1), broker.successcount.MustValue())

		nextpoint := point.NextHeight()
		bm := t.newBlockMap(nextpoint.Height(), t.Local, nil)
		hm := newHandoverMessageChallengeBlockMap(broker.ID(), base.NewStagePoint(nextpoint, base.StageACCEPT), bm)
		avp, _ := t.VoteproofsPair(nextpoint, point, bm.Manifest().Hash(), nil, nil, []base.LocalNode{base.RandomLocalNode()})

		isFinished, err = broker.sendVoteproof(context.Background(), avp)
		t.NoError(err)
		t.False(isFinished)

		t.NoError(broker.receive(hm))
		t.Equal(uint64(2), broker.successcount.MustValue())
	})

	t.Run("not serial", func() {
		args := t.xargs()
		broker := NewHandoverXBroker(context.Background(), args)

		_, ivp := t.VoteproofsPair(point.PrevRound(), point, nil, nil, nil, []base.LocalNode{base.RandomLocalNode()})
		isFinished, err := broker.sendVoteproof(context.Background(), ivp)
		t.NoError(err)
		t.False(isFinished)

		h := newHandoverMessageChallengeStagePoint(broker.ID(), ivp.Point())
		t.NoError(broker.receive(h))
		t.Equal(uint64(1), broker.successcount.MustValue())

		nextpoint := point.NextHeight()

		nextavp, nextivp := t.VoteproofsPair(point, nextpoint, valuehash.RandomSHA256(), nil, nil, []base.LocalNode{base.RandomLocalNode()})

		isFinished, err = broker.sendVoteproof(context.Background(), nextavp)
		t.NoError(err)
		t.False(isFinished)

		t.T().Log("skip challenge for next accept voteproof")

		isFinished, err = broker.sendVoteproof(context.Background(), nextivp)
		t.NoError(err)
		t.False(isFinished)

		t.T().Log("success count resetted and will be 1")
		h = newHandoverMessageChallengeStagePoint(broker.ID(), nextivp.Point())
		t.NoError(broker.receive(h))
		t.Equal(uint64(1), broker.successcount.MustValue())
	})
}

func TestHandoverXBroker(t *testing.T) {
	suite.Run(t, new(testHandoverXBroker))
}