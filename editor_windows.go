package main

import (
	"os"
	"os/exec"
	"strings"
)

const defaultEditor = "notepad.exe"

func editorCommand(editor, file string) *exec.Cmd {
	// PowerShell syntax supports editor flags. The draft path is passed through
	// the environment, not interpolated into trusted editor source.
	cmd := exec.Command("pwsh.exe", "-NoLogo", "-NoProfile", "-NonInteractive",
		"-Command", "& "+editor+" $env:CDT_EDITOR_DRAFT; if (-not $?) { exit 1 }")
	for _, value := range os.Environ() {
		if !strings.EqualFold(strings.SplitN(value, "=", 2)[0], "CDT_EDITOR_DRAFT") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "CDT_EDITOR_DRAFT="+file)
	return cmd
}
