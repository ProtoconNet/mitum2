package ballot

import (
	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/valuehash"
)

func (ib *INITV0) unpack(
	enc encoder.Encoder,
	bb BaseBallotV0,
	bf BaseFactV0,
	previousBlock valuehash.Hash,
	bVoteproof,
	bAVoteproof []byte,
) error {
	if previousBlock != nil && previousBlock.IsEmpty() {
		return errors.Errorf("empty previous_block hash found")
	}

	if bVoteproof != nil {
		i, err := base.DecodeVoteproof(bVoteproof, enc)
		if err != nil {
			return err
		}
		ib.voteproof = i
	}

	if bAVoteproof != nil {
		i, err := base.DecodeVoteproof(bAVoteproof, enc)
		if err != nil {
			return err
		}
		ib.acceptVoteproof = i
	}

	ib.BaseBallotV0 = bb
	ib.INITFactV0 = INITFactV0{
		BaseFactV0:    bf,
		previousBlock: previousBlock,
	}

	return nil
}

func (ibf *INITFactV0) unpack(
	_ encoder.Encoder,
	bf BaseFactV0,
	previousBlock valuehash.Hash,
) error {
	if previousBlock != nil && previousBlock.IsEmpty() {
		return errors.Errorf("empty previous_block hash found")
	}

	ibf.BaseFactV0 = bf
	ibf.previousBlock = previousBlock

	return nil
}
