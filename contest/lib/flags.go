package contestlib

import (
	"bytes"

	"github.com/rs/zerolog"
	"golang.org/x/xerrors"

	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
)

type LogLevel zerolog.Level

func (ll LogLevel) Zero() zerolog.Level {
	return zerolog.Level(ll)
}

func (ll LogLevel) MarshalJSON() ([]byte, error) {
	return jsonenc.Marshal(zerolog.Level(ll).String())
}

func (ll *LogLevel) UnmarshalText(b []byte) error {
	lvl, err := zerolog.ParseLevel(string(b))
	if err != nil {
		return err
	}

	*ll = LogLevel(lvl)

	return nil
}

type LogFormat string

func (lf *LogFormat) UnmarshalText(b []byte) error {
	s := string(bytes.TrimSpace(bytes.ToLower(b)))
	switch s {
	case "json":
	case "terminal":
	default:
		return xerrors.Errorf("invalid log_format: %q", s)
	}

	*lf = LogFormat(s)

	return nil
}
