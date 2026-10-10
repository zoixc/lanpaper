// SPDX-License-Identifier: MIT
'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');

const ci = fs.readFileSync('.github/workflows/ci.yml', 'utf8');
const release = fs.readFileSync('.github/workflows/release.yml', 'utf8');

function assertSupplyChainWorkflow(workflow, name) {
  assert.match(workflow, /id-token:\s*write/, `${name} must permit keyless OIDC signing`);
  assert.match(workflow, /--provenance=mode=max/, `${name} must publish full provenance`);
  assert.match(workflow, /--sbom=true/, `${name} must attach an image SBOM`);
  assert.match(workflow, /--metadata-file=/, `${name} must capture the pushed digest`);
  assert.match(workflow, /containerimage\.digest/, `${name} must read the immutable digest`);
  assert.match(workflow, /cosign sign --yes "ptabi\/lanpaper@\$\{DIGEST\}"/,
    `${name} must sign by digest, never by mutable tag`);
}

test('main and tag publication both attest and keyless-sign immutable images', () => {
  assertSupplyChainWorkflow(ci, 'CI workflow');
  assertSupplyChainWorkflow(release, 'release workflow');
});

test('release formatting excludes generated vendored source', () => {
  assert.match(release, /find \. -path \.\/vendor -prune/);
  assert.doesNotMatch(release, /gofmt -l \.\)/);
});

test('release binary version comes from VERSION in the build shell', () => {
  assert.match(release, /version="\$\(cat VERSION\)"[\s\S]*?-X main\.Version=\$\{version\}/);
  assert.doesNotMatch(release, /main\.Version=\$\{VERSION\}/);
});
