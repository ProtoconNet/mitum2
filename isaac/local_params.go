package isaac

import (
	"reflect"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/hint"
)

var LocalParamsHint = hint.MustNewHint("local-params-v0.0.1")

var DefaultTimeoutRequest = time.Second * 3 //nolint:gomnd //...

type LocalParams struct {
	id        string
	networkID base.NetworkID
	hint.BaseHinter
	intervalBroadcastBallot               time.Duration
	threshold                             base.Threshold
	waitPreparingINITBallot               time.Duration
	timeoutRequest                        time.Duration
	syncSourceCheckerInterval             time.Duration
	validProposalOperationExpire          time.Duration
	validProposalSuffrageOperationsExpire time.Duration
	maxMessageSize                        uint64
	sameMemberLimit                       uint64
	ballotStuckWait                       time.Duration
	ballotStuckResolveAfter               time.Duration
	sync.RWMutex
}

func NewLocalParams(networkID base.NetworkID) *LocalParams {
	return &LocalParams{
		id:         util.UUID().String(),
		BaseHinter: hint.NewBaseHinter(LocalParamsHint),
		networkID:  networkID,
	}
}

func DefaultLocalParams(networkID base.NetworkID) *LocalParams {
	return &LocalParams{
		id:                                    util.UUID().String(),
		BaseHinter:                            hint.NewBaseHinter(LocalParamsHint),
		networkID:                             networkID,
		threshold:                             base.DefaultThreshold,
		intervalBroadcastBallot:               time.Second * 3, //nolint:gomnd //...
		waitPreparingINITBallot:               time.Second * 5, //nolint:gomnd //...
		timeoutRequest:                        DefaultTimeoutRequest,
		syncSourceCheckerInterval:             time.Second * 30, //nolint:gomnd //...
		validProposalOperationExpire:          time.Hour * 24,   //nolint:gomnd //...
		validProposalSuffrageOperationsExpire: time.Hour * 2,
		ballotStuckWait:                       time.Second * 33, //nolint:gomnd // waitPreparingINITBallot * 10
		ballotStuckResolveAfter:               time.Second * 66, //nolint:gomnd // ballotStuckWait * 2
		maxMessageSize:                        1 << 18,          //nolint:gomnd //...
		sameMemberLimit:                       3,                //nolint:gomnd //...
	}
}

func (p *LocalParams) IsValid(networkID []byte) error {
	e := util.ErrInvalid.Errorf("invalid LocalParams")

	if len(p.id) < 1 {
		return e.Errorf("empty id")
	}

	if err := p.BaseHinter.IsValid(LocalParamsHint.Type().Bytes()); err != nil {
		return e.Wrap(err)
	}

	if !p.networkID.Equal(networkID) {
		return e.Errorf("network id does not match")
	}

	if err := util.CheckIsValiders(networkID, false, p.networkID, p.threshold); err != nil {
		return e.Wrap(err)
	}

	if p.intervalBroadcastBallot < 0 {
		return e.Errorf("wrong duration; invalid intervalBroadcastBallot")
	}

	if p.waitPreparingINITBallot < 0 {
		return e.Errorf("wrong duration; invalid waitPreparingINITBallot")
	}

	if p.timeoutRequest < 0 {
		return e.Errorf("wrong duration; invalid timeoutRequest")
	}

	if p.syncSourceCheckerInterval < 0 {
		return e.Errorf("wrong duration; invalid syncSourceCheckerInterval")
	}

	if p.validProposalOperationExpire < 0 {
		return e.Errorf("wrong duration; invalid validProposalOperationExpire")
	}

	if p.validProposalSuffrageOperationsExpire < 0 {
		return e.Errorf("wrong duration; invalid validProposalSuffrageOperationsExpire")
	}

	if p.ballotStuckWait < 0 {
		return e.Errorf("wrong duration; invalid ballotStuckWait")
	}

	if p.ballotStuckResolveAfter < 0 {
		return e.Errorf("wrong duration; invalid ballotStuckResolveAfter")
	}

	if p.maxMessageSize < 1 {
		return e.Errorf("wrong maxMessageSize")
	}

	return nil
}

func (p *LocalParams) ID() string {
	p.RLock()
	defer p.RUnlock()

	return p.id
}

func (p *LocalParams) NetworkID() base.NetworkID {
	p.RLock()
	defer p.RUnlock()

	return p.networkID
}

func (p *LocalParams) SetNetworkID(n base.NetworkID) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if n == nil {
		return p
	}

	p.networkID = n

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) Threshold() base.Threshold {
	p.RLock()
	defer p.RUnlock()

	return p.threshold
}

func (p *LocalParams) SetThreshold(t base.Threshold) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.threshold == t {
		return p
	}

	p.threshold = t

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) IntervalBroadcastBallot() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.intervalBroadcastBallot
}

func (p *LocalParams) SetIntervalBroadcastBallot(d time.Duration) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.intervalBroadcastBallot == d {
		return p
	}

	p.intervalBroadcastBallot = d

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) WaitPreparingINITBallot() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.waitPreparingINITBallot
}

func (p *LocalParams) SetWaitPreparingINITBallot(d time.Duration) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.waitPreparingINITBallot == d {
		return p
	}

	p.waitPreparingINITBallot = d

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) TimeoutRequest() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.timeoutRequest
}

func (p *LocalParams) SetTimeoutRequest(d time.Duration) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.timeoutRequest == d {
		return p
	}

	p.timeoutRequest = d

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) SyncSourceCheckerInterval() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.syncSourceCheckerInterval
}

func (p *LocalParams) SetSyncSourceCheckerInterval(d time.Duration) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.syncSourceCheckerInterval == d {
		return p
	}

	p.syncSourceCheckerInterval = d

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) ValidProposalOperationExpire() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.validProposalOperationExpire
}

func (p *LocalParams) SetValidProposalOperationExpire(d time.Duration) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.validProposalOperationExpire == d {
		return p
	}

	p.validProposalOperationExpire = d

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) ValidProposalSuffrageOperationsExpire() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.validProposalSuffrageOperationsExpire
}

func (p *LocalParams) SetValidProposalSuffrageOperationsExpire(d time.Duration) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.validProposalSuffrageOperationsExpire == d {
		return p
	}

	p.validProposalSuffrageOperationsExpire = d

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) MaxMessageSize() uint64 {
	p.RLock()
	defer p.RUnlock()

	return p.maxMessageSize
}

func (p *LocalParams) SetMaxMessageSize(s uint64) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.maxMessageSize == s {
		return p
	}

	p.maxMessageSize = s

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) SameMemberLimit() uint64 {
	p.RLock()
	defer p.RUnlock()

	return p.sameMemberLimit
}

func (p *LocalParams) SetSameMemberLimit(s uint64) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.sameMemberLimit == s {
		return p
	}

	p.sameMemberLimit = s

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) BallotStuckWait() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.ballotStuckWait
}

func (p *LocalParams) SetBallotStuckWait(d time.Duration) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.ballotStuckWait == d {
		return p
	}

	p.ballotStuckWait = d

	p.id = util.UUID().String()

	return p
}

func (p *LocalParams) BallotStuckResolveAfter() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.ballotStuckResolveAfter
}

func (p *LocalParams) SetBallotStuckResolveAfter(d time.Duration) *LocalParams {
	p.Lock()
	defer p.Unlock()

	if p.ballotStuckResolveAfter == d {
		return p
	}

	p.ballotStuckResolveAfter = d

	p.id = util.UUID().String()

	return p
}

type localParamsJSONMarshaler struct {
	//revive:disable:line-length-limit
	hint.BaseHinter
	Threshold                             base.Threshold            `json:"threshold,omitempty"`
	IntervalBroadcastBallot               util.ReadableJSONDuration `json:"interval_broadcast_ballot,omitempty"`
	WaitPreparingINITBallot               util.ReadableJSONDuration `json:"wait_preparing_init_ballot,omitempty"`
	TimeoutRequest                        util.ReadableJSONDuration `json:"timeout_request,omitempty"`
	SyncSourceCheckerInterval             util.ReadableJSONDuration `json:"sync_source_checker_interval,omitempty"`
	ValidProposalOperationExpire          util.ReadableJSONDuration `json:"valid_proposal_operation_expire,omitempty"`
	ValidProposalSuffrageOperationsExpire util.ReadableJSONDuration `json:"valid_proposal_suffrage_operations_expire,omitempty"`
	BallotStuckWait                       util.ReadableJSONDuration `json:"ballot_stuck_wait,omitempty"`
	BallotStuckResolveAfter               util.ReadableJSONDuration `json:"ballot_stuck_resolve_after,omitempty"`
	MaxMessageSize                        uint64                    `json:"max_message_size,omitempty"`
	SameMemberLimit                       uint64                    `json:"same_member_limit,omitempty"`
	//revive:enable:line-length-limit
}

func (p *LocalParams) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(localParamsJSONMarshaler{
		BaseHinter:                            p.BaseHinter,
		Threshold:                             p.threshold,
		IntervalBroadcastBallot:               util.ReadableJSONDuration(p.intervalBroadcastBallot),
		WaitPreparingINITBallot:               util.ReadableJSONDuration(p.waitPreparingINITBallot),
		TimeoutRequest:                        util.ReadableJSONDuration(p.timeoutRequest),
		SyncSourceCheckerInterval:             util.ReadableJSONDuration(p.syncSourceCheckerInterval),
		ValidProposalOperationExpire:          util.ReadableJSONDuration(p.validProposalOperationExpire),
		ValidProposalSuffrageOperationsExpire: util.ReadableJSONDuration(p.validProposalSuffrageOperationsExpire),
		MaxMessageSize:                        p.maxMessageSize,
		SameMemberLimit:                       p.sameMemberLimit,
		BallotStuckResolveAfter:               util.ReadableJSONDuration(p.ballotStuckResolveAfter),
		BallotStuckWait:                       util.ReadableJSONDuration(p.ballotStuckWait),
	})
}

type localParamsJSONUnmarshaler struct {
	Threshold                             interface{}                `json:"threshold"`
	IntervalBroadcastBallot               *util.ReadableJSONDuration `json:"interval_broadcast_ballot"`
	WaitPreparingINITBallot               *util.ReadableJSONDuration `json:"wait_preparing_init_ballot"`
	TimeoutRequest                        *util.ReadableJSONDuration `json:"timeout_request"`
	SyncSourceCheckerInterval             *util.ReadableJSONDuration `json:"sync_source_checker_interval"`
	ValidProposalOperationExpire          *util.ReadableJSONDuration `json:"valid_proposal_operation_expire"`
	ValidProposalSuffrageOperationsExpire *util.ReadableJSONDuration `json:"valid_proposal_suffrage_operations_expire"`
	BallotStuckWait                       *util.ReadableJSONDuration `json:"ballot_stuck_wait,omitempty"`
	BallotStuckResolveAfter               *util.ReadableJSONDuration `json:"ballot_stuck_resolve_after,omitempty"`
	MaxMessageSize                        *uint64                    `json:"max_message_size"`
	SameMemberLimit                       *uint64                    `json:"same_member_limit"`
	hint.BaseHinter
}

func (p *LocalParams) UnmarshalJSON(b []byte) error {
	var u localParamsJSONUnmarshaler
	if err := util.UnmarshalJSON(b, &u); err != nil {
		return errors.Wrap(err, "unmarshal LocalParams")
	}

	p.BaseHinter = u.BaseHinter

	args := [][2]interface{}{
		{u.MaxMessageSize, &p.maxMessageSize},
		{u.SameMemberLimit, &p.sameMemberLimit},
	}

	for i := range args {
		if reflect.ValueOf(args[i][0]).IsZero() {
			continue
		}

		if err := util.InterfaceSetValue(reflect.ValueOf(args[i][0]).Elem().Interface(), args[i][1]); err != nil {
			return err
		}
	}

	durargs := [][2]interface{}{
		{u.IntervalBroadcastBallot, &p.intervalBroadcastBallot},
		{u.WaitPreparingINITBallot, &p.waitPreparingINITBallot},
		{u.TimeoutRequest, &p.timeoutRequest},
		{u.SyncSourceCheckerInterval, &p.syncSourceCheckerInterval},
		{u.ValidProposalOperationExpire, &p.validProposalOperationExpire},
		{u.ValidProposalSuffrageOperationsExpire, &p.validProposalSuffrageOperationsExpire},
		{u.BallotStuckResolveAfter, &p.ballotStuckResolveAfter},
		{u.BallotStuckWait, &p.ballotStuckWait},
	}

	for i := range durargs {
		v := durargs[i][0].(*util.ReadableJSONDuration) //nolint:forcetypeassert //...

		if reflect.ValueOf(v).IsZero() {
			continue
		}

		if err := util.InterfaceSetValue(time.Duration(*v), durargs[i][1]); err != nil {
			return err
		}
	}

	switch t := u.Threshold.(type) {
	case string:
		if err := p.threshold.UnmarshalText([]byte(t)); err != nil {
			return err
		}
	case float64:
		p.threshold = base.Threshold(t)
	case int64:
		p.threshold = base.Threshold(float64(t))
	}

	p.id = util.UUID().String()

	return nil
}
