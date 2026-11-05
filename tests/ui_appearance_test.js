#!/usr/bin/env node
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const html = fs.readFileSync('web/index.html', 'utf8');
const matches = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)];
assert.equal(matches.length, 1, 'expected one embedded UI script');
const script = matches[0][1];
new vm.Script(script, { filename: 'web/index.html:inline-script' });

assert.match(html, /<option value="custom">Kustom<\/option>/, 'custom mode is selectable');
assert.match(html, /id="customBackgroundFile" type="file" accept="image\/png,image\/jpeg,image\/webp"/, 'file picker restricts image formats');
assert.match(html, /id="applyCustomBackground"[^>]*>Terapkan<\/button>/, 'apply action is present');
assert.match(html, /id="resetCustomBackground"[^>]*>Reset<\/button>/, 'reset action is present');
assert.match(html, /id="chooseCustomBackground"[^>]*tabindex="0"[^>]*role="button"/, 'file picker is keyboard-focusable');
assert.match(html, /indexedDB\.open\('xray-panel-local-appearance'/, 'image storage is browser-local IndexedDB');
assert.match(html, /\.put\(blob,'custom-background'\)/, 'image is stored locally, not sent to the panel');
assert.match(html, /body\[data-background="custom"\].*backdrop-filter:blur\(12px\)/, 'glass styling is scoped to custom mode');
assert.match(html, /backdrop-filter:blur\(8px\)/, 'mobile blur is reduced');
assert.doesNotMatch(script, /FormData|fetch\([^\n]*(?:background|image)/i, 'no image upload request is implemented');

const validatorLine = script.match(/^async function validateCustomImage\(file\)\{[^\n]*\}$/m)?.[0];
assert.ok(validatorLine, 'image validator is present');
let dimensions = [1920, 1080];
const validate = vm.runInNewContext(`${validatorLine}; validateCustomImage`, {
  imageDimensions: async () => dimensions,
});
(async () => {
  const accepted = { type: 'image/webp', size: 1024 };
  assert.equal(await validate(accepted), accepted, 'valid WebP is accepted');
  await assert.rejects(validate({ type: 'image/svg+xml', size: 1024 }), /PNG, JPEG, atau WebP/);
  await assert.rejects(validate({ type: 'image/png', size: 5 * 1024 * 1024 + 1 }), /5 MB/);
  await assert.rejects(validate({ type: 'image/jpeg', size: 0 }), /1 byte/);
  dimensions = [9000, 6000];
  await assert.rejects(validate({ type: 'image/png', size: 1024 }), /40 megapiksel/);
  dimensions = [12001, 1];
  await assert.rejects(validate({ type: 'image/png', size: 1024 }), /12.000 px/);
  console.log('UI appearance tests passed (syntax, local storage contract, styling scope, file validation).');
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
