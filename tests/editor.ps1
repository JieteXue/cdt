param([Parameter(Mandatory)][string] $Draft)
$ErrorActionPreference = 'Stop'
$data = Get-Content -LiteralPath $Draft -Raw | ConvertFrom-Json
$entry = $data.entries | Where-Object { $_.tags -ccontains 'alpha' } | Select-Object -First 1
if (-not $entry) { throw 'editor fixture could not find the alpha entry' }
$entry.tags = @('edited')
$data | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $Draft -Encoding utf8NoBOM
