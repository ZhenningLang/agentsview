//go:build !cgo

package duckdb

import (
	"database/sql"
	"fmt"
	"net"
	neturl "net/url"
	"strings"

	"go.kenn.io/agentsview/internal/config"
)

// Open is unavailable without cgo because the DuckDB Go driver requires cgo.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("duckdb path is required")
	}
	return nil, fmt.Errorf("duckdb support requires cgo")
}

func NewStoreFromConfig(cfg config.DuckDBConfig) (*Store, error) {
	if cfg.URL != "" {
		return NewQuackStore(cfg.URL, cfg.Token, cfg.AllowInsecure)
	}
	return NewStore(cfg.Path)
}

func NewQuackStore(rawURL, token string, allowInsecure bool) (*Store, error) {
	if err := ValidateQuackClientURL(rawURL, token, allowInsecure); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("duckdb quack support requires cgo")
}

func ValidateQuackClientURL(rawURL, token string, allowInsecure bool) error {
	if rawURL == "" {
		return fmt.Errorf("duckdb url is required")
	}
	if !strings.HasPrefix(rawURL, "quack:") {
		return fmt.Errorf("duckdb url must start with quack")
	}
	if token == "" {
		return fmt.Errorf("duckdb quack token is required")
	}
	transport := strings.TrimPrefix(rawURL, "quack:")
	if !strings.HasPrefix(transport, "http://") &&
		!strings.HasPrefix(transport, "https://") {
		host, err := quackURIHost(rawURL)
		if err != nil {
			return err
		}
		if !allowInsecure && !isLoopbackHost(host) {
			return fmt.Errorf(
				"duckdb native quack url host must be loopback unless allow_insecure is set",
			)
		}
		return nil
	}
	u, err := neturl.Parse(transport)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf(
			"duckdb quack url must include an http:// or https:// endpoint",
		)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("duckdb quack url must use http or https")
	}
	if u.Scheme == "http" && !allowInsecure && !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf(
			"duckdb quack url uses plain HTTP for a non-loopback host; use https or set allow_insecure",
		)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func duckLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func RedactQuackURL(rawURL string) string {
	transport := strings.TrimPrefix(rawURL, "quack:")
	u, err := neturl.Parse(transport)
	if err != nil {
		return "quack:<redacted>"
	}
	q := u.Query()
	for _, key := range []string{"token", "access_token", "auth"} {
		if q.Has(key) {
			q.Set(key, "<redacted>")
		}
	}
	u.RawQuery = q.Encode()
	return "quack:" + u.String()
}

func ValidateQuackServeURI(uri string, allowOtherHostname bool) error {
	if uri == "" {
		return fmt.Errorf("duckdb quack bind uri is required")
	}
	if !strings.HasPrefix(uri, "quack:") {
		return fmt.Errorf("duckdb quack bind uri must start with quack")
	}
	host, err := quackURIHost(uri)
	if err != nil {
		return err
	}
	if !allowOtherHostname && !isLoopbackHost(host) {
		return fmt.Errorf(
			"duckdb quack bind host must be loopback unless allow_insecure is set",
		)
	}
	return nil
}

func quackURIHost(uri string) (string, error) {
	rest := strings.TrimPrefix(uri, "quack:")
	if strings.HasPrefix(rest, "//") {
		rest = strings.TrimPrefix(rest, "//")
	}
	hostPort := rest
	if idx := strings.IndexAny(hostPort, "/?"); idx >= 0 {
		hostPort = hostPort[:idx]
	}
	if hostPort == "" {
		return "", fmt.Errorf("duckdb quack url must include a host")
	}
	host, _, err := net.SplitHostPort(hostPort)
	if err == nil {
		return strings.Trim(host, "[]"), nil
	}
	if strings.Count(hostPort, ":") > 1 {
		return strings.Trim(hostPort, "[]"), nil
	}
	return strings.Trim(hostPort, "[]"), nil
}
