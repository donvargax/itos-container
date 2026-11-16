# CI fixture timing only. Sourcing this file defines functions, with no I/O.

engine_warmup_now() {
  local engine_uptime
  read -r engine_uptime _ < /proc/uptime || return 1
  [[ $engine_uptime =~ ^([0-9]+)\.([0-9]{2})$ ]] || return 1
  printf '%s\n' "$((10#${BASH_REMATCH[1]} * 1000 + 10#${BASH_REMATCH[2]} * 10))"
}

engine_warmup_timing() {
  local label='engine warm-up timing'
  [[ ${engine_warmup_phase:-plain} != product ]] || label='engine product warm-up timing'
  printf '%s: duration_ms=%s outcome=%s\n' "$label" "$1" "$2"
}

# The clock and result stay outside timeout's process group. Measure through the
# child's removal verification, even when TERM or KILL ends that child. Shorter
# bounds let ordinary regressions exercise this same code with harmless commands.
engine_warmup_measure() {
  local term=$1 kill=$2 bound=$3 start end duration=unknown outcome=failed
  local result=0 command_status=0
  shift 3
  if start=$(engine_warmup_now) && [[ $start =~ ^[0-9]+$ ]]; then
    timeout --signal=TERM --kill-after="$kill" "$term" "$@" || command_status=$?
    if end=$(engine_warmup_now) && [[ $end =~ ^[0-9]+$ ]] && (( end >= start )); then
      duration=$((end - start))
    else
      printf 'engine-fixture: warm-up %s timing: expected monotonic end >= start; observed unavailable or backwards clock\n' "${engine_warmup_phase:-plain}" >&2
      result=1
    fi
    if (( command_status != 0 )); then
      result=$command_status
      case "$command_status" in
        124|137) printf 'engine-fixture: warm-up %s timeout: expected completion within %s; observed status=%s\n' "${engine_warmup_phase:-plain}" "$bound" "$command_status" >&2 ;;
        *) printf 'engine-fixture: warm-up %s operation: expected status=0; observed status=%s\n' "${engine_warmup_phase:-plain}" "$command_status" >&2 ;;
      esac
    fi
  else
    printf 'engine-fixture: warm-up %s timing: expected monotonic start; observed unavailable or invalid clock\n' "${engine_warmup_phase:-plain}" >&2
    result=1
  fi
  if (( result == 0 )); then outcome=passed; fi
  engine_warmup_timing "$duration" "$outcome" || {
    local timing_status=$?
    printf 'engine-fixture: warm-up %s timing output: expected writable timing line; observed status=%s\n' "${engine_warmup_phase:-plain}" "$timing_status" >&2
    return 1
  }
  return "$result"
}

# Explicit wrappers keep labels independent of the caller's environment while
# sharing the clock, timeout and failure path through Bash's local scope.
engine_warmup_run() {
  local engine_warmup_phase=plain
  engine_warmup_measure "$@"
}

engine_warmup_product_run() {
  local engine_warmup_phase=product
  engine_warmup_measure "$@"
}

engine_warmup_labels() {
  jq -cn --arg owner "$1" '{"io.itos.engine-fixture":$owner,
    "io.itos-container.intent":($owner+"-product"),
    "io.itos-container.project":($owner+"-project"),
    "io.itos-container.fingerprint":($owner+"-fingerprint")}'
}

engine_warmup_anchor_labels() {
  local labels
  labels=$(engine_warmup_labels "$1") || return 1
  jq -cn --argjson labels "$labels" --arg owner "$1" --arg device "$2" --arg inode "$3" \
    '$labels + {"io.itos-container.session":($owner+"-session"),
      "io.itos-container.device":$device, "io.itos-container.inode":$inode,
      "io.itos-container.kind":"checkout-anchor"}'
}

# Match the offline CreateRun wire body, including SDK zero-valued fields.
# The image inspection supplies defaults; HOME/TMPDIR are overlaid as in run.go.
engine_warmup_request() {
  local labels
  labels=$(engine_warmup_labels "$5") || return 1
  jq -cn --arg image "$1" --arg user "$2:$3" --arg volume "$4" \
    --argjson labels "$labels" --slurpfile inspected "$6" '
    {Hostname:"", Domainname:"", User:$user, AttachStdin:true, AttachStdout:true,
     AttachStderr:true, Tty:false, OpenStdin:true, StdinOnce:true,
     Env:((reduce ($inspected[0][0].Config.Env // [])[] as $entry ({};
       ($entry|index("=")) as $at | . + {($entry[0:$at]):$entry[$at+1:]})
       + {HOME:"/tmp", TMPDIR:"/tmp"}) | to_entries | sort_by(.key) | map(.key+"="+.value)),
     Cmd:["/bin/true"], Healthcheck:{Test:["NONE"]}, Image:$image, Volumes:null,
     WorkingDir:"/workspace", Entrypoint:[], Labels:$labels,
     HostConfig:{Binds:null, ContainerIDFile:"", LogConfig:{Type:"none", Config:null},
       NetworkMode:"none", PortBindings:null, RestartPolicy:{Name:"no", MaximumRetryCount:0},
       AutoRemove:false, VolumeDriver:"", VolumesFrom:null, ConsoleSize:[0,0],
       CapAdd:null, CapDrop:["ALL"], CgroupnsMode:"private", Dns:null, DnsOptions:null,
       DnsSearch:null, ExtraHosts:null, GroupAdd:null, IpcMode:"private", Cgroup:"",
       Links:null, OomScoreAdj:0, PidMode:"", Privileged:false, PublishAllPorts:false,
       ReadonlyRootfs:true, SecurityOpt:["no-new-privileges"],
       Tmpfs:{"/tmp":"rw,nosuid,nodev,mode=1777"}, UTSMode:"", UsernsMode:"", ShmSize:0,
       Isolation:"", CpuShares:0, Memory:0, NanoCpus:0, CgroupParent:"", BlkioWeight:0,
       BlkioWeightDevice:null, BlkioDeviceReadBps:null, BlkioDeviceWriteBps:null,
       BlkioDeviceReadIOps:null, BlkioDeviceWriteIOps:null, CpuPeriod:0, CpuQuota:0,
       CpuRealtimePeriod:0, CpuRealtimeRuntime:0, CpusetCpus:"", CpusetMems:"", Devices:null,
       DeviceCgroupRules:null, DeviceRequests:null, MemoryReservation:0, MemorySwap:0,
       MemorySwappiness:null, OomKillDisable:null, PidsLimit:4096, Ulimits:null, CpuCount:0,
       CpuPercent:0, IOMaximumIOps:0, IOMaximumBandwidth:0,
       Mounts:[{Type:"volume", Source:$volume, Target:"/workspace", VolumeOptions:{NoCopy:true}}],
       MaskedPaths:null, ReadonlyPaths:null, Init:false}}'
}

engine_warmup_response() {
  [[ $1 == 201 ]] || return 1
  jq -ser 'select(length == 1) | .[0].Id | select(type == "string" and test("^[0-9a-f]{64}$"))' "$2"
}

# Data-only predicates are shared with ordinary regressions. Do not broaden the
# registry/smoke/plain tmpfs-only predicate to accommodate the product volume.
engine_warmup_container_owned() {
  local labels
  labels=$(engine_warmup_labels "$4") || return 1
  jq -e --arg name "/$2" --arg image "$3" --argjson labels "$labels" --arg volume "$5" '
    length == 1 and .[0].Name == $name and .[0].Image == $image and
    .[0].Config.Labels == $labels and
    .[0].HostConfig.Mounts == [{Type:"volume",Source:$volume,Target:"/workspace",VolumeOptions:{NoCopy:true}}] and
    .[0].HostConfig.Tmpfs == {"/tmp":"rw,nosuid,nodev,mode=1777"} and
    ([.[0].Mounts[]? | select(.Type == "volume" and .Name == $volume and .Driver == "local" and
      .Destination == "/workspace" and .RW == true and (.Propagation == "" or .Propagation == "rprivate"))] | length == 1) and
    ([.[0].Mounts[]? | select(.Type == "tmpfs" and .Destination == "/tmp")] | length <= 1) and
    all(.[0].Mounts[]?; (.Type == "volume" and .Name == $volume and .Driver == "local" and
      .Destination == "/workspace" and .RW == true and (.Propagation == "" or .Propagation == "rprivate")) or
      (.Type == "tmpfs" and .Destination == "/tmp"))' "$1" >/dev/null
}

engine_warmup_volume_owned() {
  local labels
  labels=$(engine_warmup_anchor_labels "$3" "$5" "$6") || return 1
  jq -e --arg name "$2" --arg source "$4" --argjson labels "$labels" '
    length == 1 and .[0].Name == $name and .[0].Driver == "local" and .[0].Scope == "local" and
    .[0].Labels == $labels and .[0].Options == {type:"none",o:"bind",device:$source}' "$1" >/dev/null
}

engine_warmup_absence() {
  local kind=$1 name=$2 message
  message=$(cat "$3") || return 1
  case "$kind:$message" in
    "container:Error: No such container: $name"|"container:Error response from daemon: No such container: $name"|"container:Error: No such object: $name"|\
    "volume:Error: No such volume: $name"|"volume:Error response from daemon: get $name: no such volume"|"volume:Error response from daemon: No such volume: $name") return 0 ;;
    *) return 1 ;;
  esac
}
