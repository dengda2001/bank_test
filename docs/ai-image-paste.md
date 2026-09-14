# 在 iTerm2 + zsh 中粘贴图片给 AI

这套配置不会把图片二进制内容直接写入终端，而是把剪贴板图片保存到临时 PNG，再把 `@图片路径` 插入当前 AI 命令行。这样可以避免终端把图片当作乱码或普通文本处理。

## 安装

在项目根目录执行：

```sh
chmod +x scripts/paste-image-to-zsh.sh scripts/install-ai-image-paste.sh
./scripts/install-ai-image-paste.sh
```

脚本优先使用 `pngpaste`。如果没有安装，会自动尝试 macOS 自带的 `osascript`；如果系统权限或剪贴板格式不兼容，可以安装：

```sh
brew install pngpaste
```

## 配置 iTerm2 快捷键

打开 `Settings → Profiles → Keys → Key Mappings`，新增一条：

- Keyboard Shortcut：`Command + Option + V`
- Action：`Send Escape Sequence`
- 输入：`[99~`

然后新开一个 zsh 标签页，或执行：

```sh
source ~/.zshrc
```

复制图片后按 `Command + Option + V`，当前输入行会出现类似：

```text
@/var/folders/.../ai-image-paste/image-Ab12Cd.png
```

接着补充“请分析这张图片”等文字并提交即可。临时图片权限为当前用户可读写，文件会保留在 macOS 临时目录中，重启后通常会被系统清理。
