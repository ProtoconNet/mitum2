package launch

import (
	"reflect"
	"time"

	"github.com/rs/zerolog"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/util"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/spikeekips/mitum/util/hint"
	"gopkg.in/yaml.v3"
)

func (p *LocalParams) MarshalYAML() (interface{}, error) {
	m := map[string]interface{}{}

	if p.ISAAC != nil {
		switch b, err := util.MarshalJSON(p.ISAAC); {
		case err != nil:
			return nil, err
		default:
			var i map[string]interface{}

			if err := util.UnmarshalJSON(b, &i); err != nil {
				return nil, err
			}

			delete(i, "_hint")

			m["isaac"] = i
		}
	}

	if p.Memberlist != nil {
		m["memberlist"] = p.Memberlist
	}

	if p.MISC != nil {
		m["misc"] = p.MISC
	}

	if p.Network != nil {
		m["network"] = p.Network
	}

	return m, nil
}

type LocalParamsYAMLUnmarshaler struct {
	ISAAC      map[string]interface{} `yaml:"isaac"`
	Memberlist *MemberlistParams      `yaml:"memberlist,omitempty"`
	MISC       *MISCParams            `yaml:"misc,omitempty"`
	Network    *NetworkParams         `yaml:"network,omitempty"`
}

func (p *LocalParams) DecodeYAML(b []byte, enc *jsonenc.Encoder) error {
	if len(b) < 1 {
		return nil
	}

	e := util.StringError("decode LocalParams")

	u := LocalParamsYAMLUnmarshaler{Memberlist: p.Memberlist}

	if err := yaml.Unmarshal(b, &u); err != nil {
		return e.Wrap(err)
	}

	switch lb, err := enc.Marshal(u.ISAAC); {
	case err != nil:
		return e.Wrap(err)
	default:
		if err := enc.Unmarshal(lb, p.ISAAC); err != nil {
			return e.Wrap(err)
		}

		p.ISAAC.BaseHinter = hint.NewBaseHinter(isaac.ParamsHint)
	}

	if u.Memberlist != nil {
		p.Memberlist = u.Memberlist
	}

	if u.MISC != nil {
		p.MISC = u.MISC
	}

	if u.Network != nil {
		p.Network = u.Network
	}

	return nil
}

func (p *LocalParams) MarshalZerologObject(e *zerolog.Event) {
	e.
		Interface("isaac", p.ISAAC).
		Interface("memberlist", p.Memberlist).
		Interface("misc", p.MISC).
		Interface("network", p.Network)
}

type memberlistParamsMarshaler struct {
	//revive:disable:line-length-limit
	TCPTimeout              util.ReadableDuration `json:"tcp_timeout,omitempty" yaml:"tcp_timeout,omitempty"`
	RetransmitMult          int                   `json:"retransmit_mult,omitempty" yaml:"retransmit_mult,omitempty"`
	ProbeTimeout            util.ReadableDuration `json:"probe_timeout,omitempty" yaml:"probe_timeout,omitempty"`
	ProbeInterval           util.ReadableDuration `json:"probe_interval,omitempty" yaml:"probe_interval,omitempty"`
	SuspicionMult           int                   `json:"suspicion_mult,omitempty" yaml:"suspicion_mult,omitempty"`
	SuspicionMaxTimeoutMult int                   `json:"suspicion_max_timeout_mult,omitempty" yaml:"suspicion_max_timeout_mult,omitempty"`
	UDPBufferSize           int                   `json:"udp_buffer_size,omitempty" yaml:"udp_buffer_size,omitempty"`
	ExtraSameMemberLimit    uint64                `json:"extra_same_member_limit,omitempty" yaml:"extra_same_member_limit,omitempty"`
	//revive:enable:line-length-limit
}

func (p *MemberlistParams) marshaler() memberlistParamsMarshaler {
	return memberlistParamsMarshaler{
		TCPTimeout:              util.ReadableDuration(p.tcpTimeout),
		RetransmitMult:          p.retransmitMult,
		ProbeTimeout:            util.ReadableDuration(p.probeTimeout),
		ProbeInterval:           util.ReadableDuration(p.probeInterval),
		SuspicionMult:           p.suspicionMult,
		SuspicionMaxTimeoutMult: p.suspicionMaxTimeoutMult,
		UDPBufferSize:           p.udpBufferSize,
		ExtraSameMemberLimit:    p.extraSameMemberLimit,
	}
}

func (p *MemberlistParams) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(p.marshaler())
}

func (p *MemberlistParams) MarshalYAML() (interface{}, error) {
	return p.marshaler(), nil
}

type memberlistParamsUnmarshaler struct {
	//revive:disable:line-length-limit
	TCPTimeout              *util.ReadableDuration `json:"tcp_timeout,omitempty" yaml:"tcp_timeout,omitempty"`
	RetransmitMult          *int                   `json:"retransmit_mult,omitempty" yaml:"retransmit_mult,omitempty"`
	ProbeTimeout            *util.ReadableDuration `json:"probe_timeout,omitempty" yaml:"probe_timeout,omitempty"`
	ProbeInterval           *util.ReadableDuration `json:"probe_interval,omitempty" yaml:"probe_interval,omitempty"`
	SuspicionMult           *int                   `json:"suspicion_mult,omitempty" yaml:"suspicion_mult,omitempty"`
	SuspicionMaxTimeoutMult *int                   `json:"suspicion_max_timeout_mult,omitempty" yaml:"suspicion_max_timeout_mult,omitempty"`
	UDPBufferSize           *int                   `json:"udp_buffer_size,omitempty" yaml:"udp_buffer_size,omitempty"`
	ExtraSameMemberLimit    *uint64                `json:"extra_same_member_limit,omitempty" yaml:"extra_same_member_limit,omitempty"`
	//revive:enable:line-length-limit
}

func (p *MemberlistParams) UnmarshalJSON(b []byte) error {
	d := defaultMemberlistParams()
	*p = *d

	e := util.StringError("unmarshal MemberlistParams")

	var u memberlistParamsUnmarshaler

	if err := util.UnmarshalJSON(b, &u); err != nil {
		return e.Wrap(err)
	}

	return e.Wrap(p.unmarshal(u))
}

func (p *MemberlistParams) UnmarshalYAML(unmarshal func(interface{}) error) error {
	d := defaultMemberlistParams()
	*p = *d

	e := util.StringError("unmarshal MemberlistParams")

	var u memberlistParamsUnmarshaler

	if err := unmarshal(&u); err != nil {
		return e.Wrap(err)
	}

	return e.Wrap(p.unmarshal(u))
}

func (p *MemberlistParams) unmarshal(u memberlistParamsUnmarshaler) error {
	if u.RetransmitMult != nil {
		p.retransmitMult = *u.RetransmitMult
	}

	if u.SuspicionMult != nil {
		p.suspicionMult = *u.SuspicionMult
	}

	if u.SuspicionMaxTimeoutMult != nil {
		p.suspicionMaxTimeoutMult = *u.SuspicionMaxTimeoutMult
	}

	if u.UDPBufferSize != nil {
		p.udpBufferSize = *u.UDPBufferSize
	}

	if u.ExtraSameMemberLimit != nil {
		p.extraSameMemberLimit = *u.ExtraSameMemberLimit
	}

	durargs := [][2]interface{}{
		{u.TCPTimeout, &p.tcpTimeout},
		{u.ProbeTimeout, &p.probeTimeout},
		{u.ProbeInterval, &p.probeInterval},
	}

	for i := range durargs {
		v := durargs[i][0].(*util.ReadableDuration) //nolint:forcetypeassert //...

		if reflect.ValueOf(v).IsZero() {
			continue
		}

		if err := util.InterfaceSetValue(time.Duration(*v), durargs[i][1]); err != nil {
			return err
		}
	}

	return nil
}

type miscParamsYAMLMarshaler struct {
	//revive:disable:line-length-limit
	SyncSourceCheckerInterval             util.ReadableDuration `json:"sync_source_checker_interval,omitempty" yaml:"sync_source_checker_interval,omitempty"`
	ValidProposalOperationExpire          util.ReadableDuration `json:"valid_proposal_operation_expire,omitempty" yaml:"valid_proposal_operation_expire,omitempty"`
	ValidProposalSuffrageOperationsExpire util.ReadableDuration `json:"valid_proposal_suffrage_operations_expire,omitempty" yaml:"valid_proposal_suffrage_operations_expire,omitempty"`
	MaxMessageSize                        uint64                `json:"max_message_size,omitempty" yaml:"max_message_size,omitempty"`
	ObjectCacheSize                       uint64                `json:"object_cache_size,omitempty" yaml:"object_cache_size,omitempty"`
	//revive:enable:line-length-limit
}

func (p *MISCParams) marshaler() miscParamsYAMLMarshaler {
	return miscParamsYAMLMarshaler{
		SyncSourceCheckerInterval:             util.ReadableDuration(p.syncSourceCheckerInterval),
		ValidProposalOperationExpire:          util.ReadableDuration(p.validProposalOperationExpire),
		ValidProposalSuffrageOperationsExpire: util.ReadableDuration(p.validProposalSuffrageOperationsExpire),
		MaxMessageSize:                        p.maxMessageSize,
		ObjectCacheSize:                       p.objectCacheSize,
	}
}

func (p *MISCParams) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(p.marshaler())
}

func (p *MISCParams) MarshalYAML() (interface{}, error) {
	return p.marshaler(), nil
}

type miscParamsYAMLUnmarshaler struct {
	//revive:disable:line-length-limit
	SyncSourceCheckerInterval             *util.ReadableDuration `json:"sync_source_checker_interval,omitempty" yaml:"sync_source_checker_interval,omitempty"`
	ValidProposalOperationExpire          *util.ReadableDuration `json:"valid_proposal_operation_expire,omitempty" yaml:"valid_proposal_operation_expire,omitempty"`
	ValidProposalSuffrageOperationsExpire *util.ReadableDuration `json:"valid_proposal_suffrage_operations_expire,omitempty" yaml:"valid_proposal_suffrage_operations_expire,omitempty"`
	MaxMessageSize                        *uint64                `json:"max_message_size,omitempty" yaml:"max_message_size,omitempty"`
	ObjectCacheSize                       *uint64                `json:"object_cache_size,omitempty" yaml:"object_cache_size,omitempty"`
	//revive:enable:line-length-limit
}

func (p *MISCParams) UnmarshalJSON(b []byte) error {
	d := defaultMISCParams()
	*p = *d

	e := util.StringError("decode MISCParams")

	var u miscParamsYAMLUnmarshaler

	if err := util.UnmarshalJSON(b, &u); err != nil {
		return e.Wrap(err)
	}

	return e.Wrap(p.unmarshal(u))
}

func (p *MISCParams) UnmarshalYAML(unmarshal func(interface{}) error) error {
	d := defaultMISCParams()
	*p = *d

	e := util.StringError("decode MISCParams")

	var u miscParamsYAMLUnmarshaler

	if err := unmarshal(&u); err != nil {
		return e.Wrap(err)
	}

	return e.Wrap(p.unmarshal(u))
}

func (p *MISCParams) unmarshal(u miscParamsYAMLUnmarshaler) error {
	if u.MaxMessageSize != nil {
		p.maxMessageSize = *u.MaxMessageSize
	}

	if u.ObjectCacheSize != nil {
		p.objectCacheSize = *u.ObjectCacheSize
	}

	durargs := [][2]interface{}{
		{u.SyncSourceCheckerInterval, &p.syncSourceCheckerInterval},
		{u.ValidProposalOperationExpire, &p.validProposalOperationExpire},
		{u.ValidProposalSuffrageOperationsExpire, &p.validProposalSuffrageOperationsExpire},
	}

	for i := range durargs {
		v := durargs[i][0].(*util.ReadableDuration) //nolint:forcetypeassert //...

		if reflect.ValueOf(v).IsZero() {
			continue
		}

		if err := util.InterfaceSetValue(time.Duration(*v), durargs[i][1]); err != nil {
			return err
		}
	}

	return nil
}

type networkParamsYAMLMarshaler struct {
	//revive:disable:line-length-limit
	HandlerTimeout        map[string]util.ReadableDuration `json:"handler_timeout,omitempty" yaml:"handler_timeout,omitempty"`
	TimeoutRequest        util.ReadableDuration            `json:"timeout_request,omitempty" yaml:"timeout_request,omitempty"`
	HandshakeIdleTimeout  util.ReadableDuration            `json:"handshake_idle_timeout,omitempty" yaml:"handshake_idle_timeout,omitempty"`
	MaxIdleTimeout        util.ReadableDuration            `json:"max_idle_timeout,omitempty" yaml:"max_idle_timeout,omitempty"`
	KeepAlivePeriod       util.ReadableDuration            `json:"keep_alive_period,omitempty" yaml:"keep_alive_period,omitempty"`
	DefaultHandlerTimeout util.ReadableDuration            `json:"default_handler_timeout,omitempty" yaml:"default_handler_timeout,omitempty"`
	//revive:enable:line-length-limit
}

func (p *NetworkParams) marshaler() networkParamsYAMLMarshaler {
	handlerTimeouts := map[string]util.ReadableDuration{}

	for i := range p.handlerTimeouts {
		v := p.handlerTimeouts[i]

		// NOTE skip default value
		if d, found := defaultHandlerTimeouts[i]; found && v == d {
			continue
		}

		if v == p.defaultHandlerTimeout {
			continue
		}

		handlerTimeouts[i] = util.ReadableDuration(v)
	}

	return networkParamsYAMLMarshaler{
		TimeoutRequest:        util.ReadableDuration(p.timeoutRequest),
		HandshakeIdleTimeout:  util.ReadableDuration(p.handshakeIdleTimeout),
		MaxIdleTimeout:        util.ReadableDuration(p.maxIdleTimeout),
		KeepAlivePeriod:       util.ReadableDuration(p.keepAlivePeriod),
		DefaultHandlerTimeout: util.ReadableDuration(p.defaultHandlerTimeout),
		HandlerTimeout:        handlerTimeouts,
	}
}

func (p *NetworkParams) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(p.marshaler())
}

func (p *NetworkParams) MarshalYAML() (interface{}, error) {
	return p.marshaler(), nil
}

type networkParamsYAMLUnmarshaler struct {
	//revive:disable:line-length-limit
	TimeoutRequest        *util.ReadableDuration           `json:"timeout_request,omitempty" yaml:"timeout_request,omitempty"`
	HandshakeIdleTimeout  *util.ReadableDuration           `json:"handshake_idle_timeout,omitempty" yaml:"handshake_idle_timeout,omitempty"`
	MaxIdleTimeout        *util.ReadableDuration           `json:"max_idle_timeout,omitempty" yaml:"max_idle_timeout,omitempty"`
	KeepAlivePeriod       *util.ReadableDuration           `json:"keep_alive_period,omitempty" yaml:"keep_alive_period,omitempty"`
	DefaultHandlerTimeout *util.ReadableDuration           `json:"default_handler_timeout,omitempty" yaml:"default_handler_timeout,omitempty"`
	HandlerTimeout        map[string]util.ReadableDuration `json:"handler_timeout,omitempty" yaml:"handler_timeout,omitempty"`
	//revive:enable:line-length-limit
}

func (p *NetworkParams) UnmarshalJSON(b []byte) error {
	d := defaultNetworkParams()
	*p = *d

	e := util.StringError("decode NetworkParams")

	var u networkParamsYAMLUnmarshaler

	if err := util.UnmarshalJSON(b, &u); err != nil {
		return e.Wrap(err)
	}

	return e.Wrap(p.unmarshal(u))
}

func (p *NetworkParams) UnmarshalYAML(unmarshal func(interface{}) error) error {
	d := defaultNetworkParams()
	*p = *d

	e := util.StringError("decode NetworkParams")

	var u networkParamsYAMLUnmarshaler

	if err := unmarshal(&u); err != nil {
		return e.Wrap(err)
	}

	return e.Wrap(p.unmarshal(u))
}

func (p *NetworkParams) unmarshal(u networkParamsYAMLUnmarshaler) error {
	durargs := [][2]interface{}{
		{u.TimeoutRequest, &p.timeoutRequest},
		{u.HandshakeIdleTimeout, &p.handshakeIdleTimeout},
		{u.MaxIdleTimeout, &p.maxIdleTimeout},
		{u.KeepAlivePeriod, &p.keepAlivePeriod},
		{u.DefaultHandlerTimeout, &p.defaultHandlerTimeout},
	}

	for i := range durargs {
		v := durargs[i][0].(*util.ReadableDuration) //nolint:forcetypeassert //...

		if reflect.ValueOf(v).IsZero() {
			continue
		}

		if err := util.InterfaceSetValue(time.Duration(*v), durargs[i][1]); err != nil {
			return err
		}
	}

	for i := range u.HandlerTimeout {
		p.handlerTimeouts[i] = time.Duration(u.HandlerTimeout[i])
	}

	return nil
}

func (p *NetworkParams) MarshalZerologObject(e *zerolog.Event) {
	e.
		Stringer("timeout_request", p.timeoutRequest).
		Stringer("handshake_idle_timeout", p.handshakeIdleTimeout).
		Stringer("max_idle_timeout", p.maxIdleTimeout).
		Stringer("keep_alive_period", p.keepAlivePeriod).
		Stringer("default_handler_timeout", p.defaultHandlerTimeout)

	ed := zerolog.Dict()

	for i := range networkHandlerPrefixes {
		prefix := networkHandlerPrefixes[i]

		ed = ed.Stringer(prefix, p.handlerTimeout(prefix))
	}

	e.Dict("handler_timeout", ed)
}