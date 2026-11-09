package stress_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oswaldom-code/gtool/internal/core/stress"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

func targetNames(p *stress.Plan) []string {
	names := make([]string, 0, len(p.Targets))
	for _, t := range p.Targets {
		names = append(names, t.Name)
	}
	return names
}

func findTarget(p *stress.Plan, name string) stress.Target {
	for _, t := range p.Targets {
		if t.Name == name {
			return t
		}
	}
	Fail("target not found: " + name)
	return stress.Target{}
}

var _ = Describe("ResolveTargets", func() {
	var sc *config.StressConfig

	Context("with an OpenAPI 3 contract", func() {
		BeforeEach(func() {
			sc = &config.StressConfig{Source: config.StressSource{OpenAPI: fixtures + "openapi3.yml"}}
		})

		It("resolves GET operations only by default, using the server base path", func() {
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())
			Expect(targetNames(plan)).To(Equal([]string{"health", "listProducts", "getProduct", "GET /orders/{orderId}"}))
			Expect(findTarget(plan, "health").URLs).To(Equal([]string{"/api/v1/health"}))
			Expect(plan.Warnings).To(BeEmpty())
		})

		It("uses default/example values and only sends required optional params", func() {
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())

			list := findTarget(plan, "listProducts")
			Expect(list.URLs).To(Equal([]string{"/api/v1/products?limit=20"}))
			Expect(list.Header.Get("X-Tenant")).To(Equal("acme"))
			Expect(list.Weight).To(Equal(1))
			Expect(list.Source).To(Equal("openapi"))

			Expect(findTarget(plan, "getProduct").URLs).To(Equal([]string{"/api/v1/products/p-1"}))
			Expect(findTarget(plan, "GET /orders/{orderId}").URLs).To(Equal([]string{"/api/v1/orders/7"}))
		})

		It("rotates param-values across URL variants and sends configured optional params", func() {
			sc.Source.ParamValues = map[string][]string{"productId": {"1", "2", "3"}, "sort": {"desc"}}
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())

			Expect(findTarget(plan, "getProduct").URLs).To(Equal([]string{
				"/api/v1/products/1", "/api/v1/products/2", "/api/v1/products/3"}))
			Expect(findTarget(plan, "listProducts").URLs).To(Equal([]string{"/api/v1/products?limit=20&sort=desc"}))
		})

		It("applies the base-path override, filters and global headers with env expansion", func() {
			GinkgoT().Setenv("STRESS_TOKEN", "secret")
			sc.Source.BasePath = "/custom/"
			sc.Source.IncludeTags = []string{"products", "ops"}
			sc.Source.ExcludePaths = []string{"/health"}
			sc.Source.Headers = map[string]string{"Authorization": "Bearer ${STRESS_TOKEN}"}

			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())
			Expect(targetNames(plan)).To(Equal([]string{"listProducts", "getProduct"}))
			Expect(plan.Targets[1].URLs).To(Equal([]string{"/custom/products/p-1"}))
			Expect(plan.Targets[0].Header.Get("Authorization")).To(Equal("Bearer secret"))
		})

		It("supports include-paths globs and exclude-tags", func() {
			sc.Source.IncludePaths = []string{"/products/*", "/orders/*"}
			sc.Source.ExcludeTags = []string{"orders"}
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())
			Expect(targetNames(plan)).To(Equal([]string{"getProduct"}))
		})

		It("resolves bodies from the spec example, the schema mock and body-file overrides", func() {
			sc.Source.Methods = []string{"post"}
			sc.Source.OperationOverrides = map[string]config.StressOperationOverride{
				"createOrder": {BodyFile: fixtures + "order.json", Weight: 3},
			}
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())

			product := findTarget(plan, "createProduct")
			Expect(product.Body).To(MatchJSON(`{"name":"widget","price":10}`))
			Expect(product.Header.Get("Content-Type")).To(Equal("application/json"))

			order := findTarget(plan, "createOrder")
			Expect(order.Body).To(MatchJSON(`{"quantity":3}`))
			Expect(order.Weight).To(Equal(3))

			Expect(plan.Warnings).To(ContainElement(ContainSubstring("mutating methods enabled (createProduct, createOrder)")))
		})

		It("generates a body from the schema when there is no example", func() {
			sc.Source.Methods = []string{"POST"}
			sc.Source.IncludeTags = []string{"orders"}
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())
			Expect(findTarget(plan, "createOrder").Body).To(MatchJSON(`{"quantity":1}`))
		})

		It("fails when an override body-file is missing", func() {
			sc.Source.Methods = []string{"POST"}
			sc.Source.OperationOverrides = map[string]config.StressOperationOverride{
				"createOrder": {BodyFile: "/does/not/exist.json"},
			}
			_, err := stress.ResolveTargets(sc)
			Expect(err).To(HaveOccurred())
			Expect(gtErrors.Is(err, gtErrors.ErrConfigNotFound)).To(BeTrue())
		})

		It("fails when filters leave no targets", func() {
			sc.Source.IncludeTags = []string{"nothing"}
			_, err := stress.ResolveTargets(sc)
			Expect(err).To(HaveOccurred())
			Expect(gtErrors.Is(err, gtErrors.ErrConfigInvalid)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("no stress targets resolved"))
		})
	})

	Context("with a Swagger 2 contract", func() {
		BeforeEach(func() {
			sc = &config.StressConfig{Source: config.StressSource{
				OpenAPI: fixtures + "swagger2.yml",
				Methods: []string{"GET", "POST"},
			}}
		})

		It("uses basePath, enum/default/type values and the body schema example", func() {
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())
			Expect(targetNames(plan)).To(Equal([]string{"listPets", "addPet", "getPet"}))

			Expect(findTarget(plan, "listPets").URLs).To(Equal([]string{"/v2/pets?status=available"}))

			get := findTarget(plan, "getPet")
			Expect(get.URLs).To(Equal([]string{"/v2/pets/1"}))
			Expect(get.Header.Get("X-Trace")).To(Equal("trace-1"))

			add := findTarget(plan, "addPet")
			Expect(add.Body).To(MatchJSON(`{"name":"rex"}`))
			Expect(add.Header.Get("Content-Type")).To(Equal("application/json"))
		})
	})

	Context("with explicit targets", func() {
		It("works without a contract and expands param-values in placeholders", func() {
			sc = &config.StressConfig{
				Source: config.StressSource{
					Headers:     map[string]string{"Accept": "application/json"},
					ParamValues: map[string][]string{"id": {"a", "b"}},
				},
				Targets: []config.StressTarget{
					{Method: "get", Path: "/users/{id}", Headers: map[string]string{"X-Extra": "1"}},
					{Name: "create", Method: "POST", Path: "/users", Body: `{"name":"x"}`, Weight: 2},
				},
			}
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())
			Expect(targetNames(plan)).To(Equal([]string{"GET /users/{id}", "create"}))

			get := plan.Targets[0]
			Expect(get.URLs).To(Equal([]string{"/users/a", "/users/b"}))
			Expect(get.Header.Get("Accept")).To(Equal("application/json"))
			Expect(get.Header.Get("X-Extra")).To(Equal("1"))
			Expect(get.Source).To(Equal("config"))

			create := plan.Targets[1]
			Expect(string(create.Body)).To(Equal(`{"name":"x"}`))
			Expect(create.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(create.Weight).To(Equal(2))
		})

		It("replaces the spec operation with the same method and full path", func() {
			sc = &config.StressConfig{
				Source:  config.StressSource{OpenAPI: fixtures + "openapi3.yml"},
				Targets: []config.StressTarget{{Name: "health-override", Method: "GET", Path: "/api/v1/health", Weight: 5}},
			}
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.Targets).To(HaveLen(4))
			Expect(plan.Targets[0].Name).To(Equal("health-override"))
			Expect(plan.Targets[0].Weight).To(Equal(5))
		})

		It("reads body-file", func() {
			sc = &config.StressConfig{Targets: []config.StressTarget{
				{Method: "PUT", Path: "/orders", BodyFile: fixtures + "order.json"},
			}}
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.Targets[0].Body).To(MatchJSON(`{"quantity":3}`))
		})

		It("fails when body-file is missing", func() {
			sc = &config.StressConfig{Targets: []config.StressTarget{
				{Method: "PUT", Path: "/orders", BodyFile: "/missing.json"},
			}}
			_, err := stress.ResolveTargets(sc)
			Expect(err).To(HaveOccurred())
		})

		It("warns about many targets with equal weights", func() {
			sc = &config.StressConfig{}
			for i := 0; i < 21; i++ {
				sc.Targets = append(sc.Targets, config.StressTarget{Method: "GET", Path: "/r/" + string(rune('a'+i))})
			}
			plan, err := stress.ResolveTargets(sc)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.Warnings).To(ContainElement(ContainSubstring("21 targets with equal weights")))
		})
	})

	Context("with an unreadable contract", func() {
		It("fails when the file does not exist", func() {
			_, err := stress.ResolveTargets(&config.StressConfig{Source: config.StressSource{OpenAPI: "/nope.yml"}})
			Expect(err).To(HaveOccurred())
			Expect(gtErrors.Is(err, gtErrors.ErrConfigNotFound)).To(BeTrue())
		})

		It("fails when the file is not a contract", func() {
			path := filepath.Join(GinkgoT().TempDir(), "bad.yml")
			Expect(os.WriteFile(path, []byte("just: yaml"), 0o644)).To(Succeed())
			_, err := stress.ResolveTargets(&config.StressConfig{Source: config.StressSource{OpenAPI: path}})
			Expect(err).To(HaveOccurred())
			Expect(gtErrors.Is(err, gtErrors.ErrConfigInvalid)).To(BeTrue())
		})
	})
})
