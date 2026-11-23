import { cp, mkdir, readdir, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';

// PDF.js resources are served locally in both browser and Wails builds. Never
// depend on a public CDN, or ship the optional QuickJS scripting sandbox.
const source = fileURLToPath(new URL('../node_modules/pdfjs-dist/', import.meta.url));
const target = fileURLToPath(new URL('../public/pdfjs/', import.meta.url));
await rm(target, { recursive: true, force: true });
await mkdir(target, { recursive: true });
for (const directory of ['cmaps', 'standard_fonts', 'iccs']) {
  await cp(join(source, directory), join(target, directory), { recursive: true });
}
await mkdir(join(target, 'wasm'), { recursive: true });
for (const name of await readdir(join(source, 'wasm'))) {
  if (/^(jbig2|openjpeg|qcms|LICENSE_)/.test(name)) {
    await cp(join(source, 'wasm', name), join(target, 'wasm', name));
  }
}
await cp(join(source, 'LICENSE'), join(target, 'LICENSE'));
