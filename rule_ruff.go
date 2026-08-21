package actionlint

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// RuleRuff checks Python scripts at 'run:' using Ruff's Pyflakes rules.
type RuleRuff struct {
	RuleBase
	pythonScriptRule
}

func newRuleRuff(cmd *externalCommand) *RuleRuff {
	return &RuleRuff{
		RuleBase:         NewRuleBase("ruff", "Checks Python scripts when \"shell: python\" is configured using Ruff"),
		pythonScriptRule: pythonScriptRule{cmd: cmd},
	}
}

// VisitJobPre applies the job's default shell.
func (rule *RuleRuff) VisitJobPre(n *Job) error { return rule.pythonScriptRule.VisitJobPre(n) }

// VisitJobPost resets the job's default shell.
func (rule *RuleRuff) VisitJobPost(n *Job) error { return rule.pythonScriptRule.VisitJobPost(n) }

// VisitWorkflowPre applies the workflow's default shell.
func (rule *RuleRuff) VisitWorkflowPre(n *Workflow) error {
	return rule.pythonScriptRule.VisitWorkflowPre(n)
}

// VisitWorkflowPost waits for pending Python checks.
func (rule *RuleRuff) VisitWorkflowPost(n *Workflow) error {
	return rule.pythonScriptRule.VisitWorkflowPost(n)
}

// NewRuleRuff creates a Ruff rule. The executable may be a command name or path.
func NewRuleRuff(executable string, proc *concurrentProcess) (*RuleRuff, error) {
	cmd, err := proc.newCommandRunner(executable, false)
	if err != nil {
		return nil, err
	}
	// Ruff reserves status 2 for invocation, configuration, and internal errors.
	cmd.maxExitCode = 1
	return newRuleRuff(cmd), nil
}

// VisitStep checks run steps that use Python, including workflow and job defaults.
func (rule *RuleRuff) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil || !rule.isPythonShell(run) {
		return nil
	}
	src := sanitizeExpressionsInScript(run.Run.Value)
	pos := run.RunPos
	rule.Debug("%s: Running %s for Python script:\n%s", pos, rule.cmd.exe, src)
	// Embedded workflow scripts must not inherit repository settings or suppressions.
	// Use the current stable Python version: the workflow's interpreter can be newer
	// than Ruff's default target or the interpreter installed alongside actionlint.
	args := []string{"check", "--isolated", "--target-version", "py314", "--select", "F", "--ignore-noqa", "--no-fix", "--no-cache", "--output-format", "json", "--stdin-filename", "actionlint.py", "-"}
	rule.cmd.run(args, src, func(stdout []byte, err error) error {
		if err != nil {
			return fmt.Errorf("`%s` did not run successfully while checking script at %s: %w", rule.cmd.exe, pos, err)
		}
		return rule.parseOutput(stdout, pos)
	})
	return nil
}

type ruffDiagnostic struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Location struct {
		Row    int `json:"row"`
		Column int `json:"column"`
	} `json:"location"`
}

func (rule *RuleRuff) parseOutput(stdout []byte, pos *Pos) error {
	var diagnostics []ruffDiagnostic
	if err := json.Unmarshal(stdout, &diagnostics); err != nil {
		return fmt.Errorf("could not parse Ruff output while checking script at %s: %w", pos, err)
	}
	if !bytes.HasPrefix(bytes.TrimSpace(stdout), []byte("[")) {
		return fmt.Errorf("expected a JSON array from Ruff while checking script at %s", pos)
	}
	for _, d := range diagnostics {
		if d.Code == "" || d.Message == "" || d.Location.Row < 1 || d.Location.Column < 1 {
			return fmt.Errorf("invalid Ruff diagnostic while checking script at %s: %+v", pos, d)
		}
	}
	// Process callbacks run concurrently. Validate the complete response before recording it.
	rule.mu.Lock()
	defer rule.mu.Unlock()
	for _, d := range diagnostics {
		rule.Errorf(pos, "Ruff reported issue in this script: %d:%d: %s %s", d.Location.Row, d.Location.Column, d.Code, d.Message)
	}
	return nil
}
