package util

import (
	"net"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

func IsValidAddr(s string) error {
	if len(s) < 1 {
		return errors.Errorf("empty publish")
	}

	switch host, port, err := net.SplitHostPort(s); {
	case err != nil:
		return errors.WithStack(err)
	case len(host) < 1:
		return errors.Errorf("empty host")
	case len(port) < 1:
		return errors.Errorf("empty port")
	}

	return nil
}

var DefaultTLSInsecureFlag = "tls_insecure"

func HasTLSInsecure(s, flag string) bool {
	v, err := url.ParseQuery(s)
	if err != nil {
		return false
	}

	return v.Has(flag)
}

func ParseTLSInsecure(s string) (string, bool) {
	switch i := strings.Index(s, "#"); {
	case i < 0:
		return s, false
	case len(s[i:]) > 0:
		return s[:i], HasTLSInsecure(s[i+1:], DefaultTLSInsecureFlag)
	default:
		return s[:i], false
	}
}

func ConnInfoToString(addr string, tlsinsecure bool) string { // revive:disable-line:flag-parameter
	ti := ""
	if tlsinsecure {
		ti = "#tls_insecure"
	}

	return addr + ti
}
