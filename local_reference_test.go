package actionlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfRepositoryUsesLocalSpec(t *testing.T) {
	for _, tc := range []struct {
		spec string
		want string
	}{
		{"$/action", "./action"},
		{"$/.github/workflows/test.yml", "./.github/workflows/test.yml"},
		{"$/dir/../action", "./action"},
		{"$/.", "./."},
		{"$/", ""},
		{"$//absolute", ""},
		{"$/../outside", ""},
		{"$/dir/../../outside", ""},
		{"$/action@main", ""},
		{"$/@main", ""},
		{`$/dir\action`, ""},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			got, err := selfRepositoryUsesLocalSpec(tc.spec)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("accepted invalid reference %q as %q", tc.spec, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got (%q, %v), want %q", got, err, tc.want)
			}
		})
	}
}

func TestRuleActionSelfRepositoryFormat(t *testing.T) {
	for _, spec := range []string{"$/", "$/action@main", "$/../outside", `$/dir\action`} {
		t.Run(spec, func(t *testing.T) {
			rule := NewRuleAction(newNullLocalActionsCache(nil))
			step := &Step{Exec: &ExecAction{Uses: &String{Value: spec, Pos: &Pos{}}}}
			if err := rule.VisitStep(step); err != nil {
				t.Fatal(err)
			}
			if errs := rule.Errs(); len(errs) != 1 {
				t.Fatalf("wanted one format error, got %v", errs)
			}
		})
	}
}

func TestLocalMetadataStaysWithinProject(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	write := func(name, content string) {
		t.Helper()
		file := filepath.Join(base, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	action := "name: example\ndescription: example\nruns:\n  using: composite\n  steps: []\n"
	workflow := "on:\n  workflow_call:\n    inputs:\n      value:\n        type: string\n"
	write("project/inside/action.yml", action)
	write("project/inside.yml", workflow)
	write("outside/action.yml", action)
	write("outside.yml", workflow)
	proj := &Project{root, nil}

	checkAction := func(spec string, wantError bool) {
		t.Helper()
		cache := NewLocalActionsCache(proj, nil)
		meta, _, err := cache.FindMetadata(spec)
		if wantError && (err == nil || meta != nil) {
			t.Fatalf("action %q: got metadata %v and error %v", spec, meta, err)
		}
		if !wantError && (err != nil || meta == nil) {
			t.Fatalf("action %q: got metadata %v and error %v", spec, meta, err)
		}
	}
	checkWorkflow := func(spec string, wantError bool) {
		t.Helper()
		cache := NewLocalReusableWorkflowCache(proj, root, nil)
		meta, err := cache.FindMetadata(spec)
		if wantError && (err == nil || meta != nil) {
			t.Fatalf("workflow %q: got metadata %v and error %v", spec, meta, err)
		}
		if !wantError && (err != nil || meta == nil) {
			t.Fatalf("workflow %q: got metadata %v and error %v", spec, meta, err)
		}
	}

	checkAction("./inside", false)
	checkWorkflow("./inside.yml", false)
	checkAction("./../outside", true)
	checkWorkflow("./../outside.yml", true)

	t.Run("symlinks", func(t *testing.T) {
		if err := os.Symlink("../outside", filepath.Join(root, "outside-link")); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
		if err := os.Symlink("../outside.yml", filepath.Join(root, "outside-link.yml")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "metadata-link"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../outside/action.yml", filepath.Join(root, "metadata-link", "action.yml")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("inside", filepath.Join(root, "inside-link")); err != nil {
			t.Fatal(err)
		}
		checkAction("./outside-link", true)
		checkAction("./metadata-link", true)
		checkWorkflow("./outside-link.yml", true)
		checkAction("./inside-link", false)
	})

	t.Run("action files", func(t *testing.T) {
		write("project/javascript/action.yml", "name: example\ndescription: example\nruns:\n  using: node24\n  main: ../../outside.js\n")
		write("outside.js", "// fixture\n")
		rule := NewRuleAction(NewLocalActionsCache(proj, nil))
		step := &Step{Exec: &ExecAction{Uses: &String{Value: "$/javascript", Pos: &Pos{}}}}
		if err := rule.VisitStep(step); err != nil {
			t.Fatal(err)
		}
		if errs := rule.Errs(); len(errs) != 1 || !strings.Contains(errs[0].Message, "within the repository") {
			t.Fatalf("unexpected diagnostics: %v", errs)
		}
	})
}
