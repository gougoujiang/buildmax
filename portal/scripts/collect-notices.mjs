// Collect third-party license notices for the Portal image.
//
// The Go binaries ship NOTICE-THIRD-PARTY (see tools/mk/licenses.go); the Portal
// image ships a JavaScript bundle whose npm dependencies carry the same
// attribution obligation. This lives in node, not tools/mk, because it runs
// inside the Docker build stage, which has node_modules and no Go toolchain.
//
// It reads each root's package-lock.json, takes every production package
// (skipping dev and link entries — links are first-party workspace code), and
// concatenates the license file found in that package's node_modules
// directory. Covering every production dependency of both gui and portal is a
// superset of what the bundler emits; over-attribution is safe, silence is not.
//
// Usage: node collect-notices.mjs --out <file> <root>...

import { readFile, readdir, writeFile, mkdir } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const SEPARATOR = "=".repeat(64);

const HEADER = `BuildMax Portal third-party notices

The BuildMax Portal is licensed under Apache-2.0; see LICENSE in the
source repository. The JavaScript served by this distribution bundles
the npm packages below. Each package's own license text follows,
unmodified.

A summary of the license mix, and how to regenerate this file, is in
docs/contribute/dependency-licenses.md in the source repository.

`;

function fail(message) {
  console.error(`collect-notices: ${message}`);
  process.exit(1);
}

function parseArgs(argv) {
  let out = "";
  const roots = [];
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--out") {
      out = argv[++i] ?? "";
    } else {
      roots.push(argv[i]);
    }
  }
  if (!out || roots.length === 0) {
    fail("usage: node collect-notices.mjs --out <file> <root>...");
  }
  return { out, roots };
}

// A package's name is the path segment after the last node_modules/, so nested
// installs (node_modules/a/node_modules/b) resolve to the inner package.
function packageName(lockPath) {
  const parts = lockPath.split("node_modules/");
  return parts[parts.length - 1];
}

async function licenseFiles(dir) {
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return [];
  }
  return entries
    .filter((e) => e.isFile() && /^(licen[cs]e|copying|notice)/i.test(e.name))
    .map((e) => path.join(dir, e.name))
    .sort();
}

async function collectRoot(root, packages) {
  const lockfile = path.join(root, "package-lock.json");
  let lock;
  try {
    lock = JSON.parse(await readFile(lockfile, "utf8"));
  } catch (err) {
    fail(`read ${lockfile}: ${err.message}`);
  }
  const entries = lock.packages ?? {};
  for (const [lockPath, metadata] of Object.entries(entries)) {
    if (!lockPath.startsWith("node_modules/") || metadata.dev || metadata.link) {
      continue;
    }
    const name = packageName(lockPath);
    const key = `${name}@${metadata.version ?? "unknown"}`;
    if (packages.has(key)) {
      continue;
    }
    const files = await licenseFiles(path.join(root, lockPath));
    let body;
    if (files.length > 0) {
      const texts = await Promise.all(files.map((f) => readFile(f, "utf8")));
      body = texts.join("\n");
    } else {
      // Some packages declare a license without shipping the text; record the
      // declaration rather than dropping the package silently.
      const license = typeof metadata.license === "string" ? metadata.license : "unknown";
      body = `License: ${license} (declared in package metadata; the package ships no license file)\n`;
    }
    packages.set(key, body);
  }
}

const { out, roots } = parseArgs(process.argv.slice(2));
const packages = new Map();
for (const root of roots) {
  await collectRoot(root, packages);
}
if (packages.size === 0) {
  fail("no production packages found; refusing to write an empty notice file");
}

let doc = HEADER;
for (const key of [...packages.keys()].sort()) {
  doc += `${SEPARATOR}\n${key}\n${SEPARATOR}\n\n${packages.get(key)}\n`;
}
await mkdir(path.dirname(out), { recursive: true });
await writeFile(out, doc);
console.log(`wrote ${out} (${packages.size} packages)`);
