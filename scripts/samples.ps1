# Shared by verify-libreoffice.ps1 and validate-openxml.ps1: builds
# normokontrol, copies the two samples into $Work and writes an annotated
# copy of the bad one. Returns the paths of the three .docx files.
# Kept ASCII-only: Windows PowerShell 5.1 reads BOM-less scripts as ANSI.

function New-SampleSet([string]$Work) {
    $root = Split-Path -Parent $PSScriptRoot
    New-Item -ItemType Directory -Force $Work | Out-Null
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'go is not in PATH' }
    $exe = Join-Path $Work 'normokontrol.exe'
    Push-Location $root
    try {
        go build -o $exe ./cmd/normokontrol
        if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    } finally { Pop-Location }

    $good = Join-Path $Work 'vkr_good.docx'
    $bad = Join-Path $Work 'vkr_bad.docx'
    Copy-Item (Join-Path $root 'testdata\vkr_good.docx') $good -Force
    Copy-Item (Join-Path $root 'testdata\vkr_bad.docx') $bad -Force
    & $exe check --annotate $bad | Out-Null
    # Exit code 1 means "errors found", which is expected for the bad sample.
    if ($LASTEXITCODE -ne 1) { throw "normokontrol exited with $LASTEXITCODE" }
    # The annotated copy has a Cyrillic suffix; give it an ASCII name.
    $annotated = Get-ChildItem $Work -Filter 'vkr_bad.*.docx' | Select-Object -First 1
    $target = Join-Path $Work 'vkr_bad_annotated.docx'
    Move-Item $annotated.FullName $target -Force
    return @($good, $bad, $target)
}

function Read-ZipEntry([string]$Path, [string]$Entry) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($Path)
    try {
        $e = $zip.GetEntry($Entry)
        if (-not $e) { return $null }
        $reader = New-Object IO.StreamReader($e.Open())
        try { return $reader.ReadToEnd() } finally { $reader.Dispose() }
    } finally { $zip.Dispose() }
}

function Measure-XmlElements([string]$Xml, [string]$Namespace, [string]$LocalName) {
    if (-not $Xml) { return 0 }
    $doc = New-Object Xml.XmlDocument
    $doc.LoadXml($Xml)
    return $doc.GetElementsByTagName($LocalName, $Namespace).Count
}
