package classification

import "testing"

func TestSuggestReturnsExplainableDeweySuggestions(t *testing.T) {
	got := Suggest("An introduction to Go programming, software architecture and databases", 3)
	if len(got) == 0 {
		t.Fatal("expected at least one suggestion")
	}
	if got[0].Code != "004" {
		t.Fatalf("expected computer science first, got %+v", got[0])
	}
	if len(got[0].Evidence) == 0 {
		t.Fatal("suggestion must include evidence")
	}
}

func TestSuggestDoesNotInventClassification(t *testing.T) {
	if got := Suggest("a completely unrelated collection of words", 3); len(got) != 0 {
		t.Fatalf("expected no suggestions, got %+v", got)
	}
}

func TestSuggestUsesWholeWords(t *testing.T) {
	if got := Suggest("department annual report", 3); len(got) != 0 {
		t.Fatalf("substring match produced false classification: %+v", got)
	}
}
