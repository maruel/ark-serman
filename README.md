# ark-serman

An Ark Dedicated Server Manager

It uses user local systemd file for low overhead server management.

## Installation

```
git clone https://github.com/maruel/ark-serman
cd ark-serman

# Not sure if really needed under systemd:
./rsc/increase_limits.sh

# Install both steamcmd and the Ark Dedicated Server:
./rsc/update_ark.sh

# Install ark-serman as a systemd service:
./rsc/install_systemd.sh
```

See https://developer.valvesoftware.com/wiki/SteamCMD for more information.

All the data ends up in `~/.local/share/Steam` and `~/.steam`.
