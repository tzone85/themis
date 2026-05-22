package classify

import (
	"encoding/json"
	"testing"
)

func TestKind_SeverityOrdering(t *testing.T) {
	cases := []struct {
		lower, higher Kind
	}{
		{KindDocOnly, KindNonContract},
		{KindNonContract, KindOffCatalogue},
		{KindOffCatalogue, KindConsumerTouch},
		{KindConsumerTouch, KindProducerTouch},
		{KindProducerTouch, KindNewEvent},
		{KindNewEvent, KindSchemaBreaking},
	}
	for _, c := range cases {
		if c.lower.Severity() >= c.higher.Severity() {
			t.Errorf("severity(%q)=%d should be < severity(%q)=%d",
				c.lower, c.lower.Severity(), c.higher, c.higher.Severity())
		}
	}
}

func TestKind_UnknownSeverityNegativeOne(t *testing.T) {
	if Kind("WHO?").Severity() != -1 {
		t.Fatal("unknown kind severity should be -1")
	}
}

func TestImpact_IsContract(t *testing.T) {
	contract := []Kind{KindSchemaBreaking, KindNewEvent, KindConsumerTouch, KindProducerTouch}
	for _, k := range contract {
		if !(Impact{Kind: k}).IsContract() {
			t.Errorf("%q should be a contract kind", k)
		}
	}
	nonContract := []Kind{KindDocOnly, KindOffCatalogue, KindNonContract}
	for _, k := range nonContract {
		if (Impact{Kind: k}).IsContract() {
			t.Errorf("%q should NOT be a contract kind", k)
		}
	}
}

func TestImpact_RoundTrip(t *testing.T) {
	in := Impact{
		Kind:    KindConsumerTouch,
		Domain:  "Collections",
		Service: "dispatcher",
		EventID: "",
		Reason:  "service modified",
		AffectedEvents:    []string{"PaymentReceived"},
		AffectedConsumers: []string{"dispatcher"},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Impact
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != in.Kind || out.Service != in.Service || len(out.AffectedEvents) != 1 {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}
