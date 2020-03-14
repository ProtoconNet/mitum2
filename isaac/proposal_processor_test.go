package isaac

import (
	"testing"

	"github.com/spikeekips/mitum/network"
	"github.com/spikeekips/mitum/seal"
	"github.com/spikeekips/mitum/storage"
	"github.com/spikeekips/mitum/valuehash"
	"github.com/stretchr/testify/suite"
	"golang.org/x/xerrors"
)

type testProposalProcessor struct {
	baseTestStateHandler
}

func (t *testProposalProcessor) TestProcess() {
	pm := NewProposalMaker(t.localstate)

	ib, err := NewINITBallotV0FromLocalstate(t.localstate, Round(0))
	t.NoError(err)
	initFact := ib.INITBallotFactV0

	ivp, err := t.newVoteproof(StageINIT, initFact, t.localstate, t.remoteState)
	proposal, err := pm.Proposal(ivp.Round())

	_ = t.localstate.Storage().NewProposal(proposal)

	dp := NewProposalProcessorV0(t.localstate)

	block, err := dp.ProcessINIT(proposal.Hash(), ivp)
	t.NoError(err)
	t.NotNil(block)
}

func (t *testProposalProcessor) TestBlockOperations() {
	pm := NewProposalMaker(t.localstate)

	ib, err := NewINITBallotV0FromLocalstate(t.localstate, Round(0))
	t.NoError(err)
	initFact := ib.INITBallotFactV0

	ivp, err := t.newVoteproof(StageINIT, initFact, t.localstate, t.remoteState)

	var proposal Proposal
	{
		pr, err := pm.Proposal(ivp.Round())
		t.NoError(err)

		opl := t.newOperationSeal(t.localstate)
		t.NoError(t.localstate.Storage().NewSeals([]seal.Seal{opl}))

		newSeals := []valuehash.Hash{opl.Hash()}

		newpr, err := NewProposal(
			t.localstate,
			pr.Height(),
			pr.Round(),
			newSeals,
			t.localstate.Policy().NetworkID(),
		)
		t.NoError(err)

		proposal = newpr
		_ = t.localstate.Storage().NewProposal(proposal)
	}

	dp := NewProposalProcessorV0(t.localstate)
	dp.SetLogger(log)

	block, err := dp.ProcessINIT(proposal.Hash(), ivp)
	t.NoError(err)

	t.NotNil(block.Operations())
	t.NotNil(block.States())
}

func (t *testProposalProcessor) TestNotFoundInProposal() {
	pm := NewProposalMaker(t.localstate)

	ib, err := NewINITBallotV0FromLocalstate(t.localstate, Round(0))
	t.NoError(err)
	initFact := ib.INITBallotFactV0

	ivp, err := t.newVoteproof(StageINIT, initFact, t.localstate, t.remoteState)

	var proposal Proposal
	{
		pr, err := pm.Proposal(ivp.Round())
		t.NoError(err)

		op := t.newOperationSeal(t.remoteState)

		// add getSealHandler
		t.remoteState.Node().Channel().(*network.ChanChannel).SetGetSealHandler(
			func(hs []valuehash.Hash) ([]seal.Seal, error) {
				return []seal.Seal{op}, nil
			},
		)

		newSeals := []valuehash.Hash{op.Hash()}

		newpr, err := NewProposal(
			t.remoteState,
			pr.Height(),
			pr.Round(),
			newSeals,
			t.remoteState.Policy().NetworkID(),
		)
		t.NoError(err)

		proposal = newpr
	}

	for _, h := range proposal.Seals() {
		_, err = t.localstate.Storage().Seal(h)
		t.True(xerrors.Is(err, storage.NotFoundError))
	}

	_ = t.localstate.Storage().NewProposal(proposal)

	dp := NewProposalProcessorV0(t.localstate)
	_, err = dp.ProcessINIT(proposal.Hash(), ivp)
	t.NoError(err)

	// local node should have the missing seals
	for _, h := range proposal.Seals() {
		_, err = t.localstate.Storage().Seal(h)
		t.NoError(err)
	}
}

func TestProposalProcessor(t *testing.T) {
	suite.Run(t, new(testProposalProcessor))
}