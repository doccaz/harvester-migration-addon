#!/usr/bin/env node
// Proves a refactor only moved code: every top-level declaration of a file at a base
// git ref must exist, textually identical (whitespace-insensitive, ignoring a leading
// `export`), somewhere in src/ now.
//
//   node tools/check-moves.js [baseRef] [file]      (defaults: HEAD, ui-frontend/src/App.js)
//   --allow A,B   declarations deliberately changed or removed
const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');
const parser = require('@babel/parser');

const parse = (src) => parser.parse(src, { sourceType: 'module', plugins: ['jsx'] });
const norm = (t) => t.replace(/\s+/g, ' ').trim();

function decls(src) {
  const ast = parse(src);
  const out = new Map();
  ast.program.body.forEach((node) => {
    if (node.type === 'ImportDeclaration') return;
    let inner = node;
    if (node.type === 'ExportNamedDeclaration' && node.declaration) inner = node.declaration;
    if (node.type === 'ExportDefaultDeclaration') inner = node.declaration;
    let names = [];
    if (inner.type === 'VariableDeclaration') names = inner.declarations.map((d) => d.id.name);
    if (inner.type === 'FunctionDeclaration' && inner.id) names = [inner.id.name];
    const start = node.leadingComments && node.leadingComments.length ? node.leadingComments[0].start : node.start;
    // the `export` keyword is the one allowed difference: drop it from the declaration itself
    const text = norm(src.slice(start, node.start) + src.slice(node.start, node.end).replace(/^export\s+(default\s+)?/, ''));
    names.forEach((n) => out.set(n, text));
  });
  return out;
}

const args = process.argv.slice(2);
const allowIdx = args.indexOf('--allow');
const allow = new Set(allowIdx >= 0 ? args.splice(allowIdx, 2)[1].split(',') : []);
const [ref = 'HEAD', file = 'ui-frontend/src/App.js'] = args;
const root = execFileSync('git', ['rev-parse', '--show-toplevel']).toString().trim();
const base = decls(execFileSync('git', ['show', `${ref}:${file}`], { cwd: root, maxBuffer: 1 << 28 }).toString());

const now = new Map();
const walk = (dir) => fs.readdirSync(dir, { withFileTypes: true }).forEach((e) => {
  const p = path.join(dir, e.name);
  if (e.isDirectory()) { if (!['__snapshots__', '__fixtures__', 'testing'].includes(e.name)) walk(p); return; }
  if (!e.name.endsWith('.js') || e.name.endsWith('.test.js')) return;
  decls(fs.readFileSync(p, 'utf8')).forEach((t, n) => { if (!now.has(n)) now.set(n, []); now.get(n).push({ file: p, text: t }); });
});
walk(path.join(root, path.dirname(file)));

let bad = 0;
base.forEach((text, name) => {
  if (allow.has(name)) return;
  const found = now.get(name);
  if (!found) { console.log(`MISSING  ${name}`); bad += 1; return; }
  if (!found.some((f) => f.text === text)) { console.log(`CHANGED  ${name}  (${found.map((f) => path.relative(root, f.file)).join(', ')})`); bad += 1; }
});
const added = [...now.keys()].filter((n) => !base.has(n));
console.log(`${base.size} declarations at ${ref}:${file}; ${base.size - bad} found unchanged, ${bad} problem(s)` + (added.length ? `; new: ${added.join(', ')}` : ''));
process.exit(bad ? 1 : 0);
