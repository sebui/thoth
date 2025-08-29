package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"

	"google.golang.org/genai"
)

type RunShellCommandTool struct {
	ProjectRoot string
}

func (t *RunShellCommandTool) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        "run_shell_command",
		Description: "This tool executes a given shell command as `bash -c <command>`. Command can start background processes using `&`. Command is executed as a subprocess that leads its own process group. Command process group can be terminated as `kill -- -PGID` or signaled as `kill -s SIGNAL -- -PGID`. The following information is returned: Command: Executed command. Directory: Directory (relative to project root) where command was executed, or `(root)`. Stdout: Output on stdout stream. Can be `(empty)` or partial on error and for any unwaited background processes. Stderr: Output on stderr stream. Can be `(empty)` or partial on error and for any unwaited background processes. Error: Error or `(none)` if no error was reported for the subprocess. Exit Code: Exit code or `(none)` if terminated by signal. Signal: Signal number or `(none)` if no signal was received. Background PIDs: List of background processes started or `(none)`. Process Group PGID: Process group started or `(none)`",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"command": {
					Type:        genai.TypeString,
					Description: "Exact bash command to execute as `bash -c <command>`",
				},
				"description": {
					Type:        genai.TypeString,
					Description: "Brief description of the command for the user. Be specific and concise. Ideally a single sentence. Can be up to 3 sentences for clarity. No line breaks.",
				},
				"directory": {
					Type:        genai.TypeString,
					Description: "(OPTIONAL) Directory to run the command in, if not the project root directory. Must be relative to the project root directory and must already exist.",
				},
			},
			Required: []string{"command"},
		},
		Response: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"Command":            {Type: genai.TypeString},
				"Directory":          {Type: genai.TypeString},
				"Stdout":             {Type: genai.TypeString},
				"Stderr":             {Type: genai.TypeString},
				"Error":              {Type: genai.TypeString},
				"Exit Code":          {Type: genai.TypeNumber},
				"Signal":             {Type: genai.TypeNumber},
				"Background PIDs":    {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeNumber}},
				"Process Group PGID": {Type: genai.TypeNumber},
			},
		},
	}
}

func (t *RunShellCommandTool) Execute(ctx context.Context, args map[string]any) (map[string]any, error) {
	cmdStr, ok := args["command"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'command' argument")
	}

	dir, _ := args["directory"].(string) // Optional

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	if dir != "" {
		cmd.Dir = filepath.Join(t.ProjectRoot, dir) // Resolve relative to project root
	} else {
		cmd.Dir = t.ProjectRoot // Default to project root
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Set process group ID for background processes
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// Wait for the command to finish in a goroutine to allow context cancellation
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var exitCode int = -1
	var signal int = -1
	var cmdErr string = "(none)"

	select {
	case <-ctx.Done():
		// Context cancelled, try to kill the process group
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM) // Kill the process group
		}
		<-done // Wait for the process to actually exit
		return nil, ctx.Err()
	case err := <-done:
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				cmdErr = exitError.Error()
				if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
					exitCode = status.ExitStatus()
					if status.Signaled() {
						signal = int(status.Signal())
					}
				}
			} else {
				cmdErr = err.Error()
			}
		} else {
			exitCode = 0
		}
	}

	pgid := -1
	if cmd.Process != nil {
		if p, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
			pgid = p
		}
	}

	result := map[string]any{
		"Command":            cmdStr,
		"Directory":          dir,
		"Stdout":             stdoutBuf.String(),
		"Stderr":             stderrBuf.String(),
		"Error":              cmdErr,
		"Exit Code":          exitCode,
		"Signal":             signal,
		"Background PIDs":    []any{}, // Placeholder, difficult to get reliably
		"Process Group PGID": pgid,
	}

	return result, nil
}
