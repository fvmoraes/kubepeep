$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

function Get-SHA256Hex([string]$Path) {
    $stream = $null
    $hasher = $null
    try {
        $stream = [System.IO.File]::OpenRead($Path)
        $hasher = [System.Security.Cryptography.SHA256]::Create()
        $hashBytes = $hasher.ComputeHash($stream)
        return ([System.BitConverter]::ToString($hashBytes)).Replace('-', '').ToLowerInvariant()
    } finally {
        if ($null -ne $hasher) { $hasher.Dispose() }
        if ($null -ne $stream) { $stream.Dispose() }
    }
}

function Get-AllowlistedTestStage([string]$Stage) {
    switch -Exact ($Stage) {
        'static-hash-api' { return $Stage }
        'fixture-build' { return $Stage }
        'initial-install' { return $Stage }
        'fixture-guard' { return $Stage }
        'version-validation' { return $Stage }
        'transaction-lock' { return $Stage }
        'upgrade' { return $Stage }
        'checksum-validation' { return $Stage }
        'archive-validation' { return $Stage }
        'candidate-validation' { return $Stage }
        'rollback' { return $Stage }
        'reparse-binary' { return $Stage }
        'uninstall' { return $Stage }
        'path-ownership' { return $Stage }
        'reparse-data' { return $Stage }
        'argument-validation' { return $Stage }
        'architecture' { return $Stage }
        'purge' { return $Stage }
        'cleanup' { return $Stage }
        default { return 'unknown' }
    }
}

function Get-SafeTestDiagnostic([string]$Stage, [Exception]$Exception) {
    $safeStage = Get-AllowlistedTestStage $Stage
    for ($depth = 0; $depth -lt 16 -and $null -ne $Exception -and $null -ne $Exception.InnerException; $depth++) {
        $Exception = $Exception.InnerException
    }
    if ($null -eq $Exception) {
        $typeName = 'Exception'
        $hResult = 0
    } else {
        $typeName = $Exception.GetType().Name
        if ([string]::IsNullOrEmpty($typeName) -or $typeName -notmatch '^[A-Za-z][A-Za-z0-9]{0,63}$') {
            $typeName = 'Exception'
        }
        $hResult = [int]($Exception.HResult)
    }
    $hResultHex = '{0:X8}' -f $hResult
    return "install-test stage=$safeStage type=$typeName hresult=0x$hResultHex"
}

function Invoke-SafeTestCleanup([scriptblock]$Action) {
    try {
        & $Action
    } catch {
        if ($null -eq $script:testFailureDiagnostic) {
            $script:testFailureDiagnostic = Get-SafeTestDiagnostic 'cleanup' $_.Exception
        }
    }
}

$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('kubepeep-installer-test-' + [Guid]::NewGuid().ToString('N'))
$releaseDir = Join-Path $testRoot 'release'
$payloadDir = Join-Path $testRoot 'payload'
$testLocalAppData = Join-Path $testRoot 'localappdata'
$installDir = Join-Path $testLocalAppData 'Programs\kubePeep'
$binaryPath = Join-Path $installDir 'kubePeep.exe'
$testArchitecture = 'amd64'
if (($env:PROCESSOR_ARCHITEW6432, $env:PROCESSOR_ARCHITECTURE) -contains 'ARM64') { $testArchitecture = 'arm64' }
$archiveName = "kubepeep-windows-$testArchitecture.zip"
$archivePath = Join-Path $releaseDir $archiveName
$previousLocalAppData = $env:LOCALAPPDATA
$previousRelease = $env:KUBEPEEP_TEST_RELEASE
$previousUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$dataRoot = Join-Path $testLocalAppData 'kubePeep'
$nestedJunction = $null
$script:testStage = 'unknown'
$script:testFailureDiagnostic = $null

function Write-Checksums {
    $hash = Get-SHA256Hex $archivePath
    Set-Content -LiteralPath (Join-Path $releaseDir 'checksums.txt') -Value "$hash  $archiveName" -Encoding ASCII
}

function New-AdversarialZip([string[]]$Entries) {
    Remove-Item -LiteralPath $archivePath -Force -ErrorAction SilentlyContinue
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $stream = [IO.File]::Open($archivePath, [IO.FileMode]::CreateNew)
    $zip = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create, $false)
    try {
        foreach ($name in $Entries) {
            $entry = $zip.CreateEntry($name)
            $destination = $entry.Open()
            $source = [IO.File]::OpenRead((Join-Path $payloadDir 'kubePeep.exe'))
            try { $source.CopyTo($destination) } finally { $source.Dispose(); $destination.Dispose() }
        }
    } finally {
        $zip.Dispose()
        $stream.Dispose()
    }
    Write-Checksums
}

function Publish-ValidPayload {
    Remove-Item -LiteralPath $archivePath -Force -ErrorAction SilentlyContinue
    Compress-Archive -LiteralPath (Join-Path $payloadDir 'kubePeep.exe'), (Join-Path $payloadDir 'README.md'), (Join-Path $payloadDir 'LICENSE') -DestinationPath $archivePath
    Write-Checksums
}

function Build-VersionBinary([string]$Destination, [string]$VersionValue) {
    $flags = "-X github.com/fvmoraes/kubepeep/internal/buildinfo.Version=$VersionValue -X github.com/fvmoraes/kubepeep/internal/buildinfo.Commit=synthetic -X github.com/fvmoraes/kubepeep/internal/buildinfo.BuildDate=2026-08-17T00:00:00Z"
    & go build -trimpath -ldflags $flags -o $Destination ./cmd/kubePeep
    if ($LASTEXITCODE -ne 0) { throw "could not build version fixture $VersionValue" }
}

function Build-RollbackFixture([string]$Destination) {
    $sourcePath = Join-Path $testRoot 'rollback-fixture.go'
    $source = @'
package main
import (
    "fmt"
    "os"
)
func main() {
    if len(os.Args) != 2 || os.Args[1] != "version" { os.Exit(2) }
    marker := os.Getenv("KUBEPEEP_INSTALLER_ROLLBACK_MARKER")
    if marker == "" { os.Exit(3) }
    if _, err := os.Stat(marker); err == nil {
        fmt.Println("version=9.9.9 commit=rollback-fixture build_date=fixture")
        return
    }
    if err := os.WriteFile(marker, []byte("candidate-verified"), 0600); err != nil { os.Exit(4) }
    fmt.Println("version=0.1.0 commit=rollback-fixture build_date=fixture")
}
'@
    [IO.File]::WriteAllText($sourcePath, $source, [Text.UTF8Encoding]::new($false))
    & go build -trimpath -o $Destination $sourcePath
    if ($LASTEXITCODE -ne 0) { throw 'could not build rollback fixture' }
}

function Build-ExecutionProbe([string]$Destination) {
    $sourcePath = Join-Path $testRoot 'execution-probe.go'
    $source = @'
package main
import "os"
func main() {
    marker := os.Getenv("KUBEPEEP_INSTALLER_EXECUTION_MARKER")
    if marker != "" { _ = os.WriteFile(marker, []byte("executed"), 0600) }
    os.Exit(3)
}
'@
    [IO.File]::WriteAllText($sourcePath, $source, [Text.UTF8Encoding]::new($false))
    & go build -trimpath -o $Destination $sourcePath
    if ($LASTEXITCODE -ne 0) { throw 'could not build execution probe' }
}

function Enable-FixtureDownloadOverride {
    function global:Invoke-WebRequest {
        [CmdletBinding()]
        param([switch]$UseBasicParsing, [Parameter(Mandatory=$true)][string]$Uri, [Parameter(Mandatory=$true)][string]$OutFile)
        if (-not [string]::IsNullOrWhiteSpace($env:KUBEPEEP_INSTALLER_DOWNLOAD_MARKER)) {
            [IO.File]::AppendAllText($env:KUBEPEEP_INSTALLER_DOWNLOAD_MARKER, "downloaded`r`n")
        }
        $name = [IO.Path]::GetFileName(([Uri]$Uri).AbsolutePath)
        Copy-Item -LiteralPath (Join-Path $env:KUBEPEEP_TEST_RELEASE $name) -Destination $OutFile
    }
}

try {
    $script:testStage = 'static-hash-api'
    $legacyHashCommand = 'Get-' + 'FileHash'
    foreach ($hashCheckedScript in @('./install.ps1', './scripts/install_test.ps1')) {
        $scriptSource = [IO.File]::ReadAllText((Resolve-Path $hashCheckedScript))
        Assert-True ($scriptSource.IndexOf($legacyHashCommand, [StringComparison]::OrdinalIgnoreCase) -lt 0) 'PowerShell installer tests depend on module-autoloaded hashing'
    }

    $script:testStage = 'fixture-build'
    New-Item -ItemType Directory -Path $releaseDir, $payloadDir, $testLocalAppData -Force | Out-Null
    $ldflags = '-X github.com/fvmoraes/kubepeep/internal/buildinfo.Version=0.1.0 -X github.com/fvmoraes/kubepeep/internal/buildinfo.Commit=synthetic -X github.com/fvmoraes/kubepeep/internal/buildinfo.BuildDate=2026-08-17T00:00:00Z'
    & go build -trimpath -ldflags $ldflags -o (Join-Path $payloadDir 'kubePeep.exe') ./cmd/kubePeep
    if ($LASTEXITCODE -ne 0) { throw 'could not build the synthetic Windows release binary' }
    Copy-Item -LiteralPath README.md -Destination (Join-Path $payloadDir 'README.md')
    Copy-Item -LiteralPath LICENSE -Destination (Join-Path $payloadDir 'LICENSE')
    Compress-Archive -LiteralPath (Join-Path $payloadDir 'kubePeep.exe'), (Join-Path $payloadDir 'README.md'), (Join-Path $payloadDir 'LICENSE') -DestinationPath $archivePath
    Write-Checksums

    $env:KUBEPEEP_TEST_RELEASE = $releaseDir
    $env:LOCALAPPDATA = $testLocalAppData
    Enable-FixtureDownloadOverride

    $script:testStage = 'initial-install'
    & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null
    Assert-True (Test-Path -LiteralPath $binaryPath -PathType Leaf) 'PowerShell installer did not install the binary'
    $version = (& $binaryPath version | Out-String)
    Assert-True ($version -match 'version=0\.1\.0(?:\s|$)') 'installed binary reports an unexpected version'
    Assert-True (Test-Path -LiteralPath (Join-Path $installDir '.kubePeep.path-managed') -PathType Leaf) 'PowerShell installer did not record PATH ownership'
    $installedUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    Assert-True (@(([string]$installedUserPath).Split(';') | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }).Count -eq 1) 'PowerShell installer did not add exactly one user PATH entry'

    $script:testStage = 'fixture-guard'
    Remove-Item Function:\Invoke-WebRequest -Force
    Assert-True ($null -eq (Get-Command Invoke-WebRequest -CommandType Function -ErrorAction SilentlyContinue)) 'PowerShell fixture download override remained active'
    $rejected = $false
    try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted fixture mode without its explicit download function'
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'fail-closed fixture validation changed the installed binary'
    Enable-FixtureDownloadOverride

    $script:testStage = 'version-validation'
    $rejected = $false
    try { & ./install.ps1 -Version '01.1.0' -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted a release version with a leading zero'
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'invalid leading-zero version changed the installed binary'

    $script:testStage = 'transaction-lock'
    $heldLockPath = Join-Path $installDir '.kubePeep.install.lock'
    $transactionDownloadMarker = Join-Path $testRoot 'transaction-downloaded'
    $heldLock = [IO.File]::Open($heldLockPath, [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    try {
        $env:KUBEPEEP_INSTALLER_DOWNLOAD_MARKER = $transactionDownloadMarker
        $rejected = $false
        try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
        Assert-True $rejected 'PowerShell installer ignored a concurrent transaction lock'
        Assert-True (-not (Test-Path -LiteralPath $transactionDownloadMarker)) 'PowerShell installer downloaded assets before acquiring its transaction lock'
    } finally {
        Remove-Item Env:\KUBEPEEP_INSTALLER_DOWNLOAD_MARKER -ErrorAction SilentlyContinue
        $heldLock.Dispose()
        Remove-Item -LiteralPath $heldLockPath -Force -ErrorAction SilentlyContinue
    }

    $script:testStage = 'upgrade'
    $oldLdflags = '-X github.com/fvmoraes/kubepeep/internal/buildinfo.Version=0.0.9 -X github.com/fvmoraes/kubepeep/internal/buildinfo.Commit=synthetic-old -X github.com/fvmoraes/kubepeep/internal/buildinfo.BuildDate=2026-08-17T00:00:00Z'
    & go build -trimpath -ldflags $oldLdflags -o $binaryPath ./cmd/kubePeep
    if ($LASTEXITCODE -ne 0) { throw 'could not create the previous-version upgrade fixture' }
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.0\.9(?:\s|$)') 'previous-version fixture reports an unexpected version'
    & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'PowerShell installer did not upgrade the previous binary'
    $upgradedUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    Assert-True (@(([string]$upgradedUserPath).Split(';') | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }).Count -eq 1) 'PowerShell upgrade duplicated its user PATH entry'

    $script:testStage = 'checksum-validation'
    [IO.File]::WriteAllBytes((Join-Path $releaseDir 'checksums.txt'), (New-Object byte[] (1MB + 1)))
    $rejected = $false
    try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted an oversized checksum list'
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'oversized checksum list changed the installed binary'

    $script:testStage = 'archive-validation'
    Publish-ValidPayload
    $oversizedArchive = [IO.File]::Open($archivePath, [IO.FileMode]::Open, [IO.FileAccess]::Write, [IO.FileShare]::None)
    try { $oversizedArchive.SetLength(256MB + 1) } finally { $oversizedArchive.Dispose() }
    $rejected = $false
    try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted an oversized release archive'
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'oversized archive changed the installed binary'

    Publish-ValidPayload
    Move-Item -LiteralPath $archivePath -Destination "$archivePath.missing"
    $rejected = $false
    try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted a missing release archive'
    Move-Item -LiteralPath "$archivePath.missing" -Destination $archivePath
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'missing archive changed the installed binary'

    $script:testStage = 'checksum-validation'
    Write-Checksums
    $validChecksumLine = Get-Content -LiteralPath (Join-Path $releaseDir 'checksums.txt') -Raw
    Set-Content -LiteralPath (Join-Path $releaseDir 'checksums.txt') -Value ($validChecksumLine + $validChecksumLine) -Encoding ASCII
    $rejected = $false
    try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted duplicate checksum entries'
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'duplicate checksums changed the installed binary'

    Set-Content -LiteralPath (Join-Path $releaseDir 'checksums.txt') -Value (('0' * 64) + "  $archiveName") -Encoding ASCII
    $rejected = $false
    try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted a bad checksum'
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'bad checksum changed the installed binary'

    $script:testStage = 'archive-validation'
    New-AdversarialZip @('kubePeep.exe', 'kubePeep.exe')
    $rejected = $false
    try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted duplicate binary entries'

    New-AdversarialZip @('kubePeep.exe', '../outside.exe')
    $rejected = $false
    try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted a traversal entry'

    $script:testStage = 'candidate-validation'
    Build-VersionBinary (Join-Path $payloadDir 'kubePeep.exe') '0.1.0suffix'
    Publish-ValidPayload
    $rejected = $false
    try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted an inexact candidate version token'
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'inexact candidate version changed the installed binary'

    $script:testStage = 'rollback'
    $rollbackMarker = Join-Path $testRoot 'rollback-marker'
    Build-RollbackFixture (Join-Path $payloadDir 'kubePeep.exe')
    Publish-ValidPayload
    $env:KUBEPEEP_INSTALLER_ROLLBACK_MARKER = $rollbackMarker
    try {
        $rejected = $false
        try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
        Assert-True $rejected 'PowerShell installer accepted a candidate that failed post-install verification'
    } finally {
        Remove-Item Env:\KUBEPEEP_INSTALLER_ROLLBACK_MARKER -ErrorAction SilentlyContinue
    }
    Assert-True ((& $binaryPath version | Out-String) -match 'version=0\.1\.0(?:\s|$)') 'PowerShell installer did not atomically restore the previous binary'
    Build-VersionBinary (Join-Path $payloadDir 'kubePeep.exe') '0.1.0'
    Publish-ValidPayload

    $script:testStage = 'reparse-binary'
    $savedBinary = Join-Path $testRoot 'saved-installed-kubePeep.exe'
    $executionProbe = Join-Path $testRoot 'execution-probe.exe'
    $executionMarker = Join-Path $testRoot 'execution-marker'
    Move-Item -LiteralPath $binaryPath -Destination $savedBinary
    Build-ExecutionProbe $executionProbe
    New-Item -ItemType SymbolicLink -Path $binaryPath -Target $executionProbe | Out-Null
    $env:KUBEPEEP_INSTALLER_EXECUTION_MARKER = $executionMarker
    try {
        $rejected = $false
        try { & ./install.ps1 -Uninstall -InstallDir $installDir | Out-Null } catch { $rejected = $true }
        Assert-True $rejected 'PowerShell installer accepted a reparse-point binary'
        Assert-True (-not (Test-Path -LiteralPath $executionMarker)) 'PowerShell installer executed a reparse-point binary before rejecting it'
    } finally {
        Remove-Item Env:\KUBEPEEP_INSTALLER_EXECUTION_MARKER -ErrorAction SilentlyContinue
        [IO.File]::Delete($binaryPath)
        Move-Item -LiteralPath $savedBinary -Destination $binaryPath
    }

    $script:testStage = 'uninstall'
    New-Item -ItemType Directory -Path $dataRoot -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $dataRoot 'sentinel') -Value 'preserve-me'
    & ./install.ps1 -Uninstall -InstallDir $installDir | Out-Null
    Assert-True (-not (Test-Path -LiteralPath $binaryPath)) 'PowerShell uninstall preserved the binary'
    Assert-True (Test-Path -LiteralPath (Join-Path $dataRoot 'sentinel')) 'PowerShell uninstall removed local data'
    $uninstalledUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    Assert-True (@(([string]$uninstalledUserPath).Split(';') | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }).Count -eq 0) 'PowerShell uninstall preserved its user PATH entry'

    $script:testStage = 'path-ownership'
    $manualPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $manualEntries = if ([string]::IsNullOrWhiteSpace($manualPath)) { @($installDir) } else { @($manualPath.Split(';')) + $installDir }
    [Environment]::SetEnvironmentVariable('Path', ($manualEntries -join ';'), 'User')
    Publish-ValidPayload
    & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $installDir '.kubePeep.path-managed'))) 'PowerShell installer claimed ownership of a preexisting PATH entry'
    & ./install.ps1 -Uninstall -InstallDir $installDir | Out-Null
    $preservedUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    Assert-True (@(([string]$preservedUserPath).Split(';') | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }).Count -eq 1) 'PowerShell uninstall removed a preexisting PATH entry'
    [Environment]::SetEnvironmentVariable('Path', $manualPath, 'User')

    $script:testStage = 'reparse-data'
    $preservedData = "$dataRoot.preserved"
    $outsideData = Join-Path $testRoot 'outside-data'
    Move-Item -LiteralPath $dataRoot -Destination $preservedData
    New-Item -ItemType Directory -Path $outsideData | Out-Null
    Set-Content -LiteralPath (Join-Path $outsideData 'sentinel') -Value 'outside-must-survive'
    New-Item -ItemType Junction -Path $dataRoot -Target $outsideData | Out-Null
    $rejected = $false
    try { & ./install.ps1 -Uninstall -PurgeData -ConfirmPurge -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer purged through a reparse-point data root'
    Assert-True (Test-Path -LiteralPath (Join-Path $outsideData 'sentinel')) 'reparse-point rejection changed the outside target'
    [IO.Directory]::Delete($dataRoot)
    Move-Item -LiteralPath $preservedData -Destination $dataRoot

    $nestedOutside = Join-Path $testRoot 'nested-outside-data'
    $nestedJunction = Join-Path $dataRoot 'nested-junction'
    New-Item -ItemType Directory -Path $nestedOutside | Out-Null
    Set-Content -LiteralPath (Join-Path $nestedOutside 'sentinel') -Value 'nested-outside-must-survive'
    New-Item -ItemType Junction -Path $nestedJunction -Target $nestedOutside | Out-Null
    $rejected = $false
    try { & ./install.ps1 -Uninstall -PurgeData -ConfirmPurge -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer purged through a nested reparse point'
    Assert-True (Test-Path -LiteralPath (Join-Path $nestedOutside 'sentinel')) 'nested reparse rejection changed the outside target'
    Assert-True (Test-Path -LiteralPath (Join-Path $dataRoot 'sentinel')) 'nested reparse rejection partially deleted local data'
    [IO.Directory]::Delete($nestedJunction)

    $script:testStage = 'argument-validation'
    $rejected = $false
    try { & ./install.ps1 -Uninstall -ConfirmPurge -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer accepted -ConfirmPurge without -PurgeData'

    $script:testStage = 'architecture'
    $savedArchitecture = $env:PROCESSOR_ARCHITECTURE
    $savedArchitectureW6432 = $env:PROCESSOR_ARCHITEW6432
    $env:PROCESSOR_ARCHITECTURE = 'RISCV64'
    $env:PROCESSOR_ARCHITEW6432 = $null
    try {
        $rejected = $false
        try { & ./install.ps1 -Version 0.1.0 -InstallDir $installDir | Out-Null } catch { $rejected = $true }
        Assert-True $rejected 'PowerShell installer accepted an unsupported architecture'
    } finally {
        $env:PROCESSOR_ARCHITECTURE = $savedArchitecture
        $env:PROCESSOR_ARCHITEW6432 = $savedArchitectureW6432
    }

    $script:testStage = 'purge'
    $runtimeDir = Join-Path $dataRoot 'runtime'
    New-Item -ItemType Directory -Path $runtimeDir -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $runtimeDir 'kubePeep.lock') -Value 'synthetic-lock'
    $rejected = $false
    try { & ./install.ps1 -Uninstall -PurgeData -ConfirmPurge -InstallDir $installDir | Out-Null } catch { $rejected = $true }
    Assert-True $rejected 'PowerShell installer purged data while the runtime lock existed'
    Remove-Item -LiteralPath (Join-Path $runtimeDir 'kubePeep.lock') -Force
    & ./install.ps1 -Uninstall -PurgeData -ConfirmPurge -InstallDir $installDir | Out-Null
    Assert-True (-not (Test-Path -LiteralPath $dataRoot)) 'PowerShell confirmed purge preserved local data'

    Write-Output 'install.ps1 tests passed'
} catch {
    $script:testFailureDiagnostic = Get-SafeTestDiagnostic $script:testStage $_.Exception
} finally {
    Invoke-SafeTestCleanup { Remove-Item Function:\Invoke-WebRequest -Force -ErrorAction SilentlyContinue }
    Invoke-SafeTestCleanup { Remove-Item Env:\KUBEPEEP_INSTALLER_ROLLBACK_MARKER -ErrorAction SilentlyContinue }
    Invoke-SafeTestCleanup { Remove-Item Env:\KUBEPEEP_INSTALLER_EXECUTION_MARKER -ErrorAction SilentlyContinue }
    Invoke-SafeTestCleanup { Remove-Item Env:\KUBEPEEP_INSTALLER_DOWNLOAD_MARKER -ErrorAction SilentlyContinue }
    Invoke-SafeTestCleanup { $env:LOCALAPPDATA = $previousLocalAppData }
    Invoke-SafeTestCleanup { $env:KUBEPEEP_TEST_RELEASE = $previousRelease }
    Invoke-SafeTestCleanup { [Environment]::SetEnvironmentVariable('Path', $previousUserPath, 'User') }
    foreach ($possibleReparsePoint in @($dataRoot, $nestedJunction, $binaryPath)) {
        if ([string]::IsNullOrWhiteSpace($possibleReparsePoint)) { continue }
        Invoke-SafeTestCleanup {
            $item = Get-Item -LiteralPath $possibleReparsePoint -Force -ErrorAction SilentlyContinue
            if ($null -ne $item -and ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                if ($item.PSIsContainer) {
                    [IO.Directory]::Delete($possibleReparsePoint)
                } else {
                    [IO.File]::Delete($possibleReparsePoint)
                }
            }
        }
    }
    Invoke-SafeTestCleanup { Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue }
}

if ($null -ne $script:testFailureDiagnostic) {
    Write-Output $script:testFailureDiagnostic
    exit 1
}
exit 0
