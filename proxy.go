// Package proxy defines the reverse proxy driver used by the node agent.
// The first implementation will be Caddy through its admin API.
package proxy

import "context"

type Route struct {
	Hostname   string
	PathPrefix string
	Upstream   string // host:port reachable from the proxy
	TLSMode    string // acme-http | acme-dns | custom | off
	RedirectTo string
	Static     *StaticRoot
	Headers    map[string]string
}

type StaticRoot struct {
	Root        string
	SPAFallback bool
}

type Domain struct {
	Hostname  string
	TLSMode   string
	CustomPEM []byte
}

type Driver interface {
	// SetRoutes replaces the full route table atomically.
	SetRoutes(ctx context.Context, routes []Route) error
	EnsureCertificates(ctx context.Context, domains []Domain) error
}
