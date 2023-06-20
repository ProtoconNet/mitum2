package launch

import (
	"time"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	isaacnetwork "github.com/spikeekips/mitum/isaac/network"
	"github.com/spikeekips/mitum/network/quicmemberlist"
	"github.com/spikeekips/mitum/util"
)

var (
	defaultHandlerTimeouts  map[string]time.Duration
	networkHandlerPrefixMap = map[string]struct{}{}
)

func init() {
	defaultHandlerTimeouts = map[string]time.Duration{
		isaacnetwork.HandlerPrefixAskHandoverString:    0,
		isaacnetwork.HandlerPrefixCheckHandoverString:  0,
		isaacnetwork.HandlerPrefixCheckHandoverXString: 0,
		isaacnetwork.HandlerPrefixStartHandoverString:  0,
	}

	for i := range networkHandlerPrefixes {
		networkHandlerPrefixMap[networkHandlerPrefixes[i]] = struct{}{}
	}
}

type LocalParams struct {
	ISAAC      *isaac.Params     `yaml:"isaac,omitempty" json:"isaac,omitempty"`
	Memberlist *MemberlistParams `yaml:"memberlist,omitempty" json:"memberlist,omitempty"`
	MISC       *MISCParams       `yaml:"misc,omitempty" json:"misc,omitempty"`
	Network    *NetworkParams    `yaml:"network,omitempty" json:"network,omitempty"`
}

func defaultLocalParams(networkID base.NetworkID) *LocalParams {
	return &LocalParams{
		ISAAC:      isaac.DefaultParams(networkID),
		Memberlist: defaultMemberlistParams(),
		MISC:       defaultMISCParams(),
		Network:    defaultNetworkParams(),
	}
}

func (p *LocalParams) IsValid(networkID base.NetworkID) error {
	e := util.ErrInvalid.Errorf("invalid LocalParams")

	if p.ISAAC == nil {
		return e.Errorf("empty ISAAC")
	}

	_ = p.ISAAC.SetNetworkID(networkID)

	if err := util.CheckIsValiders(networkID, false,
		p.ISAAC,
		p.Memberlist,
		p.MISC,
		p.Network); err != nil {
		return e.Wrap(err)
	}

	return nil
}

type MemberlistParams struct {
	*util.BaseParams
	tcpTimeout              time.Duration
	retransmitMult          int
	probeTimeout            time.Duration
	probeInterval           time.Duration
	suspicionMult           int
	suspicionMaxTimeoutMult int
	udpBufferSize           int
	extraSameMemberLimit    uint64
}

func defaultMemberlistParams() *MemberlistParams {
	config := quicmemberlist.BasicMemberlistConfig()

	return &MemberlistParams{
		BaseParams:              util.NewBaseParams(),
		tcpTimeout:              config.TCPTimeout,
		retransmitMult:          config.RetransmitMult,
		probeTimeout:            config.ProbeTimeout,
		probeInterval:           config.ProbeInterval,
		suspicionMult:           config.SuspicionMult,
		suspicionMaxTimeoutMult: config.SuspicionMaxTimeoutMult,
		udpBufferSize:           config.UDPBufferSize,
		extraSameMemberLimit:    1, //nolint:gomnd //...
	}
}

func (*MemberlistParams) IsValid([]byte) error {
	return nil
}

func (p *MemberlistParams) TCPTimeout() time.Duration {
	return p.tcpTimeout
}

func (p *MemberlistParams) SetTCPTimeout(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.tcpTimeout == d {
			return false, nil
		}

		p.tcpTimeout = d

		return true, nil
	})
}

func (p *MemberlistParams) RetransmitMult() int {
	return p.retransmitMult
}

func (p *MemberlistParams) SetRetransmitMult(d int) error {
	return p.SetInt(d, func(d int) (bool, error) {
		if p.retransmitMult == d {
			return false, nil
		}

		p.retransmitMult = d

		return true, nil
	})
}

func (p *MemberlistParams) ProbeTimeout() time.Duration {
	return p.probeTimeout
}

func (p *MemberlistParams) SetProbeTimeout(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.probeTimeout == d {
			return false, nil
		}

		p.probeTimeout = d

		return true, nil
	})
}

func (p *MemberlistParams) ProbeInterval() time.Duration {
	return p.probeInterval
}

func (p *MemberlistParams) SetProbeInterval(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.probeInterval == d {
			return false, nil
		}

		p.probeInterval = d

		return true, nil
	})
}

func (p *MemberlistParams) SuspicionMult() int {
	return p.suspicionMult
}

func (p *MemberlistParams) SetSuspicionMult(d int) error {
	return p.SetInt(d, func(d int) (bool, error) {
		if p.suspicionMult == d {
			return false, nil
		}

		p.suspicionMult = d

		return true, nil
	})
}

func (p *MemberlistParams) SuspicionMaxTimeoutMult() int {
	return p.suspicionMaxTimeoutMult
}

func (p *MemberlistParams) SetSuspicionMaxTimeoutMult(d int) error {
	return p.SetInt(d, func(d int) (bool, error) {
		if p.suspicionMaxTimeoutMult == d {
			return false, nil
		}

		p.suspicionMaxTimeoutMult = d

		return true, nil
	})
}

func (p *MemberlistParams) UDPBufferSize() int {
	return p.udpBufferSize
}

func (p *MemberlistParams) SetUDPBufferSize(d int) error {
	return p.SetInt(d, func(d int) (bool, error) {
		if p.udpBufferSize == d {
			return false, nil
		}

		p.udpBufferSize = d

		return true, nil
	})
}

func (p *MemberlistParams) ExtraSameMemberLimit() uint64 {
	return p.extraSameMemberLimit
}

func (p *MemberlistParams) SetExtraSameMemberLimit(d uint64) error {
	return p.SetUint64(d, func(d uint64) (bool, error) {
		if p.extraSameMemberLimit == d {
			return false, nil
		}

		p.extraSameMemberLimit = d

		return true, nil
	})
}

type MISCParams struct {
	*util.BaseParams
	syncSourceCheckerInterval             time.Duration
	validProposalOperationExpire          time.Duration
	validProposalSuffrageOperationsExpire time.Duration
	maxMessageSize                        uint64
	objectCacheSize                       uint64
}

func defaultMISCParams() *MISCParams {
	return &MISCParams{
		BaseParams:                            util.NewBaseParams(),
		syncSourceCheckerInterval:             time.Second * 30, //nolint:gomnd //...
		validProposalOperationExpire:          time.Hour * 24,   //nolint:gomnd //...
		validProposalSuffrageOperationsExpire: time.Hour * 2,
		maxMessageSize:                        1 << 18, //nolint:gomnd //...
		objectCacheSize:                       1 << 13, //nolint:gomnd // big enough
	}
}

func (p *MISCParams) IsValid([]byte) error {
	e := util.ErrInvalid.Errorf("invalid MISCParams")

	if err := p.BaseParams.IsValid(nil); err != nil {
		return e.Wrap(err)
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

	if p.maxMessageSize < 1 {
		return e.Errorf("wrong maxMessageSize")
	}

	if p.objectCacheSize < 1 {
		return e.Errorf("wrong objectCacheSize")
	}

	return nil
}

func (p *MISCParams) SyncSourceCheckerInterval() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.syncSourceCheckerInterval
}

func (p *MISCParams) SetSyncSourceCheckerInterval(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.syncSourceCheckerInterval == d {
			return false, nil
		}

		p.syncSourceCheckerInterval = d

		return true, nil
	})
}

func (p *MISCParams) ValidProposalOperationExpire() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.validProposalOperationExpire
}

func (p *MISCParams) SetValidProposalOperationExpire(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.validProposalOperationExpire == d {
			return false, nil
		}

		p.validProposalOperationExpire = d

		return true, nil
	})
}

func (p *MISCParams) ValidProposalSuffrageOperationsExpire() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.validProposalSuffrageOperationsExpire
}

func (p *MISCParams) SetValidProposalSuffrageOperationsExpire(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.validProposalSuffrageOperationsExpire == d {
			return false, nil
		}

		p.validProposalSuffrageOperationsExpire = d

		return true, nil
	})
}

func (p *MISCParams) MaxMessageSize() uint64 {
	p.RLock()
	defer p.RUnlock()

	return p.maxMessageSize
}

func (p *MISCParams) SetMaxMessageSize(d uint64) error {
	return p.SetUint64(d, func(d uint64) (bool, error) {
		if p.maxMessageSize == d {
			return false, nil
		}

		p.maxMessageSize = d

		return true, nil
	})
}

func (p *MISCParams) ObjectCacheSize() uint64 {
	p.RLock()
	defer p.RUnlock()

	return p.objectCacheSize
}

func (p *MISCParams) SetObjectCacheSize(d uint64) error {
	return p.SetUint64(d, func(d uint64) (bool, error) {
		if p.objectCacheSize == d {
			return false, nil
		}

		p.objectCacheSize = d

		return true, nil
	})
}

type NetworkParams struct {
	*util.BaseParams
	handlerTimeouts       map[string]time.Duration
	timeoutRequest        time.Duration
	handshakeIdleTimeout  time.Duration
	maxIdleTimeout        time.Duration
	keepAlivePeriod       time.Duration
	defaultHandlerTimeout time.Duration
}

func defaultNetworkParams() *NetworkParams {
	handlerTimeouts := map[string]time.Duration{}
	for i := range defaultHandlerTimeouts {
		handlerTimeouts[i] = defaultHandlerTimeouts[i]
	}

	d := DefaultQuicConfig()

	return &NetworkParams{
		BaseParams:            util.NewBaseParams(),
		timeoutRequest:        isaac.DefaultTimeoutRequest,
		handshakeIdleTimeout:  d.HandshakeIdleTimeout,
		maxIdleTimeout:        d.MaxIdleTimeout,
		keepAlivePeriod:       d.KeepAlivePeriod,
		defaultHandlerTimeout: time.Second * 6, //nolint:gomnd //...
		handlerTimeouts:       handlerTimeouts,
	}
}

func (p *NetworkParams) IsValid([]byte) error {
	e := util.ErrInvalid.Errorf("invalid NetworkParams")

	if err := p.BaseParams.IsValid(nil); err != nil {
		return e.Wrap(err)
	}

	if p.timeoutRequest < 0 {
		return e.Errorf("wrong duration; invalid timeoutRequest")
	}

	if p.handshakeIdleTimeout < 0 {
		return e.Errorf("wrong duration; invalid handshakeIdleTimeout")
	}

	if p.maxIdleTimeout < 0 {
		return e.Errorf("wrong duration; invalid maxIdleTimeout")
	}

	if p.keepAlivePeriod < 0 {
		return e.Errorf("wrong duration; invalid keepAlivePeriod")
	}

	if p.defaultHandlerTimeout < 0 {
		return e.Errorf("wrong duration; invalid defaultHandlerTimeout")
	}

	for i := range p.handlerTimeouts {
		if _, found := networkHandlerPrefixMap[i]; !found {
			return e.Errorf("unknown handler timeout, %q", i)
		}

		if p.handlerTimeouts[i] < 0 {
			return e.Errorf("wrong duration; invalid %q", i)
		}
	}

	return nil
}

func (p *NetworkParams) TimeoutRequest() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.timeoutRequest
}

func (p *NetworkParams) SetTimeoutRequest(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.timeoutRequest == d {
			return false, nil
		}

		p.timeoutRequest = d

		return true, nil
	})
}

func (p *NetworkParams) HandshakeIdleTimeout() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.handshakeIdleTimeout
}

func (p *NetworkParams) SetHandshakeIdleTimeout(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.handshakeIdleTimeout == d {
			return false, nil
		}

		p.handshakeIdleTimeout = d

		return true, nil
	})
}

func (p *NetworkParams) MaxIdleTimeout() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.maxIdleTimeout
}

func (p *NetworkParams) SetMaxIdleTimeout(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.maxIdleTimeout == d {
			return false, nil
		}

		p.maxIdleTimeout = d

		return true, nil
	})
}

func (p *NetworkParams) KeepAlivePeriod() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.keepAlivePeriod
}

func (p *NetworkParams) SetKeepAlivePeriod(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.keepAlivePeriod == d {
			return false, nil
		}

		p.keepAlivePeriod = d

		return true, nil
	})
}

func (p *NetworkParams) DefaultHandlerTimeout() time.Duration {
	p.RLock()
	defer p.RUnlock()

	return p.defaultHandlerTimeout
}

func (p *NetworkParams) SetDefaultHandlerTimeout(d time.Duration) error {
	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		if p.defaultHandlerTimeout == d {
			return false, nil
		}

		p.defaultHandlerTimeout = d

		return true, nil
	})
}

func (p *NetworkParams) HandlerTimeout(i string) (time.Duration, error) {
	if _, found := networkHandlerPrefixMap[i]; !found {
		return 0, util.ErrNotFound.Errorf("unknown handler timeout, %q", i)
	}

	return p.handlerTimeout(i), nil
}

func (p *NetworkParams) SetHandlerTimeout(i string, d time.Duration) error {
	if _, found := networkHandlerPrefixMap[i]; !found {
		return util.ErrNotFound.Errorf("unknown handler timeout, %q", i)
	}

	return p.SetDuration(d, func(d time.Duration) (bool, error) {
		switch prev, found := p.handlerTimeouts[i]; {
		case found && prev == d:
			return false, nil
		case found && p.defaultHandlerTimeout == d:
			delete(p.handlerTimeouts, i)

			return false, nil
		}

		p.handlerTimeouts[i] = d

		return true, nil
	})
}

func (p *NetworkParams) HandlerTimeoutFunc(i string) (func() time.Duration, error) {
	if _, found := networkHandlerPrefixMap[i]; !found {
		return nil, util.ErrNotFound.Errorf("unknown handler timeout, %q", i)
	}

	return func() time.Duration {
		return p.handlerTimeout(i)
	}, nil
}

func (p *NetworkParams) handlerTimeout(i string) time.Duration {
	p.RLock()
	defer p.RUnlock()

	switch d, found := p.handlerTimeouts[i]; {
	case !found:
		return p.defaultHandlerTimeout
	default:
		return d
	}
}

var networkHandlerPrefixes = []string{
	isaacnetwork.HandlerPrefixAskHandoverString,
	isaacnetwork.HandlerPrefixBlockMapString,
	isaacnetwork.HandlerPrefixBlockMapItemString,
	isaacnetwork.HandlerPrefixCancelHandoverString,
	isaacnetwork.HandlerPrefixCheckHandoverString,
	isaacnetwork.HandlerPrefixCheckHandoverXString,
	isaacnetwork.HandlerPrefixExistsInStateOperationString,
	isaacnetwork.HandlerPrefixHandoverMessageString,
	isaacnetwork.HandlerPrefixLastBlockMapString,
	isaacnetwork.HandlerPrefixLastHandoverYLogsString,
	isaacnetwork.HandlerPrefixLastSuffrageProofString,
	isaacnetwork.HandlerPrefixMemberlistString,
	isaacnetwork.HandlerPrefixNodeChallengeString,
	isaacnetwork.HandlerPrefixNodeInfoString,
	isaacnetwork.HandlerPrefixOperationString,
	isaacnetwork.HandlerPrefixProposalString,
	isaacnetwork.HandlerPrefixRequestProposalString,
	isaacnetwork.HandlerPrefixSendBallotsString,
	isaacnetwork.HandlerPrefixSendOperationString,
	isaacnetwork.HandlerPrefixSetAllowConsensusString,
	isaacnetwork.HandlerPrefixStartHandoverString,
	isaacnetwork.HandlerPrefixStateString,
	isaacnetwork.HandlerPrefixStreamOperationsString,
	isaacnetwork.HandlerPrefixSuffrageNodeConnInfoString,
	isaacnetwork.HandlerPrefixSuffrageProofString,
	isaacnetwork.HandlerPrefixSyncSourceConnInfoString,
	HandlerPrefixMemberlistCallbackBroadcastMessageString,
	HandlerPrefixMemberlistEnsureBroadcastMessageString,
}