#!/bin/bash

# Ports to be killed
PORTS=("8000" "3000" "8001" "8002" "8003" "8004")

# Iterate over each port and kill the process using that port
for PORT in "${PORTS[@]}"; do
    # Find the PID of the process using the specified port
    PID=$(lsof -t -i:$PORT)

    if [ -n "$PID" ]; then
        # Kill the process using the PID
        echo "Killing process on port $PORT with PID $PID..."
        kill -9 $PID
        echo "Process on port $PORT killed."
    else
        echo "No process found using port $PORT."
    fi
done