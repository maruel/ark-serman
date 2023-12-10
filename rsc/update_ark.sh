#!/bin/bash
# Copyright 2023 Marc-Antoine Ruel. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

set -eu

if [ ! -f /usr/games/steamcmd ]; then
  sudo apt install steamcmd
fi

/usr/games/steamcmd \
  +login anonymous \
  +app_update 376030 \
  +quit
