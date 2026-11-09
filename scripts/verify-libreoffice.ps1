<#
Converts the samples and an annotated copy to ODT with LibreOffice and checks
that every Word comment normokontrol wrote arrives as an office:annotation.
Not part of go test: CI has no LibreOffice.

    powershell -ExecutionPolicy Bypass -File scripts\verify-libreoffice.ps1
#>
param(
    [string]$Soffice = (Join-Path $env:LOCALAPPDATA 'Programs\LibreOffice-portable\SourceDir\LibreOffice\program\soffice.com'),
    [string]$Work = (Join-Path ([IO.Path]::GetTempPath()) ('normokontrol-lo-' + [guid]::NewGuid().ToString('N')))
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'samples.ps1')

$files = New-SampleSet $Work
$odtDir = Join-Path $Work 'odt'
# A throwaway profile keeps the run independent of any installed LibreOffice.
$profileUri = 'file:///' + ((Join-Path $Work 'lo-profile') -replace '\\', '/')

& $Soffice "-env:UserInstallation=$profileUri" --headless --norestore --convert-to odt --outdir $odtDir $files |
    ForEach-Object { '  ' + $_ }
""

$wordNS = 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'
$officeNS = 'urn:oasis:names:tc:opendocument:xmlns:office:1.0'
$failed = 0
'{0,-26} {1,10} {2,14}  {3}' -f 'file', 'comments', 'annotations', 'result'
foreach ($f in $files) {
    $name = Split-Path -Leaf $f
    $comments = Measure-XmlElements (Read-ZipEntry $f 'word/comments.xml') $wordNS 'comment'
    $odt = Join-Path $odtDir ([IO.Path]::ChangeExtension($name, '.odt'))
    if (-not (Test-Path $odt)) {
        '{0,-26} {1,10} {2,14}  {3}' -f $name, $comments, '-', 'NOT LOADED'
        $failed++
        continue
    }
    $annotations = Measure-XmlElements (Read-ZipEntry $odt 'content.xml') $officeNS 'annotation'
    $result = 'ok'
    if ($annotations -ne $comments) { $result = 'MISMATCH'; $failed++ }
    '{0,-26} {1,10} {2,14}  {3}' -f $name, $comments, $annotations, $result
}
if ($failed -gt 0) { exit 1 }
