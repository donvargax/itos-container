# Shared fixture validation only. Sourcing this file performs no I/O or engine work.

fail() { printf 'engine-fixture: %s\n' "$*" >&2; exit 1; }

digest_valid() { [[ $1 =~ ^sha256:[0-9a-f]{64}$ ]]; }

# Bound before escaping, never dump arbitrary image configuration or raw output.
fixture_value() { printf '%q' "${1:0:160}"; }

manifest_observation() {
  local manifest=$1 built=$2 advertised=$3 measured=$4 summary invalid=0
  if ! summary=$(jq -asc --arg built "$built" --arg advertised "$advertised" --arg measured "$measured" '
    def bounded: if type == "string" then .[0:160]
      elif type == "object" or type == "array" then "<" + type + ">" else . end;
    def field($key): if type != "object" then "<non-object>"
      elif has($key) then .[$key] | bounded else "<absent>" end;
    if length != 1 then error("expected one JSON value") else .[0] end |
    {schemaVersion: field("schemaVersion"), mediaType: field("mediaType"),
     hasManifests: (if type == "object" then has("manifests") else "<non-object>" end),
     configDigest: (if type == "object" and has("config") then .config | field("digest") else "<absent>" end),
     builtID: ($built | bounded), advertisedDigest: ($advertised | bounded), measuredDigest: ($measured | bounded)}
    ' "$manifest" 2>/dev/null); then
    invalid=1
    summary=$(jq -acn --arg built "${built:0:160}" --arg advertised "${advertised:0:160}" --arg measured "${measured:0:160}" \
      '{schemaVersion:"<invalid JSON>",mediaType:"<invalid JSON>",hasManifests:"<invalid JSON>",configDigest:"<invalid JSON>",builtID:$built,advertisedDigest:$advertised,measuredDigest:$measured}') || fail 'manifest observation rendering: expected escaped summary; observed jq failure'
  fi
  printf 'manifest observation: %s\n' "$summary" >&2 || fail 'manifest observation output: expected writable diagnostics; observed write failure'
  (( invalid == 0 )) || fail 'manifest JSON: expected exactly one JSON value; observed invalid JSON (or unreadable response)'
}

# Preserve the failing read/hash stage even if its diagnostic JSON cannot render.
fail_manifest_read() {
  local stage=$1 manifest=$2 built=$3 advertised=$4 status=$5 summary_status=0
  (manifest_observation "$manifest" "$built" "$advertised" '<unavailable>') || summary_status=$?
  fail "$stage: expected readable captured response; observed status=$status summaryStatus=$summary_status"
}

validate_manifest() {
  local manifest=$1 headers=$2 built=$3 header_data line digest='' header_count=0 actual
  header_data=$(cat -- "$headers") || fail_manifest_read 'manifest header read' "$manifest" "$built" '<unavailable>' "$?"
  while IFS= read -r line; do
    line=${line%$'\r'}
    if [[ ${line,,} == docker-content-digest:* ]]; then
      digest=${line#*: }
      header_count=$((header_count + 1))
    fi
  done <<<"$header_data"
  actual=$(sha256sum -- "$manifest") || fail_manifest_read 'manifest byte hashing' "$manifest" "$built" "$digest" "$?"
  actual="sha256:${actual%% *}"
  manifest_observation "$manifest" "$built" "$digest" "$actual" || fail 'manifest observation: expected validated summary; observed failure'
  [[ $header_count == 1 ]] && digest_valid "$digest" || fail "manifest header: expected one sha256 digest; observed count=$header_count digest=$(fixture_value "$digest")"
  digest_valid "$actual" || fail "manifest byte hashing: expected sha256 digest; observed=$(fixture_value "$actual")"
  [[ $actual == "$digest" ]] || fail "manifest bytes: expected advertised=$digest; observed measured=$actual"
  digest_valid "$built" || fail "built identity: expected config sha256; observed=$(fixture_value "$built")"
  [[ $digest != "$built" ]] || fail "manifest identity: expected distinct manifest/config IDs; observed manifest=$digest builtID=$built"
  local predicate='length == 1 and (.[0] | type == "object" and .schemaVersion == 2 and
    (.mediaType == "application/vnd.docker.distribution.manifest.v2+json" or
     .mediaType == "application/vnd.oci.image.manifest.v1+json") and
    (has("manifests") | not) and (.config | type == "object" and .digest == $id))'
  jq -e -s --arg id "$built" "$predicate" "$manifest" >/dev/null 2>&1 || fail 'manifest validation: expected schemaVersion=2, single-image mediaType, hasManifests=false, configDigest=builtID; observed fields in manifest observation'
  printf '%s\n' "$digest" || fail 'manifest digest output: expected writable output; observed write failure'
}

validate_cached() {
  local file=$1 reference=$2 built=$3 fixture_uid=$4 fixture_gid=$5 observed
  digest_valid "$built" || fail "cached identity: expected config sha256; observed=$(fixture_value "$built")"
  [[ $fixture_uid =~ ^[1-9][0-9]*$ && $fixture_gid =~ ^[0-9]+$ ]] || fail "cached identity: expected numeric non-root UID/GID; observed=$(fixture_value "$fixture_uid:$fixture_gid")"
  local predicate='length == 1 and (.[0] | type == "array" and length == 1 and
    (.[0] | .Id == $id and (.RepoDigests | type == "array" and index($ref) != null) and
    .Os == "linux" and .Architecture == "amd64" and .Config.User == $user and
    ((if .Config.Entrypoint == null then [] else .Config.Entrypoint end) | type == "array" and length == 0) and
    ((if .Config.Volumes == null then {} else .Config.Volumes end) | type == "object" and length == 0) and
    ((if .Config.Labels == null then {} else .Config.Labels end) | type == "object" and (has("devcontainer.metadata") | not))))'
  # Select only fixture identity/metadata verdicts, never the rest of Config.
  if ! observed=$(jq -asc --arg ref "$reference" '
    def value: if type == "string" then .[0:160] elif type == "object" or type == "array" then "<"+type+">" else . end;
    if length != 1 or (.[0] | type != "array") or (.[0] | length != 1) then error("inspection shape") else .[0][0] end |
    {id:(.Id|value), hasReference:((.RepoDigests|index($ref)) != null), os:(.Os|value), architecture:(.Architecture|value),
     user:(.Config.User|value), entrypointEmpty:((if .Config.Entrypoint == null then [] else .Config.Entrypoint end) == []), volumesEmpty:((if .Config.Volumes == null then {} else .Config.Volumes end) == {}),
     hasDevcontainerMetadata:((if .Config.Labels == null then {} else .Config.Labels end)|has("devcontainer.metadata"))}' "$file" 2>/dev/null); then
    observed='<invalid inspection JSON or field type>'
  fi
  jq -e -s --arg ref "$reference" --arg id "$built" --arg user "$fixture_uid:$fixture_gid" "$predicate" "$file" >/dev/null 2>&1 || fail "cached validation: expected id=$built ref=$(fixture_value "$reference") linux/amd64 user=$fixture_uid:$fixture_gid empty entrypoint/volumes and no devcontainer metadata; observed=$observed"
}

validate_smoke() {
  local file=$1 output=$2 fixture_uid=$3 fixture_gid=$4 observed expected
  local predicate='length == 1 and (.[0] | type == "array" and length == 1 and
    (.[0].State | .Status == "exited" and .ExitCode == 0 and .OOMKilled == false and .Error == ""))'
  if ! observed=$(jq -asc '
    def value: if type == "string" then .[0:160] elif type == "object" or type == "array" then "<"+type+">" else . end;
    if length != 1 or (.[0] | type != "array") or (.[0] | length != 1) then error("inspection shape") else .[0][0].State end |
    {status:(.Status|value), exitCode:(.ExitCode|value), oomKilled:(.OOMKilled|value), error:(.Error|value)}' "$file" 2>/dev/null); then
    observed='<invalid inspection JSON or field type>'
  fi
  jq -e -s "$predicate" "$file" >/dev/null 2>&1 || fail "smoke state: expected exited exitCode=0 oomKilled=false error=empty; observed=$observed"
  observed=$(cat -- "$output") || fail "smoke output read: expected captured stdout; observed status=$?"
  printf -v expected 'uid=%s gid=%s\nT-20-engine-fixture-ok' "$fixture_uid" "$fixture_gid" || fail 'smoke expectation: expected formatted identity; observed printf failure'
  [[ $observed == "$expected" ]] || fail "smoke output: expected=$(fixture_value "$expected"); observed=$(fixture_value "$observed")"
}
