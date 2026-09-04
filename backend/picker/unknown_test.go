package picker

import "testing"

func TestUnknownTokens_Empty(t *testing.T) {
	if got := UnknownTokens(""); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}
