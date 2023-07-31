package isaacnetwork

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	isaacstates "github.com/spikeekips/mitum/isaac/states"
	quicstreamheader "github.com/spikeekips/mitum/network/quicstream/header"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
)

func QuicstreamHandlerStartHandover(
	local base.Node,
	networkID base.NetworkID,
	f isaacstates.StartHandoverYFunc,
) quicstreamheader.Handler[StartHandoverHeader] {
	return func(
		ctx context.Context, addr net.Addr, broker *quicstreamheader.HandlerBroker, header StartHandoverHeader,
	) (context.Context, error) {
		err := quicstreamHandlerVerifyNode(
			ctx, addr, broker,
			local.Publickey(), networkID,
		)

		if err == nil {
			err = f(ctx, header.Address(), header.ConnInfo())
		}

		return ctx, broker.WriteResponseHeadOK(ctx, err == nil, err)
	}
}

func QuicstreamHandlerCancelHandover(
	local base.Node,
	networkID base.NetworkID,
	f func() error,
) quicstreamheader.Handler[CancelHandoverHeader] {
	return func(
		ctx context.Context, addr net.Addr, broker *quicstreamheader.HandlerBroker, header CancelHandoverHeader,
	) (context.Context, error) {
		err := quicstreamHandlerVerifyNode(
			ctx, addr, broker,
			local.Publickey(), networkID,
		)

		if err == nil {
			err = f()
		}

		return ctx, broker.WriteResponseHeadOK(ctx, err == nil, err)
	}
}

func QuicstreamHandlerCheckHandover(
	local base.Node,
	networkID base.NetworkID,
	f isaacstates.CheckHandoverFunc,
) quicstreamheader.Handler[CheckHandoverHeader] {
	return func(
		ctx context.Context, addr net.Addr, broker *quicstreamheader.HandlerBroker, header CheckHandoverHeader,
	) (context.Context, error) {
		err := quicstreamHandlerVerifyNode(
			ctx, addr, broker,
			local.Publickey(), networkID,
		)

		if err == nil {
			err = f(ctx, header.Address(), header.ConnInfo())
		}

		return ctx, broker.WriteResponseHeadOK(ctx, err == nil, err)
	}
}

func QuicstreamHandlerAskHandover(
	local base.Node,
	networkID base.NetworkID,
	f isaacstates.AskHandoverReceivedFunc,
) quicstreamheader.Handler[AskHandoverHeader] {
	return func(
		ctx context.Context, addr net.Addr, broker *quicstreamheader.HandlerBroker, header AskHandoverHeader,
	) (context.Context, error) {
		if err := quicstreamHandlerVerifyNode(
			ctx, addr, broker,
			local.Publickey(), networkID,
		); err != nil {
			return ctx, err
		}

		id, canMoveConsensus, err := f(ctx, header.Address(), header.ConnInfo())

		return ctx, broker.WriteResponseHead(ctx, NewAskHandoverResponseHeader(canMoveConsensus, err, id))
	}
}

func QuicstreamHandlerHandoverMessage(
	networkID base.NetworkID,
	f func(isaacstates.HandoverMessage) error,
) quicstreamheader.Handler[HandoverMessageHeader] {
	return func(
		ctx context.Context, addr net.Addr, broker *quicstreamheader.HandlerBroker, header HandoverMessageHeader,
	) (context.Context, error) {
		e := util.StringError("handover message")

		var msg isaacstates.HandoverMessage

		switch _, _, body, err := broker.ReadBodyErr(ctx); {
		case err != nil:
			return ctx, e.Wrap(err)
		case body == nil:
			return ctx, e.Errorf("empty body")
		default:
			switch i, err := io.ReadAll(body); {
			case err != nil:
				return ctx, e.Wrap(err)
			default:
				if err = encoder.Decode(broker.Encoder, i, &msg); err != nil {
					return ctx, e.Wrap(err)
				}

				if msg == nil {
					return ctx, e.Errorf("empty handover message")
				}

				if err = msg.IsValid(networkID); err != nil {
					return ctx, e.Wrap(err)
				}
			}
		}

		err := f(msg)

		return ctx, broker.WriteResponseHeadOK(ctx, err == nil, err)
	}
}

func QuicstreamHandlerCheckHandoverX(
	local base.Node,
	networkID base.NetworkID,
	f isaacstates.CheckHandoverXFunc,
) quicstreamheader.Handler[CheckHandoverXHeader] {
	return func(
		ctx context.Context, addr net.Addr, broker *quicstreamheader.HandlerBroker, header CheckHandoverXHeader,
	) (context.Context, error) {
		err := quicstreamHandlerVerifyNode(
			ctx, addr, broker,
			local.Publickey(), networkID,
		)

		if err == nil {
			err = f(ctx)
		}

		return ctx, broker.WriteResponseHeadOK(ctx, err == nil, err)
	}
}

func QuicstreamHandlerLastHandoverYLogs(
	local base.Node,
	networkID base.NetworkID,
	f func() []json.RawMessage,
) quicstreamheader.Handler[LastHandoverYLogsHeader] {
	return func(
		ctx context.Context, addr net.Addr, broker *quicstreamheader.HandlerBroker, header LastHandoverYLogsHeader,
	) (context.Context, error) {
		err := quicstreamHandlerVerifyNode(
			ctx, addr, broker,
			local.Publickey(), networkID,
		)

		if eerr := broker.WriteResponseHeadOK(ctx, err == nil, err); err != nil || eerr != nil {
			return ctx, nil
		}

		body := bytes.NewBuffer(nil)
		defer body.Reset()

		s := f()

		for i := range s {
			if _, err := body.Write(append(s[i], '\n')); err != nil {
				return ctx, errors.WithStack(err)
			}
		}

		return ctx, broker.WriteBody(ctx, quicstreamheader.StreamBodyType, 0, body)
	}
}
