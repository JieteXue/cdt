//go:build !windows

package cli

import "os/exec"

const defaultEditor = "vi"

func editorCommand(editor, file string) *exec.Cmd {
	// The editor command is trusted configuration; the path is only data.
	return exec.Command("/bin/sh", "-c", editor+` "$@"`, "cdt-editor", file)
}
