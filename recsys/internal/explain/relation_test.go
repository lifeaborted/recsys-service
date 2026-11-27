package explain

import "testing"

func TestNamesBothProducts(t *testing.T) {
	if HasRelationEndpoints("Basic Plus, 12 months", "Basic", "Basic Plus") {
		t.Fatal("a title containing the other product name is only one mention")
	}
	for _, quote := range []string{"Move from Basic to Basic Plus.", "Basic Plus can replace Basic."} {
		if !HasRelationEndpoints(quote, "Basic", "Basic Plus") {
			t.Fatalf("expected two separate names in %q", quote)
		}
	}
}
