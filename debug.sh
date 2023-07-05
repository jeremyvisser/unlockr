#!/bin/bash
set -o monitor
set -o nounset

cd "$(dirname "$0")"
TMP="$(mktemp -d)"

BUILD=(
  go build -tags debug -o "${TMP}/unlockr" .
)
CMD=(
  "${TMP}/unlockr" "$@"
)

trap 'kill %1; rm -rv "${TMP}"' EXIT

"${BUILD[@]}" && "${CMD[@]}" &

while inotifywait --exclude '\.git' --event close_write -r .
do
  sleep 1
  if "${BUILD[@]}"
  then
    kill %1
    wait %1
    "${CMD[@]}" &
  fi
  sleep 2
done
