package actionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandMain(t *testing.T) {
	var output bytes.Buffer

	// Create command instance populating stdin/stdout/stderr
	cmd := Command{
		Stdin:  os.Stdin,
		Stdout: &output,
		Stderr: &output,
	}

	// Run the command end-to-end. Note that given args should contain program name
	workflow := filepath.Join("testdata", "examples", "main.yaml")
	status := cmd.Main([]string{"actionlint", "-shellcheck=", "-ruff=", "-ignore", `label .+ is unknown\.`, workflow})

	if status != 1 {
		t.Fatal("exit status should be 1 but got", status)
	}

	out := output.String()

	for _, s := range []string{
		"main.yaml:3:5:",
		"unexpected key \"branch\" for \"push\" section",
		"^~~~~~~~~~~~~~~",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("output should contain %q: %q", s, out)
		}
	}

	if strings.Contains(out, "[runner-label]") {
		t.Errorf("runner-label rule should be ignored by -ignore but it is included in output: %q", out)
	}
}

func TestCommandRejectPyflakes(t *testing.T) {
	var output bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &output, Stderr: &output}
	if status := cmd.Main([]string{"actionlint", "-pyflakes="}); status != ExitStatusInvalidCommandOption {
		t.Fatalf("status = %d, want invalid command option; output: %s", status, &output)
	}
	if !strings.Contains(output.String(), "flag provided but not defined: -pyflakes") {
		t.Fatalf("unexpected output: %s", &output)
	}
}
