package a2a

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type EndpointPolicy struct {
	AllowLoopbackHTTP bool
	AllowPrivateHosts bool
	AllowedHosts      []string
}

func validateEndpointSyntax(target *url.URL, policy EndpointPolicy) error {
	if target == nil || strings.TrimSpace(target.Hostname()) == "" {
		return fmt.Errorf("%w: endpoint host is required", ErrUnsafeEndpoint)
	}
	if target.User != nil {
		return fmt.Errorf("%w: endpoint userinfo is forbidden", ErrUnsafeEndpoint)
	}
	if target.Fragment != "" {
		return fmt.Errorf("%w: endpoint fragment is forbidden", ErrUnsafeEndpoint)
	}
	if target.RawQuery != "" {
		return fmt.Errorf("%w: endpoint query is forbidden", ErrUnsafeEndpoint)
	}
	scheme := strings.ToLower(strings.TrimSpace(target.Scheme))
	if scheme == "https" {
		return validateAllowedHost(target.Hostname(), policy.AllowedHosts)
	}
	if scheme == "http" && policy.AllowLoopbackHTTP && isLoopbackName(target.Hostname()) {
		return validateAllowedHost(target.Hostname(), policy.AllowedHosts)
	}
	return fmt.Errorf("%w: https is required", ErrUnsafeEndpoint)
}

func validateAllowedHost(host string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	host = strings.ToLower(strings.TrimSpace(host))
	for _, candidate := range allowed {
		if host == strings.ToLower(strings.TrimSpace(candidate)) {
			return nil
		}
	}
	return fmt.Errorf("%w: host is not allowlisted", ErrUnsafeEndpoint)
}

func validateNetworkEndpoint(ctx context.Context, target *url.URL, policy EndpointPolicy) error {
	if err := validateEndpointSyntax(target, policy); err != nil {
		return err
	}
	host := target.Hostname()
	if isLoopbackName(host) {
		if policy.AllowLoopbackHTTP || policy.AllowPrivateHosts {
			return nil
		}
		return fmt.Errorf("%w: private host is blocked", ErrUnsafeEndpoint)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateAddress(ip) && !policy.AllowPrivateHosts {
			return fmt.Errorf("%w: private host is blocked", ErrUnsafeEndpoint)
		}
		return nil
	}
	addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("%w: resolve endpoint host", ErrUnsafeEndpoint)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("%w: endpoint host has no addresses", ErrUnsafeEndpoint)
	}
	for _, address := range addresses {
		if isPrivateAddress(address) && !policy.AllowPrivateHosts {
			return fmt.Errorf("%w: private host is blocked", ErrUnsafeEndpoint)
		}
	}
	return nil
}

func isLoopbackName(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isPrivateAddress(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

func sameHost(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}
