#!/bin/bash

while true; do
    echo "[$(date '+%H:%M:%S')] Connecting to SSE..."
    curl -N -u "pinball:ilovepinball" "http://localhost:8090/api/events" 2>/dev/null | while read line; do
        echo "[$(date '+%H:%M:%S')] $line"
    done
    echo "[$(date '+%H:%M:%S')] Connection dropped, reconnecting in 2s..."
    sleep 2
done