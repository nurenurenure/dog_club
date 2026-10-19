package api_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestOpenAPIValidates(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Guard against a validator that parses 3.1 keywords but silently ignores
	// const: these statuses must remain distinct even though all are strings.
	for _, name := range []string{"Liveness", "Readiness", "NotReady"} {
		if err := doc.Components.Schemas[name].Value.VisitJSON(map[string]any{"status": "incorrect"}, openapi3.EnableJSONSchema2020()); err == nil {
			t.Fatalf("%s accepted an undocumented health status", name)
		}
	}
}

func TestOpenAPISchemasCompileAgainstWholeDocument(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var root any
	if err = json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err = c.AddResource("https://dogclub.test/openapi.json", root); err != nil {
		t.Fatal(err)
	}
	for name := range doc.Components.Schemas {
		if _, err = c.Compile("https://dogclub.test/openapi.json#/components/schemas/" + name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
