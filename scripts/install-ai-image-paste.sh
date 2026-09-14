#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_script="$repo_dir/scripts/paste-image-to-zsh.sh"
bin_dir="${AI_IMAGE_PASTE_BIN_DIR:-$HOME/.local/bin}"
installed_script="$bin_dir/ai-paste-image"
zshrc="${ZDOTDIR:-$HOME}/.zshrc"
marker_begin="# >>> ai-image-paste >>>"
marker_end="# <<< ai-image-paste <<<"

if [[ ! -x "$source_script" ]]; then
  printf '脚本不可执行：%s\n' "$source_script" >&2
  printf '请先运行：chmod +x %q\n' "$source_script" >&2
  exit 1
fi

mkdir -p "$bin_dir"
cp "$source_script" "$installed_script"
chmod 700 "$installed_script"

if [[ -f "$zshrc" ]] && grep -Fq "$marker_begin" "$zshrc"; then
  printf '已存在 zsh 配置块，保留原配置不重复写入。\n'
else
  if [[ -f "$zshrc" && ! -f "$zshrc.ai-image-paste.bak" ]]; then
    cp "$zshrc" "$zshrc.ai-image-paste.bak"
    printf '已备份原 zsh 配置：%s\n' "$zshrc.ai-image-paste.bak"
  fi
  cat >> "$zshrc" <<EOF

$marker_begin
ai_paste_image_widget() {
  local image_reference
  image_reference="\$("$installed_script" 2>&1)"
  if [[ "\$image_reference" == @/* ]]; then
    LBUFFER+="\$image_reference"
    zle redisplay
  else
    zle -M "\$image_reference"
  fi
}
zle -N ai_paste_image_widget
bindkey -M emacs '\\e[99~' ai_paste_image_widget
bindkey -M viins '\\e[99~' ai_paste_image_widget
$marker_end
EOF
  printf '已写入 zsh 配置：%s\n' "$zshrc"
fi

cat <<EOF

安装完成：$installed_script

接下来在 iTerm2 设置一次快捷键：
1. Settings → Profiles → Keys → Key Mappings → +
2. 快捷键选择 Command + Option + V
3. Action 选择 Send Escape Sequence
4. 输入：[99~
5. 新开一个 zsh 标签页，或执行：source "$zshrc"

复制图片后按 Command + Option + V，当前命令行会插入 @图片路径。
EOF
