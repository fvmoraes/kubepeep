//go:build windows

package updater

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func installPlatform(_ context.Context, target, staged, targetVersion, currentHash, lockPath string, _ BinaryVerifier) (bool, error) {
	candidateHash, err := fileSHA256(staged)
	if err != nil {
		return false, fmt.Errorf("update: hash staged Windows executable: %w", err)
	}
	backupFile, err := os.CreateTemp(filepath.Dir(target), ".kubePeep.backup.*.exe")
	if err != nil {
		return false, fmt.Errorf("update: reserve executable backup: %w", err)
	}
	backup := backupFile.Name()
	if err := backupFile.Close(); err != nil {
		_ = os.Remove(backup)
		return false, fmt.Errorf("update: close executable backup reservation: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return false, fmt.Errorf("update: release executable backup reservation: %w", err)
	}
	failed := backup + ".failed"
	status := filepath.Join(filepath.Dir(target), ".kubePeep.update-result")
	if err := launchWindowsReplacement(target, staged, backup, failed, status, lockPath, targetVersion, currentHash, hex.EncodeToString(candidateHash), os.Getpid()); err != nil {
		return false, err
	}
	return true, nil
}

func launchWindowsReplacement(target, candidate, backup, failed, status, lockPath, version, currentHash, candidateHash string, parentPID int) error {
	if err := os.Remove(status); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("update: clear previous Windows update status: %w", err)
	}
	helper, err := os.CreateTemp(filepath.Dir(target), ".kubePeep.update.*.ps1")
	if err != nil {
		return fmt.Errorf("update: create Windows replacement helper: %w", err)
	}
	helperPath := helper.Name()
	remove := true
	defer func() {
		_ = helper.Close()
		if remove {
			_ = os.Remove(helperPath)
		}
	}()
	if _, err := helper.WriteString(windowsHelperScript); err != nil {
		return fmt.Errorf("update: write Windows replacement helper: %w", err)
	}
	if err := helper.Sync(); err != nil {
		return fmt.Errorf("update: sync Windows replacement helper: %w", err)
	}
	if err := helper.Close(); err != nil {
		return fmt.Errorf("update: close Windows replacement helper: %w", err)
	}
	command := exec.Command(
		"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", helperPath,
		"-Target", target,
		"-Candidate", candidate,
		"-Backup", backup,
		"-Failed", failed,
		"-Status", status,
		"-LockPath", lockPath,
		"-ExpectedVersion", version,
		"-ExpectedCurrentHash", currentHash,
		"-ExpectedCandidateHash", candidateHash,
		"-ParentPid", strconv.Itoa(parentPID),
	)
	const (
		createNewProcessGroup = 0x00000200
		createNoWindow        = 0x08000000
	)
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | createNoWindow,
		HideWindow:    true,
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("update: start Windows replacement helper: %w", err)
	}
	remove = false
	_ = command.Process.Release()
	return nil
}

const windowsHelperScript = `param(
    [Parameter(Mandatory=$true)][string]$Target,
    [Parameter(Mandatory=$true)][string]$Candidate,
    [Parameter(Mandatory=$true)][string]$Backup,
    [Parameter(Mandatory=$true)][string]$Failed,
    [Parameter(Mandatory=$true)][string]$Status,
    [Parameter(Mandatory=$true)][string]$LockPath,
    [Parameter(Mandatory=$true)][string]$ExpectedVersion,
    [Parameter(Mandatory=$true)][string]$ExpectedCurrentHash,
    [Parameter(Mandatory=$true)][string]$ExpectedCandidateHash,
    [Parameter(Mandatory=$true)][int]$ParentPid
)
$ErrorActionPreference = 'Stop'
$replaced = $false
$failureStage = 'unknown'
function Assert-RegularFile([string]$Path, [string]$Description) {
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -eq $item -or $item.PSIsContainer -or
        ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "$Description is not a regular file"
    }
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
function Get-AllowlistedFailureStage([string]$Stage) {
    switch -Exact ($Stage) {
        'wait-parent' { return $Stage }
        'replace-invoke' { return $Stage }
        'replace-source-inspect' { return $Stage }
        'replace-destination-inspect' { return $Stage }
        'replace-source-hash' { return $Stage }
        'replace-destination-hash' { return $Stage }
        'replace-commit' { return $Stage }
        'verify-start' { return $Stage }
        'verify-output' { return $Stage }
        'cleanup-backup' { return $Stage }
        'write-success-status' { return $Stage }
        'rollback-invoke' { return $Stage }
        'rollback-source-inspect' { return $Stage }
        'rollback-destination-inspect' { return $Stage }
        'rollback-source-hash' { return $Stage }
        'rollback-destination-hash' { return $Stage }
        'rollback-commit' { return $Stage }
        'rollback-cleanup' { return $Stage }
        default { return 'unknown' }
    }
}
function Get-SafeFailureDiagnostic([string]$Stage, [Exception]$Exception) {
    $safeStage = Get-AllowlistedFailureStage $Stage
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
    return "stage=$safeStage type=$typeName hresult=0x$hResultHex"
}
function Test-RetryableReplaceError([Exception]$Exception) {
    while ($null -ne $Exception) {
        if ($Exception -is [IO.IOException] -or
            $Exception -is [UnauthorizedAccessException]) {
            return $true
        }
        $Exception = $Exception.InnerException
    }
    return $false
}
function Invoke-VerifiedReplaceWithRetry(
    [string]$Source,
    [string]$Destination,
    [string]$BackupPath,
    [string]$ExpectedSourceHash,
    [string]$ExpectedDestinationHash,
    [string]$SourceDescription,
    [string]$DestinationDescription,
    [string]$StagePrefix
) {
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        try {
            $script:failureStage = "$StagePrefix-source-inspect"
            Assert-RegularFile $Source $SourceDescription
            $script:failureStage = "$StagePrefix-destination-inspect"
            Assert-RegularFile $Destination $DestinationDescription
            $script:failureStage = "$StagePrefix-source-hash"
            $sourceHash = Get-SHA256Hex $Source
            if ($sourceHash -ne $ExpectedSourceHash.ToLowerInvariant()) {
                throw "$SourceDescription changed while the update was pending"
            }
            $script:failureStage = "$StagePrefix-destination-hash"
            $destinationHash = Get-SHA256Hex $Destination
            if ($destinationHash -ne $ExpectedDestinationHash.ToLowerInvariant()) {
                throw "$DestinationDescription changed while the update was pending"
            }
            $script:failureStage = "$StagePrefix-commit"
            [System.IO.File]::Replace($Source, $Destination, $BackupPath, $true)
            return
        } catch {
            if (-not (Test-RetryableReplaceError $_.Exception) -or $attempt -eq 19) {
                throw
            }
            Start-Sleep -Milliseconds 100
        }
    }
}
function Invoke-VersionWithTimeout([string]$Path) {
    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $Path
    $startInfo.Arguments = 'version'
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $startInfo
    $started = $false
    try {
        if (-not $process.Start()) {
            throw 'replacement version process did not start'
        }
        $started = $true
        $stdoutBuffer = [char[]]::new(4096)
        $stderrBuffer = [char[]]::new(4096)
        $stdout = New-Object System.Text.StringBuilder
        $stdoutDone = $false
        $stderrDone = $false
        $stdoutRead = $process.StandardOutput.ReadAsync($stdoutBuffer, 0, $stdoutBuffer.Length)
        $stderrRead = $process.StandardError.ReadAsync($stderrBuffer, 0, $stderrBuffer.Length)
        $never = [Threading.Tasks.Task]::Delay([Threading.Timeout]::Infinite)
        $timer = [Diagnostics.Stopwatch]::StartNew()
        $totalCharacters = 0
        while (-not $stdoutDone -or -not $stderrDone) {
            $remaining = 10000 - [int]$timer.ElapsedMilliseconds
            if ($remaining -le 0) {
                throw 'replacement version verification timed out'
            }
            $completed = [Threading.Tasks.Task]::WaitAny(
                [Threading.Tasks.Task[]]@($stdoutRead, $stderrRead),
                $remaining
            )
            if ($completed -lt 0) {
                throw 'replacement version verification timed out'
            }
            if ($completed -eq 0) {
                $read = $stdoutRead.GetAwaiter().GetResult()
                if ($read -eq 0) {
                    $stdoutDone = $true
                    $stdoutRead = $never
                } else {
                    $totalCharacters += $read
                    if ($totalCharacters -gt 65536) {
                        throw 'replacement version output exceeded its limit'
                    }
                    $null = $stdout.Append($stdoutBuffer, 0, $read)
                    $stdoutRead = $process.StandardOutput.ReadAsync($stdoutBuffer, 0, $stdoutBuffer.Length)
                }
            } else {
                $read = $stderrRead.GetAwaiter().GetResult()
                if ($read -eq 0) {
                    $stderrDone = $true
                    $stderrRead = $never
                } else {
                    $totalCharacters += $read
                    if ($totalCharacters -gt 65536) {
                        throw 'replacement version output exceeded its limit'
                    }
                    $stderrRead = $process.StandardError.ReadAsync($stderrBuffer, 0, $stderrBuffer.Length)
                }
            }
        }
        $remaining = 10000 - [int]$timer.ElapsedMilliseconds
        if ($remaining -le 0 -or -not $process.WaitForExit($remaining)) {
            throw 'replacement version verification timed out'
        }
        if ($process.ExitCode -ne 0) {
            throw 'replacement version process failed'
        }
        return $stdout.ToString().Trim()
    } finally {
        if ($started) {
            try {
                if (-not $process.HasExited) {
                    $process.Kill()
                    $null = $process.WaitForExit(1000)
                }
            } catch { }
        }
        $process.Dispose()
    }
}
try {
    $failureStage = 'wait-parent'
    if ($ParentPid -gt 0) {
        for ($attempt = 0; $attempt -lt 3000; $attempt++) {
            if (-not (Get-Process -Id $ParentPid -ErrorAction SilentlyContinue)) { break }
            Start-Sleep -Milliseconds 100
        }
        if (Get-Process -Id $ParentPid -ErrorAction SilentlyContinue) {
            throw 'timed out waiting for the updater process to exit'
        }
    }
    $failureStage = 'replace-invoke'
    Invoke-VerifiedReplaceWithRetry $Candidate $Target $Backup $ExpectedCandidateHash $ExpectedCurrentHash 'candidate executable' 'installed executable' 'replace'
    $replaced = $true
    $failureStage = 'verify-start'
    $versionOutput = Invoke-VersionWithTimeout $Target
    $failureStage = 'verify-output'
    $versionTokens = [regex]::Split($versionOutput, '\s+')
    if (-not ($versionTokens -contains ("version=" + $ExpectedVersion))) {
        throw 'replacement version verification failed'
    }
    $failureStage = 'cleanup-backup'
    Remove-Item -LiteralPath $Backup -Force
    $failureStage = 'write-success-status'
    [System.IO.File]::WriteAllText($Status, "installed version=$ExpectedVersion")
} catch {
    $failureDiagnostic = Get-SafeFailureDiagnostic $failureStage $_.Exception
    $statusMessage = "failed version=$ExpectedVersion $failureDiagnostic"
    if ($replaced -and (Test-Path -LiteralPath $Backup -PathType Leaf)) {
        try {
            $failureStage = 'rollback-invoke'
            Invoke-VerifiedReplaceWithRetry $Backup $Target $Failed $ExpectedCurrentHash $ExpectedCandidateHash 'rollback executable' 'failed replacement executable' 'rollback'
            $failureStage = 'rollback-cleanup'
            Remove-Item -LiteralPath $Failed -Force -ErrorAction SilentlyContinue
            $statusMessage = "rolled-back version=$ExpectedVersion"
        } catch {
            $rollbackDiagnostic = Get-SafeFailureDiagnostic $failureStage $_.Exception
            $statusMessage = "rollback-failed version=$ExpectedVersion $rollbackDiagnostic"
        }
    }
    try { [System.IO.File]::WriteAllText($Status, $statusMessage) } catch { }
    exit 1
} finally {
    Remove-Item -LiteralPath $Candidate -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $LockPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
}
`
