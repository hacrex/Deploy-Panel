// Package site holds the Site domain model, the single abstraction for every
// workload the panel hosts. See docs/SITE_DATA_MODEL.md.
package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Type decides which Spec shape is valid.
type Type string

const (
	TypeContainer Type = "container"
	TypeCompose   Type = "compose"
	TypePHP       Type = "php"
	TypeWordPress Type = "wordpress"
	TypeNode      Type = "node"
	TypePython    Type = "python"
	TypeStatic    Type = "static"
	TypeProxy     Type = "proxy"
)

// State is the observed lifecycle state of a Site.
type State string

const (
	StateDraft        State = "draft"
	StateProvisioning State = "provisioning"
	StateRunning      State = "running"
	StateStopped      State = "stopped"
	StateDegraded     State = "degraded"
	StateFailed       State = "failed"
	StateSuspended    State = "suspended"
	StateDeleting     State = "deleting"
	StateDeleted      State = "deleted"
)

// DesiredState is what a user or admin asked for.
type DesiredState string

const (
	DesiredRunning DesiredState = "running"
	DesiredStopped DesiredState = "stopped"
	DesiredDeleted DesiredState = "deleted"
)

// Limits are per-Site resource caps, bounded by the owning team's Package.
type Limits struct {
	CPUMilli int `json:"cpu_milli,omitempty"`
	MemMB    int `json:"mem_mb,omitempty"`
	Pids     int `json:"pids,omitempty"`
	DiskMB   int `json:"disk_mb,omitempty"`
	IOWeight int `json:"io_weight,omitempty"`
}

// Site is the unit everything else attaches to.
type Site struct {
	ID           string
	TeamID       string
	NodeID       string
	ParentSiteID string // staging Site points at its production Site
	Slug         string
	Name         string
	Type         Type
	State        State
	Desired      DesiredState
	Spec         json.RawMessage // shape depends on Type, see ParseSpec
	SpecRevision int64
	Limits       Limits
	SystemUser   string // per-site Linux user (php, wordpress)
	SystemUID    int
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

// Domain routes a hostname to a Site.
type Domain struct {
	ID         string
	SiteID     string
	Hostname   string
	IsPrimary  bool
	RedirectTo string
	TLSMode    string // acme-http | acme-dns | custom | off
	PathPrefix string
}

// ---- Spec shapes, one per Type -------------------------------------------

type HealthCheck struct {
	Path     string   `json:"path,omitempty"`
	Command  []string `json:"command,omitempty"`
	Interval string   `json:"interval,omitempty"`
	Timeout  string   `json:"timeout,omitempty"`
	Retries  int      `json:"retries,omitempty"`
}

type BuildSpec struct {
	Context    string `json:"context,omitempty"`
	Dockerfile string `json:"dockerfile,omitempty"`
	Buildpack  string `json:"buildpack,omitempty"`
}

type ContainerSpec struct {
	Image       string       `json:"image,omitempty"`
	Build       *BuildSpec   `json:"build,omitempty"`
	Port        int          `json:"port"`
	Command     []string     `json:"command,omitempty"`
	HealthCheck *HealthCheck `json:"healthcheck,omitempty"`
	Replicas    int          `json:"replicas,omitempty"`
}

type ComposeSpec struct {
	File           string `json:"file"`
	PrimaryService string `json:"primary_service"`
	PrimaryPort    int    `json:"primary_port"`
}

type PHPSpec struct {
	PHPVersion string            `json:"php_version"`
	DocRoot    string            `json:"docroot,omitempty"`
	Extensions []string          `json:"extensions,omitempty"`
	INI        map[string]string `json:"ini,omitempty"`
	FPM        *FPMSpec          `json:"fpm,omitempty"`
}

type FPMSpec struct {
	Mode        string `json:"mode,omitempty"` // ondemand | dynamic | static
	MaxChildren int    `json:"max_children,omitempty"`
}

type WordPressSpec struct {
	PHPSpec
	WP         WPOptions `json:"wp"`
	DatabaseID string    `json:"database_id,omitempty"`
}

type WPOptions struct {
	Version    string   `json:"version,omitempty"`
	Multisite  bool     `json:"multisite,omitempty"`
	AutoUpdate string   `json:"auto_update,omitempty"` // none | minor | all
	Cache      []string `json:"cache,omitempty"`       // page | object
}

type RuntimeAppSpec struct { // node and python
	RuntimeVersion string       `json:"runtime_version,omitempty"`
	Build          string       `json:"build,omitempty"`
	Start          string       `json:"start"`
	Port           int          `json:"port"`
	HealthCheck    *HealthCheck `json:"healthcheck,omitempty"`
}

type StaticSpec struct {
	Root        string            `json:"root,omitempty"`
	SPAFallback bool              `json:"spa_fallback,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type ProxySpec struct {
	Upstream     string            `json:"upstream"` // Site id or URL
	PreserveHost bool              `json:"preserve_host,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
}

// ErrInvalidSpec is returned when a spec does not match its Site type.
var ErrInvalidSpec = errors.New("invalid site spec")

// ParseSpec decodes raw into the struct for t, rejecting unknown fields, and
// runs type-specific validation. It returns the typed spec.
func ParseSpec(t Type, raw json.RawMessage) (any, error) {
	var out any
	switch t {
	case TypeContainer:
		out = &ContainerSpec{}
	case TypeCompose:
		out = &ComposeSpec{}
	case TypePHP:
		out = &PHPSpec{}
	case TypeWordPress:
		out = &WordPressSpec{}
	case TypeNode, TypePython:
		out = &RuntimeAppSpec{}
	case TypeStatic:
		out = &StaticSpec{}
	case TypeProxy:
		out = &ProxySpec{}
	default:
		return nil, fmt.Errorf("%w: unknown type %q", ErrInvalidSpec, t)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
	}
	if err := validate(out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
	}
	return out, nil
}

func validate(spec any) error {
	switch s := spec.(type) {
	case *ContainerSpec:
		if s.Image == "" && s.Build == nil {
			return errors.New("container needs image or build")
		}
		if s.Port < 0 || s.Port > 65535 {
			return errors.New("port out of range")
		}
	case *ComposeSpec:
		if s.File == "" || s.PrimaryService == "" {
			return errors.New("compose needs file and primary_service")
		}
	case *PHPSpec:
		if s.PHPVersion == "" {
			return errors.New("php_version is required")
		}
	case *WordPressSpec:
		if s.PHPVersion == "" {
			return errors.New("php_version is required")
		}
	case *RuntimeAppSpec:
		if s.Start == "" {
			return errors.New("start command is required")
		}
	case *ProxySpec:
		if s.Upstream == "" {
			return errors.New("upstream is required")
		}
	}
	return nil
}

// allowed lists legal observed-state transitions.
var allowed = map[State][]State{
	StateDraft:        {StateProvisioning, StateDeleting},
	StateProvisioning: {StateRunning, StateFailed, StateDeleting},
	StateRunning:      {StateStopped, StateDegraded, StateSuspended, StateDeleting, StateProvisioning},
	StateStopped:      {StateRunning, StateProvisioning, StateSuspended, StateDeleting},
	StateDegraded:     {StateRunning, StateStopped, StateFailed, StateSuspended, StateDeleting},
	StateFailed:       {StateProvisioning, StateDeleting},
	StateSuspended:    {StateStopped, StateRunning, StateDeleting},
	StateDeleting:     {StateDeleted},
}

// CanTransition reports whether from -> to is a legal state change.
func CanTransition(from, to State) bool {
	for _, s := range allowed[from] {
		if s == to {
			return true
		}
	}
	return false
}
