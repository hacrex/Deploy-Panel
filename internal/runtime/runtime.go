// Package runtime defines the driver interface the node agent uses to run Sites.
// The first implementation will be a Docker driver; Swarm and K3s come later.
package runtime

import (
	"context"
	"io"
	"time"
)

// SiteRuntimeSpec is the resolved, node-local description of what to run.
// The control plane derives it from a Site spec, limits, env and volumes.
type SiteRuntimeSpec struct {
	SiteID         string
	Revision       int64
	Image          string
	Command        []string
	Env            map[string]string
	Port           int
	Volumes        []VolumeMount
	Limits         Limits
	UID            int
	ReadOnlyRootFS bool
	Network        string
	Health         *Health
}

type VolumeMount struct {
	Name      string
	MountPath string
}

type Limits struct {
	CPUMilli int
	MemMB    int
	Pids     int
	IOWeight int
}

type Health struct {
	Path     string
	Interval time.Duration
	Timeout  time.Duration
	Retries  int
}

type ApplyResult struct {
	ContainerIDs []string
	Ready        bool
}

type LogOptions struct {
	Follow bool
	Tail   int
	Since  time.Time
}

type StatsSample struct {
	At       time.Time
	CPUMilli int64
	MemBytes int64
	NetRx    int64
	NetTx    int64
	BlkR     int64
	BlkW     int64
}

type ExecResult struct {
	ExitCode int
	Output   []byte
}

type RuntimeEvent struct {
	SiteID    string
	Container string
	Kind      string // start | stop | die | oom | health_status | restart
	At        time.Time
	Attrs     map[string]string
}

// Driver runs Sites on one node.
type Driver interface {
	// Apply starts the new revision beside the old one, waits for readiness,
	// and only then retires the old revision. A failure leaves the old one serving.
	Apply(ctx context.Context, spec SiteRuntimeSpec) (ApplyResult, error)
	Remove(ctx context.Context, siteID string) error
	Logs(ctx context.Context, siteID string, opts LogOptions) (io.ReadCloser, error)
	Stats(ctx context.Context, siteID string) (<-chan StatsSample, error)
	Exec(ctx context.Context, siteID string, cmd []string) (ExecResult, error)
	Events(ctx context.Context) (<-chan RuntimeEvent, error)
}
