# CI fixture timing only. Sourcing this file defines functions, with no I/O.

engine_warmup_now() {
  local engine_uptime
  read -r engine_uptime _ < /proc/uptime || return 1
  [[ $engine_uptime =~ ^([0-9]+)\.([0-9]{2})$ ]] || return 1
  printf '%s\n' "$((10#${BASH_REMATCH[1]} * 1000 + 10#${BASH_REMATCH[2]} * 10))"
}

engine_warmup_timing() {
  printf 'engine warm-up timing: duration_ms=%s outcome=%s\n' "$1" "$2"
}

# The clock and result stay outside timeout's process group. Measure through the
# child's removal verification, even when TERM or KILL ends that child. Shorter
# bounds let ordinary regressions exercise this same code with harmless commands.
engine_warmup_run() {
  local term=$1 kill=$2 bound=$3 start end duration=unknown outcome=failed
  local result=0 command_status=0
  shift 3
  if start=$(engine_warmup_now) && [[ $start =~ ^[0-9]+$ ]]; then
    timeout --signal=TERM --kill-after="$kill" "$term" "$@" || command_status=$?
    if end=$(engine_warmup_now) && [[ $end =~ ^[0-9]+$ ]] && (( end >= start )); then
      duration=$((end - start))
    else
      printf 'engine-fixture: warm-up timing: expected monotonic end >= start; observed unavailable or backwards clock\n' >&2
      result=1
    fi
    if (( command_status != 0 )); then
      result=$command_status
      case "$command_status" in
        124|137) printf 'engine-fixture: warm-up timeout: expected completion within %s; observed status=%s\n' "$bound" "$command_status" >&2 ;;
        *) printf 'engine-fixture: warm-up operation: expected status=0; observed status=%s\n' "$command_status" >&2 ;;
      esac
    fi
  else
    printf 'engine-fixture: warm-up timing: expected monotonic start; observed unavailable or invalid clock\n' >&2
    result=1
  fi
  if (( result == 0 )); then outcome=passed; fi
  engine_warmup_timing "$duration" "$outcome" || {
    local timing_status=$?
    printf 'engine-fixture: warm-up timing output: expected writable timing line; observed status=%s\n' "$timing_status" >&2
    return 1
  }
  return "$result"
}
