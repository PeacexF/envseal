#!/usr/bin/env bash
#
# Turn `envseal check --json` into GitHub annotations
#
# Only check names, statuses, and details are read. The report contains no
# secret values by construction, and this script must not become the place that changes that
#
#   annotate.sh <report.json>

set -euo pipefail

report="${1:-}"
if [ ! -f "$report" ]; then
	echo "usage: annotate.sh <report.json>" >&2
	exit 2
fi

# Findings are read with awk rather than jq, which is not on every runner image.
# Only the three scalar fields are parsed; the full detail, including which
# files are exposed, reaches the reader through the job summary.
findings() {
	awk '
		/"check"/  { gsub(/.*"check"[[:space:]]*:[[:space:]]*"|".*/, "");  check = $0 }
		/"status"/ { gsub(/.*"status"[[:space:]]*:[[:space:]]*"|".*/, ""); status = $0 }
		/"detail"/ { gsub(/.*"detail"[[:space:]]*:[[:space:]]*"|".*/, ""); detail = $0
		             print status "\t" check "\t" detail }
	' "$report"
}

problems=0
warnings=0

while IFS=$'\t' read -r status check detail; do
	[ -z "$status" ] && continue

	case "$status" in
	failed)
		echo "::error title=envseal: ${check}::${detail}"
		problems=$((problems + 1))
		;;
	warning)
		echo "::warning title=envseal: ${check}::${detail}"
		warnings=$((warnings + 1))
		;;
	esac
done < <(findings)

if [ -n "${GITHUB_OUTPUT:-}" ]; then
	{
		echo "problems=$problems"
		echo "warnings=$warnings"
	} >>"$GITHUB_OUTPUT"
fi

echo "problems=$problems warnings=$warnings"
