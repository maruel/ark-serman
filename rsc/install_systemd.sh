#!/bin/bash
# Copyright 2023 Marc-Antoine Ruel. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

set -eu

cd "$(dirname $0)"

mkdir -p $HOME/.config/systemd/user

go install ..

cat <<EOF > "$HOME/.config/systemd/user/ark-serman.service"
[Unit]
Description=ark-serman: ARK Server Manager
Wants=network-online.target
After=syslog.target network.target nss-lookup.target network-online.target

[Service]
ExecStart=%h/go/bin/ark-serman
ExecStop=/bin/kill -s INT \$MAINPID

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable ark-serman
systemctl --user start ark-serman
