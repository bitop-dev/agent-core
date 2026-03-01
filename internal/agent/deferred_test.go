package agent

import "testing"

func TestLooksLikeDeferredAction_DetectsPromises(t *testing.T) {
	positives := []string{
		"Let me check the file.",
		"I'll run the tests now.",
		"I am going to search for that.",
		"We'll verify the output.",
		"Let's look at the error logs.",
		"I will try a different approach.",
		"Let me read the configuration.",
		"I'll execute the build command.",
		"Let us investigate the issue.",
		"I'll fetch the latest data.",
		"Let me examine the source code.",
		"I will scan the directory.",
		"Let me review the changes.",
		"It seems absolute paths are blocked. Let me try using a relative path.",
		"Webpage opened, let's see what's new here.",
	}

	for _, text := range positives {
		if !LooksLikeDeferredAction(text) {
			t.Errorf("expected positive for: %q", text)
		}
	}
}

func TestLooksLikeDeferredAction_IgnoresFinalAnswers(t *testing.T) {
	negatives := []string{
		"The latest update is already shown above.",
		"Here is the result of the analysis.",
		"The file contains 42 lines.",
		"I found 3 matching results.",
		"The test passed successfully.",
		"No issues were detected.",
		"",
		"   ",
		"Done!",
		"The build completed without errors.",
		"Here's what I found in the logs.",
	}

	for _, text := range negatives {
		if LooksLikeDeferredAction(text) {
			t.Errorf("expected negative for: %q", text)
		}
	}
}

func TestLooksLikeDeferredAction_EmptyInput(t *testing.T) {
	if LooksLikeDeferredAction("") {
		t.Error("empty input should not match")
	}
	if LooksLikeDeferredAction("   ") {
		t.Error("whitespace input should not match")
	}
}
