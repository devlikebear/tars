#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
WORKFLOW="$ROOT/.github/workflows/sonarcloud.yml"
PROPERTIES="$ROOT/sonar-project.properties"

status=0

require_pattern() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if [ ! -f "$file" ] || ! rg -q -- "$pattern" "$file"; then
    printf '[sonarcloud-workflow] missing: %s\n' "$description" >&2
    status=1
  fi
}

forbid_pattern() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if [ -f "$file" ] && rg -q -- "$pattern" "$file"; then
    printf '[sonarcloud-workflow] unexpected: %s\n' "$description" >&2
    status=1
  fi
}

require_pattern "$WORKFLOW" 'SonarSource/sonarqube-scan-action@v7' 'official SonarQube scan action v7'
require_pattern "$WORKFLOW" 'pull-requests: read' 'job-scoped pull request read permission'
require_pattern "$WORKFLOW" 'SONAR_TOKEN' 'SONAR_TOKEN secret wiring'
require_pattern "$WORKFLOW" 'SONAR_PROJECT_KEY' 'SONAR_PROJECT_KEY variable wiring'
require_pattern "$WORKFLOW" 'SONAR_ORGANIZATION' 'SONAR_ORGANIZATION variable wiring'
require_pattern "$WORKFLOW" 'continue-on-error: true' 'non-blocking evaluation mode'
require_pattern "$WORKFLOW" 'make test-cover' 'Go coverage generation before scan'
require_pattern "$PROPERTIES" 'sonar.go.coverage.reportPaths=coverage.out' 'Go coverage path'
forbid_pattern "$PROPERTIES" 'sonar.javascript.lcov.reportPaths' 'JavaScript LCOV path before frontend coverage exists'

if [ "$status" -ne 0 ]; then
  exit "$status"
fi

printf '[sonarcloud-workflow] passed\n'
