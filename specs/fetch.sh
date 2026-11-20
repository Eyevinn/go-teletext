#!/bin/sh
# Downloads the specifications used by this project. The PDFs are not
# committed (see .gitignore); ETSI and EBU publish them free of charge.
set -e
cd "$(dirname "$0")"
get() { [ -f "$1" ] || curl -fsSL -A "Mozilla/5.0" -o "$1" "$2"; }
get en_300706.pdf https://www.etsi.org/deliver/etsi_en/300700_300799/300706/01.02.01_60/en_300706v010201p.pdf
get en_300472.pdf https://www.etsi.org/deliver/etsi_en/300400_300499/300472/01.04.01_60/en_300472v010401p.pdf
get en_301775.pdf https://www.etsi.org/deliver/etsi_en/301700_301799/301775/01.02.01_60/en_301775v010201p.pdf
get tech3264.pdf https://tech.ebu.ch/docs/tech/tech3264.pdf
ls -l *.pdf
