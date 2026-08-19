package actionlint

import (
	"slices"
	"testing"
)

func TestGeneratedAllWebhooks(t *testing.T) {
	if len(AllWebhookTypes) == 0 {
		t.Fatal("AllWebhookTypes is empty")
	}

	for name, types := range AllWebhookTypes {
		if name == "" {
			t.Errorf("Name is empty (types=%v)", types)
			continue
		}

		seen := map[string]struct{}{}
		for _, ty := range types {
			if _, ok := seen[ty]; ok {
				t.Errorf("type %q duplicates in webhook %q: %v", ty, name, types)
			} else {
				seen[ty] = struct{}{}
			}
		}
	}
}

func TestDocumentedWebhookActivityTypes(t *testing.T) {
	for _, activity := range []string{"field_added", "field_removed"} {
		if !slices.Contains(AllWebhookTypes["issues"], activity) {
			t.Errorf("issues activity %q is missing", activity)
		}
	}
	// Webhook payload types are not necessarily supported Actions triggers.
	// https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#merge_group
	if !slices.Equal(AllWebhookTypes["merge_group"], []string{"checks_requested"}) {
		t.Errorf("unexpected merge_group activity types: %v", AllWebhookTypes["merge_group"])
	}
}
