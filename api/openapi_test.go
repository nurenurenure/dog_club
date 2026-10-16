package api_test

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
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
