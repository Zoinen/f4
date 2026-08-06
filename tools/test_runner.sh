#!/bin/bash

cd f4 || exit 1

gofmt -w -s .

./filelist_update.sh

start=$(date +%s.%N)

go test ./... -timeout 30s
status=$?

end=$(date +%s.%N)

elapsed=$(awk "BEGIN { printf \"%.3f\", $end - $start }")

echo

if [ $status -eq 0 ]; then
    echo -e "\e[32m[OK]\e[0m Tests completed in ${elapsed}s"
else
    echo -e "\e[31m[FAILED]\e[0m Tests completed in ${elapsed}s"
fi

echo
git status
echo

cd ..

exit $status
