package isaacstates

import (
	"sync"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/pkg/errors"
)

type BallotBroadcaster interface {
	Broadcast(base.Ballot) error
	Ballot(_ base.Point, _ base.Stage, isSuffrageConfirm bool) (base.Ballot, bool, error)
}

type DefaultBallotBroadcaster struct {
	local         base.Address
	pool          isaac.BallotPool
	broadcastFunc func(base.Ballot) error
	sync.Mutex
}

func NewDefaultBallotBroadcaster(
	local base.Address,
	pool isaac.BallotPool,
	broadcastFunc func(base.Ballot) error,
) *DefaultBallotBroadcaster {
	return &DefaultBallotBroadcaster{local: local, pool: pool, broadcastFunc: broadcastFunc}
}

func (bb *DefaultBallotBroadcaster) Ballot(
	point base.Point,
	stage base.Stage,
	isSuffrageConfirm bool,
) (base.Ballot, bool, error) {
	return bb.pool.Ballot(point, stage, isSuffrageConfirm)
}

func (bb *DefaultBallotBroadcaster) Broadcast(bl base.Ballot) error {
	if err := bb.set(bl); err != nil {
		return errors.WithMessage(err, "failed to broadcast ballot")
	}

	return bb.broadcastFunc(bl)
}

func (bb *DefaultBallotBroadcaster) set(bl base.Ballot) error {
	bb.Lock()
	defer bb.Unlock()

	if !bl.SignFact().Node().Equal(bb.local) {
		return nil
	}

	if _, err := bb.pool.SetBallot(bl); err != nil {
		return errors.WithMessage(err, "failed to set ballot to pool")
	}

	return nil
}
