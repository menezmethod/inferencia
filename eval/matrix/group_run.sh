#!/usr/bin/env bash
# group_run.sh <seconds> <cmd...>: run cmd in its own process group. On timeout
# or when it exits, kill the whole group, so no MCP server, worker or test
# process outlives it. Exits with cmd's status (142 after a timeout).
exec perl -e '
  my $t = shift; my $pid = fork; die "fork: $!" unless defined $pid;
  if (!$pid) { setpgrp(0, 0); exec @ARGV or die "exec: $!" }
  my $timed_out = 0;
  local $SIG{ALRM} = sub { $timed_out = 1; kill "TERM", -$pid; sleep 5; kill "KILL", -$pid };
  alarm $t; waitpid($pid, 0); my $rc = $? >> 8; alarm 0;
  kill "KILL", -$pid;
  exit($timed_out ? 142 : $rc);
' "$@"
