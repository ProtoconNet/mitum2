//go:build test
// +build test

package isaac

import (
	"context"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/pkg/errors"
)

func (pps *ProposalProcessors) SetMakeNew(f func(proposal base.ProposalSignFact, previous base.Manifest) (ProposalProcessor, error)) {
	pps.makenew = f
}

func (pps *ProposalProcessors) SetGetProposal(f func(context.Context, base.Point, util.Hash) (base.ProposalSignFact, error)) {
	pps.getproposal = f
}

type DummyProposalProcessor struct {
	proposal   base.ProposalSignFact
	previous   base.Manifest
	Processerr func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error)
	Saveerr    func(context.Context, base.ACCEPTVoteproof) (base.BlockMap, error)
	Cancelerr  func() error
}

func NewDummyProposalProcessor() *DummyProposalProcessor {
	return &DummyProposalProcessor{}
}

func (p *DummyProposalProcessor) Make(proposal base.ProposalSignFact, previous base.Manifest) (ProposalProcessor, error) {
	return DummyProposalProcessor{
		proposal:   proposal,
		previous:   previous,
		Processerr: p.Processerr,
		Saveerr:    p.Saveerr,
		Cancelerr:  p.Cancelerr,
	}, nil
}

func (p DummyProposalProcessor) Process(ctx context.Context, ivp base.INITVoteproof) (base.Manifest, error) {
	if p.Processerr != nil {
		return p.Processerr(ctx, p.proposal.ProposalFact(), ivp)
	}

	return nil, errors.Errorf("wrong processing")
}

func (p DummyProposalProcessor) Save(ctx context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
	if p.Saveerr != nil {
		return p.Saveerr(ctx, avp)
	}

	return nil, nil
}

func (p DummyProposalProcessor) Cancel() error {
	if p.Cancelerr != nil {
		return p.Cancelerr()
	}

	return nil
}

func (p DummyProposalProcessor) Proposal() base.ProposalSignFact {
	return p.proposal
}
