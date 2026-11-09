package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oswaldom-code/gtool/internal/core/config"
	pkgConfig "github.com/oswaldom-code/gtool/pkg/config"
)

var _ = Describe("Validator app-config.host-port", func() {
	var cfg *pkgConfig.Config

	BeforeEach(func() {
		cfg = pkgConfig.DefaultConfig()
		cfg.AppTechnology = "generic"
		cfg.TestLauncher = "test-launcher-back"
		cfg.AppConfig.DockerImage = "app:1"
		cfg.AppConfig.Port = 8080
	})

	DescribeTable("accepts",
		func(hostPort string) {
			cfg.AppConfig.HostPort = hostPort
			Expect(config.NewValidator().Validate(cfg)).To(Succeed())
		},
		Entry("unset (same as port)", ""),
		Entry("auto", "auto"),
		Entry("AUTO in any case", "AUTO"),
		Entry("explicit port", "18080"),
	)

	DescribeTable("rejects",
		func(hostPort string) {
			cfg.AppConfig.HostPort = hostPort
			if hostPort == "__noport__" {
				cfg.AppConfig.HostPort, cfg.AppConfig.Port = "auto", 0
			}
			err := config.NewValidator().Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("app-config.host-port"))
		},
		Entry("non numeric", "eighty"),
		Entry("host-port without port", "__noport__"),
		Entry("zero", "0"),
		Entry("out of range", "70000"),
	)
})

var _ = Describe("Validator app-config.health-path", func() {
	It("accepts an absolute path and rejects a relative one", func() {
		cfg := pkgConfig.DefaultConfig()
		cfg.AppTechnology = "generic"
		cfg.TestLauncher = "test-launcher-back"
		cfg.AppConfig.DockerImage = "app:1"

		cfg.AppConfig.HealthPath = "/health"
		Expect(config.NewValidator().Validate(cfg)).To(Succeed())

		cfg.AppConfig.HealthPath = "health"
		err := config.NewValidator().Validate(cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("app-config.health-path"))
	})
})

var _ = Describe("Validator mock-config", func() {
	var cfg *pkgConfig.Config

	BeforeEach(func() {
		cfg = pkgConfig.DefaultConfig()
		cfg.AppTechnology = "generic"
		cfg.TestLauncher = "test-launcher-back"
		cfg.AppConfig.DockerImage = "app:1"
		cfg.ThirdParty.Mocks = []string{"kafka", "postgresql"}
	})

	validate := func(mockCfg map[string]interface{}) error {
		cfg.ThirdParty.MockConfig = mockCfg
		return config.NewValidator().Validate(cfg)
	}

	It("accepts known keys, auto and numeric ports", func() {
		Expect(validate(map[string]interface{}{
			"kafka":      map[string]interface{}{"port": "auto", "docker-port": 19094, "topics": []interface{}{"a"}},
			"postgresql": map[string]interface{}{"port": "5433", "user": "app"},
		})).To(Succeed())
	})

	It("rejects an unknown key", func() {
		err := validate(map[string]interface{}{"postgresql": map[string]interface{}{"usr": "app"}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("third-party.mock-config.postgresql: unknown key 'usr'"))
	})

	It("rejects an invalid port", func() {
		err := validate(map[string]interface{}{"postgresql": map[string]interface{}{"port": "aut0"}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mock-config.postgresql.port"))
	})

	It("rejects two port keys on the same host port", func() {
		err := validate(map[string]interface{}{"kafka": map[string]interface{}{"port": 9094, "docker-port": "9094"}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("both use host port 9094"))
	})

	It("ignores mock-config of mocks that are not enabled", func() {
		Expect(validate(map[string]interface{}{"redis": map[string]interface{}{"whatever": 1}})).To(Succeed())
	})
})

var _ = Describe("Validator unimplemented values", func() {
	It("rejects nodejs and test-launcher-front, which have no implementation", func() {
		cfg := pkgConfig.DefaultConfig()
		cfg.AppTechnology = "nodejs"
		cfg.TestLauncher = "test-launcher-front"

		err := config.NewValidator().Validate(cfg)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported app-technology 'nodejs'"))
		Expect(err.Error()).To(ContainSubstring("unsupported test-launcher 'test-launcher-front'"))
	})

	It("does not require binary-name, which the pipeline never uses", func() {
		cfg := pkgConfig.DefaultConfig()
		cfg.AppTechnology = "golang"
		cfg.TestLauncher = "test-launcher-back"
		Expect(config.NewValidator().Validate(cfg)).To(Succeed())
	})
})
