package base

type VoteResult string

const (
	VoteResultNotYet   = VoteResult("NOT YET")
	VoteResultDraw     = VoteResult("DRAW")
	VoteResultMajority = VoteResult("MAJORITY")
)

func (v VoteResult) Bytes() []byte {
	return []byte(v)
}

func (v VoteResult) String() string {
	switch v {
	case VoteResultNotYet, VoteResultDraw, VoteResultMajority:
		return string(v)
	default:
		return "NOT YET"
	}
}

func (VoteResult) IsValid([]byte) error {
	return nil
}

func (v VoteResult) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

func (v *VoteResult) UnmarshalText(b []byte) error {
	i := VoteResult(string(b))

	switch i {
	case VoteResultNotYet, VoteResultDraw, VoteResultMajority:
	default:
		i = VoteResultNotYet
	}

	*v = i

	return nil
}

func FindVoteResult(quorum, threshold uint, s []string) (result VoteResult, key string) {
	th := threshold
	if th > quorum {
		th = quorum
	}

	total := uint(len(s))
	if total == 0 {
		return VoteResultNotYet, ""
	}

	var remain uint
	if total < quorum {
		remain = quorum - total
	}

	count := make(map[string]uint, len(s))
	for _, v := range s {
		count[v]++
	}

	var (
		maxCount uint
		topTies  uint
		mhs      string
	)
	for hs, cn := range count {
		if cn > maxCount {
			maxCount = cn
			mhs = hs
			topTies = 1
		} else if cn == maxCount {
			topTies++
			if hs < mhs {
				mhs = hs
			}
		}
	}

	if maxCount >= th {
		return VoteResultMajority, mhs
	}

	if remain == 0 {
		return VoteResultDraw, ""
	}

	if maxCount+remain < th {
		return VoteResultDraw, ""
	}

	return VoteResultNotYet, ""
}
