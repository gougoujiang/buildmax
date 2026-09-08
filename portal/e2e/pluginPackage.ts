import { gzipSync } from "node:zlib"

/**
 * Builds the bytes `POST /api/admin/plugins/{name}/releases` accepts: a
 * gzipped tar, read back with Go's own archive/tar and compress/gzip.
 *
 * Publishing a release is deliberately not a Portal form -- see
 * AdminPlugins.tsx -- so the only way an e2e spec can seed one is the bytes
 * the CLI would send. This writes a minimal classic ustar archive by hand
 * rather than pulling in a tar dependency for one test: two small files,
 * which is all `plugin.yaml` plus one skill needs.
 */
export function buildPluginPackage(manifest: string): Buffer {
  const entries = Buffer.concat([
    tarEntry("plugin.yaml", manifest),
    tarEntry("skills/review/SKILL.md", "# review\n"),
    // Two 512-byte zero blocks mark the end of a tar archive.
    Buffer.alloc(1024),
  ])
  return gzipSync(entries)
}

function tarEntry(name: string, content: string): Buffer {
  const data = Buffer.from(content, "utf8")
  const header = tarHeader(name, data.length)
  const padLen = (512 - (data.length % 512)) % 512
  return Buffer.concat([header, data, Buffer.alloc(padLen)])
}

function tarHeader(name: string, size: number): Buffer {
  const buf = Buffer.alloc(512)
  buf.write(name, 0, "ascii")
  writeOctal(buf, 100, 8, 0o644) // mode
  writeOctal(buf, 108, 8, 0) // uid
  writeOctal(buf, 116, 8, 0) // gid
  writeOctal(buf, 124, 12, size)
  writeOctal(buf, 136, 12, Math.floor(Date.now() / 1000)) // mtime
  buf.fill(0x20, 148, 156) // chksum field, computed with spaces in place
  buf[156] = "0".charCodeAt(0) // typeflag: regular file
  buf.write("ustar", 257, "ascii")
  buf.write("00", 263, "ascii") // ustar version, no NUL terminator

  let sum = 0
  for (let i = 0; i < 512; i += 1) sum += buf[i]
  buf.write(`${sum.toString(8).padStart(6, "0")}\0 `, 148, "ascii")
  return buf
}

/** An octal ASCII field, NUL-terminated, left-padded with zeros. */
function writeOctal(buf: Buffer, offset: number, length: number, value: number): void {
  buf.write(value.toString(8).padStart(length - 1, "0"), offset, "ascii")
  buf[offset + length - 1] = 0
}
