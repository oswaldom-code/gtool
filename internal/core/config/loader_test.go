package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oswaldom-code/gtool/internal/core/config"
	pkgConfig "github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const (
	fixturesBasePath     = "../../../test/fixtures/config"
	minimalConfigPath    = fixturesBasePath + "/minimal.yml"
	fullConfigPath       = fixturesBasePath + "/full.yml"
	defaultsOnlyPath     = fixturesBasePath + "/defaults-only.yml"
	invalidYAMLPath      = fixturesBasePath + "/invalid-yaml.yml"
	withEnvVarsPath      = fixturesBasePath + "/with-env-vars.yml"
	baseForOverridesPath = fixturesBasePath + "/base-for-overrides.yml"
)

func TestLoader(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Loader Suite")
}

var _ = Describe("Loader", func() {
	var (
		loader           *config.Loader
		tempDir          string
		absMinimalConfig string
		absFullConfig    string
		absInvalidYAML   string
	)

	BeforeEach(func() {
		loader = config.NewLoader()
		var err error
		tempDir, err = os.MkdirTemp("", "gtool-loader-test-*")
		Expect(err).NotTo(HaveOccurred())

		absMinimalConfig, err = filepath.Abs(minimalConfigPath)
		Expect(err).NotTo(HaveOccurred())
		absFullConfig, err = filepath.Abs(fullConfigPath)
		Expect(err).NotTo(HaveOccurred())
		absInvalidYAML, err = filepath.Abs(invalidYAMLPath)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("NewLoader", func() {
		It("should create a new loader instance", func() {
			Expect(loader).NotTo(BeNil())
		})
	})

	Describe("SetExpandEnv", func() {
		It("should toggle environment variable expansion", func() {
			loader.SetExpandEnv(false)
			loader.SetExpandEnv(true)
		})
	})

	Describe("Load", func() {
		Context("with valid configuration", func() {
			It("should load minimal config successfully", func() {
				cfg, err := loader.Load(minimalConfigPath)

				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).NotTo(BeNil())
				Expect(cfg.Version).To(Equal("v1"))
				Expect(cfg.AppTechnology).To(Equal("golang"))
				Expect(cfg.AppConfig.BinaryName).To(Equal("test-app"))
				Expect(cfg.AppConfig.Port).To(Equal(8080))
				Expect(cfg.TestLauncher).To(Equal("test-launcher-back"))
			})

			It("should load full config with all fields", func() {
				cfg, err := loader.Load(fullConfigPath)

				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Version).To(Equal("v1"))
				Expect(cfg.AppTechnology).To(Equal("nodejs"))
				Expect(cfg.AppConfig.BinaryName).To(Equal("my-service"))
				Expect(cfg.AppConfig.BinaryPath).To(Equal("./dist"))
				Expect(cfg.AppConfig.DockerImage).To(Equal("myapp:latest"))
				Expect(cfg.AppConfig.Port).To(Equal(3000))
				Expect(cfg.AppConfig.Environment["NODE_ENV"]).To(Equal("test"))
				Expect(cfg.AppConfig.Environment["LOG_LEVEL"]).To(Equal("debug"))
				Expect(cfg.TestLauncher).To(Equal("test-launcher-front"))
				Expect(cfg.TestConfig.Tags).To(Equal("@smoke"))
				Expect(cfg.TestConfig.FeaturesPath).To(Equal("./features"))
				Expect(cfg.TestConfig.ReportsPath).To(Equal("./reports"))
				Expect(cfg.TestConfig.Parallel).To(BeTrue())
				Expect(cfg.ThirdParty.Mocks).To(HaveLen(2))
				Expect(cfg.ThirdParty.Mocks).To(ContainElements("couchbase", "postgresql"))
				Expect(cfg.Orchestration.ParallelMocks).To(BeFalse())
				Expect(cfg.Orchestration.StartupTimeout).To(Equal(60 * time.Second))
				Expect(cfg.Orchestration.HealthCheckInterval).To(Equal(5 * time.Second))
				Expect(cfg.Orchestration.HealthCheckRetries).To(Equal(30))
				Expect(cfg.Orchestration.CleanupOnFailure).To(BeFalse())
				Expect(cfg.Orchestration.PreserveLogs).To(BeFalse())
				Expect(cfg.Observability.StructuredLogs).To(BeTrue())
				Expect(cfg.Observability.LogLevel).To(Equal("debug"))
				Expect(cfg.Observability.MetricsEnabled).To(BeFalse())
				Expect(cfg.Observability.ReportFormat).To(Equal("json"))
			})

			It("should apply default values for missing fields", func() {
				cfg, err := loader.Load(defaultsOnlyPath)

				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Orchestration.ParallelMocks).To(BeTrue())
				Expect(cfg.Orchestration.StartupTimeout).To(Equal(180 * time.Second))
				Expect(cfg.Orchestration.HealthCheckInterval).To(Equal(3 * time.Second))
				Expect(cfg.Orchestration.HealthCheckRetries).To(Equal(60))
				Expect(cfg.Orchestration.CleanupOnFailure).To(BeTrue())
				Expect(cfg.Orchestration.PreserveLogs).To(BeTrue())
				Expect(cfg.Observability.StructuredLogs).To(BeFalse())
				Expect(cfg.Observability.LogLevel).To(Equal("info"))
				Expect(cfg.Observability.MetricsEnabled).To(BeTrue())
				Expect(cfg.Observability.ReportFormat).To(Equal("text"))
			})

			It("should handle empty file with defaults", func() {
				emptyFile := filepath.Join(tempDir, "empty.yml")
				err := os.WriteFile(emptyFile, []byte(""), 0644)
				Expect(err).NotTo(HaveOccurred())

				cfg, err := loader.Load(emptyFile)

				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Version).To(Equal("v1"))
			})
		})

		Context("with invalid configuration", func() {
			It("should return error for invalid YAML syntax", func() {
				_, err := loader.Load(invalidYAMLPath)

				Expect(err).To(HaveOccurred())
				Expect(gtErrors.Is(err, gtErrors.ErrConfigInvalid)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("failed to parse YAML"))
			})

			It("should return error for non-existent file", func() {
				_, err := loader.Load("/nonexistent/path/config.yml")

				Expect(err).To(HaveOccurred())
				Expect(gtErrors.Is(err, gtErrors.ErrConfigNotFound)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("configuration file not found"))
			})
		})

		Context("with unreadable file", func() {
			It("should return error when file cannot be read", func() {
				if os.Getuid() == 0 {
					Skip("skipping test when running as root")
				}

				unreadableFile := filepath.Join(tempDir, "unreadable.yml")
				err := os.WriteFile(unreadableFile, []byte("version: v1"), 0644)
				Expect(err).NotTo(HaveOccurred())

				err = os.Chmod(tempDir, 0000)
				Expect(err).NotTo(HaveOccurred())
				defer os.Chmod(tempDir, 0755)

				_, err = loader.Load(unreadableFile)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("LoadFromPath", func() {
		var originalDir string

		BeforeEach(func() {
			var err error
			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.Chdir(originalDir)
		})

		DescribeTable("should load config from standard paths",
			func(filename string) {
				content, err := os.ReadFile(absMinimalConfig)
				Expect(err).NotTo(HaveOccurred())

				err = os.WriteFile(filename, content, 0644)
				Expect(err).NotTo(HaveOccurred())
				defer os.Remove(filename)

				cfg, err := loader.LoadFromPath()

				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).NotTo(BeNil())
				Expect(cfg.Version).To(Equal("v1"))
				Expect(cfg.AppTechnology).To(Equal("golang"))
			},
			Entry("component-config.yml", "component-config.yml"),
			Entry("component-config.yaml", "component-config.yaml"),
			Entry("gtool-config.yml", "gtool-config.yml"),
			Entry("gtool-config.yaml", "gtool-config.yaml"),
		)

		It("should return error when no config file found", func() {
			_, err := loader.LoadFromPath()

			Expect(err).To(HaveOccurred())
			Expect(gtErrors.Is(err, gtErrors.ErrConfigNotFound)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("no configuration file found"))
		})

		It("should return error when config file has invalid content", func() {
			content, err := os.ReadFile(absInvalidYAML)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile("component-config.yml", content, 0644)
			Expect(err).NotTo(HaveOccurred())

			_, err = loader.LoadFromPath()

			Expect(err).To(HaveOccurred())
			Expect(gtErrors.Is(err, gtErrors.ErrConfigInvalid)).To(BeTrue())
		})

		It("should respect priority order (component-config.yml first)", func() {
			minimalContent, err := os.ReadFile(absMinimalConfig)
			Expect(err).NotTo(HaveOccurred())
			fullContent, err := os.ReadFile(absFullConfig)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile("component-config.yml", minimalContent, 0644)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile("gtool-config.yml", fullContent, 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.LoadFromPath()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AppConfig.BinaryName).To(Equal("test-app"))
		})
	})

	Describe("LoadFromPathOrDefault", func() {
		var originalDir string

		BeforeEach(func() {
			var err error
			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.Chdir(originalDir)
		})

		It("should return loaded config when file exists", func() {
			content, err := os.ReadFile(absFullConfig)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile("component-config.yml", content, 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg := loader.LoadFromPathOrDefault()

			Expect(cfg.AppTechnology).To(Equal("nodejs"))
			Expect(cfg.AppConfig.BinaryName).To(Equal("my-service"))
		})

		It("should return default config when no file exists", func() {
			cfg := loader.LoadFromPathOrDefault()

			defaultCfg := pkgConfig.DefaultConfig()
			Expect(cfg.Version).To(Equal(defaultCfg.Version))
			Expect(cfg.Orchestration.ParallelMocks).To(Equal(defaultCfg.Orchestration.ParallelMocks))
		})
	})

	Describe("Environment Variable Expansion", func() {
		AfterEach(func() {
			os.Unsetenv("APP_NAME")
			os.Unsetenv("DB_HOST")
			os.Unsetenv("DB_PORT")
			os.Unsetenv("TEST_VAR")
			os.Unsetenv("MY_VAR_NAME")
			os.Unsetenv("VAR123")
			os.Unsetenv("EMPTY_VAR")
		})

		Context("with fixture file containing env vars", func() {
			It("should expand ${VAR} syntax", func() {
				os.Setenv("APP_NAME", "expanded-app")
				os.Setenv("DB_HOST", "localhost")
				os.Setenv("DB_PORT", "5432")

				cfg, err := loader.Load(withEnvVarsPath)

				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AppConfig.BinaryName).To(Equal("expanded-app"))
				Expect(cfg.AppConfig.Environment["DATABASE_URL"]).To(Equal("localhost:5432"))
			})

			It("should keep original when env var not set", func() {
				cfg, err := loader.Load(withEnvVarsPath)

				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AppConfig.BinaryName).To(Equal("${APP_NAME}"))
			})
		})

		Context("when expansion is disabled", func() {
			It("should not expand variables", func() {
				os.Setenv("APP_NAME", "expanded-value")

				loader.SetExpandEnv(false)
				cfg, err := loader.Load(withEnvVarsPath)

				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AppConfig.BinaryName).To(Equal("${APP_NAME}"))
			})
		})

		Context("with inline config for edge cases", func() {
			DescribeTable("edge cases",
				func(envVars map[string]string, binaryName, expected string) {
					for k, v := range envVars {
						os.Setenv(k, v)
					}

					content := "version: v1\napp-technology: golang\napp-config:\n  binary-name: " + binaryName + "\n  port: 8080\ntest-launcher: test-launcher-back\n"
					configFile := filepath.Join(tempDir, "edge-case.yml")
					err := os.WriteFile(configFile, []byte(content), 0644)
					Expect(err).NotTo(HaveOccurred())

					cfg, err := loader.Load(configFile)

					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.AppConfig.BinaryName).To(Equal(expected))
				},
				Entry("underscore in var name",
					map[string]string{"MY_VAR_NAME": "value"},
					"${MY_VAR_NAME}", "value"),
				Entry("numbers in var name",
					map[string]string{"VAR123": "num-value"},
					"${VAR123}", "num-value"),
				Entry("empty env var keeps original",
					map[string]string{"EMPTY_VAR": ""},
					"${EMPTY_VAR}", "${EMPTY_VAR}"),
				Entry("plain text without variables",
					map[string]string{},
					"plain-text", "plain-text"),
			)
		})
	})

	Describe("Environment Overrides (GTOOL_* variables)", func() {
		AfterEach(func() {
			os.Unsetenv("GTOOL_LOG_LEVEL")
			os.Unsetenv("GTOOL_PARALLEL_MOCKS")
			os.Unsetenv("GTOOL_DOCKER_IMAGE")
		})

		It("should override log level", func() {
			os.Setenv("GTOOL_LOG_LEVEL", "debug")

			cfg, err := loader.Load(baseForOverridesPath)

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Observability.LogLevel).To(Equal("debug"))
		})

		It("should override parallel mocks to false", func() {
			os.Setenv("GTOOL_PARALLEL_MOCKS", "false")

			cfg, err := loader.Load(baseForOverridesPath)

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Orchestration.ParallelMocks).To(BeFalse())
		})

		It("should override parallel mocks to true", func() {
			os.Setenv("GTOOL_PARALLEL_MOCKS", "true")

			cfg, err := loader.Load(baseForOverridesPath)

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Orchestration.ParallelMocks).To(BeTrue())
		})

		It("should override docker image", func() {
			os.Setenv("GTOOL_DOCKER_IMAGE", "custom-image:v2")

			cfg, err := loader.Load(baseForOverridesPath)

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AppConfig.DockerImage).To(Equal("custom-image:v2"))
		})

		It("should override multiple values", func() {
			os.Setenv("GTOOL_LOG_LEVEL", "warn")
			os.Setenv("GTOOL_PARALLEL_MOCKS", "false")
			os.Setenv("GTOOL_DOCKER_IMAGE", "override:latest")

			cfg, err := loader.Load(baseForOverridesPath)

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Observability.LogLevel).To(Equal("warn"))
			Expect(cfg.Orchestration.ParallelMocks).To(BeFalse())
			Expect(cfg.AppConfig.DockerImage).To(Equal("override:latest"))
		})

		It("should not override when env vars not set", func() {
			cfg, err := loader.Load(baseForOverridesPath)

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Observability.LogLevel).To(Equal("info"))
			Expect(cfg.Orchestration.ParallelMocks).To(BeTrue())
			Expect(cfg.AppConfig.DockerImage).To(BeEmpty())
		})
	})

	Describe("LoadConfig helper function", func() {
		It("should load from specified path", func() {
			cfg, err := config.LoadConfig(minimalConfigPath)

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AppConfig.BinaryName).To(Equal("test-app"))
			Expect(cfg.AppConfig.Port).To(Equal(8080))
		})

		It("should load from default path when empty string provided", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			defer os.Chdir(originalDir)

			// Solve absolute path for minimal config
			absPath, err := filepath.Abs(minimalConfigPath)
			Expect(err).NotTo(HaveOccurred())

			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(absPath)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile("component-config.yml", content, 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig("")

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AppConfig.BinaryName).To(Equal("test-app"))
		})

		It("should return error when file not found", func() {
			_, err := config.LoadConfig("/nonexistent/config.yml")

			Expect(err).To(HaveOccurred())
			Expect(gtErrors.Is(err, gtErrors.ErrConfigNotFound)).To(BeTrue())
		})
	})
})
