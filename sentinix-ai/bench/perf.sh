#!/bin/sh
# Wall time, RSS and CPU of one PR review over N-file repos. Usage: bench/perf.sh 5000 50000
# Needs semgrep, gitleaks, osv-scanner on PATH. Run from sentinix-ai/. Gates: 5000 <= 10s, 50000 <= 45s.
for n in "${@:-5000}"; do
  PERF_FILES=$n go test -count=1 -v -run TestPerfReview -timeout 30m ./pkg/scheduler | grep PERF
done
