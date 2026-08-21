package actionlint

import (
	"strings"
	"testing"
)

func TestParallelStepLiteralFields(t *testing.T) {
	for _, tc := range []struct {
		step string
		want string
	}{
		{"run: echo ok\n        background: ${{ no_such_context.value }}", "\"background\" must be a boolean literal"},
		{"run: echo ok\n        background: ${{ 1 + }}", "\"background\" must be a boolean literal"},
		{"wait: ${{ no_such_context.value }}", "\"wait\" requires literal background step IDs"},
		{"wait: [server, '${{ 1 + }}']", "\"wait\" requires literal background step IDs"},
		{"cancel: ${{ no_such_context.value }}", "\"cancel\" requires a literal background step ID"},
		{"cancel: ${{ 1 + }}", "\"cancel\" requires a literal background step ID"},
		{"wait-all: true", "\"wait-all\" does not take a value"},
		{"wait-all: ${{ no_such_context.value }}", "\"wait-all\" does not take a value"},
		{"wait-all: ${{ 1 + }}", "\"wait-all\" does not take a value"},
	} {
		t.Run(tc.step, func(t *testing.T) {
			src := "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - " + tc.step + "\n"
			_, errs := Parse([]byte(src))
			for _, err := range errs {
				if strings.Contains(err.Message, tc.want) {
					return
				}
			}
			t.Fatalf("wanted %q, got %v", tc.want, errs)
		})
	}
}
