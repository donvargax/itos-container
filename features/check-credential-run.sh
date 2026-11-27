#!/bin/sh

# Match explicit PASS lines because go test succeeds when no tests match.
out=$(go test ./features -run '^(TestEngineShards|TestCredentialRunPolicyEditBracket|TestCredentialRunNativeStepsDefined)$' -v 2>&1)
status=$?
printf '%s\n' "$out" | tail -n 40
[ "$status" -eq 0 ] &&
    printf '%s\n' "$out" | grep -qF -- '--- PASS: TestEngineShards/credentials-run_red_inventory (' &&
    printf '%s\n' "$out" | grep -qF -- '--- PASS: TestCredentialRunPolicyEditBracket (' &&
    printf '%s\n' "$out" | grep -qF -- '--- PASS: TestCredentialRunNativeStepsDefined ('
