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
	e := util.StringError("prepare nodeinfo")

	var log *logging.Logging
	var version util.Version
	var local base.LocalNode
	var isaacparams *isaac.Params
	var design NodeDesign
	var db isaac.Database

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		VersionContextKey, &version,
		DesignContextKey, &design,
		LocalContextKey, &local,
		ISAACParamsContextKey, &isaacparams,
		CenterDatabaseContextKey, &db,
	); err != nil {
		return pctx, e.Wrap(err)
	}

	nodeinfo := isaacnetwork.NewNodeInfoUpdater(design.NetworkID, local, version)
	_ = nodeinfo.SetConsensusState(isaacstates.StateBooting)
	_ = nodeinfo.SetConnInfo(network.ConnInfoToString(
		design.Network.PublishString,
		design.Network.TLSInsecure,
	))
	_ = nodeinfo.SetLocalParams(isaacparams)

	nctx := context.WithValue(pctx, NodeInfoContextKey, nodeinfo)

	switch err := UpdateNodeInfoWithNewBlock(db, nodeinfo); {
	case err == nil:
	case errors.Is(err, util.ErrNotFound):
		log.Log().Debug().Err(err).Msg("nodeinfo not updated")
	default:
		log.Log().Error().Err(err).Msg("failed to update nodeinfo")
	}

	return nctx, nil
}

func UpdateNodeInfoWithNewBlock(
	db isaac.Database,
	nodeinfo *isaacnetwork.NodeInfoUpdater,
) error {
	switch m, found, err := db.LastBlockMap(); {
	case err != nil:
		return err
	case !found:
		return util.ErrNotFound.Errorf("last BlockMap")
	case !nodeinfo.SetLastManifest(m.Manifest()):
		return nil
	}

	switch proof, found, err := db.LastSuffrageProof(); {
	case err != nil:
		return errors.WithMessage(err, "last SuffrageProof not found")
	case found && nodeinfo.SetSuffrageHeight(proof.SuffrageHeight()):
		suf, err := proof.Suffrage()
		if err != nil {
			return errors.WithMessage(err, "suffrage from proof")
		}

		_ = nodeinfo.SetConsensusNodes(suf.Nodes())
	}

	_ = nodeinfo.SetNetworkPolicy(db.LastNetworkPolicy())

	return nil
}
