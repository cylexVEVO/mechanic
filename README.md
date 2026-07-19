# mechanic

`mechanic` turns ad-hoc commands into managed services. It supports systemd and OpenRC on Linux, launchd on macOS, and an optional interactive terminal UI.

## Installation

Download the archive for your operating system and architecture from the [latest GitHub release](https://github.com/cylexVEVO/mechanic/releases/latest). Each release includes Linux, macOS, and Windows builds for AMD64 and ARM64, plus a `checksums.txt` file.

On Linux or macOS, extract the archive and install the executable somewhere on your `PATH`:

```sh
tar -xzf mechanic_linux_amd64.tar.gz
sudo install -m 0755 mechanic /usr/local/bin/mechanic
```

Development builds are also available for 14 days from successful [GitHub Actions runs](https://github.com/cylexVEVO/mechanic/actions/workflows/build.yml).

User services are the default:

```sh
mechanic create worker 'python3 worker.py' --dir /path/to/app --start
mechanic list
mechanic tui
```

Use the global `--system` flag for system-wide services. System scope has its own registry and normally requires root:

```sh
sudo mechanic --system create worker '/usr/local/bin/worker' --start
sudo mechanic --system enable worker
sudo mechanic --system tui
```

On Linux, the active init system is detected automatically. Set `MECHANIC_INIT=systemd` or `MECHANIC_INIT=openrc` to override detection. OpenRC user services require OpenRC 0.60 or newer, an active OpenRC user session, and `XDG_RUNTIME_DIR`. For systemd user services that must survive logout, enable lingering with `loginctl enable-linger`.

The TUI refreshes status automatically and covers the service-management features of the CLI. Press `c` to create a service, `enter` for detailed status, or `l` for the auto-refreshing log viewer. Use arrow keys or `j`/`k` to select a service, then `s` to start, `x` to stop, `R` to restart, `e` to toggle boot enablement, or `d` to remove it.
