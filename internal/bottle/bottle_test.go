package bottle_test

import (
	"encoding/json"
	"strings"
	"testing"

	"capper/internal/bottle"
)

func minimalSpec(t *testing.T) []byte {
	t.Helper()
	spec := bottle.BottleSpec{
		APIVersion: "capper.io/v1",
		Kind:       "bottle",
		Metadata: bottle.BottleMetadata{
			Name:    "test-bottle",
			Version: "1.0.0",
			Author:  "test",
			Tags:    []string{"test"},
		},
		Spec: bottle.BottleSpecBody{
			Parameters: map[string]bottle.ParameterSpec{
				"app.port": {Type: "port", Default: "8080"},
				"app.name": {Type: "string", Default: "myapp", Required: false},
			},
			Services: []bottle.ServiceSpec{
				{Name: "web", Image: "{{ build.outputImage }}", Replicas: "1"},
			},
			Outputs: map[string]bottle.OutputSpec{
				"port": {Description: "Listen port", Value: "{{ parameters.app.port }}"},
			},
		},
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseSpec(t *testing.T) {
	data := minimalSpec(t)
	spec, err := bottle.ParseSpec(data)
	if err != nil {
		t.Fatalf("ParseSpec error: %v", err)
	}
	if spec.Metadata.Name != "test-bottle" {
		t.Errorf("name: got %q, want %q", spec.Metadata.Name, "test-bottle")
	}
	if spec.Metadata.Version != "1.0.0" {
		t.Errorf("version: got %q", spec.Metadata.Version)
	}
	if len(spec.Spec.Parameters) != 2 {
		t.Errorf("parameters: got %d, want 2", len(spec.Spec.Parameters))
	}
}

func TestParseSpec_Invalid(t *testing.T) {
	_, err := bottle.ParseSpec([]byte(`not json`))
	if err == nil {
		t.Fatal("expected parse error for invalid JSON")
	}
}

func TestValidateSpec_Valid(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	errs := bottle.ValidateSpec(spec, nil)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateSpec_RejectsLegacyNetworks(t *testing.T) {
	spec, _ := bottle.ParseSpec(minimalSpec(t))
	spec.Spec.Resources.Networks = []bottle.NetworkSpec{{Name: "app-net"}}
	errs := bottle.ValidateSpec(spec, nil)
	if len(errs) == 0 || !strings.Contains(errs[0], "VPC subnet placement") {
		t.Fatalf("expected actionable legacy network validation error, got %v", errs)
	}
}

func TestValidateSpec_MissingName(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	spec.Metadata.Name = ""
	errs := bottle.ValidateSpec(spec, nil)
	if len(errs) == 0 {
		t.Fatal("expected validation error for missing name")
	}
}

func TestValidateSpec_MissingVersion(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	spec.Metadata.Version = ""
	errs := bottle.ValidateSpec(spec, nil)
	if len(errs) == 0 {
		t.Fatal("expected validation error for missing version")
	}
}

func TestValidateSpec_RequiredParam(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	spec.Spec.Parameters["eula"] = bottle.ParameterSpec{Type: "boolean", Required: true}
	errs := bottle.ValidateSpec(spec, nil) // no params supplied
	found := false
	for _, e := range errs {
		if e != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected validation error for missing required parameter")
	}
}

func TestValidateSpec_WrongKind(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	spec.Kind = "Stack"
	errs := bottle.ValidateSpec(spec, nil)
	if len(errs) == 0 {
		t.Fatal("expected error for wrong kind")
	}
}

func TestValidateSpec_ServiceRefsUndefinedNetwork(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	spec.Spec.Services[0].Network = "nonexistent-net"
	errs := bottle.ValidateSpec(spec, nil)
	if len(errs) == 0 {
		t.Fatal("expected error for service referencing undefined network")
	}
}

func TestRenderTemplate(t *testing.T) {
	cases := []struct {
		tmpl string
		ctx  map[string]string
		want string
	}{
		{"hello {{ parameters.name }}", map[string]string{"parameters.name": "world"}, "hello world"},
		{"port {{ parameters.port }}", map[string]string{"parameters.port": "8080"}, "port 8080"},
		{"no-op", map[string]string{}, "no-op"},
		{"{{ metadata.version }}", map[string]string{"metadata.version": "1.2.3"}, "1.2.3"},
	}
	for _, tc := range cases {
		got := bottle.RenderTemplate(tc.tmpl, tc.ctx)
		if got != tc.want {
			t.Errorf("RenderTemplate(%q) = %q, want %q", tc.tmpl, got, tc.want)
		}
	}
}

func TestBuildContext_Defaults(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	ctx := bottle.BuildContext(spec, nil)
	if ctx["parameters.app.port"] != "8080" {
		t.Errorf("default not applied: got %q", ctx["parameters.app.port"])
	}
	if ctx["metadata.name"] != "test-bottle" {
		t.Errorf("metadata.name: got %q", ctx["metadata.name"])
	}
}

func TestBuildContext_Override(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	ctx := bottle.BuildContext(spec, map[string]string{"app.port": "9090"})
	if ctx["parameters.app.port"] != "9090" {
		t.Errorf("override not applied: got %q", ctx["parameters.app.port"])
	}
}

func TestPlan_Basic(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	// Add build so the image plan action fires.
	spec.Spec.Build = &bottle.BuildSpec{Enabled: true, OutputImage: "test-img", Steps: []bottle.BuildStep{{Run: "echo hi"}}}

	plan, err := bottle.Plan(spec, nil, "my-deploy")
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	kinds := make(map[string]int)
	for _, a := range plan {
		kinds[a.Kind]++
	}
	if kinds["image"] == 0 {
		t.Error("expected image build action in plan")
	}
	if kinds["instance-group"] == 0 {
		t.Error("expected instance-group action in plan")
	}
}

func TestPlan_BlocksOnValidation(t *testing.T) {
	data := minimalSpec(t)
	spec, _ := bottle.ParseSpec(data)
	spec.Metadata.Name = ""
	plan, err := bottle.Plan(spec, nil, "my-deploy")
	if err == nil {
		t.Fatal("expected Plan to return error for invalid spec")
	}
	for _, a := range plan {
		if a.Action == "block" {
			return
		}
	}
	t.Error("expected block action in plan")
}

func TestDigest(t *testing.T) {
	d1 := bottle.Digest([]byte("hello"))
	d2 := bottle.Digest([]byte("hello"))
	d3 := bottle.Digest([]byte("world"))
	if d1 != d2 {
		t.Error("digest not deterministic")
	}
	if d1 == d3 {
		t.Error("different inputs produced same digest")
	}
	if len(d1) < 10 {
		t.Error("digest too short")
	}
}
