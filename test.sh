#!/bin/bash

# test.sh - Test script for Prism cluster with tmux
# Creates a 3-tab tmux session to test cluster functionality

SESSION_NAME="prism-test"

# Check if tmux session already exists
if tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
    echo "Tmux session '$SESSION_NAME' already exists. Killing it first..."
    tmux kill-session -t "$SESSION_NAME"
fi

echo "Creating new tmux session: $SESSION_NAME"

# Create new tmux session with first window (control node)
tmux new-session -d -s "$SESSION_NAME" -n "control1"

# Send command to first window (control node)
tmux send-keys -t "$SESSION_NAME:control1" \
    "go run cmd/prismd/main.go --name=control1 --role=control --bind=127.0.0.1:4200" C-m

# Create second window (agent node that joins the control)
tmux new-window -t "$SESSION_NAME" -n "agent1"
tmux send-keys -t "$SESSION_NAME:agent1" \
    "sleep 3 && go run cmd/prismd/main.go --bind=127.0.0.1:4300 --name=agent1 --join=127.0.0.1:4200" C-m

# Create third window (prismctl client)
tmux new-window -t "$SESSION_NAME" -n "prismctl"
tmux send-keys -t "$SESSION_NAME:prismctl" \
    "sleep 5 && go run cmd/prismctl/main.go members" C-m

# Switch to first window
tmux select-window -t "$SESSION_NAME:control1"

# Attach to the session
echo "Attaching to tmux session. Use Ctrl-b + number to switch between tabs:"
echo "  Tab 0: control1 (control node on :4200)"
echo "  Tab 1: agent1 (agent node on :4300, joins :4200)"
echo "  Tab 2: prismctl (members command)"
echo ""
echo "To exit: Ctrl-b + d (detach) or exit each process and tmux kill-session -t $SESSION_NAME"

tmux attach-session -t "$SESSION_NAME"