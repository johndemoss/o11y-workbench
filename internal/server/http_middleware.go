package server

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

func setSecurityResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityResponseHeaders(w)
		next.ServeHTTP(w, r)
	})
}

type hostPolicy struct {
	exact        map[string]struct{}
	wildcardBind bool
}

func validateListenAddr(addr string) error {
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("empty listen address")
	}
	host, _, err := splitListenAddr(addr)
	if err != nil {
		return err
	}
	if host != "" {
		if _, err := validateHostEntry(host); err != nil {
			return fmt.Errorf("listen host %q: %w", host, err)
		}
	}
	return nil
}

func splitListenAddr(addr string) (host, port string, err error) {
	host, port, err = net.SplitHostPort(addr)
	if err != nil {
		if strings.Count(addr, ":") == 1 && strings.HasPrefix(addr, ":") {
			return "", strings.TrimPrefix(addr, ":"), nil
		}
		return "", "", fmt.Errorf("parse listen address %q: %w", addr, err)
	}
	host = strings.Trim(host, "[]")
	return host, port, nil
}

func listenAddrWildcard(addr string) (bool, error) {
	host, _, err := splitListenAddr(addr)
	if err != nil {
		return false, err
	}
	return host == "" || host == "0.0.0.0" || host == "::", nil
}

func buildHostPolicy(listenAddr string, allowedHosts []string) (*hostPolicy, error) {
	if err := validateListenAddr(listenAddr); err != nil {
		return nil, err
	}
	normalized, err := normalizeAllowedHostEntries(allowedHosts)
	if err != nil {
		return nil, err
	}
	wildcard, err := listenAddrWildcard(listenAddr)
	if err != nil {
		return nil, err
	}
	bindHost, _, err := splitListenAddr(listenAddr)
	if err != nil {
		return nil, err
	}
	p := &hostPolicy{
		exact:        make(map[string]struct{}),
		wildcardBind: wildcard,
	}
	if bindHost != "" && !wildcard {
		key, err := canonicalHostKey(bindHost)
		if err != nil {
			return nil, fmt.Errorf("listen host: %w", err)
		}
		p.exact[key] = struct{}{}
	}
	for _, h := range normalized {
		p.exact[h] = struct{}{}
	}
	return p, nil
}

func normalizeAllowedHostEntries(allowedHosts []string) ([]string, error) {
	out := make([]string, 0, len(allowedHosts))
	for i, raw := range allowedHosts {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			return nil, fmt.Errorf("allowed host entry %d: empty", i)
		}
		key, err := validateHostEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("allowed host entry %q: %w", entry, err)
		}
		out = append(out, key)
	}
	return out, nil
}

func validateHostEntry(entry string) (string, error) {
	if strings.ContainsAny(entry, "/\\") {
		return "", fmt.Errorf("path not allowed")
	}
	if strings.Contains(entry, "://") {
		return "", fmt.Errorf("scheme not allowed")
	}
	if strings.Contains(entry, "*") {
		return "", fmt.Errorf("wildcards not allowed")
	}
	if _, _, err := net.SplitHostPort(entry); err == nil {
		return "", fmt.Errorf("port not allowed in host entry")
	}
	if strings.HasPrefix(entry, "[") {
		if !strings.HasSuffix(entry, "]") {
			return "", fmt.Errorf("malformed bracketed IPv6")
		}
		inner := strings.TrimPrefix(strings.TrimSuffix(entry, "]"), "[")
		ip := net.ParseIP(inner)
		if ip == nil {
			return "", fmt.Errorf("malformed IP literal")
		}
		return canonicalIPKey(ip), nil
	}
	if net.ParseIP(entry) != nil {
		return canonicalIPKey(net.ParseIP(entry)), nil
	}
	if !validDNSNameASCII(entry) {
		return "", fmt.Errorf("malformed hostname")
	}
	return strings.ToLower(strings.TrimSuffix(entry, ".")), nil
}

func validDNSNameASCII(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	labels := strings.Split(name, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return false
		}
		for i, c := range label {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
				if c == '-' && (i == 0 || i == len(label)-1) {
					return false
				}
				continue
			}
			return false
		}
	}
	return true
}

func canonicalIPKey(ip net.IP) string {
	if ip.To4() != nil {
		return ip.To4().String()
	}
	return strings.ToLower(ip.String())
}

func canonicalHostKey(host string) (string, error) {
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return canonicalIPKey(ip), nil
	}
	if !validDNSNameASCII(host) {
		return "", fmt.Errorf("malformed hostname %q", host)
	}
	return strings.ToLower(strings.TrimSuffix(host, ".")), nil
}

func hostHeaderAllowedChars(host string) bool {
	for _, c := range host {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-', c == '.', c == ':', c == '[', c == ']':
		default:
			return false
		}
	}
	return true
}

func parseRequestHost(raw string) (hostKey string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty host")
	}
	if !hostHeaderAllowedChars(raw) {
		return "", fmt.Errorf("disallowed characters in host")
	}
	var hostPart, portPart string
	if strings.HasPrefix(raw, "[") {
		close := strings.Index(raw, "]")
		if close < 0 {
			return "", fmt.Errorf("unclosed bracketed IPv6")
		}
		hostPart = raw[1:close]
		rest := raw[close+1:]
		if rest == "" {
			portPart = ""
		} else if !strings.HasPrefix(rest, ":") {
			return "", fmt.Errorf("malformed host after IPv6")
		} else {
			portPart = rest[1:]
		}
	} else {
		if h, p, splitErr := net.SplitHostPort(raw); splitErr == nil {
			hostPart, portPart = h, p
		} else {
			hostPart = raw
		}
	}
	if portPart != "" {
		if !validPortSyntax(portPart) {
			return "", fmt.Errorf("invalid port")
		}
	}
	hostPart = strings.Trim(hostPart, "[]")
	if hostPart == "" {
		return "", fmt.Errorf("empty host")
	}
	return canonicalHostKey(hostPart)
}

func validPortSyntax(port string) bool {
	if port == "" {
		return false
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func isLoopbackHostKey(key string) bool {
	if key == "localhost" {
		return true
	}
	ip := net.ParseIP(key)
	return ip != nil && ip.IsLoopback()
}

func (p *hostPolicy) allowedRequestHost(hostKey string) bool {
	if isLoopbackHostKey(hostKey) {
		return true
	}
	if _, ok := p.exact[hostKey]; ok {
		return true
	}
	if p.wildcardBind {
		if ip := net.ParseIP(hostKey); ip != nil {
			return true
		}
	}
	return false
}

func validateHost(listenAddr string, allowedHosts []string, next http.Handler) http.Handler {
	policy, err := buildHostPolicy(listenAddr, allowedHosts)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "misconfigured host policy", http.StatusInternalServerError)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hostKey, err := parseRequestHost(r.Host)
		if err != nil || !policy.allowedRequestHost(hostKey) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
