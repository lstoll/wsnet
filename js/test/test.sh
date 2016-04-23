#!/bin/bash

trap 'kill -9 $(jobs -p);' EXIT

echo '--> Starting services to test'

# Start the go server
go run js/test/echo.go &
sleep 1

# Start the proxy
node js/test/testproxy.js &
sleep 1

echo '--> Start basic in/out test'
out=$(printf "abc\ndef\nghi\n" | nc 127.0.0.1 9090 -i 2)
exit=$?
if [ $exit -ne 0 ]; then
    echo "--> nc returned non-0 exit code: $exit"
    exit 1
fi
if [ $(echo "$out"|tr -d '\n') != "abcdefghi" ]; then
    echo "--> Unexpected response: $out"
    exit 1
fi

echo '--> Start stress test'
for i in {1..20}; do
    echo "--> run $i"
    out=$(printf "abc\ndef\nghi\n" | nc 127.0.0.1 9090)
    exit=$?
    if [ $exit -ne 0 ]; then
	echo "--> nc returned non-0 exit code: $exit"
	exit 1
    fi
done
