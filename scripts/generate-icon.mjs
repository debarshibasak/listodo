// Regenerate the native icon from the same SVG used in the interface.
// Run on macOS after installing the frontend's Playwright Chromium.
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
const root = fileURLToPath(new URL('..', import.meta.url))
const require = createRequire(new URL('../frontend/package.json', import.meta.url))
const { chromium } = require('playwright')
const browser = await chromium.launch({ args: ['--single-process', '--no-zygote', '--disable-gpu'] })
try {
  const page = await browser.newPage({ viewport: { width: 1024, height: 1024 }, deviceScaleFactor: 1 })
  const svg = readFileSync(resolve(root, 'frontend/public/favicon.svg'), 'utf8')
    .replace('viewBox="0 0 64 64"', 'viewBox="-7 -7 78 78"')
  await page.setContent(`<style>body{margin:0}svg{display:block;width:1024px;height:1024px}</style>${svg}`)
  await page.screenshot({ path: resolve(root, 'frontend/public/app-icon.png'), omitBackground: true })
} finally {
  await browser.close()
}
const temporary = mkdtempSync(join(tmpdir(), 'listodo-icon-'))
try {
  mkdirSync(resolve(root, 'assets'), { recursive: true })
  // Modern ICNS stores PNG representations in a length-prefixed icon family.
  // Build it directly because iconutil's encoder is unavailable on some hosts.
  const chunk = (type, data) => {
    const header = Buffer.alloc(8)
    header.write(type, 0, 4, 'ascii')
    header.writeUInt32BE(data.length + 8, 4)
    return Buffer.concat([header, data])
  }
  const representations = []
  for (const [type, size] of [
    ['icp4', 16], ['icp5', 32], ['icp6', 64], ['ic07', 128],
    ['ic08', 256], ['ic09', 512], ['ic10', 1024],
    ['ic11', 32], ['ic12', 64], ['ic13', 256], ['ic14', 512],
  ]) {
    const target = join(temporary, `${type}.png`)
    execFileSync('sips', ['-z', String(size), String(size),
      resolve(root, 'frontend/public/app-icon.png'), '--out', target], { stdio: 'ignore' })
    representations.push(chunk(type, readFileSync(target)))
  }
  writeFileSync(resolve(root, 'assets/Listodo.icns'), chunk('icns', Buffer.concat(representations)))
} finally {
  rmSync(temporary, { recursive: true, force: true })
}
