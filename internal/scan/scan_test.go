package scan

import "testing"

func TestSeverity_RankOrdering(t *testing.T) {
	order := []Severity{SeverityInfo, SeverityLow, SeverityMed, SeverityHigh, SeverityCritical}
	for i := 1; i < len(order); i++ {
		if order[i-1].Rank() >= order[i].Rank() {
			t.Errorf("severity(%q)=%d should be < severity(%q)=%d",
				order[i-1], order[i-1].Rank(), order[i], order[i].Rank())
		}
	}
}

func TestSeverity_UnknownIsNegativeOne(t *testing.T) {
	if Severity("WHO?").Rank() != -1 {
		t.Fatal("unknown severity rank should be -1")
	}
}
