package stress

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const (
	sourceOpenAPI = "openapi"
	sourceConfig  = "config"

	// manyTargetsWarning is the target count above which `plan` warns when no
	// weight was customized.
	manyTargetsWarning = 20
)

// Target is one resolved endpoint the load is distributed across.
type Target struct {
	Name   string
	Method string
	// Path is the full request path template, base path included.
	Path string
	// URLs are rendered path+query variants, rotated per request so every
	// configured param value is exercised.
	URLs   []string
	Header http.Header
	Body   []byte
	Weight int
	Source string
}

// Plan is the resolved list of targets plus resolution warnings.
type Plan struct {
	Targets  []Target
	Warnings []string
}

var mutatingMethods = map[string]bool{
	http.MethodPost: true, http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
}

func normalizeMethods(methods []string) map[string]bool {
	if len(methods) == 0 {
		methods = []string{http.MethodGet}
	}
	out := make(map[string]bool, len(methods))
	for _, m := range methods {
		out[strings.ToUpper(m)] = true
	}
	return out
}

func matchesAnyPath(patterns []string, p string) bool {
	for _, pattern := range patterns {
		if ok, _ := path.Match(pattern, p); ok {
			return true
		}
	}
	return false
}

func hasAnyTag(tags, wanted []string) bool {
	for _, t := range tags {
		for _, w := range wanted {
			if t == w {
				return true
			}
		}
	}
	return false
}

// keepOperation applies the methods, tags and paths filters. Path filters
// match the spec path (without base path) using path.Match globs.
func keepOperation(op specOperation, src config.StressSource, methods map[string]bool) bool {
	if !methods[op.method] {
		return false
	}
	if len(src.IncludeTags) > 0 && !hasAnyTag(op.tags, src.IncludeTags) {
		return false
	}
	if hasAnyTag(op.tags, src.ExcludeTags) {
		return false
	}
	if len(src.IncludePaths) > 0 && !matchesAnyPath(src.IncludePaths, op.path) {
		return false
	}
	return !matchesAnyPath(src.ExcludePaths, op.path)
}

// variantCount is the longest param-values list used by the given params.
func variantCount(params []specParam, values map[string][]string) int {
	n := 1
	for _, p := range params {
		if l := len(values[p.name]); l > n {
			n = l
		}
	}
	return n
}

// paramValue resolves the k-th variant: param-values (rotating) > spec value.
func paramValue(p specParam, values map[string][]string, k int) (string, bool) {
	if vals := values[p.name]; len(vals) > 0 {
		return vals[k%len(vals)], true
	}
	return p.value, p.value != ""
}

// renderURLs substitutes path params and appends query params for every
// variant. Optional query params are only sent when param-values sets them.
func renderURLs(pathTemplate string, params []specParam, values map[string][]string) []string {
	n := variantCount(params, values)
	urls := make([]string, 0, n)
	for k := 0; k < n; k++ {
		p := pathTemplate
		query := url.Values{}
		var queryKeys []string
		for _, param := range params {
			switch param.in {
			case "path":
				v, ok := paramValue(param, values, k)
				if !ok {
					v = "1"
				}
				p = strings.ReplaceAll(p, "{"+param.name+"}", url.PathEscape(v))
			case "query":
				_, configured := values[param.name]
				if !param.required && !configured {
					continue
				}
				if v, ok := paramValue(param, values, k); ok {
					query.Set(param.name, v)
					queryKeys = append(queryKeys, param.name)
				}
			}
		}
		urls = append(urls, p+encodeQuery(query, queryKeys))
	}
	return urls
}

// encodeQuery keeps the declaration order so rendered URLs are stable.
func encodeQuery(q url.Values, keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(q.Get(k)))
	}
	return "?" + strings.Join(parts, "&")
}

// expandHeaders builds a header set, expanding ${VAR} references.
func expandHeaders(headers map[string]string) http.Header {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, os.ExpandEnv(v))
	}
	return h
}

func weightOrDefault(w int) int {
	if w <= 0 {
		return 1
	}
	return w
}

func readBodyFile(file string) ([]byte, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigNotFound, fmt.Sprintf("failed to read body file %s", file))
	}
	return data, nil
}

// specTarget turns a filtered spec operation into a Target.
func specTarget(op specOperation, src config.StressSource, basePath string) (Target, error) {
	override := src.OperationOverrides[op.id]
	header := expandHeaders(src.Headers)
	for _, p := range op.params {
		if p.in != "header" {
			continue
		}
		_, configured := src.ParamValues[p.name]
		if v, ok := paramValue(p, src.ParamValues, 0); ok && (p.required || configured) {
			header.Set(p.name, v)
		}
	}

	body := op.body
	if override.BodyFile != "" {
		b, err := readBodyFile(override.BodyFile)
		if err != nil {
			return Target{}, err
		}
		body = b
	}
	if len(body) > 0 && header.Get("Content-Type") == "" {
		ct := op.contentType
		if ct == "" {
			ct = jsonContentType
		}
		header.Set("Content-Type", ct)
	}

	name := op.id
	if name == "" {
		name = op.method + " " + op.path
	}
	fullPath := basePath + op.path
	return Target{
		Name:   name,
		Method: op.method,
		Path:   fullPath,
		URLs:   renderURLs(fullPath, op.params, src.ParamValues),
		Header: header,
		Body:   body,
		Weight: weightOrDefault(override.Weight),
		Source: sourceOpenAPI,
	}, nil
}

// pathPlaceholders returns the {name} placeholders of an explicit path as
// path params, so param-values also apply to explicit targets.
func pathPlaceholders(p string) []specParam {
	var params []specParam
	for {
		start := strings.Index(p, "{")
		if start < 0 {
			return params
		}
		end := strings.Index(p[start:], "}")
		if end < 0 {
			return params
		}
		params = append(params, specParam{name: p[start+1 : start+end], in: "path", required: true})
		p = p[start+end+1:]
	}
}

// explicitTarget builds a Target from the `targets` block. Its path is used
// as-is (no base path is prepended).
func explicitTarget(t config.StressTarget, src config.StressSource) (Target, error) {
	method := strings.ToUpper(t.Method)
	header := expandHeaders(src.Headers)
	for k, v := range expandHeaders(t.Headers) {
		header[k] = v
	}

	body := []byte(t.Body)
	if t.BodyFile != "" {
		b, err := readBodyFile(t.BodyFile)
		if err != nil {
			return Target{}, err
		}
		body = b
	}
	if len(body) > 0 && header.Get("Content-Type") == "" {
		header.Set("Content-Type", jsonContentType)
	}

	name := t.Name
	if name == "" {
		name = method + " " + t.Path
	}
	return Target{
		Name:   name,
		Method: method,
		Path:   t.Path,
		URLs:   renderURLs(t.Path, pathPlaceholders(t.Path), src.ParamValues),
		Header: header,
		Body:   body,
		Weight: weightOrDefault(t.Weight),
		Source: sourceConfig,
	}, nil
}

func targetKey(method, p string) string {
	return method + " " + p
}

// planWarnings flags setups that tend to distort the measurement.
func planWarnings(targets []Target) []string {
	var warnings []string
	var mutating []string
	customWeights := false
	for _, t := range targets {
		if mutatingMethods[t.Method] {
			mutating = append(mutating, t.Name)
		}
		if t.Weight != 1 {
			customWeights = true
		}
	}
	if len(mutating) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"mutating methods enabled (%s): writes fill the mocks and can make them the bottleneck",
			strings.Join(mutating, ", ")))
	}
	if len(targets) > manyTargetsWarning && !customWeights {
		warnings = append(warnings, fmt.Sprintf(
			"%d targets with equal weights: consider filters or weights to focus the load", len(targets)))
	}
	return warnings
}

// ResolveTargets builds the target plan from the OpenAPI contract and the
// explicit `targets` block. Explicit targets replace spec operations with the
// same method and full path.
func ResolveTargets(sc *config.StressConfig) (*Plan, error) {
	plan := &Plan{}
	src := sc.Source

	if src.OpenAPI != "" {
		doc, err := loadSpec(src.OpenAPI)
		if err != nil {
			return nil, err
		}
		plan.Warnings = append(plan.Warnings, doc.warnings...)

		basePath := doc.basePath
		if src.BasePath != "" {
			basePath = strings.TrimSuffix(src.BasePath, "/")
		}
		methods := normalizeMethods(src.Methods)
		for _, op := range doc.operations {
			if !keepOperation(op, src, methods) {
				continue
			}
			t, err := specTarget(op, src, basePath)
			if err != nil {
				return nil, err
			}
			plan.Targets = append(plan.Targets, t)
		}
	}

	for _, et := range sc.Targets {
		t, err := explicitTarget(et, src)
		if err != nil {
			return nil, err
		}
		replaced := false
		for i := range plan.Targets {
			if targetKey(plan.Targets[i].Method, plan.Targets[i].Path) == targetKey(t.Method, t.Path) {
				plan.Targets[i] = t
				replaced = true
				break
			}
		}
		if !replaced {
			plan.Targets = append(plan.Targets, t)
		}
	}

	if len(plan.Targets) == 0 {
		return nil, gtErrors.New(gtErrors.ErrConfigInvalid,
			"no stress targets resolved: check the openapi filters (methods, tags, paths) or the targets block")
	}
	plan.Warnings = append(plan.Warnings, planWarnings(plan.Targets)...)
	return plan, nil
}
