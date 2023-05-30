package launch

import (
	"context"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	isaacnetwork "github.com/ProtoconNet/mitum2/isaac/network"
	isaacstates "github.com/ProtoconNet/mitum2/isaac/states"
	"github.com/ProtoconNet/mitum2/network"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/logging"
	"github.com/ProtoconNet/mitum2/util/ps"
	"github.com/pkg/errors"
)

var (
	PNameNodeInfo      = ps.Name("nodeinfo")
	NodeInfoContextKey = util.ContextKey("nodeinfo")
)

func PNodeInfo(pctx context.Context) (context.Context, error) {
	e := util.StringErrorFunc("failed to prepare nodeinfo")

	var log *logging.Logging
	var version util.Version
	var local base.LocalNode
	var params *isaac.LocalParams
	var design NodeDesign
	var db isaac.Database

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		VersionContextKey, &version,
		DesignContextKey, &design,
		LocalContextKey, &local,
		LocalParamsContextKey, &params,
		CenterDatabaseContextKey, &db,
	); err != nil {
		return pctx, e(err, "")
	}

	nodeinfo := isaacnetwork.NewNodeInfoUpdater(design.NetworkID, local, version)
	_ = nodeinfo.SetConsensusState(isaacstates.StateBooting)
	_ = nodeinfo.SetConnInfo(network.ConnInfoToString(
		design.Network.PublishString,
		design.Network.TLSInsecure,
	))
	_ = nodeinfo.SetLocalParams(params)

	pctx = context.WithValue(pctx, NodeInfoContextKey, nodeinfo) //revive:disable-line:modifies-parameter

	if err := UpdateNodeInfoWithNewBlock(db, nodeinfo); err != nil {
		log.Log().Error().Err(err).Msg("failed to update nodeinfo")
	}

	return pctx, nil
}

func UpdateNodeInfoWithNewBlock(
	db isaac.Database,
	nodeinfo *isaacnetwork.NodeInfoUpdater,
) error {
	switch m, found, err := db.LastBlockMap(); {
	case err != nil:
		return err
	case !found:
		return errors.Errorf("last BlockMap not found")
	case !nodeinfo.SetLastManifest(m.Manifest()):
		return nil
	}

	switch proof, found, err := db.LastSuffrageProof(); {
	case err != nil:
		return errors.Errorf("last SuffrageProof not found")
	case found && nodeinfo.SetSuffrageHeight(proof.SuffrageHeight()):
		suf, err := proof.Suffrage()
		if err != nil {
			return errors.Errorf("failed suffrage from proof")
		}

		_ = nodeinfo.SetConsensusNodes(suf.Nodes())
	}

	_ = nodeinfo.SetNetworkPolicy(db.LastNetworkPolicy())

	return nil
}
