[CmdletBinding()]
param(
    [ValidatePattern('^v?(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$')]
    [string]$Version,
    [string]$InstallDir,
    [switch]$Uninstall,
    [switch]$PurgeData,
    [switch]$ConfirmPurge
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

function Fail([string]$Message) {
    throw "kubePeep installer: $Message"
}

if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
    Fail 'LOCALAPPDATA is required.'
}

$defaultInstallDir = Join-Path $env:LOCALAPPDATA 'Programs\kubePeep'
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = $defaultInstallDir
}
$InstallDir = [System.IO.Path]::GetFullPath($InstallDir)
if ($InstallDir -eq [System.IO.Path]::GetPathRoot($InstallDir) -or $InstallDir.Contains(';')) {
    Fail 'refusing an unsafe install directory.'
}
$binaryPath = Join-Path $InstallDir 'kubePeep.exe'
$dataRoot = [System.IO.Path]::GetFullPath((Join-Path $env:LOCALAPPDATA 'kubePeep'))
$canonicalDataRoot = [System.IO.Path]::GetFullPath((Join-Path $env:LOCALAPPDATA 'kubePeep'))
$pathMarker = Join-Path $InstallDir '.kubePeep.path-managed'
$maximumChecksumsBytes = 1MB
$maximumArchiveBytes = 256MB
$maximumBinaryBytes = 256MB
$script:installerLock = $null
$script:installerLockPath = Join-Path $InstallDir '.kubePeep.install.lock'
$script:installDirectoryCreated = $false

function Assert-NotReparsePoint([string]$Path, [string]$Description) {
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -ne $item -and ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        Fail "refusing a reparse-point $Description."
    }
}

function Assert-SafeExistingBinary {
    $item = Get-Item -LiteralPath $binaryPath -Force -ErrorAction SilentlyContinue
    if ($null -eq $item) {
        return $false
    }
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        Fail 'refusing a reparse-point binary.'
    }
    if ($item.PSIsContainer) {
        Fail 'refusing a non-file binary path.'
    }
    return $true
}

function Assert-SafePathMarker {
    $item = Get-Item -LiteralPath $pathMarker -Force -ErrorAction SilentlyContinue
    if ($null -eq $item) {
        return $false
    }
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        Fail 'refusing a reparse-point PATH ownership marker.'
    }
    if ($item.PSIsContainer) {
        Fail 'refusing a non-file PATH ownership marker.'
    }
    return $true
}

function Enter-InstallerLock {
    if (-not (Test-Path -LiteralPath $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        $script:installDirectoryCreated = $true
    }
    Assert-NotReparsePoint $InstallDir 'install directory'
    if (-not (Test-Path -LiteralPath $InstallDir -PathType Container)) {
        Fail 'install directory is not a directory.'
    }
    Assert-NotReparsePoint $script:installerLockPath 'installer lock'
    try {
        $script:installerLock = [IO.File]::Open(
            $script:installerLockPath,
            [IO.FileMode]::OpenOrCreate,
            [IO.FileAccess]::ReadWrite,
            [IO.FileShare]::None
        )
    } catch {
        Fail 'another install or uninstall transaction is already in progress.'
    }
}

function Exit-InstallerLock {
    if ($null -ne $script:installerLock) {
        $script:installerLock.Dispose()
        $script:installerLock = $null
        Remove-Item -LiteralPath $script:installerLockPath -Force -ErrorAction SilentlyContinue
    }
    if ($script:installDirectoryCreated) {
        try { [IO.Directory]::Delete($InstallDir, $false) } catch { }
        $script:installDirectoryCreated = $false
    }
}

function Test-KubePeepRunning {
    if (-not (Assert-SafeExistingBinary)) {
        return $false
    }
    & $binaryPath status *> $null
    return $LASTEXITCODE -eq 0
}

function Remove-UserPathEntry([string]$Directory) {
    if (-not (Assert-SafePathMarker)) {
        return
    }
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not [string]::IsNullOrWhiteSpace($current)) {
        $target = $Directory.TrimEnd('\')
        $removed = $false
        $kept = New-Object System.Collections.Generic.List[string]
        foreach ($entry in $current.Split(';')) {
            if (-not $removed -and $entry.TrimEnd('\') -ieq $target) {
                $removed = $true
                continue
            }
            $kept.Add($entry)
        }
        [Environment]::SetEnvironmentVariable('Path', ($kept -join ';'), 'User')
    }
    Remove-Item -LiteralPath $pathMarker -Force
}

function Add-UserPathEntry([string]$Directory) {
    $markerExists = Assert-SafePathMarker
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = if ([string]::IsNullOrWhiteSpace($current)) { @() } else { @($current.Split(';')) }
    if (-not ($entries | Where-Object { $_.TrimEnd('\') -ieq $Directory.TrimEnd('\') })) {
        $updated = (($entries + $Directory) -join ';')
        [Environment]::SetEnvironmentVariable('Path', $updated, 'User')
        if (-not $markerExists) {
            $createdMarker = $false
            try {
                $marker = [IO.File]::Open($pathMarker, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
                $createdMarker = $true
                try {
                    $markerBytes = [Text.Encoding]::ASCII.GetBytes("managed`r`n")
                    $marker.Write($markerBytes, 0, $markerBytes.Length)
                    $marker.Flush($true)
                } finally {
                    $marker.Dispose()
                }
            } catch {
                [Environment]::SetEnvironmentVariable('Path', $current, 'User')
                if ($createdMarker) {
                    Remove-Item -LiteralPath $pathMarker -Force -ErrorAction SilentlyContinue
                }
                throw
            }
        }
    }
}

function Assert-SafeTree([string]$Path) {
    $root = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -eq $root) {
        return
    }
    Assert-SafeTreeItem $root
}

function Assert-SafeTreeItem([IO.FileSystemInfo]$Item) {
    $Item.Refresh()
    if (($Item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        Fail "refusing a reparse point inside the data root: $($Item.FullName)"
    }
    if (($Item.Attributes -band [IO.FileAttributes]::Directory) -ne 0) {
        $directory = [IO.DirectoryInfo]$Item
        foreach ($child in $directory.EnumerateFileSystemInfos()) {
            Assert-SafeTreeItem $child
        }
    }
}

function Remove-SafeTree([string]$Path) {
    $root = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -eq $root) {
        return
    }
    Remove-SafeTreeItem $root
}

function Remove-SafeTreeItem([IO.FileSystemInfo]$Item) {
    $Item.Refresh()
    if (($Item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        Fail "refusing a reparse point inside the data root: $($Item.FullName)"
    }
    if (($Item.Attributes -band [IO.FileAttributes]::Directory) -ne 0) {
        $directory = [IO.DirectoryInfo]$Item
        $children = @($directory.EnumerateFileSystemInfos())
        foreach ($child in $children) {
            Remove-SafeTreeItem $child
        }
        [IO.File]::SetAttributes($directory.FullName, [IO.FileAttributes]::Normal)
        $directory.Delete()
        return
    }
    [IO.File]::SetAttributes($Item.FullName, [IO.FileAttributes]::Normal)
    $Item.Delete()
}

function Assert-PurgeAllowed {
    if (-not $PurgeData) {
        return
    }
    if (-not $ConfirmPurge) {
        Fail '-PurgeData also requires -ConfirmPurge.'
    }
    if ($dataRoot -ne $canonicalDataRoot -or $dataRoot -eq [System.IO.Path]::GetPathRoot($dataRoot)) {
        Fail 'refusing a non-canonical data path.'
    }
    Assert-NotReparsePoint $dataRoot 'data root'
    if (Test-Path -LiteralPath (Join-Path $dataRoot 'runtime\kubePeep.lock')) {
        Fail 'runtime lock exists; stop kubePeep before purging data.'
    }
    Assert-SafeTree $dataRoot
}

function Copy-BoundedStream([IO.Stream]$Source, [IO.Stream]$Destination, [long]$MaximumBytes, [int]$TimeoutMilliseconds = 0) {
    $buffer = New-Object byte[] 81920
    [long]$total = 0
    $timer = if ($TimeoutMilliseconds -gt 0) { [Diagnostics.Stopwatch]::StartNew() } else { $null }
    while ($true) {
        if ($null -ne $timer) {
            $remaining = $TimeoutMilliseconds - [int]$timer.ElapsedMilliseconds
            if ($remaining -le 0) {
                Fail 'release asset transfer timed out.'
            }
            $readTask = $Source.ReadAsync($buffer, 0, $buffer.Length)
            if (-not $readTask.Wait($remaining)) {
                Fail 'release asset transfer timed out.'
            }
            $read = $readTask.GetAwaiter().GetResult()
        } else {
            $read = $Source.Read($buffer, 0, $buffer.Length)
        }
        if ($read -le 0) {
            break
        }
        if ($total -gt ($MaximumBytes - $read)) {
            Fail 'downloaded release asset exceeds its size limit.'
        }
        $Destination.Write($buffer, 0, $read)
        $total += $read
    }
    return $total
}

function Save-BoundedDownload([string]$Uri, [string]$OutFile, [long]$MaximumBytes) {
    if (([Uri]$Uri).Scheme -ne 'https') {
        Fail 'release download URL must use HTTPS.'
    }
    $fixtureDownloadsEnabled = (-not [string]::IsNullOrWhiteSpace($env:KUBEPEEP_TEST_RELEASE)) -or
        (-not [string]::IsNullOrWhiteSpace($env:KUBEPEEP_CANDIDATE_DIR))
    if ($fixtureDownloadsEnabled) {
        $override = Get-Command Invoke-WebRequest -CommandType Function -ErrorAction SilentlyContinue
        if ($null -eq $override) {
            Fail 'local release fixture requires an explicit download function.'
        }
        try {
            Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $OutFile
            if ((Get-Item -LiteralPath $OutFile).Length -gt $MaximumBytes) {
                Fail 'downloaded release asset exceeds its size limit.'
            }
        } catch {
            Remove-Item -LiteralPath $OutFile -Force -ErrorAction SilentlyContinue
            throw
        }
        return
    }

    Add-Type -AssemblyName System.Net.Http
    $handler = New-Object Net.Http.HttpClientHandler
    $handler.AllowAutoRedirect = $true
    $handler.MaxAutomaticRedirections = 10
    $client = [Net.Http.HttpClient]::new($handler)
    $client.Timeout = [TimeSpan]::FromSeconds(45)
    $response = $null
    $source = $null
    $destination = $null
    $completed = $false
    try {
        $response = $client.GetAsync($Uri, [Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
        if (-not $response.IsSuccessStatusCode) {
            Fail "release asset returned HTTP $([int]$response.StatusCode)."
        }
        if ($response.RequestMessage.RequestUri.Scheme -ne 'https') {
            Fail 'release download redirect must use HTTPS.'
        }
        $contentLength = $response.Content.Headers.ContentLength
        if ($null -ne $contentLength -and $contentLength -gt $MaximumBytes) {
            Fail 'downloaded release asset exceeds its size limit.'
        }
        $source = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        $destination = [IO.File]::Open($OutFile, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
        $null = Copy-BoundedStream $source $destination $MaximumBytes 45000
        $destination.Flush($true)
        $completed = $true
    } finally {
        if ($null -ne $destination) { $destination.Dispose() }
        if ($null -ne $source) { $source.Dispose() }
        if ($null -ne $response) { $response.Dispose() }
        $client.Dispose()
        $handler.Dispose()
        if (-not $completed) {
            Remove-Item -LiteralPath $OutFile -Force -ErrorAction SilentlyContinue
        }
    }
}

function Test-ExactVersionOutput([string]$Output, [string]$ExpectedVersion) {
    return (($Output -split '\s+') -contains ("version=" + $ExpectedVersion))
}

if ($Uninstall) {
    if (-not [string]::IsNullOrWhiteSpace($Version)) {
        Fail '-Version cannot be combined with -Uninstall.'
    }
    if ($ConfirmPurge -and -not $PurgeData) {
        Fail '-ConfirmPurge requires -PurgeData.'
    }
    Enter-InstallerLock
    try {
        $binaryExists = Assert-SafeExistingBinary
        $null = Assert-SafePathMarker
        Assert-PurgeAllowed
        if ($binaryExists -and (Test-KubePeepRunning)) {
            Fail 'kubePeep is running; stop it before uninstalling.'
        }
        if ($binaryExists) {
            Remove-Item -LiteralPath $binaryPath -Force
        }
        Remove-UserPathEntry $InstallDir
        if ($PurgeData) {
            Remove-SafeTree $dataRoot
            Write-Output "Purged local data at $dataRoot"
        } else {
            Write-Output "Removed kubePeep; local data was preserved at $dataRoot"
        }
    } finally {
        Exit-InstallerLock
    }
    exit 0
}

if ($PurgeData -or $ConfirmPurge) {
    Fail 'purge switches require -Uninstall.'
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    Fail '-Version X.Y.Z is required.'
}
$Version = $Version.TrimStart('v')

$architecture = if (-not [string]::IsNullOrWhiteSpace($env:PROCESSOR_ARCHITEW6432)) {
    $env:PROCESSOR_ARCHITEW6432
} else {
    $env:PROCESSOR_ARCHITECTURE
}
switch ($architecture.ToUpperInvariant()) {
    'AMD64' { $targetArch = 'amd64' }
    'ARM64' { $targetArch = 'arm64' }
    default { Fail "unsupported architecture: $architecture" }
}

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$archiveName = "kubePeep_${Version}_windows_${targetArch}.zip"
$releaseUrl = "https://github.com/fvmoraes/kubepeep/releases/download/v$Version"
$temporaryRoot = Join-Path ([IO.Path]::GetTempPath()) ("kubepeep-install-" + [Guid]::NewGuid().ToString('N'))
$staged = $null
$backup = $null
$failedReplacement = $null
$preserveRecoveryFiles = $false
$pathStateApplied = $false
$pathBeforeInstall = $null
$pathMarkerBeforeInstall = $false

Enter-InstallerLock
try {
    $binaryExists = Assert-SafeExistingBinary
    $null = Assert-SafePathMarker
    if ($binaryExists -and (Test-KubePeepRunning)) {
        Fail 'kubePeep is running; stop it before upgrading.'
    }

    New-Item -ItemType Directory -Path $temporaryRoot | Out-Null
    $checksumsPath = Join-Path $temporaryRoot 'checksums.txt'
    $archivePath = Join-Path $temporaryRoot $archiveName
    Save-BoundedDownload "$releaseUrl/checksums.txt" $checksumsPath $maximumChecksumsBytes
    Save-BoundedDownload "$releaseUrl/$archiveName" $archivePath $maximumArchiveBytes
    if ((Get-Item -LiteralPath $checksumsPath).Length -gt $maximumChecksumsBytes -or (Get-Item -LiteralPath $archivePath).Length -gt $maximumArchiveBytes) {
        Fail 'downloaded release asset exceeds its size limit.'
    }

    $escapedName = [Regex]::Escape($archiveName)
    $matches = @(Get-Content -LiteralPath $checksumsPath | ForEach-Object {
        if ($_ -match "^([0-9A-Fa-f]{64})\s+\*?$escapedName$") { $Matches[1].ToLowerInvariant() }
    })
    if ($matches.Count -ne 1) {
        Fail 'release checksum entry is missing or duplicated.'
    }
    $actual = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $matches[0]) {
        Fail 'SHA-256 verification failed.'
    }

    $extractDir = Join-Path $temporaryRoot 'extract'
    New-Item -ItemType Directory -Path $extractDir | Out-Null
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $extractRoot = [System.IO.Path]::GetFullPath($extractDir + [System.IO.Path]::DirectorySeparatorChar)
    $archive = [System.IO.Compression.ZipFile]::OpenRead($archivePath)
    $binaryEntries = 0
    $candidate = Join-Path $extractDir 'kubePeep.exe'
    try {
        foreach ($entry in $archive.Entries) {
            $destination = [System.IO.Path]::GetFullPath((Join-Path $extractDir $entry.FullName))
            if (-not $destination.StartsWith($extractRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
                Fail 'archive contains an unsafe path.'
            }
            if ($entry.FullName -ceq 'kubePeep.exe') {
                $binaryEntries++
                if ($entry.Length -le 0 -or $entry.Length -gt $maximumBinaryBytes) {
                    Fail 'archive binary has an invalid size.'
                }
                if ($binaryEntries -eq 1) {
                    $source = $entry.Open()
                    $output = [System.IO.File]::Open($candidate, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
                    try {
                        $written = Copy-BoundedStream $source $output $maximumBinaryBytes
                        if ($written -ne $entry.Length) {
                            Fail 'archive binary is truncated.'
                        }
                        $output.Flush($true)
                    } finally {
                        $source.Dispose()
                        $output.Dispose()
                    }
                }
            }
        }
    } finally {
        $archive.Dispose()
    }
    if ($binaryEntries -ne 1) {
        Fail 'archive must contain exactly one root kubePeep.exe binary.'
    }
    if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) {
        Fail 'archive does not contain kubePeep.exe.'
    }
    Assert-NotReparsePoint $candidate 'archive binary'
    $candidateVersion = (& $candidate version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or -not (Test-ExactVersionOutput $candidateVersion $Version)) {
        Fail 'downloaded binary version does not match the requested release.'
    }

    $binaryExists = Assert-SafeExistingBinary
    if ($binaryExists -and (Test-KubePeepRunning)) {
        Fail 'kubePeep is running; stop it before upgrading.'
    }

    $staged = Join-Path $InstallDir ('.kubePeep.install.' + [Guid]::NewGuid().ToString('N') + '.exe')
    $backup = Join-Path $InstallDir ('.kubePeep.backup.' + [Guid]::NewGuid().ToString('N') + '.exe')
    Copy-Item -LiteralPath $candidate -Destination $staged
    $hadBackup = $binaryExists
    $replacementActive = $false
    try {
        if ($hadBackup) {
            [IO.File]::Replace($staged, $binaryPath, $backup, $true)
        } else {
            Move-Item -LiteralPath $staged -Destination $binaryPath
        }
        $replacementActive = $true
        $installedVersion = (& $binaryPath version | Out-String).Trim()
        if ($LASTEXITCODE -ne 0 -or -not (Test-ExactVersionOutput $installedVersion $Version)) {
            throw 'post-install version verification failed'
        }
        $pathBeforeInstall = [Environment]::GetEnvironmentVariable('Path', 'User')
        $pathMarkerBeforeInstall = Assert-SafePathMarker
        Add-UserPathEntry $InstallDir
        $pathStateApplied = $true
        if ($hadBackup) {
            Remove-Item -LiteralPath $backup -Force
            $backup = $null
        }
        $replacementActive = $false
    } catch {
        $transactionError = $_
        $rollbackErrors = New-Object System.Collections.Generic.List[string]
        if ($replacementActive) {
            try {
                if ($hadBackup -and (Test-Path -LiteralPath $backup -PathType Leaf)) {
                    $failedReplacement = Join-Path $InstallDir ('.kubePeep.failed.' + [Guid]::NewGuid().ToString('N') + '.exe')
                    [IO.File]::Replace($backup, $binaryPath, $failedReplacement, $true)
                    $backup = $null
                    Remove-Item -LiteralPath $failedReplacement -Force -ErrorAction SilentlyContinue
                    $failedReplacement = $null
                } else {
                    Remove-Item -LiteralPath $binaryPath -Force -ErrorAction SilentlyContinue
                }
            } catch {
                $preserveRecoveryFiles = $true
                $rollbackErrors.Add("binary rollback failed: $($_.Exception.Message)")
            }
        }
        if ($pathStateApplied) {
            try {
                [Environment]::SetEnvironmentVariable('Path', $pathBeforeInstall, 'User')
                if (-not $pathMarkerBeforeInstall -and (Assert-SafePathMarker)) {
                    Remove-Item -LiteralPath $pathMarker -Force
                }
            } catch {
                $rollbackErrors.Add("PATH rollback failed: $($_.Exception.Message)")
            }
        }
        if ($rollbackErrors.Count -gt 0) {
            throw "kubePeep installer: installation failed and rollback was incomplete: $($rollbackErrors -join '; ')"
        }
        if ($transactionError.Exception.Message -like 'kubePeep installer:*') {
            throw $transactionError
        }
        Fail 'post-install verification failed; the previous binary was restored.'
    }

    Write-Output "Installed kubePeep $Version at $binaryPath"
    Write-Output 'Open a new terminal, then run: kubePeep start'
} finally {
    if ($null -ne $staged) { Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue }
    if (-not $preserveRecoveryFiles) {
        if ($null -ne $backup) { Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue }
        if ($null -ne $failedReplacement) { Remove-Item -LiteralPath $failedReplacement -Force -ErrorAction SilentlyContinue }
    }
    Remove-Item -LiteralPath $temporaryRoot -Recurse -Force -ErrorAction SilentlyContinue
    Exit-InstallerLock
}
