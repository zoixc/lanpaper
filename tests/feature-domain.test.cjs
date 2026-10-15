// SPDX-License-Identifier: MIT
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const moduleURL = pathToFileURL(path.join(__dirname, '..', 'static', 'js', 'feature-domain.js')).href;

test('access and history models expose stable display metadata', async () => {
  const { accessMeta, entryMeta, entryTitle } = await import(moduleURL);
  const bytes = value => `${value} B`;
  assert.deepEqual(accessMeta('token'), { icon: 'key', key: 'access_token' });
  assert.deepEqual(accessMeta('unknown'), { icon: 'globe', key: 'access_public' });
  assert.equal(entryMeta({ ext: 'png', sizeBytes: 12 }, bytes), 'PNG · 12 B');
  assert.equal(entryTitle({ sizeBytes: 7 }, bytes), '7 B');
});

test('upload model serializes append mode without changing replace contracts', async () => {
  const { uploadForm } = await import(moduleURL);
  const appended = uploadForm('wall', 'append', 'url', 'https://example.test/a.png');
  assert.equal(appended.get('linkName'), 'wall');
  assert.equal(appended.get('mode'), 'append');
  assert.equal(appended.get('url'), 'https://example.test/a.png');
  const replaced = uploadForm('wall', 'replace', 'url', '/gallery/a.png');
  assert.equal(replaced.has('mode'), false);
});

test('a pasted address is normalized the way the server will fetch it', async () => {
  const { normalizeMediaUrl } = await import(moduleURL);
  // An address copied out of a page source or a markdown file keeps the HTML
  // escape of the query separator: sent verbatim, imgix drops every parameter
  // after it (q, w, fit) and answers with the full-size original.
  assert.equal(
    normalizeMediaUrl('https://images.unsplash.com/photo-1?q=80&amp;w=1336&amp;fit=crop'),
    'https://images.unsplash.com/photo-1?q=80&w=1336&fit=crop');
  assert.equal(normalizeMediaUrl('  https://example.test/a.png  '), 'https://example.test/a.png');
  assert.equal(normalizeMediaUrl('<https://example.test/a.png>'), 'https://example.test/a.png');
  assert.equal(normalizeMediaUrl('[photo](https://example.test/a.png)'), 'https://example.test/a.png');
  assert.equal(normalizeMediaUrl('https://example.test/a.png?x=1&y=2'), 'https://example.test/a.png?x=1&y=2');
  assert.equal(normalizeMediaUrl(null), '');
  // Chat clients and JSON wrap an address in quotes, prose leaves a closing
  // paren behind, and a PDF adds invisible characters that break the parse on
  // the server. All three are copy artefacts, not part of the address.
  assert.equal(normalizeMediaUrl('"https://example.test/a.png"'), 'https://example.test/a.png');
  assert.equal(normalizeMediaUrl("'https://example.test/a.png'"), 'https://example.test/a.png');
  assert.equal(normalizeMediaUrl('(https://example.test/a.png)'), 'https://example.test/a.png');
  assert.equal(normalizeMediaUrl('https://example.test/a.png)'), 'https://example.test/a.png');
  assert.equal(normalizeMediaUrl('https://images.unsplash\u200b.com/a.png'), 'https://images.unsplash.com/a.png');
  assert.equal(normalizeMediaUrl('https://images.unsplash.com/photo\u00ad-1.png'), 'https://images.unsplash.com/photo-1.png');
  // A balanced bracket belongs to the address (Wikipedia file pages) and must
  // survive: only unmatched ones are text around the link.
  assert.equal(normalizeMediaUrl('https://en.wikipedia.org/wiki/File:A_(b).jpg'),
    'https://en.wikipedia.org/wiki/File:A_(b).jpg');
  // A non-breaking space is a real space: it is not silently removed, because
  // the address it came from has one.
  assert.equal(normalizeMediaUrl('https://example.test/a\u00a0b.png'), 'https://example.test/a\u00a0b.png');
  // A host in another script is sent to the server as punycode: the server
  // hands the string to DNS as it is, and «пример.рф» does not resolve. The
  // browser is the only party here that can do the conversion, so the field
  // shows the address that will really be fetched.
  assert.equal(normalizeMediaUrl('https://пример.рф/картинка.jpg'),
    'https://xn--e1afmkfd.xn--p1ai/картинка.jpg');
  assert.equal(normalizeMediaUrl('https://пример.рф'), 'https://xn--e1afmkfd.xn--p1ai');
  assert.equal(normalizeMediaUrl('HTTPS://Пример.РФ:8443/а.jpg?q=1#top'),
    'HTTPS://xn--e1afmkfd.xn--p1ai:8443/а.jpg?q=1#top');
  // Credentials are refused by the server as a whole; rewriting such an
  // address would fetch something the user did not ask for.
  assert.equal(normalizeMediaUrl('https://user:пароль@пример.рф/a.jpg'),
    'https://user:пароль@пример.рф/a.jpg');
  // Only http(s) reaches the downloader, and only a parsed address has a host
  // to convert: everything else stays as typed, for the server to refuse.
  assert.equal(normalizeMediaUrl('ftp://пример.рф/a.jpg'), 'ftp://пример.рф/a.jpg');
  assert.equal(normalizeMediaUrl('пример.рф/картинка.jpg'), 'пример.рф/картинка.jpg');
});

test('an address without a media extension is accepted, not rejected', async () => {
  const { mediaUrlKind, MEDIA_URL_MAX } = await import(moduleURL);
  // The CDN shape that used to be refused by a pre-flight check: no extension
  // in the path, the format is decided by the content.
  assert.equal(mediaUrlKind('https://images.unsplash.com/photo-1791391279415-fefa6e26f0e2?q=80&w=1336'),
    'no-extension');
  assert.equal(mediaUrlKind('https://picsum.photos/200/300'), 'no-extension');
  assert.equal(mediaUrlKind('https://example.test/a.jpg'), 'media');
  assert.equal(mediaUrlKind('https://example.test/a.jpg?w=100'), 'media');
  assert.equal(mediaUrlKind('https://example.test/download.php?id=5'), 'other-extension');
  // A version in the query string is not an extension.
  assert.equal(mediaUrlKind('https://example.test/v1.2/photo'), 'no-extension');
  assert.equal(mediaUrlKind(''), 'empty');
  assert.equal(mediaUrlKind('ftp://example.test/a.jpg'), 'invalid');
  assert.equal(mediaUrlKind('example.test/a.png'), 'invalid');
  // Something that is not an address at all: a space (including the
  // non-breaking one from a PDF), or a "%" that never became an escape. The
  // server's url.Parse refuses both, so the dialog must not send them.
  assert.equal(mediaUrlKind('https://example.test/a.png x'), 'malformed');
  assert.equal(mediaUrlKind('https://example.test/a\u00a0b.png'), 'malformed');
  assert.equal(mediaUrlKind('https://example.test/%zz'), 'malformed');
  assert.equal(mediaUrlKind('https://example.test/a%20b.png'), 'media');
  // The limit is the server's and the server counts bytes (len() in Go), so
  // the boundary is where the UTF-8 length crosses it, not the character count.
  const prefix = 'https://example.test/';
  assert.equal(mediaUrlKind(prefix + 'a'.repeat(MEDIA_URL_MAX - prefix.length)), 'no-extension');
  assert.equal(mediaUrlKind(prefix + 'a'.repeat(MEDIA_URL_MAX - prefix.length + 1)), 'too-long');
  assert.equal(mediaUrlKind(prefix + 'я'.repeat(Math.floor((MEDIA_URL_MAX - prefix.length) / 2))), 'no-extension');
  assert.equal(mediaUrlKind(prefix + 'я'.repeat(Math.ceil((MEDIA_URL_MAX - prefix.length) / 2) + 1)), 'too-long');
});

test('settings/export snapshot cannot mutate top-level panel preferences', async () => {
  const { panelSnapshot } = await import(moduleURL);
  const state = { lang: 'de', theme: 'dark', view: 'list', sort: 'name_asc', links: [{ linkName: 'wall' }] };
  const snapshot = panelSnapshot(state);
  assert.equal(Object.isFrozen(snapshot), true);
  snapshot.links[0].linkName = 'copy';
  assert.equal(state.links[0].linkName, 'wall');
});
