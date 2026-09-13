# We source this file with -NoExit -File
$env:PATH = {{.WSHBINDIR_PWSH}} + "{{.PATHSEP}}" + $env:PATH

# Source dynamic script from wsh token
$waveterm_swaptoken_output = wsh token $env:WAVETERM_SWAPTOKEN pwsh 2>$null | Out-String
if ($waveterm_swaptoken_output -and $waveterm_swaptoken_output -ne "") {
    Invoke-Expression $waveterm_swaptoken_output
}
Remove-Variable -Name waveterm_swaptoken_output
Remove-Item Env:WAVETERM_SWAPTOKEN

# Load Wave completions
wsh completion powershell | Out-String | Invoke-Expression

if ($PSVersionTable.PSVersion.Major -lt 5) {
    return  # OSC integration requires Windows PowerShell 5+
}

if ($PSVersionTable.PSVersion.Major -ge 7 -and $PSStyle.FileInfo.Directory -eq "`e[44;1m") {
    $PSStyle.FileInfo.Directory = "`e[34;1m"
}

$Global:_WAVETERM_SI_FIRSTPROMPT = $true
$Global:_WAVETERM_SI_RANCOMMAND = $false

# shell integration
function Global:_waveterm_si_blocked {
    # Check if we're in tmux or screen
    return ($env:TMUX -or $env:STY -or $env:TERM -like "tmux*" -or $env:TERM -like "screen*")
}

function Global:_waveterm_si_osc7 {
    if (_waveterm_si_blocked) { return }
    
    # Percent-encode the raw path as-is (handles UNC, drive letters, etc.)
    $encoded_pwd = [System.Uri]::EscapeDataString($PWD.Path)
    
    # OSC 7 - current directory (ESC/BEL via code points: `e/`a are PS6+ only)
    Write-Host -NoNewline "$([char]27)]7;file://localhost/$encoded_pwd$([char]7)"
}

function Global:_waveterm_si_prompt {
    $__wave_ok = $?
    $__wave_exitcode = $LASTEXITCODE
    if (_waveterm_si_blocked) { return }
    
    if ($Global:_WAVETERM_SI_RANCOMMAND) {
        $__wave_code = 0
        if (-not $__wave_ok) {
            if ($null -ne $__wave_exitcode) { $__wave_code = $__wave_exitcode } else { $__wave_code = 1 }
        }
        Write-Host -NoNewline "$([char]27)]16162;D;{`"exitcode`":$__wave_code}$([char]7)"
        $Global:_WAVETERM_SI_RANCOMMAND = $false
    }
    if ($Global:_WAVETERM_SI_FIRSTPROMPT) {
        $shellversion = $PSVersionTable.PSVersion.ToString()
        Write-Host -NoNewline "$([char]27)]16162;M;{`"shell`":`"pwsh`",`"shellversion`":`"$shellversion`",`"integration`":true}$([char]7)"
        $Global:_WAVETERM_SI_FIRSTPROMPT = $false
    }
    
    _waveterm_si_osc7
    Write-Host -NoNewline "$([char]27)]16162;A$([char]7)"
}

# Command tracking (OSC 16162 "C"): the console host calls this function
# instead of its built-in line reader when it is defined. We wrap PSReadLine
# (or the plain console reader) and publish the command line (base64 of the
# UTF-8 bytes) right before it executes.
function Global:PSConsoleHostReadLine {
    $__wave_line = $null
    if (Get-Module PSReadLine) {
        try {
            $__wave_line = [Microsoft.PowerShell.PSConsoleReadLine]::ReadLine($host.Runspace, $ExecutionContext)
        } catch {
            $__wave_line = $host.UI.ReadLine()
        }
    } else {
        $__wave_line = $host.UI.ReadLine()
    }
    if (-not (_waveterm_si_blocked) -and $null -ne $__wave_line -and $__wave_line.Trim().Length -gt 0) {
        try {
            $__wave_text = $__wave_line
            if ($__wave_text.Length -gt 8192) {
                $__wave_text = "# command too large ($($__wave_text.Length) bytes)"
            }
            $__wave_cmd64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($__wave_text))
            Write-Host -NoNewline "$([char]27)]16162;C;{`"cmd64`":`"$__wave_cmd64`"}$([char]7)"
            $Global:_WAVETERM_SI_RANCOMMAND = $true
        } catch {}
    }
    return $__wave_line
}

# Add the OSC 7 call to the prompt function
if (Test-Path Function:\prompt) {
    $global:_waveterm_original_prompt = $function:prompt
    function Global:prompt {
        _waveterm_si_prompt
        & $global:_waveterm_original_prompt
    }
} else {
    function Global:prompt {
        _waveterm_si_prompt
        "PS $($executionContext.SessionState.Path.CurrentLocation)$('>' * ($nestedPromptLevel + 1)) "
    }
}
