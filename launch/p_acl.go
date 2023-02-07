package launch

import (
	"context"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/logging"
	"github.com/ProtoconNet/mitum2/util/ps"
	"github.com/pkg/errors"
)

var (
	PNameLoadACL  = ps.Name("load-acl")
	ACLContextKey = util.ContextKey("acl")
)

func PLoadACL(pctx context.Context) (context.Context, error) {
	e := util.StringError("load acl")

	var log *logging.Logging
	var aclfrom ACLFlags
	var local base.LocalNode
	var encs *encoder.Encoders

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		ACLFlagsContextKey, &aclfrom,
		LocalContextKey, &local,
		EncodersContextKey, &encs,
	); err != nil {
		return pctx, e.Wrap(err)
	}

	var acl *YAMLACL

	switch i, err := NewACL(33, local.Publickey().String()); { //nolint:gomnd //...
	case err != nil:
		return pctx, e.Wrap(err)
	default:
		acl = NewYAMLACL(i)
	}

	if b := aclfrom.Flag.Body(); len(b) > 0 {
		log.Log().Debug().Msg("acl source found")

		if _, err := acl.Import(b, encs.JSON()); err != nil {
			return pctx, e.Wrap(err)
		}
	}

	return context.WithValue(pctx, ACLContextKey, acl), nil
}

func pACLAllowFunc(pctx context.Context) (ACLAllowFunc, error) {
	var acl *YAMLACL
	var eventLogging *EventLogging

	if err := util.LoadFromContextOK(pctx,
		ACLContextKey, &acl,
		EventLoggingContextKey, &eventLogging,
	); err != nil {
		return nil, err
	}

	switch i, found := eventLogging.Logger(ACLEventLogger); {
	case !found:
		return nil, errors.Errorf("acl event logger not found")
	default:
		return NewACLAllowFunc(acl.ACL, &i), nil
	}
}
