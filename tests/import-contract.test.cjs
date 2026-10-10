// SPDX-License-Identifier: MIT
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, '..', 'static/js/export-import.js'), 'utf8');

test('bulk import validates all batches before mutation', () => {
  const dryRun = source.indexOf('submitBatches(records, true)');
  const confirmation = source.indexOf('a.confirm(', dryRun);
  const mutation = source.indexOf('submitBatches(records, false', confirmation);
  assert.ok(dryRun >= 0 && confirmation > dryRun && mutation > confirmation);
  assert.match(source, /offset \+= 100/);
});

test('cancelled imports preserve a partial per-record report', () => {
  assert.match(source, /error\.importReport = report/);
  assert.match(source, /report\.cancelled/);
  assert.match(source, /if \(error\.importReport\) downloadReport\(error\.importReport\)/);
  assert.match(source, /await a\.reloadLinks\(\)/);
});

test('link-list exports and reports use truthful filenames', () => {
  assert.match(source, /lanpaper-link-list-/);
  assert.match(source, /lanpaper-link-list-import-report-/);
  assert.doesNotMatch(source, /lanpaper-backup-/);
});
