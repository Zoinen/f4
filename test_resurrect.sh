#!/bin/bash
# Rigorous test suite for f4's session resurrection feature.
set -e

# --- Test Configuration ---
F4_CMD="./f4_test_binary --no-plugins" # Disable plugins for clean UI state
TEST_DIR="/tmp/f4-test-$$"
SOCKET_PATH="$TEST_DIR/f4.sock"
DAEMON_LOG="$TEST_DIR/debug_daemon.log"

pass() { echo -e "\033[0;32mPASS: $1\033[0m"; }
fail() { 
    echo -e "\033[0;31mFAIL: $1\033[0m"
    echo -e "\033[1;33m--- Daemon Log (Tail) ---\033[0m"
    tail -n 50 "$DAEMON_LOG" || true
    exit 1 
}

cleanup() {
    echo "--- Cleaning up ---"
    pkill -f "f4_test_binary" || true
    rm -rf "$TEST_DIR"
    stty sane || true
}

trap cleanup EXIT INT TERM

run_in_pty() {
    local sock=$1
    local keys_hex=$2
    local timeout=$3
    
    # We use a slightly more complex python script to ensure session leader
    # and proper TTY association.
    python3 -c "
import os, pty, time, select, fcntl, termios, struct, signal

master, slave = pty.openpty()
pid = os.fork()

if pid == 0:
    os.close(master)
    # Make child the session leader and set controlling TTY
    os.setsid()
    fcntl.ioctl(slave, termios.TIOCSCTTY, 0)
    
    os.dup2(slave, 0); os.dup2(slave, 1); os.dup2(slave, 2)
    os.execv('./f4_test_binary', ['./f4_test_binary', '--client', '$sock'])
else:
    os.close(slave)
    # Wait for client to connect to daemon
    time.sleep(1.8)
    os.write(master, bytes.fromhex('$keys_hex'))
    
    start = time.time()
    while time.time() - start < $timeout:
        r, _, _ = select.select([master], [], [], 0.1)
        if master in r:
            try:
                if not os.read(master, 1024): break
            except OSError: break
    # Ensure client is dead
    try: os.kill(pid, signal.SIGTERM)
    except: pass
"
}

echo "Building f4..."
go build -o f4_test_binary ./cmd/f4

mkdir -p "$TEST_DIR"
touch "$DAEMON_LOG"

echo -e "\n===== TEST 1: Manual Backgrounding ====="
VTUI_DEBUG=1 $F4_CMD --server "$SOCKET_PATH" >> "$DAEMON_LOG" 2>&1 &
DAEMON_PID=$!

for i in {1..20}; do [ -S "$SOCKET_PATH" ] && break || sleep 0.2; done
[ -S "$SOCKET_PATH" ] || fail "Daemon failed to create socket."

echo "Client 1: Sending F9, Right, 'k' (Background)..."
# 1b5b32307e = F9, 1b5b43 = Right, 6b = k
run_in_pty "$SOCKET_PATH" "1b5b32307e1b5b436b" 5

sleep 1.5
ps -p $DAEMON_PID > /dev/null || fail "Daemon died after client 1 detached."
pass "Daemon survived clean detach."

echo "Client 2: Connecting and sending F10, Enter (Quit)..."
# 1b5b32317e = F10, 0d = Enter
run_in_pty "$SOCKET_PATH" "1b5b32317e0d" 5

sleep 1.5
if ps -p $DAEMON_PID > /dev/null; then
    fail "Daemon did not shut down after Ctrl+Q."
fi
pass "Test 1 successful."

echo -e "\n===== TEST 2: Terminal Close (SIGHUP) ====="
rm -f "$SOCKET_PATH"
VTUI_DEBUG=1 $F4_CMD --server "$SOCKET_PATH" >> "$DAEMON_LOG" 2>&1 &
DAEMON_PID=$!
for i in {1..20}; do [ -S "$SOCKET_PATH" ] && break || sleep 0.2; done

echo "Client 1: Connecting and killing TTY..."
python3 -c "
import os, pty, time, signal, fcntl, termios, struct
master, slave = pty.openpty()
size = struct.pack('HHHH', 24, 80, 0, 0)
fcntl.ioctl(slave, termios.TIOCSWINSZ, size)
pid = os.fork()
if pid == 0:
    os.close(master)
    os.dup2(slave, 0); os.dup2(slave, 1); os.dup2(slave, 2)
    os.execv('./f4_test_binary', ['./f4_test_binary', '--client', '$SOCKET_PATH'])
else:
    time.sleep(1.5)
    os.close(master)
    os.kill(pid, signal.SIGHUP)
" &
sleep 2.5

ps -p $DAEMON_PID > /dev/null || fail "Daemon died after SIGHUP."
pass "Daemon survived terminal close."

echo "Client 2: Final cleanup (F10, Enter)..."
run_in_pty "$SOCKET_PATH" "1b5b32317e0d" 5
if ps -p $DAEMON_PID > /dev/null; then
    fail "Daemon stayed alive after final Quit."
fi
pass "All resurrection tests passed."