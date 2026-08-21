package actionlint

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/execabs"
)

const ruffTestDiagnostic = `[{"code":"F821","message":"Undefined name ` + "`missing`" + `","location":{"row":2,"column":7}}]`

func TestRuleRuffParseOutput(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		wantError    bool
		wantCount    int
	}{
		{"clean", "[]", false, 0},
		{"diagnostic", ruffTestDiagnostic, false, 1},
		{"syntax", `[{"code":"invalid-syntax","message":"unexpected EOF","location":{"row":1,"column":2}}]`, false, 1},
		{"empty", "", true, 0},
		{"null", "null", true, 0},
		{"object", "{}", true, 0},
		{"malformed", "[", true, 0},
		{"missing fields", "[{}]", true, 0},
		{"invalid position", `[{"code":"F821","message":"missing","location":{"row":0,"column":1}}]`, true, 0},
		{"trailing data", ruffTestDiagnostic + "oops", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := newRuleRuff(&externalCommand{})
			err := rule.parseOutput([]byte(tc.output), &Pos{Line: 10, Col: 9})
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error %v", err, tc.wantError)
			}
			if len(rule.Errs()) != tc.wantCount {
				t.Fatalf("diagnostics = %v, want %d", rule.Errs(), tc.wantCount)
			}
			if tc.name == "diagnostic" {
				want := ":10:9: Ruff reported issue in this script: 2:7: F821 Undefined name `missing` [ruff]"
				if got := rule.Errs()[0].Error(); got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
			}
		})
	}
}

// The test executable provides a portable stand-in for an external Ruff process.
func TestRuffHelperProcess(t *testing.T) {
	mode := os.Getenv("ACTIONLINT_TEST_RUFF_HELPER")
	if mode == "" {
		return
	}
	separator := slices.Index(os.Args, "--")
	want := []string{"check", "--isolated", "--target-version", "py314", "--select", "F", "--ignore-noqa", "--no-fix", "--no-cache", "--output-format", "json", "--stdin-filename", "actionlint.py", "-"}
	if separator < 0 || !slices.Equal(os.Args[separator+1:], want) {
		fmt.Fprintln(os.Stderr, "unexpected arguments", os.Args)
		os.Exit(2)
	}
	src, err := io.ReadAll(os.Stdin)
	if err != nil || strings.Contains(string(src), "${{") {
		fmt.Fprintln(os.Stderr, "unsanitized input", err)
		os.Exit(2)
	}
	if mode == "failure" {
		fmt.Println("[]")
		fmt.Fprintln(os.Stderr, "simulated Ruff failure")
		os.Exit(2)
	}
	fmt.Println(ruffTestDiagnostic)
	os.Exit(1)
}

func TestRuleRuffProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"findings", "failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ACTIONLINT_TEST_RUFF_HELPER", mode)
			proc := newConcurrentProcess(2)
			defer proc.wait()
			rule, err := NewRuleRuff(fmt.Sprintf("%q -test.run=^TestRuffHelperProcess$ --", exe), proc)
			if err != nil {
				t.Fatal(err)
			}
			err = rule.VisitStep(&Step{Exec: &ExecRun{
				Run: &String{Value: "print('${{ runner.os }}')"}, RunPos: &Pos{Line: 1, Col: 1},
				Shell: &String{Value: "python"},
			}})
			if err == nil {
				err = rule.VisitWorkflowPost(&Workflow{})
			}
			if mode == "failure" {
				if err == nil || !strings.Contains(err.Error(), "status 2") {
					t.Fatalf("Ruff status 2 was not fatal: %v", err)
				}
				if len(rule.Errs()) != 0 {
					t.Fatal("failed Ruff process recorded diagnostics")
				}
			} else if err != nil || len(rule.Errs()) != 1 {
				t.Fatalf("diagnostics %v, error %v", rule.Errs(), err)
			}
		})
	}
}

func TestCommandRuffIntegration(t *testing.T) {
	ruff, err := execabs.LookPath("ruff")
	if err != nil {
		t.Skip("Ruff is not installed")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ruff.toml"), []byte("exclude = ['*']\n[lint]\nignore = ['ALL']\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	for _, tc := range []struct {
		name, script, want string
		args               []string
		status             int
	}{
		{"undefined despite noqa", "print(missing) # noqa", "F821", nil, 1},
		{"file-level noqa", "# ruff: noqa\nprint(missing)", "F821", nil, 1},
		{"syntax error", "print(", "invalid-syntax", nil, 1},
		{"expression", "print('${{ runner.os }}')", "", nil, 0},
		{"newer builtins", "print(ExceptionGroup, PythonFinalizationError)", "", nil, 0},
		{"disabled", "print(missing)", "", []string{"-ruff="}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: python\n    steps:\n      - run: |\n          " + strings.ReplaceAll(tc.script, "\n", "\n          ") + "\n"
			var out bytes.Buffer
			cmd := Command{Stdin: strings.NewReader(src), Stdout: &out, Stderr: &out}
			args := append([]string{"actionlint", "-shellcheck=", "-ruff=" + ruff}, tc.args...)
			status := cmd.Main(append(args, "-"))
			if status != tc.status || (tc.want != "" && !strings.Contains(out.String(), tc.want)) {
				t.Fatalf("status %d, output %s; want status %d and %q", status, &out, tc.status, tc.want)
			}
			if strings.Contains(out.String(), "[pyflakes]") {
				t.Fatal("Pyflakes also ran by default")
			}
		})
	}
}

func TestRuleRuffExistingPythonFixtures(t *testing.T) {
	ruff, err := execabs.LookPath("ruff")
	if err != nil {
		t.Skip("Ruff is not installed")
	}
	for _, kind := range []string{"ok", "err"} {
		paths, err := filepath.Glob(filepath.Join("testdata", kind, "pyflakes*.yaml"))
		if err != nil || len(paths) == 0 {
			t.Fatalf("fixtures for %s: %v, %v", kind, paths, err)
		}
		for _, path := range paths {
			t.Run(path, func(t *testing.T) {
				linter, err := NewLinter(io.Discard, &LinterOptions{Ruff: ruff})
				if err != nil {
					t.Fatal(err)
				}
				diagnostics, err := linter.LintFile(path, nil)
				if err != nil {
					t.Fatal(err)
				}
				if (len(diagnostics) != 0) != (kind == "err") {
					t.Fatalf("unexpected diagnostics: %v", diagnostics)
				}
				for _, diagnostic := range diagnostics {
					if diagnostic.Kind != "ruff" {
						t.Fatalf("unexpected rule: %v", diagnostic)
					}
				}
			})
		}
	}
}
