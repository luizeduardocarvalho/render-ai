#!/usr/bin/env node
// Fails (non-zero exit) if the locale JSON files under src/i18n/locales don't
// all carry exactly the same set of translation keys. Run via `pnpm run
// check:i18n`.

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const localesDir = path.join(__dirname, "..", "src", "i18n", "locales");

const LOCALE_FILES = ["en.json", "pt-BR.json"];

function flattenKeys(obj, prefix = "") {
  const keys = [];
  for (const [key, value] of Object.entries(obj)) {
    const fullKey = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === "object" && !Array.isArray(value)) {
      keys.push(...flattenKeys(value, fullKey));
    } else {
      keys.push(fullKey);
    }
  }
  return keys;
}

function loadKeySet(fileName) {
  const filePath = path.join(localesDir, fileName);
  const raw = readFileSync(filePath, "utf8");
  const json = JSON.parse(raw);
  return new Set(flattenKeys(json));
}

const keySets = LOCALE_FILES.map((file) => ({ file, keys: loadKeySet(file) }));
const [reference, ...rest] = keySets;

let ok = true;

for (const { file, keys } of rest) {
  const missing = [...reference.keys].filter((k) => !keys.has(k));
  const extra = [...keys].filter((k) => !reference.keys.has(k));

  if (missing.length > 0) {
    ok = false;
    console.error(`\n${file} is missing ${missing.length} key(s) present in ${reference.file}:`);
    for (const k of missing) console.error(`  - ${k}`);
  }
  if (extra.length > 0) {
    ok = false;
    console.error(`\n${file} has ${extra.length} extra key(s) not present in ${reference.file}:`);
    for (const k of extra) console.error(`  - ${k}`);
  }
}

if (!ok) {
  console.error("\nLocale files are out of sync. Every locale must have exactly the same key set.");
  process.exit(1);
}

console.log(`OK: ${LOCALE_FILES.join(", ")} all have the same ${reference.keys.size} translation keys.`);
