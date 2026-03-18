#!/bin/sh
python3 /app/control_server.py &
exec rnsd
