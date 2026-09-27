const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const code = fs.readFileSync(path.join(__dirname, '../static/sw.js'), 'utf8');

function loadWorker() {
  const listeners = {};
  const stores = new Map();
  const origin = 'https://lanpaper.example';
  let offline = false;
  let claimed = false;
  const key = input => new URL(typeof input === 'string' ? input : input.url, origin).href;
  const caches = {
    keys: async () => [...stores.keys()],
    delete: async name => stores.delete(name),
    open: async name => {
      if (!stores.has(name)) stores.set(name, new Map());
      return {
        put: async (input, response) => stores.get(name).set(key(input), response.clone()),
        match: async input => stores.get(name).get(key(input))?.clone()
      };
    },
    match: async input => {
      for (const store of stores.values()) {
        if (store.has(key(input))) return store.get(key(input)).clone();
      }
      return undefined;
    }
  };
  const self = {
    location: { origin },
    addEventListener: (name, handler) => { listeners[name] = handler; },
    skipWaiting: async () => {},
    clients: { claim: async () => { claimed = true; } }
  };
  const fetch = async input => {
    if (offline) throw new Error('offline');
    const url = new URL(typeof input === 'string' ? input : input.url, origin);
    const contentType = url.pathname.endsWith('.js') ? 'application/javascript' :
      url.pathname.endsWith('.css') ? 'text/css' :
      url.pathname.endsWith('.json') ? 'application/json' : 'image/svg+xml';
    return new Response('static asset', { headers: { 'content-type': contentType } });
  };
  vm.runInNewContext(code, { self, URL, Response, caches, fetch });

  const lifecycle = async name => {
    let pending;
    listeners[name]({ waitUntil: work => { pending = work; } });
    assert.ok(pending, `missing ${name} waitUntil`);
    await pending;
  };
  const intercept = async route => {
    const request = new Request(new URL(route, origin));
    let response;
    listeners.fetch({ request, respondWith: value => { response = value; } });
    return response ? await response : undefined;
  };
  return { stores, caches, lifecycle, intercept, setOffline: value => { offline = value; }, claimed: () => claimed, origin };
}

test('old admin/media caches are purged, only public application assets remain cached', async () => {
  const worker = loadWorker();
  worker.stores.set('lanpaper-v1.0.0', new Map([
    [worker.origin + '/admin', new Response('private admin data')],
    [worker.origin + '/photo?token=secret', new Response('revoked media')]
  ]));
  worker.stores.set('unrelated-application', new Map());
  await worker.lifecycle('activate');
  assert.deepEqual([...worker.stores.keys()], ['unrelated-application']);
  assert.equal(worker.claimed(), true);

  await worker.lifecycle('install');
  const assetCache = worker.stores.get('lanpaper-static-v2');
  assert.ok(assetCache.has(worker.origin + '/static/css/style.css'));
  assert.ok(!assetCache.has(worker.origin + '/admin'));

  for (const route of ['/admin', '/api/wallpapers', '/api/preview/photo',
    '/photo?token=secret', '/static/images/photo.png']) {
    assert.equal(await worker.intercept(route), undefined, `${route} must use the network`);
  }
  const fresh = await worker.intercept('/static/css/style.css');
  assert.equal(await fresh.text(), 'static asset');
  worker.setOffline(true);
  assert.equal(await (await worker.intercept('/static/css/style.css')).text(), 'static asset');
  assert.equal(await worker.intercept('/photo?token=secret'), undefined);
});
