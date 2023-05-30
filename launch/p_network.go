package launch

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	"github.com/ProtoconNet/mitum2/base"
	isaacnetwork "github.com/ProtoconNet/mitum2/isaac/network"
	"github.com/ProtoconNet/mitum2/network/quicstream"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/logging"
	"github.com/ProtoconNet/mitum2/util/ps"
	"github.com/lucas-clemente/quic-go"
	"github.com/pkg/errors"
)

var (
	PNameNetwork                 = ps.Name("network")
	PNameStartNetwork            = ps.Name("start-network")
	PNameQuicstreamClient        = ps.Name("network-client")
	QuicstreamClientContextKey   = util.ContextKey("network-client")
	QuicstreamServerContextKey   = util.ContextKey("quicstream-server")
	QuicstreamHandlersContextKey = util.ContextKey("quicstream-handlers")
)

func PQuicstreamClient(pctx context.Context) (context.Context, error) {
	var encs *encoder.Encoders
	var enc encoder.Encoder
	var params base.LocalParams

	if err := util.LoadFromContextOK(pctx,
		EncodersContextKey, &encs,
		EncoderContextKey, &enc,
		LocalParamsContextKey, &params,
	); err != nil {
		return pctx, errors.WithMessage(err, "failed network client")
	}

	client := NewNetworkClient(encs, enc, time.Second*2, params.NetworkID()) //nolint:gomnd //...

	return context.WithValue(pctx, QuicstreamClientContextKey, client), nil
}

func PNetwork(pctx context.Context) (context.Context, error) {
	e := util.StringErrorFunc("failed to prepare network")

	var log *logging.Logging
	var encs *encoder.Encoders
	var enc encoder.Encoder
	var design NodeDesign
	var params base.LocalParams

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		EncodersContextKey, &encs,
		EncoderContextKey, &enc,
		DesignContextKey, &design,
		LocalParamsContextKey, &params,
	); err != nil {
		return pctx, e(err, "")
	}

	handlers := quicstream.NewPrefixHandler(isaacnetwork.QuicstreamErrorHandler(enc))

	quicconfig := DefaultQuicConfig()
	quicconfig.RequireAddressValidation = func(net.Addr) bool {
		return true // TODO NOTE manage blacklist
	}

	server, err := quicstream.NewServer(
		design.Network.Bind,
		GenerateNewTLSConfig(params.NetworkID()),
		quicconfig,
		handlers.Handler,
	)
	if err != nil {
		return pctx, err
	}

	_ = server.SetLogging(log)

	pctx = context.WithValue(pctx, QuicstreamServerContextKey, server)     //revive:disable-line:modifies-parameter
	pctx = context.WithValue(pctx, QuicstreamHandlersContextKey, handlers) //revive:disable-line:modifies-parameter

	return pctx, nil
}

func PStartNetwork(pctx context.Context) (context.Context, error) {
	var server *quicstream.Server
	if err := util.LoadFromContextOK(pctx, QuicstreamServerContextKey, &server); err != nil {
		return pctx, err
	}

	return pctx, server.Start(context.Background())
}

func PCloseNetwork(pctx context.Context) (context.Context, error) {
	var server *quicstream.Server
	if err := util.LoadFromContext(pctx, QuicstreamServerContextKey, &server); err != nil {
		return pctx, err
	}

	if server != nil {
		if err := server.Stop(); err != nil && !errors.Is(err, util.ErrDaemonAlreadyStopped) {
			return pctx, err
		}
	}

	return pctx, nil
}

func NewNetworkClient(
	encs *encoder.Encoders,
	enc encoder.Encoder,
	idleTimeout time.Duration,
	networkID base.NetworkID,
) *isaacnetwork.QuicstreamClient {
	return isaacnetwork.NewQuicstreamClient(encs, enc, idleTimeout, string(networkID), DefaultQuicConfig())
}

func GenerateNewTLSConfig(networkID base.NetworkID) *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024) //nolint:gomnd //...
	if err != nil {
		panic(err)
	}

	template := x509.Certificate{SerialNumber: big.NewInt(1)}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{string(networkID)},
	}
}

func DefaultQuicConfig() *quic.Config {
	return &quic.Config{
		HandshakeIdleTimeout: time.Second * 2,  //nolint:gomnd //...
		MaxIdleTimeout:       time.Second * 30, //nolint:gomnd //...
		KeepAlivePeriod:      time.Second * 6,  //nolint:gomnd //...
	}
}
