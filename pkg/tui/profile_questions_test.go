package tui

import "testing"

func TestQuestionsCount(t *testing.T) {
	const want = 10
	if got := len(Questions); got != want {
		t.Fatalf("len(Questions) = %d, want %d", got, want)
	}
}

func TestQuestions_NoEmptyKeys(t *testing.T) {
	for i, q := range Questions {
		if q.FieldKey == "" {
			t.Errorf("Questions[%d].FieldKey is empty", i)
		}
		if q.LabelKey == "" {
			t.Errorf("Questions[%d].LabelKey is empty", i)
		}
		if len(q.Options) == 0 {
			t.Errorf("Questions[%d] has no options", i)
		}
		for j, opt := range q.Options {
			if opt.Key == "" {
				t.Errorf("Questions[%d].Options[%d].Key is empty", i, j)
			}
			if opt.LabelKey == "" {
				t.Errorf("Questions[%d].Options[%d].LabelKey is empty", i, j)
			}
		}
	}
}

func TestQuestions_MultiSelectQuestions(t *testing.T) {
	// Q4 (index 3) and Q8 (index 7) and Q10 (index 9) must be MultiSelect.
	multiSelectIndices := []int{3, 7, 9}
	for _, idx := range multiSelectIndices {
		if Questions[idx].Type != MultiSelect {
			t.Errorf("Questions[%d] (%s) should be MultiSelect", idx, Questions[idx].FieldKey)
		}
	}
}
