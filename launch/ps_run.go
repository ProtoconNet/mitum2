package launch

import "github.com/ProtoconNet/mitum2/util/ps"

func DefaultRunPS() *ps.PS {
	pps := ps.NewPS("cmd-run")

	_ = pps.
		AddOK(PNameEncoder, PEncoder, nil).
		AddOK(PNameDesign, PLoadDesign, nil, PNameEncoder).
		AddOK(PNameTimeSyncer, PStartTimeSyncer, PCloseTimeSyncer, PNameDesign).
		AddOK(PNameLocal, PLocal, nil, PNameDesign).
		AddOK(PNameStorage, PStorage, nil, PNameLocal).
		AddOK(PNameProposalMaker, PProposalMaker, nil, PNameStorage).
		AddOK(PNameNetwork, PNetwork, nil, PNameStorage).
		AddOK(PNameMemberlist, PMemberlist, nil, PNameNetwork).
		AddOK(PNameStartNetwork, PStartNetwork, PCloseNetwork, PNameStates).
		AddOK(PNameStartStorage, PStartStorage, PCloseStorage, PNameStartNetwork).
		AddOK(PNameStartMemberlist, PStartMemberlist, PCloseMemberlist, PNameStartNetwork).
		AddOK(PNameStartSyncSourceChecker, PStartSyncSourceChecker, PCloseSyncSourceChecker, PNameStartNetwork).
		AddOK(PNameStartLastConsensusNodesWatcher,
			PStartLastConsensusNodesWatcher, PCloseLastConsensusNodesWatcher, PNameStartNetwork).
		AddOK(PNameStates, PStates, nil, PNameNetwork).
		AddOK(PNameStatesReady, nil, PCloseStates,
			PNameStartStorage,
			PNameStartSyncSourceChecker,
			PNameStartLastConsensusNodesWatcher,
			PNameStartMemberlist,
			PNameStartNetwork,
			PNameStates,
		)

	_ = pps.POK(PNameEncoder).
		PostAddOK(PNameAddHinters, PAddHinters)

	_ = pps.POK(PNameDesign).
		PostAddOK(PNameCheckDesign, PCheckDesign)

	_ = pps.POK(PNameLocal).
		PostAddOK(PNameDiscoveryFlag, PDiscoveryFlag)

	_ = pps.POK(PNameStorage).
		PreAddOK(PNameCheckLocalFS, PCheckLocalFS).
		PreAddOK(PNameLoadDatabase, PLoadDatabase).
		PostAddOK(PNameCheckLeveldbStorage, PCheckLeveldbStorage).
		PostAddOK(PNameCheckLoadFromDatabase, PLoadFromDatabase).
		PostAddOK(PNameNodeInfo, PNodeInfo)

	_ = pps.POK(PNameNetwork).
		PreAddOK(PNameQuicstreamClient, PQuicstreamClient).
		PostAddOK(PNameSyncSourceChecker, PSyncSourceChecker).
		PostAddOK(PNameSuffrageCandidateLimiterSet, PSuffrageCandidateLimiterSet)

	_ = pps.POK(PNameMemberlist).
		PreAddOK(PNameLastConsensusNodesWatcher, PLastConsensusNodesWatcher).
		PostAddOK(PNameBallotbox, PBallotbox).
		PostAddOK(PNameLongRunningMemberlistJoin, PLongRunningMemberlistJoin).
		PostAddOK(PNameCallbackBroadcaster, PCallbackBroadcaster).
		PostAddOK(PNameSuffrageVoting, PSuffrageVoting)

	_ = pps.POK(PNameStates).
		PreAddOK(PNameProposerSelector, PProposerSelector).
		PreAddOK(PNameOperationProcessorsMap, POperationProcessorsMap).
		PreAddOK(PNameNetworkHandlers, PNetworkHandlers).
		PreAddOK(PNameNodeInConsensusNodesFunc, PNodeInConsensusNodesFunc).
		PreAddOK(PNameProposalProcessors, PProposalProcessors).
		PreAddOK(PNameBallotStuckResolver, PBallotStuckResolver).
		PostAddOK(PNamePatchLastConsensusNodesWatcher, PPatchLastConsensusNodesWatcher).
		PostAddOK(PNameStatesSetHandlers, PStatesSetHandlers).
		PostAddOK(PNameWatchDesign, PWatchDesign).
		PostAddOK(PNamePatchMemberlist, PPatchMemberlist)

	return pps
}
