// Package shell embeds the shell integrations distributed with cdt.
package shell

import _ "embed"

//go:embed cdt.zsh
var Zsh string

//go:embed cdt.ps1
var PowerShell string
