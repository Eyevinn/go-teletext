#!/bin/sh
# all-langs-stream.sh makes a transport stream with video, audio and a teletext subtitle page for
# every language preset of ttx-gen, for checking a player or decoder by eye. docs/test-stream.md
# lists the text that each page shows and says what to look for.
#
# Usage: scripts/all-langs-stream.sh [output.ts [ttx-gen options]]
#
# The output is ttx-all-langs.ts unless given. Options after it go to ttx-gen, for example
# -single-height, -designation m29 or -muxrate 3500000. DURATION sets the length in seconds
# (default 60). Needs go, and ffmpeg with an H.264 encoder.
set -eu

out=${1:-ttx-all-langs.ts}
if [ $# -gt 0 ]; then
	shift
fi
duration=${DURATION:-60}
root=$(cd "$(dirname "$0")/.." && pwd)

venc=
encoders=$(ffmpeg -hide_banner -encoders 2>/dev/null)
for e in libx264 h264_videotoolbox libopenh264; do
	if echo "$encoders" | grep -q " $e "; then
		venc=$e
		break
	fi
done
if [ -z "$venc" ]; then
	echo "all-langs-stream: ffmpeg has none of the H.264 encoders libx264, h264_videotoolbox and libopenh264" >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# The test pattern has a running clock in the picture. Without B-frames and without mux delay, the
# PTS of a frame is the time on its clock plus the delay of the audio encoder, about 21 ms.
ffmpeg -v error -y \
	-f lavfi -i "testsrc2=size=1280x720:rate=25" \
	-f lavfi -i "sine=frequency=440:sample_rate=48000" \
	-t "$duration" \
	-c:v "$venc" -b:v 2500k -g 25 -bf 0 -pix_fmt yuv420p \
	-c:a aac -b:a 96k -af volume=0.05 \
	-muxdelay 0 -muxpreload 0 -f mpegts "$tmp/video.ts"

go build -C "$root" -o "$tmp/ttx-gen" ./cmd/ttx-gen
langs=$("$tmp/ttx-gen" -list | awk '{print $1}' | paste -sd, -)

# -clock pts prints the PTS in each cue, to compare with the clock in the picture.
"$tmp/ttx-gen" -i "$tmp/video.ts" -o "$out" -clock pts -langs "$langs" "$@"
echo "wrote $out"
