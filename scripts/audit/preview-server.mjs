// 给 TestWritePrototypePreviewHTML 产出的静态预览页起一个只读服务。
//
// 预览页是模板直接 Execute 出来的，里面的 /static/... 是应用的绝对路径，所以
// 光开一个目录不够：/static 得指回仓库里的 web/static。这个脚本就干这一件事。
//
// 只绑 127.0.0.1。之前有个绑 0.0.0.0 的预览服务忘了关，局域网里谁都能看到
// 带真实姓名和金额的页面，所以这里不给 host 参数。
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize, resolve } from 'node:path';

const ROOT = resolve(process.env.PREVIEW_ROOT || '/private/tmp/rentops-prototype-preview');
const STATIC = resolve(process.env.PREVIEW_STATIC || 'cmd/truelayer-demo/web/static');
const PORT = Number(process.env.PREVIEW_PORT || 8099);

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.woff2': 'font/woff2',
};

// 只允许落在给定的两个根里面，挡掉 ../ 穿越。
const within = (base, target) => {
  const full = normalize(join(base, target));
  return full === base || full.startsWith(base + '/') ? full : null;
};

createServer(async (req, res) => {
  const url = new URL(req.url, 'http://localhost');
  const path = decodeURIComponent(url.pathname);
  const candidates = path.startsWith('/static/')
    ? [within(STATIC, path.slice('/static/'.length))]
    : [within(ROOT, path === '/' ? 'rent-workspace.html' : path.replace(/^\//, ''))];

  for (const file of candidates) {
    if (!file) continue;
    try {
      const body = await readFile(file);
      res.writeHead(200, { 'content-type': TYPES[extname(file)] || 'application/octet-stream' });
      res.end(body);
      return;
    } catch {
      /* 落到下面的 404 */
    }
  }
  res.writeHead(404, { 'content-type': 'text/plain; charset=utf-8' });
  res.end('not found: ' + path);
}).listen(PORT, '127.0.0.1', () => {
  console.log(`preview on http://127.0.0.1:${PORT}/  root=${ROOT}`);
});
