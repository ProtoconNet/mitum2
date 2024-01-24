package isaacstates

import (
	"context"
	"sort"
	"testing"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	isaacblock "github.com/ProtoconNet/mitum2/isaac/block"
	isaacdatabase "github.com/ProtoconNet/mitum2/isaac/database"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/fixedtree"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type baseTestSuffrageStateBuilder struct {
	isaac.BaseTestBallots
	isaacdatabase.BaseTestDatabase
}

func (t *baseTestSuffrageStateBuilder) SetupTest() {
	t.BaseTestBallots.SetupTest()
	t.BaseTestDatabase.SetupTest()
}

func (t *baseTestSuffrageStateBuilder) prepare(point base.Point, previous base.State, locals, newlocals []base.LocalNode) isaacblock.SuffrageProof {
	newnodes := make([]base.Node, len(newlocals))
	for i := range newnodes {
		newnodes[i] = newlocals[i]
	}

	switch {
	case point.Height() == base.GenesisHeight && previous != nil:
		t.Fail("previous state was given for genesis")
	case point.Height() != base.GenesisHeight && previous == nil:
		t.Fail("empty previous state was given")
	}

	var previoushash util.Hash
	if previous != nil {
		previoushash = previous.Hash()
	}

	blockMap, err := newTestBlockMap(point.Height(), nil, previoushash, t.Local, t.LocalParams.NetworkID())
	t.NoError(err)

	var suffrageheight base.Height
	if previous != nil {
		suffrageheight = previous.Value().(base.SuffrageNodesStateValue).Height() + 1
	}

	newstate, _ := t.SuffrageState(point.Height(), suffrageheight, newnodes)
	newstate = base.NewBaseState(
		point.Height(),
		isaac.SuffrageStateKey,
		newstate.Value(),
		previoushash,
		[]util.Hash{valuehash.RandomSHA256(), valuehash.RandomSHA256(), valuehash.RandomSHA256()},
	)

	states := t.States(point.Height(), 6)
	states = append(states, newstate)

	w, _ := fixedtree.NewWriter(base.StateFixedtreeHint, uint64(len(states)))
	for i := range states {
		t.NoError(w.Add(uint64(i), fixedtree.NewBaseNode(states[i].Hash().String())))
	}
	_ = w.Write(func(uint64, fixedtree.Node) error {
		return nil
	})

	tr, err := w.Tree()
	t.NoError(err)

	proof, err := tr.Proof(newstate.Hash().String())
	t.NoError(err)

	return isaacblock.NewSuffrageProof(blockMap, newstate, proof)
}

func (t *baseTestSuffrageStateBuilder) newProofs(n int) map[base.Height]base.SuffrageProof {
	locals := []base.LocalNode{t.Local}

	p := base.GenesisPoint
	proofs := map[base.Height]base.SuffrageProof{}
	for i := range make([]byte, n) {
		newnodes, _ := t.Locals(i)
		newlocals := make([]base.LocalNode, len(locals)+len(newnodes))
		copy(newlocals[:len(locals)], locals)
		copy(newlocals[len(locals):], newnodes)

		var previous base.State
		if p.Height() == base.GenesisHeight {
			previous = nil
		} else {
			previous = proofs[p.Height()-1].State()
		}

		proof := t.prepare(p, previous, locals, newlocals)
		proofs[p.Height()] = proof

		p = p.NextHeight()
		locals = newlocals
	}

	return proofs
}

func (t *baseTestSuffrageStateBuilder) compareSuffrage(expectedstate, foundstate base.State) {
	expected, err := isaac.NewSuffrageFromState(expectedstate)
	t.NoError(err)

	found, err := isaac.NewSuffrageFromState(foundstate)
	t.NoError(err)

	t.Equal(len(expected.Nodes()), len(found.Nodes()))
	for i := range expected.Nodes() {
		a := expected.Nodes()[i]

		t.True(found.Exists(a.Address()))
		t.True(found.ExistsPublickey(a.Address(), a.Publickey()))
	}
}

type testSuffrageStateBuilder struct {
	baseTestSuffrageStateBuilder
}

func (t *testSuffrageStateBuilder) candidateState(height base.Height, n int) base.State {
	candidates := make([]base.SuffrageCandidateStateValue, n)
	for i := range candidates {
		candidates[i] = isaac.NewSuffrageCandidateStateValue(base.RandomNode(), height+1, height+3)
	}

	v := isaac.NewSuffrageCandidatesStateValue(candidates)
	return base.NewBaseState(
		height,
		isaac.SuffrageCandidateStateKey,
		v,
		nil,
		[]util.Hash{valuehash.RandomSHA256()},
	)
}

func (t *testSuffrageStateBuilder) TestBuildOneFromGenesis() {
	proofs := t.newProofs(1)
	last := proofs[0]
	lastheight := base.Height(66)

	cst := t.candidateState(base.Height(33), 1)

	expected := []base.Height{0}
	var fetched []base.Height

	s := isaac.NewSuffrageStateBuilder(
		t.LocalParams.NetworkID(),
		func(context.Context) (base.Height, base.SuffrageProof, bool, error) {
			h := last.State().Height()

			return lastheight, proofs[h], true, nil
		},
		func(_ context.Context, height base.Height) (base.SuffrageProof, bool, error) {
			switch {
			case height < base.GenesisHeight, height > last.State().Height():
				return nil, false, errors.Errorf("invalid height request, %d", height)
			}

			fetched = append(fetched, height)

			proof, found := proofs[height]
			t.True(found)

			return proof, found, nil
		},
		func(context.Context) (base.State, bool, error) {
			return cst, true, nil
		},
	)
	s.SetBatchLimit(3)

	rlastheight, rproofs, st, err := s.Build(context.Background(), nil)
	t.NoError(err)
	t.True(len(rproofs) > 0)
	t.NotNil(st)
	t.Equal(lastheight, rlastheight)

	proof := rproofs[len(rproofs)-1]

	t.True(base.IsEqualState(last.State(), proof.State()))
	t.True(base.IsEqualState(cst, st))
	t.compareSuffrage(last.State(), proof.State())

	sort.Slice(fetched, func(i, j int) bool {
		return fetched[i] < fetched[j]
	})

	t.Equal(expected, fetched)
}

func (t *testSuffrageStateBuilder) TestBuildFromGenesis() {
	proofs := t.newProofs(14)
	last := proofs[13]

	expected := []base.Height{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	fetched := make([]base.Height, len(expected))

	s := isaac.NewSuffrageStateBuilder(
		t.LocalParams.NetworkID(),
		func(context.Context) (base.Height, base.SuffrageProof, bool, error) {
			h := last.State().Height()

			return h, proofs[h], true, nil
		},
		func(_ context.Context, height base.Height) (base.SuffrageProof, bool, error) {
			switch {
			case height < base.GenesisHeight, height > last.State().Height():
				return nil, false, errors.Errorf("invalid height request, %d", height)
			}

			fetched[height.Int64()] = height

			proof, found := proofs[height]
			t.True(found)

			return proof, found, nil
		},
		func(context.Context) (base.State, bool, error) { return nil, false, nil },
	)
	s.SetBatchLimit(3)

	_, rproofs, _, err := s.Build(context.Background(), nil)
	t.NoError(err)

	proof := rproofs[len(rproofs)-1]

	t.NotNil(proof)

	t.True(base.IsEqualState(last.State(), proof.State()))
	t.compareSuffrage(last.State(), proof.State())

	sort.Slice(fetched, func(i, j int) bool {
		return fetched[i] < fetched[j]
	})

	t.Equal(expected, fetched)
}

func (t *testSuffrageStateBuilder) TestBuildNotFromGenesis() {
	proofs := t.newProofs(14)
	last := proofs[13]

	localheight := base.Height(3)

	expected := []base.Height{4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	fetched := make([]base.Height, len(expected))

	s := isaac.NewSuffrageStateBuilder(
		t.LocalParams.NetworkID(),
		func(context.Context) (base.Height, base.SuffrageProof, bool, error) {
			h := last.State().Height()

			return h, proofs[h], true, nil
		},
		func(_ context.Context, height base.Height) (base.SuffrageProof, bool, error) {
			switch {
			case height <= localheight, height > last.State().Height():
				return nil, false, errors.Errorf("invalid height request, %d", height)
			}

			fetched[(height - localheight - 1).Int64()] = height

			proof, found := proofs[height]
			t.True(found)

			return proof, found, nil
		},
		func(context.Context) (base.State, bool, error) { return nil, false, nil },
	)
	s.SetBatchLimit(3)

	_, rproofs, _, err := s.Build(context.Background(), proofs[localheight].State())
	t.NoError(err)

	proof := rproofs[len(rproofs)-1]
	t.NotNil(proof)

	t.True(base.IsEqualState(last.State(), proof.State()))
	t.compareSuffrage(last.State(), proof.State())

	sort.Slice(fetched, func(i, j int) bool {
		return fetched[i] < fetched[j]
	})

	t.Equal(expected, fetched)
}

func (t *testSuffrageStateBuilder) TestBuildLastNotFromGenesis() {
	proofs := t.newProofs(14)
	last := proofs[13]

	localheight := base.Height(13)

	s := isaac.NewSuffrageStateBuilder(
		t.LocalParams.NetworkID(),
		func(context.Context) (base.Height, base.SuffrageProof, bool, error) {
			h := last.State().Height()

			return h, proofs[h], true, nil
		},
		func(_ context.Context, height base.Height) (base.SuffrageProof, bool, error) {
			return nil, false, errors.Errorf("invalid height request, %d", height)
		},
		func(context.Context) (base.State, bool, error) { return nil, false, nil },
	)
	s.SetBatchLimit(3)

	_, rproofs, _, err := s.Build(context.Background(), proofs[localheight].State())
	t.NoError(err)
	t.True(len(rproofs) < 1)
}

func TestSuffrageStateBuilder(t *testing.T) {
	suite.Run(t, new(testSuffrageStateBuilder))
}
