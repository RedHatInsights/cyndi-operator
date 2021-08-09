#!/bin/bash
killall __debug_bin
killall dlv
#PORT_PID=$!
#echo $PORT_PID
#trap 'killall $(jobs -p)' EXIT
trap 'pkill -f "__DEBUG_BIN" && pkill -f "dlv"' EXIT
make delve-test
