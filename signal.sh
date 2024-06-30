#!/bin/sh -e

trap 'echo "SIGINT received" && exit' INT
trap 'echo "SIGTERM received" && exit' TERM

echo "this script is running with pid $$"

echo waiting signals INT and TERM

while true
do
  sleep 1
done
