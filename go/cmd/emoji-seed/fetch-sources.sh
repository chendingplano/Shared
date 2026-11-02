#!/bin/sh
# Fetch the exact upstream inputs recorded in source-manifest.json.
set -eu
if [ "$#" -ne 1 ]; then echo "usage: $0 OUTPUT_DIRECTORY" >&2; exit 2; fi
dest=$1
mkdir -p "$dest/cldr" "$dest/twemoji"
curl -fsSL https://github.com/jdecked/twemoji/archive/b6b55fef1e8636b540a6d016a4729ca8cdf2e60b.tar.gz -o "$dest/twemoji.tar.gz"
tar -xzf "$dest/twemoji.tar.gz" --strip-components 1 -C "$dest/twemoji"
curl -fsSL https://www.unicode.org/Public/17.0.0/emoji/emoji-test.txt -o "$dest/emoji-test.txt"
cldr=https://raw.githubusercontent.com/unicode-org/cldr/225136a3bd3eff573f3d64c8fa25d6f9afa974ad/common
for kind in annotations annotationsDerived; do
  for lang in en zh; do
    curl -fsSL "$cldr/$kind/$lang.xml" -o "$dest/cldr/$kind-$lang.xml"
  done
done
echo "Fetched sources to $dest; emoji-seed checks every source checksum before use."
