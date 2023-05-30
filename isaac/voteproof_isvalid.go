package isaac

import (
	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
)

func IsValidVoteproofWithSuffrage(vp base.Voteproof, suf base.Suffrage) error {
	e := util.ErrInvalid.Errorf("invalid voteproof with suffrage")

	if vp == nil {
		return e.Errorf("nil voteproof")
	}

	var withdraws []base.SuffrageWithdrawOperation

	if w, ok := vp.(base.HasWithdraws); ok {
		withdraws = w.Withdraws()
	}

	th := vp.Threshold()
	rsuf := suf

	if len(withdraws) > 0 {
		for i := range withdraws {
			if err := IsValidWithdrawWithSuffrage(vp.Point().Height(), withdraws[i], suf); err != nil {
				return e.Wrap(err)
			}
		}

		switch i, err := NewSuffrageWithWithdraws(suf, vp.Threshold(), withdraws); {
		case err != nil:
			return e.Wrap(err)
		default:
			rsuf = i
			th = base.MaxThreshold
		}
	}

	if _, ok := vp.(base.StuckVoteproof); ok {
		if suf.Len() != len(vp.SignFacts())+len(withdraws) {
			return e.Errorf("not enough sign facts with withdraws")
		}
	}

	if err := base.IsValidVoteproofWithSuffrage(vp, rsuf, th); err != nil {
		return e.Wrap(err)
	}

	return nil
}
