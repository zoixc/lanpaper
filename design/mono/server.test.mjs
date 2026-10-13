// SPDX-License-Identifier: MIT
// Read-only sandbox regression tests; no Go, browser or external packages.
import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

test('preview dataset and version contracts', { timeout: 15000 }, async t => {
    const child = spawn(process.execPath, [fileURLToPath(new URL('./server.mjs', import.meta.url))], {
        env: { ...process.env, PORT: '0' }, stdio: ['ignore', 'pipe', 'pipe']
    });
    t.after(() => child.kill());
    const base = await new Promise((resolve, reject) => {
        let output = '';
        const timeout = setTimeout(() => reject(new Error('Sandbox did not start')), 5000);
        const fail = error => { clearTimeout(timeout); reject(error); };
        child.once('error', fail);
        child.once('exit', code => fail(new Error(`Sandbox exited: ${code}`)));
        child.stdout.on('data', data => {
            output += data;
            const match = output.match(/http:\/\/0\.0\.0\.0:(\d+)/);
            if (match) { clearTimeout(timeout); resolve(`http://127.0.0.1:${match[1]}`); }
        });
    });
    await t.test('video fixtures stream real bytes on demand, while GIF preview is static', async () => {
        const res = await fetch(base + '/api/preview/video-wide', { headers: { Range: 'bytes=0-63' } });
        assert.equal(res.status, 206);
        assert.equal(res.headers.get('content-type'), 'video/mp4');
        const data = Buffer.from(await res.arrayBuffer());
        assert.equal(data.length, 64);
        assert.equal(data.subarray(4, 8).toString(), 'ftyp');
        const list = await (await fetch(base + '/api/wallpapers?demo_sample=collection')).json();
        assert.equal(list.find(link => link.linkName === 'animated-gif').preview, '/demo/fixtures/sample.webp');
        assert.equal((await fetch(base + '/api/preview/video-wide', { method: 'POST' })).status, 403);
    });
    const script = await (await fetch(base + '/static/js/app.js')).text();
    await t.test('the served fetchLinks sends the selected dataset without any Referer', async () => {
        const fn = script.match(/async function fetchLinks\(\) \{[\s\S]*?\n    \}/)?.[0];
        assert.ok(fn, 'real fetchLinks must be exercised');
        for (const [sample, count] of [['single', 1], ['collection', 13]]) {
            const run = vm.runInNewContext(`(${fn})`, {
                location: { search: '?design=studio&sample=' + sample },
                URLSearchParams, encodeURIComponent, setTimeout,
                state: {}, normalizeLink: link => link,
                apiCall: async url => (await fetch(base + url)).json()
            });
            assert.equal((await run()).length, count);
        }
    });
    await t.test('explicit selection wins over an absent or contradictory Referer', async () => {
        const many = await (await fetch(base + '/api/wallpapers?demo_sample=collection', {
            headers: { Referer: base + '/admin?sample=single' }
        })).json();
        assert.equal(many.length, 13);
        const one = await (await fetch(base + '/api/wallpapers?demo_sample=single')).json();
        assert.equal(one.length, 1);
    });
    await t.test('VERSION reaches the footer through the real renderer', async () => {
        const expected = readFileSync(new URL('../../VERSION', import.meta.url), 'utf8').trim();
        assert.match(expected, /^\d+\.\d+\.\d+/);
        const config = await (await fetch(base + '/api/compression-config')).json();
        assert.equal(config.version, expected);
        const el = { hidden: true, textContent: '' };
        const fn = script.match(/function showAppVersion\(version\) \{[\s\S]*?\n    \}/)?.[0];
        assert.ok(fn);
        vm.runInNewContext(`(${fn})`, { $: () => el })(config.version);
        assert.equal(el.hidden, false);
        assert.equal(el.textContent, 'v' + expected);
    });
    await t.test('preview remains read-only', async () => {
        assert.equal((await fetch(base + '/api/link', { method: 'POST' })).status, 403);
    });
});
