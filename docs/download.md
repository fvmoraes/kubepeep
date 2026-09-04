# Download KubePeep

All official assets are published on
[GitHub Releases](https://github.com/fvmoraes/kubepeep/releases). The links
below are **permanent**: they always deliver the latest stable release without
requiring any documentation update.

Latest release page:
<https://github.com/fvmoraes/kubepeep/releases/latest>

## Windows 10 / 11 (x86_64)

Installer (recommended — embeds the WebView2 bootstrapper):

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-windows-amd64-setup.exe>

Portable archive (raw `kubePeep.exe`, used by `install.ps1`):

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-windows-amd64.zip>

Notes:

- Requires Windows 10 or 11, 64-bit.
- Builds are not code-signed yet: SmartScreen may ask for confirmation on the
  first run. Code signing will be added once certificates are provisioned.

## Linux — Debian / Ubuntu (DEB)

x86_64 (AMD64):

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-amd64.deb>

ARM64:

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-arm64.deb>

Installs `kubePeep` into `/usr/bin`, registers the desktop entry
(`kubepeep.desktop`) and the hicolor icon, and declares the Wails runtime
dependencies (`libgtk-3`, `libwebkit2gtk-4.1`) so the package installs and
uninstalls cleanly with `apt`/`dpkg`.

## Linux — Fedora / RHEL / Rocky / AlmaLinux (RPM)

x86_64 (AMD64):

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-amd64.rpm>

ARM64:

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-arm64.rpm>

Same layout and dependencies as the DEB (`gtk3`, `webkit2gtk4.1`), installable
with `dnf`/`zypper`/`rpm`.

## Linux / macOS — scripted CLI install

The installers download assets only from the immutable release tag, verify the
published SHA-256 checksum, and install without administrator privileges:

```sh
version=0.1.0
curl --fail --location --proto '=https' --tlsv1.2 \
  "https://github.com/fvmoraes/kubepeep/releases/download/${version}/install.sh" \
  --output /tmp/kubepeep-install.sh
sh /tmp/kubepeep-install.sh --version "$version"
```

```powershell
$Version = '0.1.0'
$Installer = Join-Path $env:TEMP 'kubepeep-install.ps1'
Invoke-WebRequest -UseBasicParsing \
  -Uri "https://github.com/fvmoraes/kubepeep/releases/download/$Version/install.ps1" \
  -OutFile $Installer
& $Installer -Version $Version
```

CLI archives (raw `kubePeep` binary + documentation), used by the script above
or for manual installs:

| Platform | Architecture | Asset |
|----------|--------------|-------|
| Linux | x86_64 | [kubepeep-linux-amd64.tar.gz](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-amd64.tar.gz) |
| Linux | ARM64 | [kubepeep-linux-arm64.tar.gz](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-arm64.tar.gz) |
| macOS | x86_64 | [kubepeep-darwin-amd64.tar.gz](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-amd64.tar.gz) |
| macOS | ARM64 | [kubepeep-darwin-arm64.tar.gz](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-arm64.tar.gz) |

## macOS — Intel (x86_64)

DMG (recommended):

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-amd64.dmg>

ZIP (`.app`):

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-amd64.zip>

## macOS — Apple Silicon (ARM64, M1/M2/M3/M4+)

DMG (recommended):

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-arm64.dmg>

ZIP (`.app`):

<https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-arm64.zip>

Notes:

- Builds are not signed/notarized yet: on first launch, right-click the app and
  choose **Open**, or allow it in *System Settings → Privacy & Security*.
- Drag `KubePeep.app` (bundle name `kubePeep.app`) into `/Applications`.

## Verification

Every release publishes `checksums.txt` with SHA-256 entries for all assets:

<https://github.com/fvmoraes/kubepeep/releases/latest/download/checksums.txt>

```sh
sha256sum --ignore-missing -c checksums.txt
```

Every release also publishes an SPDX SBOM
(`kubepeep-<version>.spdx.json`) generated from the release source tree.

## Versioned asset names

Alongside the fixed-name files above, every release publishes the same
installers with the release version in the name (for archiving and
version-pinned automation):

```text
kubepeep-<version>-windows-amd64-setup.exe
kubepeep-<version>-linux-amd64.deb
kubepeep-<version>-linux-arm64.deb
kubepeep-<version>-linux-amd64.rpm
kubepeep-<version>-linux-arm64.rpm
kubepeep-<version>-darwin-amd64.dmg
kubepeep-<version>-darwin-arm64.dmg
kubepeep-<version>-darwin-amd64.zip
kubepeep-<version>-darwin-arm64.zip
```

Download a versioned asset at
`https://github.com/fvmoraes/kubepeep/releases/download/<version>/kubepeep-<version>-...`.

## Integration notes (kubepeep.online)

The official website consumes the same permanent endpoints:

```text
https://github.com/fvmoraes/kubepeep/releases/latest
https://github.com/fvmoraes/kubepeep/releases/latest/download/checksums.txt
https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-windows-amd64-setup.exe
https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-amd64.deb
https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-amd64.rpm
https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-arm64.deb
https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-arm64.rpm
https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-amd64.dmg
https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-arm64.dmg
```

These URLs never change between releases, so download buttons on the site do
not need maintenance.
