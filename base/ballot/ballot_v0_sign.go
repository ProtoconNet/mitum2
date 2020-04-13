package ballot

import (
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/base/key"
	"github.com/spikeekips/mitum/base/valuehash"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/hint"
	"github.com/spikeekips/mitum/util/isvalid"
	"github.com/spikeekips/mitum/util/localtime"
)

var (
	SIGNBallotV0Hint     hint.Hint = hint.MustHint(SIGNBallotType, "0.0.1")
	SIGNBallotFactV0Hint hint.Hint = hint.MustHint(SIGNBallotFactType, "0.0.1")
)

type SIGNBallotFactV0 struct {
	BaseBallotFactV0
	proposal valuehash.Hash
	newBlock valuehash.Hash
}

func (sbf SIGNBallotFactV0) Hint() hint.Hint {
	return SIGNBallotFactV0Hint
}

func (sbf SIGNBallotFactV0) IsValid([]byte) error {
	if err := isvalid.Check([]isvalid.IsValider{
		sbf.BaseBallotFactV0,
		sbf.proposal,
		sbf.newBlock,
	}, nil, false); err != nil {
		return err
	}

	return nil
}

func (sbf SIGNBallotFactV0) Hash() valuehash.Hash {
	return valuehash.NewSHA256(sbf.Bytes())
}

func (sbf SIGNBallotFactV0) Bytes() []byte {
	return util.ConcatBytesSlice(
		sbf.BaseBallotFactV0.Bytes(),
		sbf.proposal.Bytes(),
		sbf.newBlock.Bytes(),
	)
}

func (sbf SIGNBallotFactV0) Proposal() valuehash.Hash {
	return sbf.proposal
}

func (sbf SIGNBallotFactV0) NewBlock() valuehash.Hash {
	return sbf.newBlock
}

type SIGNBallotV0 struct {
	BaseBallotV0
	SIGNBallotFactV0
}

func NewSIGNBallotV0(
	node base.Address,
	height base.Height,
	round base.Round,
	proposal valuehash.Hash,
	newBlock valuehash.Hash,
) SIGNBallotV0 {
	return SIGNBallotV0{
		BaseBallotV0: BaseBallotV0{
			node: node,
		},
		SIGNBallotFactV0: SIGNBallotFactV0{
			BaseBallotFactV0: BaseBallotFactV0{
				height: height,
				round:  round,
			},
			proposal: proposal,
			newBlock: newBlock,
		},
	}
}

func (sb SIGNBallotV0) Hash() valuehash.Hash {
	return sb.BaseBallotV0.Hash()
}

func (sb SIGNBallotV0) Hint() hint.Hint {
	return SIGNBallotV0Hint
}

func (sb SIGNBallotV0) Stage() base.Stage {
	return base.StageSIGN
}

func (sb SIGNBallotV0) IsValid(b []byte) error {
	if err := isvalid.Check([]isvalid.IsValider{
		sb.BaseBallotV0,
		sb.SIGNBallotFactV0,
	}, b, false); err != nil {
		return err
	}

	if err := IsValidBallot(sb, b); err != nil {
		return err
	}

	return nil
}

func (sb SIGNBallotV0) GenerateHash() (valuehash.Hash, error) {
	return valuehash.NewSHA256(util.ConcatBytesSlice(sb.BaseBallotV0.Bytes(), sb.SIGNBallotFactV0.Bytes())), nil
}

func (sb SIGNBallotV0) GenerateBodyHash() (valuehash.Hash, error) {
	if err := sb.SIGNBallotFactV0.IsValid(nil); err != nil {
		return nil, err
	}

	return valuehash.NewSHA256(sb.SIGNBallotFactV0.Bytes()), nil
}

func (sb SIGNBallotV0) Fact() base.Fact {
	return sb.SIGNBallotFactV0
}

func (sb *SIGNBallotV0) Sign(pk key.Privatekey, b []byte) error { // nolint
	if err := sb.BaseBallotV0.IsReadyToSign(b); err != nil {
		return err
	}

	var bodyHash valuehash.Hash
	if h, err := sb.GenerateBodyHash(); err != nil {
		return err
	} else {
		bodyHash = h
	}

	var sig key.Signature
	if s, err := pk.Sign(util.ConcatBytesSlice(bodyHash.Bytes(), b)); err != nil {
		return err
	} else {
		sig = s
	}

	factHash := sb.SIGNBallotFactV0.Hash()
	factSig, err := pk.Sign(util.ConcatBytesSlice(factHash.Bytes(), b))
	if err != nil {
		return err
	}

	sb.BaseBallotV0.signer = pk.Publickey()
	sb.BaseBallotV0.signature = sig
	sb.BaseBallotV0.signedAt = localtime.Now()
	sb.BaseBallotV0.bodyHash = bodyHash
	sb.BaseBallotV0.factHash = factHash
	sb.BaseBallotV0.factSignature = factSig

	if h, err := sb.GenerateHash(); err != nil {
		return err
	} else {
		sb.BaseBallotV0 = sb.BaseBallotV0.SetHash(h)
	}

	return nil
}