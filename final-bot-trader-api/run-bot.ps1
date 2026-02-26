# Copy Trading Bot - 24/7 Runner (PowerShell)
# Run with: powershell -ExecutionPolicy Bypass -File run-bot.ps1

$ErrorActionPreference = "Continue"
$LogFile = "bot_output.log"
$RestartDelay = 30  # seconds
$BotPath = Join-Path $PSScriptRoot "live-trading.exe"

function Write-Log {
    param([string]$Message)
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logMessage = "[$timestamp] $Message"
    Write-Host $logMessage
    Add-Content -Path $LogFile -Value $logMessage
}

Write-Log "=== Copy Trading Bot 24/7 Runner ==="
Write-Log "Bot path: $BotPath"
Write-Log "Log file: $LogFile"
Write-Log "Restart delay: $RestartDelay seconds"
Write-Log ""

# Check if bot exists
if (-not (Test-Path $BotPath)) {
    Write-Log "ERROR: Bot executable not found at $BotPath"
    Write-Log "Please run 'go build -o live-trading.exe ./cmd/live-trading' first"
    exit 1
}

# Main loop
while ($true) {
    Write-Log "Starting bot..."

    try {
        # Start the bot process with YES piped to stdin
        $process = Start-Process -FilePath $BotPath `
            -ArgumentList "" `
            -NoNewWindow `
            -PassThru `
            -RedirectStandardInput "NUL" `
            -Wait

        # Also run with echo YES for confirmation
        $pinfo = New-Object System.Diagnostics.ProcessStartInfo
        $pinfo.FileName = $BotPath
        $pinfo.RedirectStandardInput = $true
        $pinfo.RedirectStandardOutput = $true
        $pinfo.RedirectStandardError = $true
        $pinfo.UseShellExecute = $false
        $pinfo.CreateNoWindow = $true

        $p = New-Object System.Diagnostics.Process
        $p.StartInfo = $pinfo
        $p.Start() | Out-Null

        # Send YES confirmation
        $p.StandardInput.WriteLine("YES")
        $p.StandardInput.Close()

        # Read output continuously
        while (-not $p.HasExited) {
            $line = $p.StandardOutput.ReadLine()
            if ($line) {
                Write-Log $line
            }
            Start-Sleep -Milliseconds 100
        }

        # Get any remaining output
        $remaining = $p.StandardOutput.ReadToEnd()
        if ($remaining) {
            Write-Log $remaining
        }

        $exitCode = $p.ExitCode
        Write-Log "Bot exited with code: $exitCode"

    } catch {
        Write-Log "ERROR: $($_.Exception.Message)"
    }

    Write-Log "Restarting in $RestartDelay seconds..."
    Start-Sleep -Seconds $RestartDelay
}
