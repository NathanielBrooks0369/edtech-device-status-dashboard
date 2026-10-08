#!/bin/sh
set -eu

curl --fail-with-body --request POST "${DASHBOARD_URL:-http://localhost:8080}/device-status" \
  --header 'Content-Type: application/json' \
  --data '{"device_id":"tablet-17","learner_id":"learner-42","course_id":"algebra-1","online":true,"progress_pct":68,"deadline":"2026-09-27T08:00:00Z","observed_at":"2026-09-26T08:00:00Z"}'
