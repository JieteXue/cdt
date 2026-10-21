param([string] $Binary = (Join-Path $PSScriptRoot '..' 'cdt.exe'))
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$Binary = (Resolve-Path -LiteralPath $Binary).ProviderPath
$originalPath = $env:PATH
$originalConfig = $env:XDG_CONFIG_HOME
$originalVisual = $env:VISUAL
$originalLocation = Get-Location
$root = Join-Path ([IO.Path]::GetTempPath()) ('cdt-test-' + [guid]::NewGuid())

function Assert([bool] $Condition, [string] $Message) {
    if (-not $Condition) { throw $Message }
}
function Invoke-Binary {
    $result = & $Binary @args
    if ($LASTEXITCODE -ne 0) { throw "native command failed: $args" }
    return $result
}

try {
    $null = [IO.Directory]::CreateDirectory($root)
    $env:PATH = (Split-Path $Binary -Parent) + [IO.Path]::PathSeparator + $env:PATH
    $env:XDG_CONFIG_HOME = Join-Path $root 'config'
    $a = Join-Path $root 'a space '' [literal] $(INJECTED)'
    $b = Join-Path $root ('b ' + [char]0x4e2d + [char]0x6587)
    $target = Join-Path $root 'target-completion'
    foreach ($path in @($a, $b, $target)) { $null = [IO.Directory]::CreateDirectory($path) }
    $null = Invoke-Binary -ap $a api alpha alpha
    $null = Invoke-Binary -ap $b api beta
    $null = Invoke-Binary -ap $root work
    $a = (Invoke-Binary __query -- alpha | ConvertFrom-Json).entries[0].path
    $b = (Invoke-Binary __query -- beta | ConvertFrom-Json).entries[0].path
    $root = (Invoke-Binary __query -- work | ConvertFrom-Json).entries[0].path
    Invoke-Binary init powershell | Out-String | Invoke-Expression
    Assert ((Get-Command cdt).CommandType -eq 'Function') 'integration did not install a function'
    cdt alpha
    Assert ($LASTEXITCODE -eq 0 -and (Get-Location).ProviderPath -eq $a) 'literal path jump failed'
    $before = (Get-Location).ProviderPath
    cdt api
    Assert ($LASTEXITCODE -eq 2 -and (Get-Location).ProviderPath -eq $before) 'ambiguous query changed directory'
    cdt absent
    Assert ($LASTEXITCODE -eq 1 -and (Get-Location).ProviderPath -eq $before) 'missing query changed directory'
    $report = cdt -a api new new | Out-String
    Assert ($LASTEXITCODE -eq 0) 'add through function failed'
    Assert ($report.Contains('Already present (skipped): [api]')) 'duplicate feedback missing'
    Assert ($report.Contains('Repeated input (ignored): [new]')) 'repeated-input feedback missing'
    $null = cdt -r new
    $tree = cdt -l | Out-String
    Assert ($tree.Contains(([string][char]0x251c) + ([char]0x2500) + ([char]0x2500))) 'tree connector missing'
    $query = Invoke-Binary __query -- work alpha | ConvertFrom-Json
    Assert (@($query.entries).Count -eq 1) 'inheritance query failed'
    Assert (@($query.entries[0].tags).Count -eq 2) 'inherited tag was persisted'
    foreach ($line in @('cdt alph', 'cdt work alph', ("cdt -ap '" + $a.Replace("'", "''") + "' alph"))) {
        $completion = TabExpansion2 $line $line.Length
        Assert (@($completion.CompletionMatches | Where-Object ListItemText -eq 'alpha').Count -eq 1) "tag completion failed: $line"
    }
    $line = "cdt -ap '" + (Join-Path $root 'target-comp')
    $completion = TabExpansion2 $line $line.Length
    Assert (@($completion.CompletionMatches | Where-Object ListItemText -eq 'target-completion').Count -eq 1) 'directory completion failed'
    $before = (Get-Location).ProviderPath
    Set-Location Env:
    cdt -a should-not-register
    Assert ($LASTEXITCODE -eq 1) 'mutation from a non-filesystem provider was allowed'
    Set-Location -LiteralPath $before
    $null = cdt -a -- -special
    cdt -- -special
    Assert ($LASTEXITCODE -eq 0) 'end-of-options tag failed'
    $null = cdt -r -- -special
    $module = Get-Module CdtIntegration | Select-Object -Last 1
    $actions = & $module {
        @(
            (Get-CdtMenuAction ([ConsoleKeyInfo]::new([char]0, 'Tab', $false, $false, $false))),
            (Get-CdtMenuAction ([ConsoleKeyInfo]::new([char]0, 'Tab', $true, $false, $false))),
            (Get-CdtMenuAction ([ConsoleKeyInfo]::new([char]0, 'UpArrow', $false, $false, $false))),
            (Get-CdtMenuAction ([ConsoleKeyInfo]::new([char]0, 'DownArrow', $false, $false, $false))),
            (Get-CdtMenuAction ([ConsoleKeyInfo]::new([char]13, 'Enter', $false, $false, $false))),
            (Get-CdtMenuAction ([ConsoleKeyInfo]::new([char]27, 'Escape', $false, $false, $false))),
            (Get-CdtMenuAction ([ConsoleKeyInfo]::new([char]3, 'C', $false, $false, $true))),
            (Get-CdtMenuAction ([ConsoleKeyInfo]::new([char]7, 'G', $false, $false, $true)))
        )
    }
    Assert (($actions -join ',') -eq 'next,previous,previous,next,accept,cancel,cancel,cancel') 'menu key mapping failed'
    $pwsh = (Get-Command pwsh -CommandType Application).Source
    $editor = Join-Path $PSScriptRoot 'editor.ps1'
    $env:VISUAL = "'" + $pwsh.Replace("'", "''") + "' -NoProfile -File '" + $editor.Replace("'", "''") + "'"
    $null = cdt --edit
    Assert ($LASTEXITCODE -eq 0) 'editor failed'
    cdt edited
    Assert ($LASTEXITCODE -eq 0 -and (Get-Location).ProviderPath -eq $a) 'edited tags not available'
    $null = cdt -dp $a
    Assert ($LASTEXITCODE -eq 0 -and [IO.Directory]::Exists($a)) 'unregister deleted the real directory'
    Invoke-Binary init powershell | Out-String | Invoke-Expression
    cdt beta
    Assert ($LASTEXITCODE -eq 0 -and (Get-Location).ProviderPath -eq $b) 'reinitialization failed'
    Write-Output 'PASS: PowerShell navigation, feedback, completion, editor, and menu actions'
} finally {
    Set-Location -LiteralPath $originalLocation.ProviderPath
    $env:PATH = $originalPath
    $env:XDG_CONFIG_HOME = $originalConfig
    $env:VISUAL = $originalVisual
    Remove-Module CdtIntegration -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
}
