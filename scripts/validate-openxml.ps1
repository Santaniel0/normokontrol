<#
Validates the samples and an annotated copy against the Office 2019 schemas
with Microsoft's Open XML SDK (DocumentFormat.OpenXml 3.5.1, net46 build,
loaded into Windows PowerShell 5.1). Exits 1 if any schema error is found.

    powershell -ExecutionPolicy Bypass -File scripts\validate-openxml.ps1
#>
param(
    [string]$Sdk = (Join-Path $env:LOCALAPPDATA 'Programs\_downloads\openxml\x'),
    [string]$Work = (Join-Path ([IO.Path]::GetTempPath()) ('normokontrol-oxml-' + [guid]::NewGuid().ToString('N')))
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'samples.ps1')

# The framework assembly must be loaded before the one that depends on it.
Add-Type -Path (Join-Path $Sdk 'documentformat.openxml.framework.3.5.1\lib\net46\DocumentFormat.OpenXml.Framework.dll')
Add-Type -Path (Join-Path $Sdk 'documentformat.openxml.3.5.1\lib\net46\DocumentFormat.OpenXml.dll')

$files = New-SampleSet $Work
$validator = [DocumentFormat.OpenXml.Validation.OpenXmlValidator]::new([DocumentFormat.OpenXml.FileFormatVersions]::Office2019)
$total = 0
foreach ($f in $files) {
    $doc = [DocumentFormat.OpenXml.Packaging.WordprocessingDocument]::Open($f, $false)
    try { $errors = @($validator.Validate($doc)) } finally { $doc.Dispose() }
    '{0}: {1} error(s)' -f (Split-Path -Leaf $f), $errors.Count
    foreach ($e in $errors) {
        '  [{0}] {1} {2}' -f $e.ErrorType, $e.Part.Uri, $e.Path.XPath
        '      {0}' -f $e.Description
    }
    $total += $errors.Count
}
if ($total -gt 0) { exit 1 }
