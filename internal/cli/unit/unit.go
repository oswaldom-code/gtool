// Package unit implements gtool unit: it generates the mocks declared in
// build-config.yml and runs the unit-test suite with Ginkgo, producing a
// coverage profile and a JUnit report.
package unit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/oswaldom-code/gtool/internal/infra/termout"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const (
	defaultBuildConfig = "build-config.yml"
	mocksFolder        = "mocks"
	coverFolder        = "coverage"

	// Tool versions are pinned so the generated mocks and the Ginkgo CLI flags
	// stay identical across machines.
	mockgenURL = "go.uber.org/mock/mockgen@v0.3.0"
	ginkgoURL  = "github.com/onsi/ginkgo/v2/ginkgo@v2.2.0"
)

// modRoot expands to the Go module cache root in mock sources.
const modRoot = "<LIB-ROOT>"

// modPlaceholder matches <MOD:name>, which expands to the directory of the
// module in the build list whose path is name or ends with "/"+name.
var modPlaceholder = regexp.MustCompile(`<MOD:([^<>]+)>`)

// buildConfig is the minimal view of build-config.yml needed for unit tests.
type buildConfig struct {
	Mocks []mockSpec `yaml:"mocks"`
}

type mockSpec struct {
	Source   string `yaml:"source"`
	Filename string `yaml:"filename"`
}

// NewUnitCmd creates the `gtool unit` command.
func NewUnitCmd() *cobra.Command {
	var buildConfigFile string
	var skipMocks bool

	cmd := &cobra.Command{
		Use:     "unit",
		Aliases: []string{"u"},
		Short:   "Run unit tests (mock generation + Ginkgo)",
		Long: `Run the unit tests of a Go service:

  1. Generates the mocks declared in build-config.yml (mockgen).
  2. Runs the unit-test suite recursively with Ginkgo, generating a
     coverage profile and a JUnit report under ./coverage.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnit(buildConfigFile, skipMocks)
		},
	}

	cmd.Flags().StringVar(&buildConfigFile, "build-config", defaultBuildConfig, "path to build-config.yml")
	cmd.Flags().BoolVar(&skipMocks, "skip-mocks", false, "skip mock generation and only run the tests")

	return cmd
}

func runUnit(buildConfigFile string, skipMocks bool) error {
	gobin, err := goBin()
	if err != nil {
		return err
	}

	if !skipMocks {
		cfg, err := loadBuildConfig(buildConfigFile)
		if err != nil {
			return err
		}
		if err := buildMocks(gobin, cfg.Mocks); err != nil {
			return err
		}
	}

	return execUnitTests(gobin)
}

func loadBuildConfig(path string) (*buildConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, gtErrors.New(gtErrors.ErrConfigNotFound,
				fmt.Sprintf("%s not found (required to generate mocks)", path))
		}
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to read build config")
	}

	cfg := &buildConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to parse build config")
	}
	return cfg, nil
}

// buildMocks resolves module placeholders and runs mockgen for every declared
// interface.
func buildMocks(gobin string, mocks []mockSpec) error {
	if len(mocks) == 0 {
		return nil
	}

	if err := os.MkdirAll(mocksFolder, 0o755); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to create mocks folder")
	}

	if err := installTool(gobin, mockgenURL); err != nil {
		return err
	}

	var modCache string
	var modules map[string]string
	if usesWildcards(mocks) {
		if err := runStreaming("", nil, "go", "mod", "download"); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "go mod download failed")
		}
		var err error
		if modCache, err = goEnv("GOMODCACHE"); err != nil {
			return err
		}
		if modules, err = listModules(); err != nil {
			return err
		}
	}

	mockgen := filepath.Join(gobin, "mockgen")
	for _, m := range mocks {
		source, err := resolveSource(m.Source, modCache, modules)
		if err != nil {
			return err
		}

		dest := filepath.Join(mocksFolder, m.Filename)
		fmt.Printf("Generating mock %s\n", m.Filename)
		if err := runStreaming("", nil, mockgen,
			"-package", mocksFolder,
			"-source", source,
			"-destination", dest); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument,
				fmt.Sprintf("mockgen failed for %s", m.Source))
		}
	}

	return nil
}

func usesWildcards(mocks []mockSpec) bool {
	for _, m := range mocks {
		if strings.Contains(m.Source, "<") {
			return true
		}
	}
	return false
}

// listModules returns the directory of every module in the build list, as the
// go command resolves it (replace directives, case escaping, GOMODCACHE).
var listModules = func() (map[string]string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Path}} {{.Dir}}", "all").Output()
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to list Go modules")
	}
	modules := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path, dir, _ := strings.Cut(line, " ")
		if path != "" {
			modules[path] = dir
		}
	}
	return modules, nil
}

// moduleDir returns the directory of the module whose path is name or ends
// with "/"+name, so foo-library never matches foo-library-v2.
func moduleDir(name string, modules map[string]string) (string, error) {
	paths := make([]string, 0, len(modules))
	for path := range modules {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if path != name && !strings.HasSuffix(path, "/"+name) {
			continue
		}
		if modules[path] == "" {
			return "", gtErrors.New(gtErrors.ErrInvalidArgument, fmt.Sprintf("module %s is not downloaded", path))
		}
		return modules[path], nil
	}
	return "", gtErrors.New(gtErrors.ErrConfigInvalid, fmt.Sprintf("module %q not found in the build list", name))
}

// resolveSource expands <LIB-ROOT> and <MOD:name> placeholders in a mock
// source path to absolute paths inside the Go module cache.
func resolveSource(source, modCache string, modules map[string]string) (string, error) {
	source = strings.ReplaceAll(source, modRoot, modCache)

	var resolveErr error
	source = modPlaceholder.ReplaceAllStringFunc(source, func(match string) string {
		name := modPlaceholder.FindStringSubmatch(match)[1]
		dir, err := moduleDir(name, modules)
		if err != nil && resolveErr == nil {
			resolveErr = err
		}
		return dir
	})
	if resolveErr != nil {
		return "", resolveErr
	}
	return source, nil
}

// execUnitTests runs the suite recursively with Ginkgo.
func execUnitTests(gobin string) error {
	if err := installTool(gobin, ginkgoURL); err != nil {
		return err
	}
	if err := os.MkdirAll(coverFolder, 0o755); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to create coverage folder")
	}

	ginkgo := filepath.Join(gobin, "ginkgo")
	env := append(os.Environ(), "LOG_LEVEL=panic")
	err := runStreaming("", env, ginkgo,
		"-r",
		"--randomize-suites",
		"--fail-on-pending",
		"--progress",
		"--cover",
		"--covermode=set",
		"--coverprofile=coverage.out",
		"--junit-report=report.xml",
		"--output-dir="+coverFolder,
	)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrTestFailed, "unit tests failed")
	}

	termout.Printf("\n✅ Unit tests passed. Coverage and report under ./%s\n", coverFolder)
	return nil
}

// installTool runs `go install <url>` so the pinned tool version is available.
// A failure is fatal because the exact version is required.
func installTool(gobin, url string) error {
	if err := runStreaming("", nil, "go", "install", url); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument,
			fmt.Sprintf("failed to install %s", url))
	}
	return nil
}

// runStreaming runs a command wiring its stdio to the current process.
func runStreaming(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func goBin() (string, error) {
	gobin, err := goEnv("GOBIN")
	if err != nil {
		return "", err
	}
	if gobin != "" {
		return gobin, nil
	}
	gopath, err := goEnv("GOPATH")
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.SplitList(gopath)[0], "bin"), nil
}

func goEnv(key string) (string, error) {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return "", gtErrors.Wrap(err, gtErrors.ErrInvalidArgument,
			fmt.Sprintf("failed to read go env %s", key))
	}
	return strings.TrimSpace(string(out)), nil
}
