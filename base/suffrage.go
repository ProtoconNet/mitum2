package base

import (
	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/fixedtree"
)

type Suffrage interface {
	Exists(Address) bool
	ExistsPublickey(Address, Publickey) bool
	Nodes() []Node
	Len() int
}

func IsInSuffrage(suf Suffrage, node Node) (bool, error) {
	switch {
	case suf == nil:
		return false, errors.Errorf("empty suffrage")
	case node == nil:
		return false, errors.Errorf("empty node")
	default:
		return suf.ExistsPublickey(node.Address(), node.Publickey()), nil
	}
}

type SuffrageStateValue interface {
	StateValue
	Height() Height // NOTE not manifest height
	Nodes() []Node
	Suffrage() (Suffrage, error)
}

type SuffrageCandidate interface {
	util.HashByter
	util.IsValider
	Node
	Start() Height
	Deadline() Height
}

type SuffrageCandidateStateValue interface {
	StateValue
	Nodes() []SuffrageCandidate
}

func InterfaceIsSuffrageState(i interface{}) (State, error) {
	switch st, ok := i.(State); {
	case !ok:
		return nil, errors.Errorf("not suffrage state: %T", i)
	default:
		if _, err := LoadSuffrageState(st); err != nil {
			return nil, err
		}

		return st, nil
	}
}

func IsSuffrageState(st State) bool {
	_, err := LoadSuffrageState(st)

	return err == nil
}

func LoadSuffrageState(st State) (SuffrageStateValue, error) {
	if st == nil || st.Value() == nil {
		return nil, errors.Errorf("empty state")
	}

	j, ok := st.Value().(SuffrageStateValue)
	if !ok {
		return nil, errors.Errorf("expected SuffrageStateValue, but %T", st.Value())
	}

	return j, nil
}

type SuffrageProof interface {
	util.IsValider
	Map() BlockMap
	State() State
	ACCEPTVoteproof() ACCEPTVoteproof
	Proof() fixedtree.Proof
	Suffrage() (Suffrage, error)
	SuffrageHeight() Height
	Prove(previousState State) error
}

type (
	SuffrageCandidateLimiterFunc func(rule SuffrageCandidateLimiterRule) (SuffrageCandidateLimiter, error)
	SuffrageCandidateLimiter     func() (uint64, error)
)
