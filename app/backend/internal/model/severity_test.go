package model

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestSeverities(t *testing.T) {
	if got := SeverityNames(); !slices.Equal(got, []string{"critical", "error", "warning", "info"}) {
		t.Fatalf("names = %v", got)
	}
	for i := 1; i < len(Severities); i++ {
		if Severities[i].Rank >= Severities[i-1].Rank {
			t.Fatalf("severities are not ordered most severe first: %v", Severities)
		}
	}
	if ValidSeverity("bogus") || !ValidSeverity("warning") {
		t.Fatal("ValidSeverity")
	}
}

func TestSeverityCountsJSON(t *testing.T) {
	type counts struct {
		Total int `json:"total"`
		SeverityCounts
		Open int `json:"open"`
	}
	c := counts{Total: 4}
	for _, s := range []string{"critical", "critical", "info", "bogus"} {
		c.Add(s, 1)
	}
	b, _ := json.Marshal(c)
	if want := `{"total":4,"critical":2,"error":0,"warning":0,"info":1,"open":0}`; string(b) != want {
		t.Fatalf("json = %s, want %s", b, want)
	}
	if c.Get("critical") != 2 || c.Get("bogus") != 0 {
		t.Fatal("Get")
	}
}

func TestMethods(t *testing.T) {
	if MethodCounterpart(MethodRED) != MethodUSE || MethodCounterpart(MethodUSE) != MethodRED || MethodCounterpart(MethodOther) != "" {
		t.Fatal("counterparts")
	}
	if !ValidMethod("other") || ValidMethod("") || ValidMethod("RED") {
		t.Fatal("ValidMethod")
	}
}
