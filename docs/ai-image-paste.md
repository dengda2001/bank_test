# 在 iTerm2 + zsh 中粘贴图片给 AI

这套配置使用 iTerm2 全局快捷键和 Run Coprocess：Swift 工具读取剪贴板图片，保存为临时 PNG，再把图片绝对路径输入当前终端。这样不依赖 zsh 当前输入行，在 tmux 中也能工作。

## 安装

在项目根目录执行：

```sh
chmod +x scripts/install-ai-image-paste.sh
./scripts/install-ai-image-paste.sh
```

安装器会使用 `swiftc` 编译 `scripts/codex-paste-image.swift` 到 `~/.local/bin/codex-paste-image`。它支持 PNG、TIFF，以及 Finder 复制的图片文件。

## 配置 iTerm2 全局快捷键

安装器会自动配置 iTerm2 的全局快捷键。如果 iTerm2 没有及时刷新，也可以手动打开 `Settings → Keys → Key Mappings`，新增：

- Keyboard Shortcut：`Command + Option + V`
- Action：`Run Coprocess`
- Command：`/Users/你的用户名/.local/bin/codex-paste-image`

如果你需要手动配置，运行 Coprocess 的命令输出会被 iTerm2 当作当前终端输入，因此不需要 zsh 或 tmux 额外绑定。

复制图片后按 `Command + Option + V`，当前输入行会出现类似：

```text
/var/folders/.../ai-image-paste/image-AB12CD34.png
```

接着补充“请分析这张图片”等文字并提交即可。临时图片权限为当前用户可读写，文件会保留在 macOS 临时目录中，重启后通常会被系统清理。
