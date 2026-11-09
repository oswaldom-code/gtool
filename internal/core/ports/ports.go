// Package ports resolves dynamic host ports for one gtool run and exposes the
// resulting values to the app (placeholders) and to the test runner (env).
package ports

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/oswaldom-code/gtool/internal/plugin/services"
	"github.com/oswaldom-code/gtool/internal/plugin/services/svcconfig"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

// Auto is the config value that asks gtool to pick a free host port.
const Auto = "auto"

// Allocator returns a free host port.
type Allocator func() (int, error)

// maxAllocAttempts bounds retries when the OS hands back an already-used port.
const maxAllocAttempts = 20

// FreePortAllocator asks the OS for free ports and never returns the same port
// twice. The port is released before use, so another process could grab it in
// between; the window is tiny and acceptable for CI isolation.
func FreePortAllocator() Allocator {
	used := map[int]bool{}
	return func() (int, error) {
		for i := 0; i < maxAllocAttempts; i++ {
			l, err := net.Listen("tcp", ":0")
			if err != nil {
				return 0, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to allocate a free port")
			}
			port := l.Addr().(*net.TCPAddr).Port
			_ = l.Close()
			if !used[port] {
				used[port] = true
				return port, nil
			}
		}
		return 0, gtErrors.New(gtErrors.ErrServiceFailed, "failed to allocate a unique free port")
	}
}

// NoAuto is the Allocator for commands run as separate invocations
// ("services up", then "app start"): nothing persists the ports one of them
// would pick, so the next one could not find them.
func NoAuto() (int, error) {
	return 0, gtErrors.New(gtErrors.ErrConfigInvalid, fmt.Sprintf(
		"port %q is only supported by single-invocation pipelines (gtool test, gtool stress); "+
			"separate commands cannot share the ports it allocates, so set explicit ports", Auto))
}

// isPortKey reports whether a mock-config key holds a host port.
func isPortKey(key string) bool {
	return key == "port" || strings.HasSuffix(key, "-port")
}

// isAuto reports whether a config value requests a dynamic port.
func isAuto(v interface{}) bool {
	s, ok := v.(string)
	return ok && strings.EqualFold(strings.TrimSpace(s), Auto)
}

// sortedKeys returns map keys in a stable order so allocation is deterministic.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Allocate replaces every "auto" port in mock-config and app-config.host-port
// with a free port, in place. Explicit ports are left untouched.
func Allocate(cfg *config.Config, alloc Allocator) error {
	for _, svc := range sortedKeys(cfg.ThirdParty.MockConfig) {
		svcCfg, ok := cfg.ThirdParty.MockConfig[svc].(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range sortedKeys(svcCfg) {
			if !isPortKey(key) || !isAuto(svcCfg[key]) {
				continue
			}
			port, err := alloc()
			if err != nil {
				return err
			}
			svcCfg[key] = strconv.Itoa(port)
		}
	}

	hp := strings.TrimSpace(cfg.AppConfig.HostPort)
	switch {
	case hp == "":
	case isAuto(hp):
		port, err := alloc()
		if err != nil {
			return err
		}
		cfg.AppConfig.HostPort = strconv.Itoa(port)
	default:
		if n, err := strconv.Atoi(hp); err != nil || n < 1 || n > 65535 {
			return gtErrors.New(gtErrors.ErrConfigInvalid,
				fmt.Sprintf("app-config.host-port must be %q or a port between 1 and 65535, got %q", Auto, cfg.AppConfig.HostPort))
		}
	}
	return nil
}

// AppHostPort returns the host port the app container is published on:
// app-config.host-port when set, otherwise app-config.port.
func AppHostPort(cfg *config.Config) int {
	if p, err := strconv.Atoi(cfg.AppConfig.HostPort); err == nil && p > 0 {
		return p
	}
	return cfg.AppConfig.Port
}

// Values returns the placeholder table of the run: "mocks.<svc>.<port-key>"
// for every port of every enabled mock (its default when not configured), plus
// "app.port" and "app.host-port".
func Values(cfg *config.Config) map[string]string {
	values := map[string]string{}
	defaults := services.DefaultPorts()
	for _, svc := range cfg.ThirdParty.Mocks {
		for key, port := range defaults[svc] {
			values["mocks."+svc+"."+key] = port
		}
	}
	for svc, raw := range cfg.ThirdParty.MockConfig {
		svcCfg, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		for key := range svcCfg {
			if !isPortKey(key) {
				continue
			}
			if v := svcconfig.Port(svcCfg, key, ""); v != "" {
				values["mocks."+svc+"."+key] = v
			}
		}
	}
	if cfg.AppConfig.Port > 0 {
		values["app.port"] = strconv.Itoa(cfg.AppConfig.Port)
		values["app.host-port"] = strconv.Itoa(AppHostPort(cfg))
	}
	return values
}

var placeholder = regexp.MustCompile(`\$\{((?:mocks|app)\.[A-Za-z0-9_.-]+)}`)

// Interpolate replaces ${mocks.<svc>.<key>} and ${app.<key>} placeholders in
// app-config.environment with the run's values, in place. An unknown
// placeholder is an error: silently passing it through would hand the app a
// broken address.
func Interpolate(cfg *config.Config, values map[string]string) error {
	var missing []string
	for _, k := range sortedKeys(cfg.AppConfig.Environment) {
		cfg.AppConfig.Environment[k] = placeholder.ReplaceAllStringFunc(cfg.AppConfig.Environment[k], func(m string) string {
			name := placeholder.FindStringSubmatch(m)[1]
			if v, ok := values[name]; ok {
				return v
			}
			missing = append(missing, fmt.Sprintf("%s (in app-config.environment.%s)", m, k))
			return m
		})
	}
	if len(missing) > 0 {
		return gtErrors.New(gtErrors.ErrConfigInvalid, fmt.Sprintf(
			"unresolved placeholders: %s; they must reference a port of a mock listed in third-party.mocks, or app.port / app.host-port",
			strings.Join(missing, ", ")))
	}
	return nil
}

// Resolve allocates "auto" ports and interpolates the app environment. It is
// the single entry point a pipeline calls before starting any phase.
func Resolve(cfg *config.Config, alloc Allocator) (map[string]string, error) {
	if err := Allocate(cfg, alloc); err != nil {
		return nil, err
	}
	values := Values(cfg)
	if err := Interpolate(cfg, values); err != nil {
		return nil, err
	}
	return values, nil
}

// envName turns "mocks.kafka.docker-port" into "GTOOL_KAFKA_DOCKER_PORT" and
// "app.host-port" into "GTOOL_APP_HOST_PORT".
func envName(key string) string {
	key = strings.TrimPrefix(key, "mocks.")
	return "GTOOL_" + strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(key))
}

// TestEnv returns the variables handed to the test runner so features and
// launcher helpers can reach the app and mocks on this run's ports. Tests run
// on the host network, hence localhost.
func TestEnv(values map[string]string) map[string]string {
	env := make(map[string]string, len(values)+1)
	for k, v := range values {
		env[envName(k)] = v
	}
	if hp, ok := values["app.host-port"]; ok {
		env["GTOOL_APP_URL"] = "http://localhost:" + hp
	}
	return env
}
