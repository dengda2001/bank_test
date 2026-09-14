#!/usr/bin/env bash
set -euo pipefail

# Save the macOS clipboard image and print a file reference for an AI CLI.
# stdout intentionally contains only the value to insert into the prompt.

if [[ "${1:-}" == "--help" ]]; then
  cat <<'EOF'
用法：paste-image-to-zsh.sh

把 macOS 剪贴板中的图片保存为临时 PNG，并输出 @图片路径。
优先使用 pngpaste；没有安装时自动尝试 macOS 自带的 osascript。
EOF
  exit 0
fi

if [[ $# -gt 0 ]]; then
  printf '不支持的参数：%s\n' "$1" >&2
  exit 2
fi

image_dir="${AI_IMAGE_PASTE_DIR:-${TMPDIR:-/tmp}/ai-image-paste}"
umask 077
mkdir -p "$image_dir"

capture_dir="$(mktemp -d "$image_dir/session-XXXXXX")"
image_path="$capture_dir/image.png"
cleanup() {
  rm -f "$image_path"
  rmdir "$capture_dir" 2>/dev/null || true
}
trap cleanup EXIT

capture_with_pngpaste() {
  command -v pngpaste >/dev/null 2>&1 || return 1
  pngpaste "$image_path" >/dev/null 2>&1
}

capture_with_osascript() {
  command -v osascript >/dev/null 2>&1 || return 1

  osascript - "$image_path" <<'APPLESCRIPT'
on run argv
  set outputPath to item 1 of argv
  try
    set pngData to the clipboard as «class PNGf»
  on error
    error "剪贴板中没有可用的图片"
  end try

  set outputFile to open for access (POSIX file outputPath) with write permission
  try
    set eof of outputFile to 0
    write pngData to outputFile
    close access outputFile
  on error errorMessage number errorNumber
    try
      close access outputFile
    end try
    error errorMessage number errorNumber
  end try
end run
APPLESCRIPT
}

if ! capture_with_pngpaste && ! capture_with_osascript; then
  printf '%s\n' '剪贴板中没有图片，或当前 macOS 不允许读取剪贴板。' >&2
  printf '%s\n' '如果经常使用，建议安装 pngpaste：brew install pngpaste' >&2
  exit 1
fi

if [[ ! -s "$image_path" ]]; then
  printf '%s\n' '没有捕获到图片数据。' >&2
  exit 1
fi

# Keep the file after this process exits; the AI CLI reads it from the prompt.
trap - EXIT
printf '@%s' "$image_path"
