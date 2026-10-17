package updater

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// netErr turns an HTTP client error into one short sentence for the UI.
// Go's errors include the full request URL, and GitHub's download URLs are
// long signed links that fill the Settings page.
func netErr(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	host := ue.URL
	if u, perr := url.Parse(ue.URL); perr == nil && u.Host != "" {
		host = u.Host
	}
	return fmt.Errorf("couldn't reach %s: %s", host, reason(ue.Err))
}

func reason(err error) string {
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	var unknownCA x509.UnknownAuthorityError
	switch {
	case errors.As(err, &dnsErr):
		return "no internet connection (the address could not be looked up)"
	case errors.As(err, &certErr), errors.As(err, &unknownCA):
		return "its security certificate was not accepted; a firewall or antivirus may be inspecting the connection"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "tls handshake"), strings.Contains(msg, "connection reset"), strings.Contains(msg, "forcibly closed"):
		return "the secure connection was cut off; a firewall, antivirus or network filter may be blocking it"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline exceeded"):
		return "the connection timed out"
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "unreachable"):
		return "the connection was refused"
	case strings.Contains(msg, "context canceled"):
		return "canceled"
	}
	return "a network error occurred"
}
