#!/bin/sh
# Test double for git, driven by FAKEGIT_MODE.
case "$FAKEGIT_MODE" in
env)
	env
	;;
sleep)
	sleep 30 &
	echo $! > "$FAKEGIT_PIDFILE"
	sleep 30
	;;
fail)
	echo "fatal: boom" >&2
	exit 128
	;;
marker)
	touch "$FAKEGIT_MARKER"
	;;
bgout)
	sleep 30 &
	echo $! > "$FAKEGIT_PIDFILE"
	echo out
	;;
lock)
	# Like git, remove the lock file on SIGTERM.
	trap 'rm -f "$FAKEGIT_LOCK"; exit 143' TERM
	touch "$FAKEGIT_LOCK"
	sleep 30 &
	wait
	;;
ignoreterm)
	# The child inherits the ignored SIGTERM.
	trap '' TERM
	sleep 30 &
	echo $! > "$FAKEGIT_PIDFILE"
	wait
	;;
esac
