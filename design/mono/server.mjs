// SPDX-License-Identifier: MIT
// Development-only presentation sandbox. No Go server or real storage is used.
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root = fileURLToPath(new URL('../../', import.meta.url));
const port = Number(process.env.PORT || 8080);
const version = (await readFile(path.join(root, 'VERSION'), 'utf8')).trim();
const photos = ['wp-city', 'wp-mono', 'wp-dawn', 'wp-forest', 'wp-panorama', 'wp-abstract', 'wp-square', 'wp-vertical'];
const names = ['homepage', 'quiet-lines', 'morning-light', 'forest-study', 'horizon', 'soft-shapes', 'still-life', 'vertical', 'studio', 'weekend', 'north', 'paper', 'evening'];
const dimensions = JSON.parse(await readFile(path.join(root, 'design/mono/media-dimensions.json'), 'utf8'));
const now = Math.floor(Date.now() / 1000);
const links = names.map((linkName, i) => ({
    linkName, hasImage: true, mimeType: 'jpg', category: 'image',
    preview: `/demo/media/${photos[i % photos.length]}.jpg`,
    ...dimensions[photos[i % photos.length] + '.jpg'],
    currentVersion: 1, accessLevel: 'public', pinned: i === 0,
    created: now - 86400 * (i + 1), modTime: now - 86400 * i,
    history: [], items: [], rotate: null
}));
// Read-only coverage fixtures: real image dimensions, alpha PNG, video
// fixtures with on-demand playback (never fake posters), and an empty link.
Object.assign(links[4], { linkName: 'portrait', preview: '/demo/media/wp-vertical.jpg', ...dimensions['wp-vertical.jpg'] });
Object.assign(links[5], { linkName: 'square', preview: '/demo/media/wp-square.jpg', ...dimensions['wp-square.jpg'] });
Object.assign(links[6], { linkName: 'icon-png', mimeType: 'png', preview: '/demo/media/app-icon.png', ...dimensions['app-icon.png'] });
Object.assign(links[7], { linkName: 'video-wide', mimeType: 'mp4', category: 'video', width: 320, height: 180, durationSec: 2, preview: '' });
Object.assign(links[8], { linkName: 'video-portrait', mimeType: 'webm', category: 'video', width: 180, height: 320, durationSec: 2, preview: '' });
Object.assign(links[9], { linkName: 'empty-link', hasImage: false, mimeType: '', preview: '', width: 0, height: 0, sizeBytes: 0 });
Object.assign(links[10], { linkName: 'panorama', preview: '/demo/media/wp-panorama.jpg', ...dimensions['wp-panorama.jpg'] });
Object.assign(links[11], { linkName: 'animated-gif', mimeType: 'gif', preview: '/demo/fixtures/sample.webp', sizeBytes: (await readFile(path.join(root, 'design/mono/fixtures/sample.gif'))).length, width: 360, height: 201 });
Object.assign(links[12], { linkName: 'image-webp', mimeType: 'webp', preview: '/demo/fixtures/sample.webp', sizeBytes: (await readFile(path.join(root, 'design/mono/fixtures/sample.webp'))).length, width: 360, height: 201 });
const videos = { 'video-wide': 'preview.mp4', 'video-portrait': 'preview-portrait.webm' };
for (const [name, file] of Object.entries(videos)) links.find(link => link.linkName === name).sizeBytes = (await readFile(path.join(root, 'tests/e2e/fixtures', file))).length;
const mime = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.css': 'text/css; charset=utf-8', '.json': 'application/json', '.svg': 'image/svg+xml', '.jpg': 'image/jpeg', '.png': 'image/png', '.gif': 'image/gif', '.webp': 'image/webp', '.woff2': 'font/woff2' };
const server = http.createServer(async (req, res) => {
    const url = new URL(req.url, 'http://preview.local');
    if (url.pathname === '/') { res.writeHead(302, { Location: process.env.DEMO_START || '/admin?design=studio&sample=collection' }); return res.end(); }
    const json = (data, status = 200) => { res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); res.end(JSON.stringify(data)); };
    try {
        const video = videos[url.pathname.replace(/^\/(?:api\/preview\/)?/, '')];
        if (video && ['GET', 'HEAD'].includes(req.method)) {
            const bytes = await readFile(path.join(root, 'tests/e2e/fixtures', video));
            const headers = { 'Content-Type': video.endsWith('.mp4') ? 'video/mp4' : 'video/webm', 'Accept-Ranges': 'bytes', 'Cache-Control': 'no-store' };
            const range = req.headers.range && /^bytes=(\d+)-(\d*)$/.exec(req.headers.range);
            let start = 0, end = bytes.length - 1;
            if (req.headers.range) {
                if (!range || Number(range[1]) >= bytes.length || (range[2] && Number(range[2]) < Number(range[1]))) {
                    res.writeHead(416, { ...headers, 'Content-Range': `bytes */${bytes.length}` }); return res.end();
                }
                start = Number(range[1]); end = range[2] ? Math.min(Number(range[2]), end) : end;
                headers['Content-Range'] = `bytes ${start}-${end}/${bytes.length}`;
            }
            headers['Content-Length'] = end - start + 1;
            res.writeHead(range ? 206 : 200, headers); return res.end(req.method === 'HEAD' ? undefined : bytes.subarray(start, end + 1));
        }
        if (url.pathname.startsWith('/api/')) {
            if (req.method !== 'GET') return json({ error: 'Это макет: изменение данных и загрузка файлов отключены.' }, 403);
            if (url.pathname === '/api/wallpapers') {
                // Explicit request state survives iframe/proxy referrer stripping.
                return json(url.searchParams.get('demo_sample') === 'collection' ? links : links.slice(0, 1));
            }
            if (url.pathname === '/api/compression-config') return json({ version, quality: 85, scale: 100 });
            if (url.pathname === '/api/sessions') return json([]);
            if (url.pathname === '/api/external-images') return json([]);
            if (/\/history$/.test(url.pathname)) return json({ history: [], limit: 3, bytes: 0 });
            return json({ error: 'Недоступно в макете' }, 404);
        }
        if (url.pathname === '/' || url.pathname === '/admin') {
            let html = await readFile(path.join(root, 'admin.html'), 'utf8');
            html = html.replace('<script src="/static/js/prepaint.js">', '<script src="/demo/prepare.js"></script><script src="/static/js/prepaint.js">');
            const studio = url.searchParams.get('design') === 'studio';
            if (!studio) html = html.replace('data-layout="studio"', 'data-layout="legacy"').replace('<link rel="stylesheet" href="/static/css/studio.css">', '');
            html = html.replace('</head>', '<link rel="stylesheet" href="/demo/preview.css">' +
                ((studio || url.searchParams.get('design') === 'original') ? '' : '<link rel="stylesheet" href="/demo/design.css">') + (url.searchParams.get('design') === 'radical' ? '<link rel="stylesheet" href="/demo/radical.css">' : '') + '</head>');
            html = html.replace('<body>', `<body><aside class="demo-bar" aria-label="Управление макетом">
              <span class="demo-label"><b>LP / LAB</b> Монохромный фотокаталог <span class="demo-note">· Демо, без сохранения данных</span></span>
              <nav><a data-demo="studio" href="/admin?design=studio&sample=collection">Studio / 03</a><a data-demo="radical" href="/admin?design=radical&sample=collection">Каталог / 02</a><a data-demo="new" href="/admin">Фотопринт / 01</a><a data-demo="original" href="/admin?design=original">Исходный</a><span class="demo-divider"></span><a data-sample="single" href="/admin">1 ссылка</a><a data-sample="collection" href="/admin?sample=collection">13 ссылок</a></nav>
            </aside>`);
            html = html.replace('</body>', (url.searchParams.get('design') === 'radical' ? '<script src="/demo/radical.js"></script>' : '') + '<script src="/demo/preview.js"></script></body>');
            res.writeHead(200, { 'Content-Type': mime['.html'], 'Cache-Control': 'no-store' }); return res.end(html);
        }
        let relative;
        if (url.pathname.startsWith('/static/')) relative = url.pathname.slice(1);
        else if (url.pathname.startsWith('/demo/media/')) relative = 'design/prototypes/media/' + path.basename(url.pathname);
        else if (url.pathname.startsWith('/demo/')) relative = 'design/mono/' + url.pathname.slice(6);
        else {
            const item = links.find(link => '/' + link.linkName === url.pathname);
            if (item && !item.preview) { res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' }); return res.end('В макете нет видеофайла: показаны только его тип и размеры.'); }
            if (item) { res.writeHead(302, { Location: item.linkName === 'animated-gif' ? '/demo/fixtures/sample.gif' : item.preview }); return res.end(); }
            res.writeHead(404); return res.end('Not found');
        }
        const file = path.resolve(root, relative);
        const allowed = ['static', 'design/mono', 'design/prototypes/media'].some(dir => file.startsWith(path.join(root, dir) + path.sep));
        if (!allowed || !mime[path.extname(file)]) { res.writeHead(404); return res.end(); }
        let data = await readFile(file);
        // Isolate demo preferences and disable PWA registration in the served
        // copy only; production assets on disk remain completely unchanged.
        if (relative === 'static/js/app.js') data = data.toString().replace("const STORE_KEY = 'lp-ui'", "const STORE_KEY = 'lp-mono-demo'").replace('        initPWA();', '        /* PWA disabled in the design sandbox. */')
            .replace("'/api/wallpapers?page='", "'/api/wallpapers?demo_sample=' + encodeURIComponent(new URLSearchParams(location.search).get('sample') || 'single') + '&page='");
        if (relative === 'static/js/studio-layout.js') data = data.toString().replaceAll('lp-studio-nav-collapsed', 'lp-mono-demo-nav-collapsed');
        if (relative === 'static/js/prepaint.js') data = data.toString().replace("getItem('lp-ui')", "getItem('lp-mono-demo')");
        res.writeHead(200, { 'Content-Type': mime[path.extname(file)], 'Cache-Control': 'no-store' }); res.end(data);
    } catch (error) {
        if (error.code !== 'ENOENT') console.error(error);
        if (!res.headersSent) res.writeHead(404); res.end('Not found');
    }
});
server.listen(port, '0.0.0.0', () => console.log(`Lanpaper design sandbox: http://0.0.0.0:${server.address().port}/admin (read-only demo)`));
