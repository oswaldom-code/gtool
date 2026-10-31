package stress

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/pb33f/go-yaml"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/renderer"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const jsonContentType = "application/json"

// specParam is a path, query or header parameter with the value the spec
// suggests for it (example/default/enum[0], or one generated from its schema).
type specParam struct {
	name     string
	in       string
	required bool
	value    string
}

// specOperation is the spec-agnostic view of one operation, shared by the
// Swagger 2 and OpenAPI 3 readers.
type specOperation struct {
	id          string
	method      string
	path        string
	tags        []string
	params      []specParam
	contentType string
	body        []byte
}

// specDocument is what the resolver needs from a contract.
type specDocument struct {
	basePath   string
	operations []specOperation
	warnings   []string
}

// newMockGenerator returns a seeded JSON mock generator so generated values
// are stable between runs and `plan` matches what `stress` sends.
func newMockGenerator() *renderer.MockGenerator {
	mg := renderer.NewMockGenerator(renderer.JSON)
	mg.SetSeed(1)
	return mg
}

// nodeString renders a YAML node as a request value: scalars verbatim, any
// other node as JSON.
func nodeString(n *yaml.Node) (string, bool) {
	if n == nil {
		return "", false
	}
	if n.Kind == yaml.ScalarNode {
		return n.Value, true
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return "", false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// mockString converts a generated JSON mock into a parameter value.
func mockString(raw []byte) (string, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case json.Number, bool:
		return fmt.Sprint(t), true
	case nil:
		return "", false
	default:
		return strings.TrimSpace(string(raw)), true
	}
}

// schemaValue picks default > example > enum[0] > generated mock.
func schemaValue(s *base.Schema, mg *renderer.MockGenerator) (string, bool) {
	if s == nil {
		return "", false
	}
	if v, ok := nodeString(s.Default); ok {
		return v, true
	}
	if v, ok := nodeString(s.Example); ok {
		return v, true
	}
	if len(s.Examples) > 0 {
		if v, ok := nodeString(s.Examples[0]); ok {
			return v, true
		}
	}
	if len(s.Enum) > 0 {
		if v, ok := nodeString(s.Enum[0]); ok {
			return v, true
		}
	}
	raw, err := mg.GenerateMock(s, "")
	if err != nil {
		return "", false
	}
	return mockString(raw)
}

func proxySchema(p *base.SchemaProxy) *base.Schema {
	if p == nil {
		return nil
	}
	return p.Schema()
}

func isRequired(b *bool) bool {
	return b != nil && *b
}

// v3ParamValue picks example > examples > schema value.
func v3ParamValue(p *v3.Parameter, mg *renderer.MockGenerator) string {
	if v, ok := nodeString(p.Example); ok {
		return v
	}
	for _, ex := range p.Examples.FromOldest() {
		if v, ok := nodeString(ex.Value); ok {
			return v
		}
	}
	v, _ := schemaValue(proxySchema(p.Schema), mg)
	return v
}

// mergeParams overlays operation parameters on path-level ones (same name+in).
func mergeParams(pathLevel, opLevel []specParam) []specParam {
	out := make([]specParam, 0, len(pathLevel)+len(opLevel))
	seen := make(map[string]bool, len(opLevel))
	for _, p := range opLevel {
		seen[p.in+":"+p.name] = true
	}
	for _, p := range pathLevel {
		if !seen[p.in+":"+p.name] {
			out = append(out, p)
		}
	}
	return append(out, opLevel...)
}

func v3Params(params []*v3.Parameter, mg *renderer.MockGenerator) []specParam {
	out := make([]specParam, 0, len(params))
	for _, p := range params {
		if p == nil || p.In == "cookie" {
			continue
		}
		out = append(out, specParam{
			name:     p.Name,
			in:       p.In,
			required: isRequired(p.Required) || p.In == "path",
			value:    v3ParamValue(p, mg),
		})
	}
	return out
}

// v3Body picks the JSON media type when present (else the first one) and
// renders its example, or a mock generated from its schema.
func v3Body(rb *v3.RequestBody, mg *renderer.MockGenerator) (string, []byte, error) {
	if rb == nil || rb.Content == nil || rb.Content.Len() == 0 {
		return "", nil, nil
	}
	contentType := ""
	var media *v3.MediaType
	for ct, mt := range rb.Content.FromOldest() {
		if contentType == "" || ct == jsonContentType {
			contentType, media = ct, mt
		}
	}
	body, err := mg.GenerateMock(media, "")
	return contentType, body, err
}

// serverBasePath extracts the path of the first server URL ("" when absent or
// templated).
func serverBasePath(servers []*v3.Server) string {
	if len(servers) == 0 || strings.Contains(servers[0].URL, "{") {
		return ""
	}
	u, err := url.Parse(servers[0].URL)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(u.Path, "/")
}

func readV3(doc libopenapi.Document) (*specDocument, error) {
	model, err := doc.BuildV3Model()
	if model == nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to build OpenAPI 3 model")
	}
	mg := newMockGenerator()
	out := &specDocument{basePath: serverBasePath(model.Model.Servers)}
	if model.Model.Paths == nil {
		return out, nil
	}
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		pathParams := v3Params(item.Parameters, mg)
		for method, op := range item.GetOperations().FromOldest() {
			ct, body, err := v3Body(op.RequestBody, mg)
			if err != nil {
				out.warnings = append(out.warnings,
					fmt.Sprintf("%s %s: could not generate request body: %v", strings.ToUpper(method), path, err))
			}
			out.operations = append(out.operations, specOperation{
				id:          op.OperationId,
				method:      strings.ToUpper(method),
				path:        path,
				tags:        op.Tags,
				params:      mergeParams(pathParams, v3Params(op.Parameters, mg)),
				contentType: ct,
				body:        body,
			})
		}
	}
	return out, nil
}

// v2SimpleValue picks default > enum[0] > a placeholder for the declared type.
// Swagger 2 non-body parameters have no schema to generate from.
func v2SimpleValue(p *v2.Parameter) string {
	if v, ok := nodeString(p.Default); ok {
		return v
	}
	if len(p.Enum) > 0 {
		if v, ok := nodeString(p.Enum[0]); ok {
			return v
		}
	}
	switch p.Type {
	case "integer", "number":
		return "1"
	case "boolean":
		return "true"
	default:
		return "string"
	}
}

func v2Params(params []*v2.Parameter) []specParam {
	out := make([]specParam, 0, len(params))
	for _, p := range params {
		if p == nil || (p.In != "path" && p.In != "query" && p.In != "header") {
			continue
		}
		out = append(out, specParam{
			name:     p.Name,
			in:       p.In,
			required: isRequired(p.Required) || p.In == "path",
			value:    v2SimpleValue(p),
		})
	}
	return out
}

func v2Body(params []*v2.Parameter, mg *renderer.MockGenerator) ([]byte, error) {
	for _, p := range params {
		if p != nil && p.In == "body" {
			return mg.GenerateMock(proxySchema(p.Schema), "")
		}
	}
	return nil, nil
}

func v2ContentType(op *v2.Operation, consumes []string) string {
	list := op.Consumes
	if len(list) == 0 {
		list = consumes
	}
	for _, ct := range list {
		if ct == jsonContentType {
			return ct
		}
	}
	if len(list) > 0 {
		return list[0]
	}
	return jsonContentType
}

func readV2(doc libopenapi.Document) (*specDocument, error) {
	model, err := doc.BuildV2Model()
	if model == nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to build Swagger 2 model")
	}
	mg := newMockGenerator()
	out := &specDocument{basePath: strings.TrimSuffix(model.Model.BasePath, "/")}
	if model.Model.Paths == nil {
		return out, nil
	}
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		pathParams := v2Params(item.Parameters)
		for method, op := range item.GetOperations().FromOldest() {
			body, err := v2Body(op.Parameters, mg)
			if err != nil {
				out.warnings = append(out.warnings,
					fmt.Sprintf("%s %s: could not generate request body: %v", strings.ToUpper(method), path, err))
			}
			ct := ""
			if body != nil {
				ct = v2ContentType(op, model.Model.Consumes)
			}
			out.operations = append(out.operations, specOperation{
				id:          op.OperationId,
				method:      strings.ToUpper(method),
				path:        path,
				tags:        op.Tags,
				params:      mergeParams(pathParams, v2Params(op.Parameters)),
				contentType: ct,
				body:        body,
			})
		}
	}
	return out, nil
}

// loadSpec reads a Swagger 2.0 or OpenAPI 3.x contract from disk.
func loadSpec(path string) (*specDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigNotFound, fmt.Sprintf("failed to read OpenAPI file %s", path))
	}
	doc, err := libopenapi.NewDocument(data)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, fmt.Sprintf("failed to parse OpenAPI file %s", path))
	}
	if doc.GetSpecInfo().SpecFormat == datamodel.OAS2 {
		return readV2(doc)
	}
	return readV3(doc)
}
