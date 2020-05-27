package contrib

import (
	"golang.org/x/xerrors"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/base/ballot"
	"github.com/spikeekips/mitum/base/block"
	"github.com/spikeekips/mitum/base/key"
	"github.com/spikeekips/mitum/base/operation"
	"github.com/spikeekips/mitum/base/state"
	"github.com/spikeekips/mitum/base/tree"
	"github.com/spikeekips/mitum/base/valuehash"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/util/encoder"
	bsonencoder "github.com/spikeekips/mitum/util/encoder/bson"
	jsonencoder "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/spikeekips/mitum/util/hint"
)

var hinters = [][2]interface{}{
	{"encoder-bson", bsonencoder.Encoder{}},
	{"encoder-json", jsonencoder.Encoder{}},
	{"ballot-init", ballot.INITBallotV0{}},
	{"ballot=proposal", ballot.ProposalV0{}},
	{"ballot-sign", ballot.SIGNBallotV0{}},
	{"ballot-accept", ballot.ACCEPTBallotV0{}},
	{"ballot-init-fact", ballot.INITBallotFactV0{}},
	{"ballot-proposal-fact", ballot.ProposalFactV0{}},
	{"ballot-sign-fact", ballot.SIGNBallotFactV0{}},
	{"ballot-accept-fact", ballot.ACCEPTBallotFactV0{}},
	{"voteproof", base.VoteproofV0{}},
	{"block", block.BlockV0{}},
	{"manifest", block.ManifestV0{}},
	{"block-consensus-info", block.BlockConsensusInfoV0{}},
	{"privatekey-ether", key.EtherPrivatekey{}},
	{"publickey-ether", key.EtherPublickey{}},
	{"privatekey-btc", key.BTCPrivatekey{}},
	{"publickey-btc", key.BTCPublickey{}},
	{"privatekey-stellar", key.StellarPrivatekey{}},
	{"publickey-stellar", key.StellarPublickey{}},
	{"hash-sha256", valuehash.SHA256{}},
	{"hash-sha512", valuehash.SHA512{}},
	{"hash-dummy", valuehash.Dummy{}},
	{"operation-seal", operation.Seal{}},
	{"avltree", tree.AVLTree{}},
	{"avltree-node", operation.OperationAVLNode{}},
	{"policy-body-v0", isaac.PolicyOperationBodyV0{}},
	{"set-policy-operation-v0", isaac.SetPolicyOperationV0{}},
	{"set-policy-operation-fact-v0", isaac.SetPolicyOperationFactV0{}},
	{"state-v0", state.StateV0{}},
	{"operation-info-v0", state.OperationInfoV0{}},
	{"state-v0-avlnode", state.StateV0AVLNode{}},
	{"state-bytes-value", state.BytesValue{}},
	{"state-duration-value", state.DurationValue{}},
	{"state-hinted-value", state.HintedValue{}},
	{"state-number-value", state.NumberValue{}},
	{"state-slice-value", state.SliceValue{}},
	{"state-string-value", state.StringValue{}},
}

func LoadEncoder(extra ...hint.Hinter) (*encoder.Encoders, error) {
	// TODO merge with contestlib/LoadEncoder

	encs := encoder.NewEncoders()
	{
		enc := jsonencoder.NewEncoder()
		if err := encs.AddEncoder(enc); err != nil {
			return nil, err
		}
	}

	{
		enc := bsonencoder.NewEncoder()
		if err := encs.AddEncoder(enc); err != nil {
			return nil, err
		}
	}

	all := make([]hint.Hinter, len(hinters)+len(extra))
	for i := range hinters {
		all[i] = hinters[i][1].(hint.Hinter)
	}

	for i := range extra {
		all[len(hinters)+i] = extra[i]
	}

	for i := range all {
		hinter, ok := all[i].(hint.Hinter)
		if !ok {
			return nil, xerrors.Errorf("not hint.Hinter: %T", all[i])
		}

		if err := encs.AddHinter(hinter); err != nil {
			return nil, err
		}
	}

	return encs, nil
}