#!/bin/sh
# Runs the reference interpreter over every program in /work.
for f in /work/*.mb; do
  [ -e "$f" ] || continue
  base="${f%.mb}"
  in="$base.in"; [ -e "$in" ] || in=/dev/null
  steps=1000000; [ -e "$base.steps" ] && steps=$(cat "$base.steps")
  MAL_STEPS="$steps" /usr/local/bin/malbolge-ref "$f" < "$in" > "$base.refout" 2> "$base.referr"
  echo $? > "$base.refrc"
done
