#!/bin/bash
set -o monitor
set -o nounset

cd "$(dirname "$0")"
TMP="$(mktemp -d)"

BUILD=(
  go build -o "${TMP}/unlockr" .
)
CMD=(
  "${TMP}/unlockr" -debug "$@"
)

trap 'kill %1; rm -rv "${TMP}"' EXIT

"${BUILD[@]}" && "${CMD[@]}" &

while fswatch --one-event --exclude '\.git' --event Updated -r .
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
