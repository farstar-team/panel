import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));
const port = Number(process.env.PORT || 4173);
const mime = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon'
};

const demoHealth = {
  ok: true,
  mode: process.env.PANEL_MODE || 'demo',
  service: 'FARSTAR PANEL',
  xray: 'adapter-ready',
  updatedAt: new Date().toISOString()
};

const server = http.createServer(async (req, res) => {
  const pathname = new URL(req.url, `http://${req.headers.host || 'localhost'}`).pathname;

  if (pathname === '/api/health') {
    res.writeHead(200, { 'content-type': 'application/json; charset=utf-8' });
    res.end(JSON.stringify(demoHealth));
    return;
  }

  const requested = pathname === '/' ? '/index.html' : pathname;
  const safePath = normalize(join(root, requested));
  if (!safePath.startsWith(root)) {
    res.writeHead(403);
    res.end('Forbidden');
    return;
  }

  try {
    const body = await readFile(safePath);
    res.writeHead(200, { 'content-type': mime[extname(safePath)] || 'application/octet-stream' });
    res.end(body);
  } catch {
    const fallback = await readFile(join(root, 'index.html'));
    res.writeHead(200, { 'content-type': mime['.html'] });
    res.end(fallback);
  }
});

server.listen(port, () => {
  console.log(`FARSTAR PANEL running at http://localhost:${port}`);
});
