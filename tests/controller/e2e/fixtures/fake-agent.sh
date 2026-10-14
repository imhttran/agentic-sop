#!/bin/sh
# Deterministic SOP agent double for the controller's Playwright E2E tests.
#
# SOP invokes a configured command agent (SOP_AGENT_COMMAND) by writing a JSON request
# to its stdin and reading the model's raw output from stdout. This fixture is not a
# model: it answers a PLAN request with the single-stage plan the test passes in
# E2E_PLAN_JSON, and an IMPLEMENT/FIX request with an explicit destructive/authorization
# boundary, so SOP — running its real lifecycle — classifies it NEEDS_HUMAN and records
# a genuine approval gate. Any other capability is a benign no-op.
#
# It never touches SOP state or the repository; it only prints JSON.
req=$(cat)
case "$req" in
  *'"capability":"PLAN"'*)
    printf '%s\n' "$E2E_PLAN_JSON"
    ;;
  *'"capability":"IMPLEMENT"'*|*'"capability":"FIX"'*)
    printf '%s\n' '{"status":"needs_human","summary":"authorization required","reason":"the requested change is destructive and cannot be undone; it needs an explicit human decision before proceeding"}'
    ;;
  *)
    printf '%s\n' '{"status":"completed","summary":"no-op","changes_expected":false}'
    ;;
esac
