# Generated media fixtures

`preview.webm` (VP8) and `preview.mp4` (H.264) are two-second, silent 320×180
FFmpeg `testsrc2` patterns at 12 fps. No third-party footage or personal data.
`preview-portrait.webm` uses the same pattern at 180×320.
`animated.gif` is two solid 48×32 frames (orange/dark), 200 ms per frame.
These tiny fixtures test actual upload, protected streaming and static GIF
thumbnails; FFmpeg is **not** an application or test-runner dependency.

Regenerate videos using `ffmpeg -f lavfi -i testsrc2=size=320x180:rate=12 -t 2
-an -c:v libvpx -pix_fmt yuv420p preview.webm` (or `libx264` for MP4).
