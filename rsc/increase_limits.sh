#!/bin/bash
# Copyright 2023 Marc-Antoine Ruel. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

set -eu

echo "Increasing server limits"

echo "# Change done by https://github.com/maruel/ark-serman" | sudo tee --append /etc/sysctl.conf
echo "fs.file-max=100000" | sudo tee --append /etc/sysctl.conf
sudo sysctl -p /etc/sysctl.conf

echo "# Change done by https://github.com/maruel/ark-serman" | sudo tee --append /etc/security/limits.conf
echo "*               soft    nofile          1000000" | sudo tee --append /etc/security/limits.conf
echo "*               hard    nofile          1000000" | sudo tee --append /etc/security/limits.conf

echo "# Change done by https://github.com/maruel/ark-serman" | sudo tee --append /etc/pam.d/common-session
echo "session required pam_limits.so" | sudo tee --append /etc/pam.d/common-session

