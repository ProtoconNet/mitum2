package isaacnetwork

import (
	"encoding/json"
	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	isaacstates "github.com/ProtoconNet/mitum2/isaac/states"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/ProtoconNet/mitum2/util/localtime"
	"github.com/pkg/errors"
	"reflect"
)

type NodeInfoLocalJSONMarshaler struct {
	Address     base.Address   `json:"address"`
	Publickey   base.Publickey `json:"publickey"`
	LocalParams *isaac.Params  `json:"parameters"` //nolint:tagliatelle //...
	ConnInfo    string         `json:"conn_info"`
	StartedAt   localtime.Time `json:"started_at"`
	Version     util.Version   `json:"version"`
}

type NodeInfoSuffrageJSONMarshaler struct {
	Nodes  []base.Node `json:"nodes"`
	Height base.Height `json:"height"`
}

type NodeInfoConsensusJSONMarshaler struct {
	LastVote NodeInfoLastVote              `json:"last_vote"`
	State    isaacstates.StateType         `json:"state"`
	Suffrage NodeInfoSuffrageJSONMarshaler `json:"suffrage"`
}

type NodeInfoJSONMarshaler struct {
	NetworkID     base.NetworkID                 `json:"network_id"`
	LastManifest  base.Manifest                  `json:"last_manifest"`
	NetworkPolicy base.NetworkPolicy             `json:"network_policy"`
	Local         NodeInfoLocalJSONMarshaler     `json:"local"`
	Consensus     NodeInfoConsensusJSONMarshaler `json:"consensus"`
	hint.BaseHinter
}

func (info NodeInfo) JSONMarshaler() NodeInfoJSONMarshaler {
	return NodeInfoJSONMarshaler{
		BaseHinter: info.BaseHinter,
		NetworkID:  info.networkID,
		Local: NodeInfoLocalJSONMarshaler{
			Address:     info.address,
			Publickey:   info.publickey,
			LocalParams: info.localParams,
			ConnInfo:    info.connInfo,
			Version:     info.version,
			StartedAt:   localtime.New(info.startedAt),
		},
		Consensus: NodeInfoConsensusJSONMarshaler{
			State: info.consensusState,
			Suffrage: NodeInfoSuffrageJSONMarshaler{
				Height: info.suffrageHeight,
				Nodes:  info.consensusNodes,
			},
			LastVote: info.lastVote,
		},
		LastManifest:  info.lastManifest,
		NetworkPolicy: info.networkPolicy,
	}
}

func (info NodeInfo) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(info.JSONMarshaler())
}

type nodeInfoJSONUnmarshaler struct {
	NetworkID     base.NetworkID                   `json:"network_id"`
	LastManifest  json.RawMessage                  `json:"last_manifest"`
	NetworkPolicy json.RawMessage                  `json:"network_policy"`
	Consensus     nodeInfoConsensusJSONUnmarshaler `json:"consensus"`
	Local         nodeInfoLocalJSONUnmarshaler     `json:"local"`
}

type nodeInfoLocalJSONUnmarshaler struct {
	Address     string          `json:"address"`
	Publickey   string          `json:"publickey"`
	ConnInfo    string          `json:"conn_info"`
	StartedAt   localtime.Time  `json:"started_at"`
	LocalParams json.RawMessage `json:"parameters"` //nolint:tagliatelle //...
	Version     util.Version    `json:"version"`
}

type nodeInfoConsensusJSONUnmarshaler struct {
	LastVote NodeInfoLastVote                `json:"last_vote"`
	State    isaacstates.StateType           `json:"state"`
	Suffrage nodeInfoSuffrageJSONUnmarshaler `json:"suffrage"`
}

type nodeInfoSuffrageJSONUnmarshaler struct {
	Nodes  []json.RawMessage `json:"nodes"`
	Height base.Height       `json:"height"`
}

func (info *NodeInfo) DecodeJSON(b []byte, enc encoder.Encoder) error {
	e := util.StringError("decode NodeInfo")

	var u nodeInfoJSONUnmarshaler

	if err := enc.Unmarshal(b, &u); err != nil {
		return e.Wrap(err)
	}

	info.networkID = u.NetworkID
	info.startedAt = u.Local.StartedAt.Time

	// NOTE local
	switch i, err := base.DecodeAddress(u.Local.Address, enc); {
	case err != nil:
		return e.Wrap(err)
	default:
		info.address = i
	}

	switch i, err := base.DecodePublickeyFromString(u.Local.Publickey, enc); {
	case err != nil:
		return e.Wrap(err)
	default:
		info.publickey = i
	}

	//params := isaac.NewParams(info.networkID)
	var params *isaac.Params

	hinter, err := enc.Decode(u.Local.LocalParams)
	if err != nil {
		return e.Wrap(err)
	}

	if hinter == nil {
		return nil
	}

	i, ok := hinter.(*isaac.Params)
	if !ok {
		return errors.Errorf("expected %v, but %T", reflect.TypeOf(params).Elem(), hinter)
	}

	params = i

	if err := params.SetNetworkID(info.networkID); err != nil {
		return e.Wrap(err)
	}

	info.localParams = params

	info.connInfo = u.Local.ConnInfo
	info.version = u.Local.Version

	// NOTE consensus
	info.consensusState = u.Consensus.State

	// NOTE suffrage
	info.suffrageHeight = u.Consensus.Suffrage.Height

	info.consensusNodes = make([]base.Node, len(u.Consensus.Suffrage.Nodes))
	for i := range u.Consensus.Suffrage.Nodes {
		if err := encoder.Decode(enc, u.Consensus.Suffrage.Nodes[i], &info.consensusNodes[i]); err != nil {
			return e.Wrap(err)
		}
	}

	// NOTE last manifest
	if err := encoder.Decode(enc, u.LastManifest, &info.lastManifest); err != nil {
		return e.Wrap(err)
	}

	// NOTE network policy
	if err := encoder.Decode(enc, u.NetworkPolicy, &info.networkPolicy); err != nil {
		return e.Wrap(err)
	}

	info.lastVote = u.Consensus.LastVote

	return nil
}
