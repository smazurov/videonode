#!/usr/bin/env bash
set -euo pipefail
SOFT=500; HARD=700
bad=0
while IFS= read -r f; do
  lines=$(wc -l < "$f")
  if [ "$lines" -gt "$HARD" ]; then
    echo "FAIL ($lines > $HARD): $f"; bad=1
  elif [ "$lines" -gt "$SOFT" ]; then
    echo "WARN ($lines > $SOFT): $f"
  fi
done < <(git ls-files \
           'composer/src/*.cpp' 'composer/src/*.hpp' \
           'composer/src/**/*.cpp' 'composer/src/**/*.hpp' \
           'composer/tools/*.cpp' 'composer/tests/*.cpp' \
           'composer/fuzz/*.cpp' 'composer/fuzz/*.hpp')
exit $bad
