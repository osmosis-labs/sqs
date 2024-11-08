#!/usr/bin/env bash

set -eo pipefail

echo "Generating gogo proto code"
cd proto

proto_dirs=$(find ./sqs -path -prune -o -name '*.proto' -print0 | xargs -0 -n1 dirname | sort | uniq)
for dir in $proto_dirs; do
  for file in $(find "${dir}" -maxdepth 1 -name '*.proto'); do
    if grep go_package $file &>/dev/null; then
      echo "Generating gogo proto code for $file"
      buf generate $file --template buf.gen.gogo.yaml
    fi
  done
done

# jump to the root directory
cd ..

# move proto files to the right places
cp -r github.com/osmosis-labs/sqs/pkg ./

# remove github.com directory
rm -rf github.com

go mod tidy
