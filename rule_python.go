package actionlint

import (
	"strings"
	"sync"
)

type shellIsPythonKind int

const (
	shellIsPythonKindUnspecified shellIsPythonKind = iota
	shellIsPythonKindPython
	shellIsPythonKindNotPython
)

func getShellIsPythonKind(shell *String) shellIsPythonKind {
	if shell == nil {
		return shellIsPythonKindUnspecified
	}
	if shell.Value == "python" || strings.HasPrefix(shell.Value, "python ") {
		return shellIsPythonKindPython
	}
	return shellIsPythonKindNotPython
}

// pythonScriptRule tracks shell selection and process lifetime for Python scripts.
type pythonScriptRule struct {
	cmd                   *externalCommand
	workflowShellIsPython shellIsPythonKind
	jobShellIsPython      shellIsPythonKind
	mu                    sync.Mutex
}

func (rule *pythonScriptRule) VisitJobPre(n *Job) error {
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.jobShellIsPython = getShellIsPythonKind(n.Defaults.Run.Shell)
	}
	return nil
}

func (rule *pythonScriptRule) VisitJobPost(*Job) error {
	rule.jobShellIsPython = shellIsPythonKindUnspecified
	return nil
}

func (rule *pythonScriptRule) VisitWorkflowPre(n *Workflow) error {
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.workflowShellIsPython = getShellIsPythonKind(n.Defaults.Run.Shell)
	}
	return nil
}

func (rule *pythonScriptRule) VisitWorkflowPost(*Workflow) error {
	rule.workflowShellIsPython = shellIsPythonKindUnspecified
	return rule.cmd.wait()
}

func (rule *pythonScriptRule) isPythonShell(r *ExecRun) bool {
	if k := getShellIsPythonKind(r.Shell); k != shellIsPythonKindUnspecified {
		return k == shellIsPythonKindPython
	}
	if rule.jobShellIsPython != shellIsPythonKindUnspecified {
		return rule.jobShellIsPython == shellIsPythonKindPython
	}
	return rule.workflowShellIsPython == shellIsPythonKindPython
}
