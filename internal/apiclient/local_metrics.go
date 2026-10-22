package apiclient

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
)

// ValidateLocalMetricsURL keeps local collectors away from DNS, proxies and
// arbitrary URLs. Redirects must also be disabled by the caller.
func ValidateLocalMetricsURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid local metrics URL")
	}
	address, err := netip.ParseAddr(u.Hostname())
	port, portErr := strconv.Atoi(u.Port())
	if err != nil || !address.IsLoopback() || portErr != nil || port < 1 || port > 65535 || u.Scheme != "http" || u.User != nil || u.Path != "/metrics" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errors.New("loopback HTTP /metrics URL with explicit port required")
	}
	return nil
}
