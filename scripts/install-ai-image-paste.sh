#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_script="$repo_dir/scripts/codex-paste-image.swift"
bin_dir="${AI_IMAGE_PASTE_BIN_DIR:-$HOME/.local/bin}"
installed_binary="$bin_dir/codex-paste-image"
zshrc="${ZDOTDIR:-$HOME}/.zshrc"
iterm_prefs="$HOME/Library/Preferences/com.googlecode.iterm2.plist"
iterm_backup="$HOME/Library/Preferences/com.googlecode.iterm2.plist.ai-image-paste.bak"
global_key="0x76-0x180000-0x9"
marker_begin="# >>> ai-image-paste >>>"
marker_end="# <<< ai-image-paste <<<"

if [[ ! -f "$source_script" ]]; then
  printf 'Swift 源码不存在：%s\n' "$source_script" >&2
  exit 1
fi

if ! command -v swiftc >/dev/null 2>&1; then
  printf '未找到 swiftc，请安装 Xcode Command Line Tools。\n' >&2
  exit 1
fi

mkdir -p "$bin_dir"

compiled_binary="$(mktemp "$bin_dir/.codex-paste-image.XXXXXX")"
cleanup() {
  rm -f "$compiled_binary"
}
trap cleanup EXIT
swiftc "$source_script" -O -o "$compiled_binary"
chmod 700 "$compiled_binary"
mv -f "$compiled_binary" "$installed_binary"
trap - EXIT

if [[ -f "$zshrc" ]] && grep -Fq "$marker_begin" "$zshrc"; then
  if [[ -f "$zshrc" && ! -f "$zshrc.ai-image-paste.bak" ]]; then
    cp "$zshrc" "$zshrc.ai-image-paste.bak"
    printf '已备份原 zsh 配置：%s\n' "$zshrc.ai-image-paste.bak"
  fi
  awk -v begin="$marker_begin" -v end="$marker_end" '
    $0 == begin { skipping = 1; next }
    $0 == end { skipping = 0; next }
    !skipping { print }
  ' "$zshrc" > "$zshrc.tmp.ai-image-paste"
  mv "$zshrc.tmp.ai-image-paste" "$zshrc"
  printf '已移除旧的 zsh 拦截配置：%s\n' "$zshrc"
fi

if [[ -f "$iterm_prefs" ]] && command -v defaults >/dev/null 2>&1 && command -v plutil >/dev/null 2>&1; then
  if [[ ! -f "$iterm_backup" ]]; then
    cp "$iterm_prefs" "$iterm_backup"
    printf '已备份 iTerm2 配置：%s\n' "$iterm_backup"
  fi

  iterm_tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/iterm2-prefs.XXXXXX")"
  iterm_tmp="$iterm_tmp_dir/preferences.plist"
  cleanup_iterm() {
    rm -rf "$iterm_tmp_dir"
  }
  trap cleanup_iterm EXIT
  defaults export com.googlecode.iterm2 "$iterm_tmp" >/dev/null

  if plutil -extract 'GlobalKeyMap' xml -o /dev/null "$iterm_tmp" 2>/dev/null; then
    if plutil -extract "GlobalKeyMap.$global_key" xml -o /dev/null "$iterm_tmp" 2>/dev/null; then
      plutil -replace "GlobalKeyMap.$global_key" -json "{\"Action\":35,\"Text\":\"$installed_binary\"}" "$iterm_tmp"
    else
      plutil -insert "GlobalKeyMap.$global_key" -json "{\"Action\":35,\"Text\":\"$installed_binary\"}" "$iterm_tmp"
    fi
  else
    plutil -insert GlobalKeyMap -xml '<dict></dict>' "$iterm_tmp"
    plutil -insert "GlobalKeyMap.$global_key" -json "{\"Action\":35,\"Text\":\"$installed_binary\"}" "$iterm_tmp"
  fi

  # Remove the older profile-level escape-sequence mapping from this setup.
  if plutil -extract 'New Bookmarks.0.Keyboard Map.0x9-0x180000' xml -o /dev/null "$iterm_tmp" 2>/dev/null; then
    plutil -remove 'New Bookmarks.0.Keyboard Map.0x9-0x180000' "$iterm_tmp"
  fi

  defaults import com.googlecode.iterm2 "$iterm_tmp"
  trap - EXIT
  cleanup_iterm
  printf '已配置 iTerm2 全局 Coprocess 快捷键：Command + Option + V\n'
else
  printf '未检测到 iTerm2 配置文件，请按文档手动添加全局快捷键。\n'
fi

cat <<EOF

已编译安装：$installed_binary

 iTerm2 全局快捷键已配置为：
  Keyboard Map: 0x76-0x180000-0x9
  Action:       Run Coprocess (35)
  Text:         $installed_binary

如果需要手动恢复 iTerm2 配置，请关闭 iTerm2 后恢复：
  ~/Library/Preferences/com.googlecode.iterm2.plist.ai-image-paste.bak

复制图片后按 Command + Option + V，iTerm2 会将图片绝对路径输入当前终端。
EOF
