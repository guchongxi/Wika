package safefetch

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type Validator struct {
	resolver Resolver
}

type FetchTarget struct {
	URL *url.URL
	IPs []net.IP
}

func NewValidator(resolver Resolver) *Validator {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Validator{resolver: resolver}
}

func (v *Validator) ValidateURL(ctx context.Context, raw string) (*url.URL, error) {
	target, err := v.ValidateFetchTarget(ctx, raw)
	if err != nil {
		return nil, err
	}
	return target.URL, nil
}

func (v *Validator) ValidateFetchTarget(ctx context.Context, raw string) (*FetchTarget, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("unsupported url scheme")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("url userinfo is not allowed")
	}
	host, err := normalizeHost(parsed.Hostname())
	if err != nil {
		return nil, err
	}
	if host == "" {
		return nil, fmt.Errorf("url host is required")
	}
	if isBlockedHostname(host) {
		return nil, fmt.Errorf("blocked hostname")
	}
	if isObfuscatedIPLikeHost(host) {
		return nil, fmt.Errorf("ip-like hostname is not allowed")
	}
	var resolvedIPs []net.IP
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("blocked ip address")
		}
		resolvedIPs = append(resolvedIPs, ip)
	} else {
		ips, err := v.resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("dns lookup failed: %w", err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("dns lookup returned no addresses")
		}
		for _, item := range ips {
			if isBlockedIP(item.IP) {
				return nil, fmt.Errorf("hostname resolves to blocked ip")
			}
			resolvedIPs = append(resolvedIPs, item.IP)
		}
	}
	parsed.Scheme = scheme
	parsed.Host = hostWithPort(host, parsed.Port())
	return &FetchTarget{URL: parsed, IPs: resolvedIPs}, nil
}

func normalizeHost(host string) (string, error) {
	normalized := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if normalized == "" || net.ParseIP(normalized) != nil {
		return normalized, nil
	}
	ascii, err := idna.Lookup.ToASCII(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid idna hostname: %w", err)
	}
	return strings.ToLower(ascii), nil
}

func hostWithPort(host string, port string) string {
	if port == "" {
		return host
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + port
	}
	return host + ":" + port
}

func isBlockedHostname(host string) bool {
	switch host {
	case "localhost",
		"metadata.google.internal",
		"metadata.aws.internal",
		"metadata.tencentyun.com",
		"host.docker.internal",
		"gateway.docker.internal",
		"kubernetes.docker.internal",
		"kubernetes",
		"kubernetes.default",
		"kubernetes.default.svc",
		"kubernetes.default.svc.cluster.local":
		return true
	}
	return strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".corp") ||
		strings.HasSuffix(host, ".lan") ||
		strings.HasSuffix(host, ".home") ||
		strings.HasSuffix(host, ".localdomain") ||
		strings.HasSuffix(host, ".svc.cluster.local") ||
		strings.HasSuffix(host, ".pod.cluster.local")
}

func isObfuscatedIPLikeHost(host string) bool {
	if host == "" || net.ParseIP(host) != nil {
		return false
	}
	if strings.HasPrefix(host, "0x") {
		return true
	}
	if allASCII(host, func(r rune) bool { return r >= '0' && r <= '9' }) {
		return true
	}
	parts := strings.Split(host, ".")
	if len(parts) == 4 {
		allNumeric := true
		for _, part := range parts {
			if strings.HasPrefix(part, "0x") {
				return true
			}
			if part == "" || !allASCII(part, func(r rune) bool { return r >= '0' && r <= '9' }) {
				allNumeric = false
				break
			}
			if len(part) > 1 && strings.HasPrefix(part, "0") {
				return true
			}
		}
		return allNumeric
	}
	return false
}

func allASCII(value string, allow func(rune) bool) bool {
	for _, r := range value {
		if !allow(r) {
			return false
		}
	}
	return value != ""
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() ||
		ip.IsInterfaceLocalMulticast() {
		return true
	}
	for _, restricted := range restrictedIPRanges {
		if restricted.Contains(ip) {
			return true
		}
	}
	return false
}

var restrictedIPRanges = []*net.IPNet{
	mustParseCIDR("0.0.0.0/8"),
	mustParseCIDR("100.64.0.0/10"),
	mustParseCIDR("192.0.0.0/24"),
	mustParseCIDR("192.0.2.0/24"),
	mustParseCIDR("198.18.0.0/15"),
	mustParseCIDR("198.51.100.0/24"),
	mustParseCIDR("203.0.113.0/24"),
	mustParseCIDR("240.0.0.0/4"),
	mustParseCIDR("255.255.255.255/32"),
}

func mustParseCIDR(value string) *net.IPNet {
	_, parsed, err := net.ParseCIDR(value)
	if err != nil {
		panic(fmt.Sprintf("invalid cidr %s: %v", value, err))
	}
	return parsed
}
