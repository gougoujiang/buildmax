#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";

const defaultLockfiles = [
  "gui/package-lock.json",
  "portal/package-lock.json",
  "desktop/frontend/package-lock.json",
];

const allowedLicenses = new Set([
  "Apache-2.0",
  "BSD-2-Clause",
  "BSD-3-Clause",
  "ISC",
  "MIT",
  "0BSD",
]);

const lockfiles = process.argv.slice(2);
const targets = lockfiles.length > 0 ? lockfiles : defaultLockfiles;
const failures = [];

for (const lockfile of targets) {
  const lock = JSON.parse(fs.readFileSync(lockfile, "utf8"));
  if (!lock.packages || typeof lock.packages !== "object") {
    failures.push(`${lockfile}: package-lock.json has no packages map`);
    continue;
  }

  const counts = new Map();
  let checked = 0;
  for (const [packagePath, metadata] of Object.entries(lock.packages)) {
    // The empty path is the repository's own package. Linked @buildmax/gui
    // entries point to that same local package and carry no lockfile license.
    if (!packagePath || metadata.dev === true || metadata.link === true) {
      continue;
    }

    checked += 1;
    const license = metadata.license;
    const packageName = metadata.name || path.basename(packagePath);
    if (typeof license !== "string" || license.length === 0) {
      failures.push(`${lockfile}: ${packageName} has no license metadata`);
      continue;
    }
    if (!allowedLicenses.has(license)) {
      failures.push(`${lockfile}: ${packageName} uses unapproved license ${license}`);
      continue;
    }
    counts.set(license, (counts.get(license) || 0) + 1);
  }

  const summary = [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([license, count]) => `${license}=${count}`)
    .join(", ");
  console.log(`${lockfile}: checked ${checked} production packages (${summary})`);
}

if (failures.length > 0) {
  console.error("npm production dependency license check failed:");
  for (const failure of failures) {
    console.error(`- ${failure}`);
  }
  process.exit(1);
}
