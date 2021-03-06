package config

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/ironsmile/nedomi/types"
)

const defaultMaxIOTranferSize = 1024 * 1024 // 1m
const defaultMinIOTranferSize = 1024 * 128  // 128k

// BaseHTTP contains the basic configuration options for HTTP.
type BaseHTTP struct {
	HeadersRewrite
	Listen            string                     `json:"listen"`
	Upstreams         map[string]json.RawMessage `json:"upstreams"`
	Servers           map[string]json.RawMessage `json:"virtual_hosts"`
	MaxHeadersSize    int                        `json:"max_headers_size"`
	MinIOTransferSize types.BytesSize            `json:"min_io_transfer_size"`
	MaxIOTransferSize types.BytesSize            `json:"max_io_transfer_size"`
	ReadTimeout       uint32                     `json:"read_timeout"`
	WriteTimeout      uint32                     `json:"write_timeout"`

	// Defaults for vhosts:
	DefaultHandlers  []Handler `json:"default_handlers"`
	DefaultCacheZone string    `json:"default_cache_zone"`
	AccessLog        string    `json:"access_log"`
	Logger           Logger    `json:"logger"`
}

// HTTP contains all configuration options for HTTP.
type HTTP struct {
	BaseHTTP
	Upstreams []*Upstream
	Servers   []*VirtualHost
	parent    *Config
}

// UnmarshalJSON is a custom JSON unmashalling that also implements inheritance,
// custom field initiation and data validation for the HTTP config.
func (h *HTTP) UnmarshalJSON(buff []byte) error {
	if err := json.Unmarshal(buff, &h.BaseHTTP); err != nil {
		return err
	}

	// Parse all the upstreams
	for key, upstreamBuff := range h.BaseHTTP.Upstreams {
		upstream := &Upstream{ID: key, Settings: GetDefaultUpstreamSettings()}
		if err := json.Unmarshal(upstreamBuff, upstream); err != nil {
			return err
		}
		h.Upstreams = append(h.Upstreams, upstream)
	}

	// Parse all the vhosts
	for key, vhostBuff := range h.BaseHTTP.Servers {
		vhost := newVHostFromHTTP(h)
		vhost.Name = key
		if err := json.Unmarshal(vhostBuff, &vhost); err != nil {
			return err
		}
		h.Servers = append(h.Servers, &vhost)
	}

	h.BaseHTTP.Servers = nil      // Cleanup
	if h.MaxIOTransferSize <= 0 { // set default
		h.MaxIOTransferSize = defaultMaxIOTranferSize
	}
	if h.MinIOTransferSize <= 0 { // set default
		h.MinIOTransferSize = defaultMinIOTranferSize
	}
	return nil
}

// Validate checks the HTTP config for logical errors.
func (h *HTTP) Validate() error {

	if h.Listen == "" {
		return errors.New("Empty `http.listen` directive")
	}

	if len(h.Servers) == 0 {
		return errors.New("There has to be at least one virtual host")
	}

	//!TODO: make sure Listen is valid tcp address
	if _, err := net.ResolveTCPAddr("tcp", h.Listen); err != nil {
		return err
	}

	return nil
}

// GetSubsections returns a slice with all the subsections of the HTTP config.
func (h *HTTP) GetSubsections() []Section {
	res := []Section{h.Logger}

	for _, handler := range h.DefaultHandlers {
		res = append(res, handler)
	}
	for _, s := range h.Servers {
		res = append(res, s)
	}
	return res
}
