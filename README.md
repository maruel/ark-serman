# ark-serman

An Ark Dedicated Server Manager

It uses user local systemd file for low overhead server management.

## Installation

Run `./rsc/increase_limits.sh`

(Not sure if really needed under systemd)

Run `./rsc/update_ark.sh` to install both steamcmd and the Ark Dedicated Server.
See https://developer.valvesoftware.com/wiki/SteamCMD for more information.

All the data ends up in `~/.local/share/Steam` and `~/.steam`.
