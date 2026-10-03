# Installs tars (and optionally the tars-desktop app) on Windows from a
# GitHub release.
#
#   irm https://raw.githubusercontent.com/devlikebear/tars/main/install.ps1 | iex
#
# With options:
#
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/devlikebear/tars/main/install.ps1))) -Desktop -StartAtLogin
#
# Everything lands in one folder (default %LOCALAPPDATA%\Programs\TARS), so
# the desktop app finds tars.exe next to itself. No administrator rights are
# needed: the folder goes on the user's PATH, not the machine's. Re-running
# the script updates an existing install, even while tars or tars-desktop is
# running (restart them afterwards to use the new version).
#
# Every archive is checked against the release's checksums.txt before
# anything is installed.
param(
  # Release to install, such as 0.44.0. Defaults to the latest release.
  [string]$Version = $env:TARS_VERSION,
  # Folder to install into.
  [string]$InstallDir = $(if ($env:TARS_INSTALL_DIR) { $env:TARS_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\TARS' }),
  # Also install the tars-desktop app.
  [switch]$Desktop,
  # Start tars-desktop (hidden, in the tray) when you sign in. Needs -Desktop.
  [switch]$StartAtLogin,
  # Leave the user PATH alone.
  [switch]$NoPath,
  # Install from local files instead of downloading (used by CI). Both are
  # needed; -Version must name the archive's release.
  [string]$ArchivePath,
  [string]$ChecksumsPath
)

# The body runs in a child scope so that, under `irm | iex`, it neither
# changes the caller's preferences nor closes their shell: errors throw
# instead of calling exit.
& {
  $ErrorActionPreference = 'Stop'
  # Windows PowerShell 5.1 draws a progress bar per downloaded chunk, which
  # makes large downloads many times slower.
  $ProgressPreference = 'SilentlyContinue'
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

  $repo = 'devlikebear/tars'
  # Release archives are amd64 only; Windows on Arm runs them emulated.
  $arch = 'amd64'

  function Get-LatestVersion {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ 'User-Agent' = 'tars-install' }
    $tag = [string]$release.tag_name
    if (-not $tag.StartsWith('v')) {
      throw "could not read the latest release tag (got '$tag')"
    }
    return $tag.Substring(1)
  }

  function Get-Asset([string]$name, [string]$dir) {
    $path = Join-Path $dir $name
    $url = "https://github.com/$repo/releases/download/v$resolvedVersion/$name"
    try {
      Invoke-WebRequest -Uri $url -OutFile $path -UseBasicParsing -Headers @{ 'User-Agent' = 'tars-install' }
    } catch {
      throw "download failed: $url ($($_.Exception.Message))"
    }
    return $path
  }

  function Assert-Checksum([string]$archive, [string]$checksums) {
    $name = Split-Path $archive -Leaf
    $expected = $null
    foreach ($line in Get-Content -LiteralPath $checksums) {
      $fields = $line.Trim() -split '\s+'
      if ($fields.Count -eq 2 -and $fields[1].TrimStart('*') -eq $name) {
        $expected = $fields[0].ToLowerInvariant()
      }
    }
    if (-not $expected) {
      throw "checksums.txt has no entry for $name"
    }
    $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
      throw "checksum mismatch for ${name}: expected $expected, got $actual"
    }
  }

  # Install-File puts $source at $target. A running executable cannot be
  # overwritten on Windows but can be renamed, so an existing file is moved
  # aside to <name>.old first; the .old copy is removed on the next run.
  function Install-File([string]$source, [string]$target) {
    $old = "$target.old"
    if (Test-Path -LiteralPath $old) {
      Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $target) {
      if (Test-Path -LiteralPath $old) {
        throw "cannot replace ${target}: $old is still in use; close tars and tars-desktop and try again"
      }
      Move-Item -LiteralPath $target -Destination $old -Force
    }
    Copy-Item -LiteralPath $source -Destination $target -Force
  }

  # Join-PathEntry returns $path with $dir appended, or $null when $path
  # already lists it. Entries are compared without a trailing backslash and
  # with environment variables expanded, so %LOCALAPPDATA%\Programs\TARS
  # counts as the folder; the result keeps the other entries as written.
  function Join-PathEntry([string]$path, [string]$dir) {
    $want = $dir.TrimEnd('\')
    $entries = @()
    if ($path) {
      $entries = @($path -split ';' | Where-Object { $_ })
    }
    foreach ($entry in $entries) {
      if ([Environment]::ExpandEnvironmentVariables($entry).TrimEnd('\') -ieq $want) {
        return $null
      }
    }
    return (@($entries) + $dir) -join ';'
  }

  function Expand-Release([string]$archive, [string]$dir) {
    $out = Join-Path $dir ([IO.Path]::GetFileNameWithoutExtension($archive))
    Expand-Archive -LiteralPath $archive -DestinationPath $out -Force
    return $out
  }

  if ($StartAtLogin -and -not $Desktop) {
    throw '-StartAtLogin needs -Desktop'
  }
  if ([bool]$ArchivePath -ne [bool]$ChecksumsPath) {
    throw '-ArchivePath and -ChecksumsPath go together'
  }
  if ($ArchivePath -and $Desktop) {
    throw '-Desktop cannot be combined with -ArchivePath'
  }

  $resolvedVersion = ([string]$Version).Trim().TrimStart('v')
  if (-not $resolvedVersion) {
    if ($ArchivePath) {
      throw '-ArchivePath needs -Version'
    }
    $resolvedVersion = Get-LatestVersion
  }

  $work = Join-Path ([IO.Path]::GetTempPath()) ("tars-install-" + [guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $work | Out-Null
  try {
    $archiveName = "tars_$($resolvedVersion)_windows_$arch.zip"
    if ($ArchivePath) {
      $archive = Join-Path $work $archiveName
      Copy-Item -LiteralPath $ArchivePath -Destination $archive
      $checksums = $ChecksumsPath
    } else {
      $checksums = Get-Asset 'checksums.txt' $work
      $archive = Get-Asset $archiveName $work
    }
    Assert-Checksum $archive $checksums

    $desktopArchive = $null
    if ($Desktop) {
      $desktopArchive = Get-Asset "tars-desktop_$($resolvedVersion)_windows_$arch.zip" $work
      Assert-Checksum $desktopArchive $checksums
    }

    $extracted = Expand-Release $archive $work
    $exe = Join-Path $extracted 'tars.exe'
    if (-not (Test-Path -LiteralPath $exe)) {
      throw "$archiveName has no tars.exe"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Install-File $exe (Join-Path $InstallDir 'tars.exe')
    $share = Join-Path $extracted 'share'
    if (Test-Path -LiteralPath $share) {
      $targetShare = Join-Path $InstallDir 'share'
      if (Test-Path -LiteralPath $targetShare) {
        Remove-Item -LiteralPath $targetShare -Recurse -Force
      }
      Copy-Item -LiteralPath $share -Destination $targetShare -Recurse
    }

    if ($desktopArchive) {
      $desktopExe = Join-Path (Expand-Release $desktopArchive $work) 'tars-desktop.exe'
      if (-not (Test-Path -LiteralPath $desktopExe)) {
        throw 'the tars-desktop archive has no tars-desktop.exe'
      }
      Install-File $desktopExe (Join-Path $InstallDir 'tars-desktop.exe')
    }
  } finally {
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
  }

  $installed = Join-Path $InstallDir 'tars.exe'
  $reported = & $installed --version
  if ($LASTEXITCODE -ne 0 -or -not ("$reported" -like "tars $($resolvedVersion) *")) {
    throw "installed tars.exe failed its version check: $reported"
  }

  if (-not $NoPath) {
    # Read and write the raw registry value: the user PATH is usually
    # REG_EXPAND_SZ with entries like %USERPROFILE%\..., which
    # [Environment]::GetEnvironmentVariable expands and SetEnvironmentVariable
    # would write back expanded, as a plain string.
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
      $raw = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
      $updated = Join-PathEntry $raw $InstallDir
      if ($null -ne $updated) {
        $key.SetValue('Path', $updated, [Microsoft.Win32.RegistryValueKind]::ExpandString)
        # Writing the registry does not tell running programs; setting and
        # clearing a variable through .NET broadcasts the change, so a
        # terminal opened from Explorer afterwards sees the new PATH.
        [Environment]::SetEnvironmentVariable('TARS_INSTALL_PATH_REFRESH', '1', 'User')
        [Environment]::SetEnvironmentVariable('TARS_INSTALL_PATH_REFRESH', $null, 'User')
        Write-Host "Added $InstallDir to your PATH. Open a new terminal to use 'tars'."
      }
    } finally {
      $key.Close()
    }
    if ($null -ne (Join-PathEntry $env:Path $InstallDir)) {
      $env:Path = "$env:Path;$InstallDir"
    }
  }

  if ($StartAtLogin) {
    $shortcut = Join-Path ([Environment]::GetFolderPath('Startup')) 'TARS.lnk'
    $shell = New-Object -ComObject WScript.Shell
    $link = $shell.CreateShortcut($shortcut)
    $link.TargetPath = Join-Path $InstallDir 'tars-desktop.exe'
    $link.Arguments = '--hidden'
    $link.WorkingDirectory = $InstallDir
    $link.Save()
    Write-Host "tars-desktop will start in the tray when you sign in ($shortcut)."
  }

  Write-Host "Installed $reported to $InstallDir"
  if ($Desktop) {
    Write-Host "Start the app with: & '$(Join-Path $InstallDir 'tars-desktop.exe')'"
  } else {
    Write-Host 'Next: tars init   (or install the app too with -Desktop)'
  }
}
