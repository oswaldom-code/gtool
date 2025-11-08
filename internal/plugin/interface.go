package plugin

import (
	"context"
	"time"
)

type ServicePlugin interface {
	Name() string
	Launch(ctx context.Context, config map[string]interface{}) error
	IsReady(ctx context.Context) (bool, error)
	Stop(ctx context.Context) error
	GetConnectionInfo() (*ConnectionInfo, error)
	GetLogs(ctx context.Context, opts *LogOptions) ([]string, error)
}

type AppLauncher interface {
	Technology() string
	Launch(ctx context.Context, config *AppConfig) error
	IsReady(ctx context.Context) (bool, error)
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
	GetPID() (int, error)
}

type TestExecutor interface {
	Framework() string
	Execute(ctx context.Context, config *TestConfig) (*TestResult, error)
	Cancel(ctx context.Context) error
	GetProgress(ctx context.Context) (*TestProgress, error)
}

type ConnectionInfo struct {
	Host     string
	Port     int
	Protocol string
	Metadata map[string]string
}

type LogOptions struct {
	Since  time.Time
	Until  time.Time
	Tail   int
	Follow bool
}

type AppConfig struct {
	BinaryName  string
	BinaryPath  string
	DockerImage string
	Port        int
	Environment map[string]string
	WorkDir     string
}

type TestConfig struct {
	Tags         []string
	FeaturesPath string
	ReportsPath  string
	Parallel     bool
	Environment  map[string]string
}

type TestResult struct {
	Total     int
	Passed    int
	Failed    int
	Skipped   int
	Duration  time.Duration
	ReportURL string
	Failures  []TestFailure
}

type TestFailure struct {
	Name    string
	Feature string
	Message string
	Stack   string
}

type TestProgress struct {
	Current int
	Total   int
	Running string
}
