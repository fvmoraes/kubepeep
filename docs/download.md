# Download, installation and removal

Find available versions on [GitHub Releases](https://github.com/fvmoraes/kubepeep/releases).
The [release workflow](../.github/workflows/release.yml) builds the packages
below. Latest links are convenient for discovery; use an explicit release tag
and checksums from that same release for reproducible installation.

## Packages

| Platform | Architecture | Desktop package |
| --- | --- | --- |
| Windows | x86_64 | [Setup EXE](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-windows-amd64-setup.exe) |
| Linux | x86_64 | [DEB](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-amd64.deb), [RPM](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-amd64.rpm) |
| Linux | ARM64 | [DEB](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-arm64.deb), [RPM](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-arm64.rpm) |
| macOS | Intel | [DMG](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-amd64.dmg) |
| macOS | Apple Silicon | [DMG](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-arm64.dmg) |

Linux packages install `/usr/bin/kubepeep`, the desktop entry and icons, and
declare GTK 3/WebKit2GTK 4.1 runtime dependencies. Windows setup embeds the
WebView2 bootstrapper. macOS packages contain `kubePeep.app`; install the app
in Applications. Desktop library and build details are in
[desktop-build.md](desktop-build.md).

CLI archives use `kubePeep` (`kubePeep.exe` on Windows):

| Platform | Asset name |
| --- | --- |
| Linux AMD64 / ARM64 | `kubepeep-linux-amd64.tar.gz` / `kubepeep-linux-arm64.tar.gz` |
| macOS Intel / Apple Silicon | `kubepeep-darwin-amd64.tar.gz` / `kubepeep-darwin-arm64.tar.gz` |
| Windows AMD64 | `kubepeep-windows-amd64.zip` |

Windows ARM64 is not in the current publication matrix. A successful
cross-build does not imply a published or natively validated package.

## Scripted installation

Select an existing release version. Current tags have no `v` prefix.
The scripts install into `~/.local/bin/kubePeep` on Unix or
`%LOCALAPPDATA%\Programs\kubePeep\kubePeep.exe` on Windows, without administrator
privileges. They download the matching archive, require SHA-256 and validate
the candidate before installation.

Linux or macOS:

```sh
version=0.2.2 # replace with the chosen existing release
curl --fail --location --proto '=https' --tlsv1.2 \
  "https://github.com/fvmoraes/kubepeep/releases/download/${version}/install.sh" \
  --output /tmp/kubepeep-install.sh
sh /tmp/kubepeep-install.sh --version "$version"
```

PowerShell:

```powershell
$Version = '0.2.2' # replace with the chosen existing release
$Installer = Join-Path $env:TEMP 'kubepeep-install.ps1'
$Uri = "https://github.com/fvmoraes/kubepeep/releases/download/$Version/install.ps1"
Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $Installer
& $Installer -Version $Version
```

## Verification and naming

Each release publishes `checksums.txt` and an SPDX SBOM. Download checksums
from the same immutable tag as the chosen asset, then verify the selected
filename and digest before running it. For example, in a directory containing
only the chosen release files:

```sh
sha256sum --ignore-missing -c checksums.txt
```

The command must report `OK` for the asset being installed. The checksum
checks integrity against the release manifest; it is not a code signature.

Assets have fixed names within each release, such as
`kubepeep-linux-amd64.deb`. Desktop installers also have versioned aliases,
such as `kubepeep-0.2.2-linux-amd64.deb`. Their download paths are
`/releases/download/<version>/<asset>`; `/releases/latest/download/<asset>`
is a moving link and is not a version pin.

## Update and removal

For scripted CLI installations, stop the web service and choose the target:

```sh
kubePeep stop
kubePeep update --version X.Y.Z
```

Update validates the checksum and candidate version, then replaces the binary
with rollback on failure. On Windows, replacement finishes after the running
update process exits. For system packages or app bundles, use the corresponding
package installer. Local data is preserved.

The script uninstallers remove their binary and owned PATH entry:

```sh
sh /tmp/kubepeep-install.sh --uninstall
```

```powershell
& $Installer -Uninstall
```

Deleting local data requires both purge and confirmation flags:

```sh
sh /tmp/kubepeep-install.sh --uninstall --purge-data --confirm-purge
```

```powershell
& $Installer -Uninstall -PurgeData -ConfirmPurge
```

Purge is restricted to the canonical KubePeep data root. It never removes
kubeconfig files. System packages should be removed through their package
manager; the script uninstallers only own scripted installations.
