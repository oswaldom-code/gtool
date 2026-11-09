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
			err := config.NewValidator().Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("app-config.host-port"))
		},
		Entry("non numeric", "eighty"),
		Entry("zero", "0"),
		Entry("out of range", "70000"),
	)
})
