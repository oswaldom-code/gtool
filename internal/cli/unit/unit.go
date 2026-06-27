// Package unit reproduces the behaviour of the legacy "go-tool u" command:
// it generates mocks declared in build-config.yml and runs the unit-test
// suite with Ginkgo, producing a coverage profile and a JUnit report.
package unit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
)

const (
	defaultBuildConfig = "build-config.yml"
	mocksFolder        = "mocks"
	coverFolder        = "coverage"

	// Tool versions are pinned to match the legacy go-tool behaviour so the
	// generated mocks and the Ginkgo CLI flags stay identical.
	mockgenURL = "go.uber.org/mock/mockgen@v0.3.0"
	ginkgoURL  = "github.com/onsi/ginkgo/v2/ginkgo@v2.2.0"
)

// libWildcards maps the placeholders used in build-config.yml mock sources to
// the go.mod module substring used to resolve their path in the module cache.
// <LIB-ROOT> is handled separately because it expands to the module cache root.
var libWildcards = map[string]string{
	"<COMMON-LIB>":     "common-library-back",
	"<SEARCH-LIB>":     "search-library",
	"<BUSINESS-LIB>":   "business-library",
	"<ANALYTICS-LIB>":  "analytics-library",
	"<BACKOFFICE-LIB>": "backoffice-library",
}

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
		Long: `Reproduces the legacy "go-tool u" workflow:

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

// buildMocks reproduces go-tool's build_mocks: it resolves library wildcards and
// runs mockgen for every declared interface.
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
	if err := runStreaming("", nil, "go", "mod", "download"); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "go mod download failed")
	}

	gopath, err := goEnv("GOPATH")
	if err != nil {
		return err
	}
	modCache := filepath.Join(gopath, "pkg", "mod")

	mockgen := filepath.Join(gobin, "mockgen")
	for _, m := range mocks {
		source, err := resolveSource(m.Source, modCache)
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

// resolveSource expands library wildcards in a mock source path to an absolute
// path inside the Go module cache, mirroring go-tool's wildcard handling.
func resolveSource(source, modCache string) (string, error) {
	if strings.Contains(source, "<LIB-ROOT>") {
		source = strings.ReplaceAll(source, "<LIB-ROOT>", modCache)
	}

	for wildcard, module := range libWildcards {
		if !strings.Contains(source, wildcard) {
			continue
		}
		modPath, err := moduleCachePath(module, modCache)
		if err != nil {
			return "", err
		}
		source = strings.ReplaceAll(source, wildcard, modPath)
	}

	return source, nil
}

// moduleCachePath finds the first go.mod require line containing the given
// module substring and returns its "<module>@<version>" path in the cache.
func moduleCachePath(moduleSubstr, modCache string) (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", gtErrors.Wrap(err, gtErrors.ErrConfigNotFound, "failed to read go.mod")
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, moduleSubstr) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "//") {
			continue
		}
		return filepath.Join(modCache, fields[0]+"@"+fields[1]), nil
	}

	return "", gtErrors.New(gtErrors.ErrConfigInvalid,
		fmt.Sprintf("module %q not found in go.mod", moduleSubstr))
}

// execUnitTests reproduces go-tool's exec_unit_tests Ginkgo invocation.
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

	fmt.Printf("\n✅ Unit tests passed. Coverage and report under ./%s\n", coverFolder)
	return nil
}

// installTool runs `go install <url>` so the pinned tool version is available,
// matching go-tool. A failure is fatal because the exact version is required.
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
	return filepath.Join(gopath, "bin"), nil
}

func goEnv(key string) (string, error) {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return "", gtErrors.Wrap(err, gtErrors.ErrInvalidArgument,
			fmt.Sprintf("failed to read go env %s", key))
	}
	return strings.TrimSpace(string(out)), nil
}
