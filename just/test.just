# Tests.

v_test_procs := env("TEST_PROCS", "2")
v_test_timeout := env("TEST_TIMEOUT", "10m")

# Run tests with coverage.
[doc("Run all tests, then report coverage")]
test folder="./..." *args:
    mkdir -p work/logs
    export PROJECT_ROOT={{ justfile_directory() }}; \
    export DATE=`date +%Y%m%d-%H%M%S`; \
    status=0; \
    gotestsum --hide-summary=skipped --format testname --jsonfile=work/logs/${DATE}.json --packages={{ folder }} \
      --post-run-command "bash -c 'echo; echo Slowest tests; gotestsum tool slowest --num 10 --jsonfile work/logs/${DATE}.json'" \
      -- -race -parallel {{ v_test_procs }} -timeout {{ v_test_timeout }} -cover -coverpkg=./... -covermode atomic -coverprofile=work/logs/${DATE}-cover.out -v "${@:2}" || status=$?; \
    just cover work/logs/${DATE}-cover.out; \
    exit ${status}


# Per-package statement coverage.
[doc("Per-package and total statement coverage of a run")]
cover profile="":
    #!/usr/bin/env bash
    set -euo pipefail

    profile="{{ profile }}"
    if [[ -z "${profile}" ]]; then
      for candidate in work/logs/*-cover.out; do
        [[ -f "${candidate}" ]] || continue
        [[ -z "${profile}" || "${candidate}" -nt "${profile}" ]] && profile="${candidate}"
      done
    fi

    if [[ -z "${profile}" || ! -f "${profile}" ]]; then
      echo "no coverage profile: run just test first" >&2
      exit 1
    fi

    echo "Profile: ${profile}"
    go tool cover -func="${profile}"

# Run local unit tests (no external dependencies needed).
[env("TEST_LOCAL", "1")]
test-local folder="./..." *args:
    @just test {{ folder }} "${@:2}"

# Run end-to-end tests against real Incus.
[env("TEST_E2E", "1")]
test-e2e folder="./..." *args:
    @just test {{ folder }} "${@:2}"

