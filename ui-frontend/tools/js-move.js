#!/usr/bin/env node
// Moves top-level declarations (components, helpers) from one source file to a new
// one without touching their text:
//
//   node tools/js-move.js src/App.js src/shared/CopyButton.js CopyButton[,Other...]
//
// The moved text is copied verbatim (with its leading comments); the only edit is an
// `export ` in front. The new file gets exactly the imports its declarations use, the
// origin gets an import for whatever it still uses, and imports the origin no longer
// needs are dropped. A dependency that still lives in the origin (it would make the
// new module import its origin) is an error: move that declaration first or together.
// tools/check-moves.js then proves nothing but placement changed.
const fs = require('fs');
const path = require('path');
const parser = require('@babel/parser');
const traverse = require('@babel/traverse').default;

const parse = (src) => parser.parse(src, { sourceType: 'module', plugins: ['jsx'], tokens: false });

function topLevel(ast, src) {
  return ast.program.body.map((node, index) => {
    let names = [];
    let inner = node;
    if (node.type === 'ExportNamedDeclaration' && node.declaration) inner = node.declaration;
    if (node.type === 'ExportDefaultDeclaration') inner = node.declaration;
    if (inner.type === 'VariableDeclaration') names = inner.declarations.map((d) => d.id.name).filter(Boolean);
    if (inner.type === 'FunctionDeclaration' && inner.id) names = [inner.id.name];
    const leading = node.leadingComments && node.leadingComments.length ? node.leadingComments[0].start : node.start;
    return { index, node, names, start: leading, nodeStart: node.start, end: node.end, text: src.slice(leading, node.end), exported: node.type.startsWith('Export') };
  });
}

function depsPerStatement(ast, stmts) {
  const programScope = null;
  const out = stmts.map(() => new Set());
  const indexOf = (p) => {
    let cur = p;
    while (cur.parentPath && cur.parentPath.node !== ast.program) cur = cur.parentPath;
    return stmts.findIndex((s) => s.node === cur.node);
  };
  let scope0 = null;
  traverse(ast, {
    Program(p) { scope0 = p.scope; },
    ReferencedIdentifier(p) {
      const name = p.node.name;
      const binding = p.scope.getBinding(name);
      if (!binding || binding.scope !== scope0) return;
      const i = indexOf(p);
      if (i < 0) return;
      // a reference to a binding declared by the same statement is internal
      if (stmts[i].names.includes(name)) return;
      out[i].add(name);
    },
  });
  void programScope;
  return out;
}

function importsOf(ast) {
  const map = new Map(); // local name -> { source, imported, kind }
  ast.program.body.forEach((n) => {
    if (n.type !== 'ImportDeclaration') return;
    n.specifiers.forEach((s) => {
      const kind = s.type === 'ImportDefaultSpecifier' ? 'default' : s.type === 'ImportNamespaceSpecifier' ? 'namespace' : 'named';
      map.set(s.local.name, { source: n.source.value, imported: s.imported ? s.imported.name : null, kind });
    });
  });
  return map;
}

function exportsRegistry(srcRoot, exceptFile) {
  const reg = new Map();
  const walk = (dir) => fs.readdirSync(dir, { withFileTypes: true }).forEach((e) => {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) { if (e.name !== '__snapshots__' && e.name !== '__fixtures__') walk(p); return; }
    if (!e.name.endsWith('.js') || e.name.endsWith('.test.js') || p === exceptFile) return;
    const src = fs.readFileSync(p, 'utf8');
    topLevel(parse(src), src).forEach((s) => { if (s.exported && s.node.type === 'ExportNamedDeclaration') s.names.forEach((n) => reg.set(n, p)); });
  });
  walk(srcRoot);
  return reg;
}

const rel = (fromFile, toFile) => {
  let r = path.relative(path.dirname(fromFile), toFile).replace(/\.js$/, '');
  if (!r.startsWith('.')) r = `./${r}`;
  return r;
};

function renderImports(items) { // items: [{source, kind, imported, local}]
  const bySource = new Map();
  items.forEach((it) => {
    if (!bySource.has(it.source)) bySource.set(it.source, { def: null, ns: null, named: [] });
    const b = bySource.get(it.source);
    if (it.kind === 'default') b.def = it.local;
    else if (it.kind === 'namespace') b.ns = it.local;
    else b.named.push(it.imported === it.local ? it.local : `${it.imported} as ${it.local}`);
  });
  return [...bySource.entries()].map(([source, b]) => {
    const parts = [];
    if (b.def) parts.push(b.def);
    if (b.ns) parts.push(`* as ${b.ns}`);
    if (b.named.length) parts.push(`{ ${[...new Set(b.named)].join(', ')} }`);
    return `import ${parts.join(', ')} from '${source}';`;
  });
}

function main() {
  const [from, to, namesArg] = process.argv.slice(2);
  if (!from || !to || !namesArg) { console.error('usage: js-move.js <from> <to> <Name[,Name...]>'); process.exit(2); }
  if (fs.existsSync(to)) { console.error(`${to} exists; refusing to overwrite`); process.exit(2); }
  const names = namesArg.split(',');
  const src = fs.readFileSync(from, 'utf8');
  const ast = parse(src);
  const stmts = topLevel(ast, src);
  const deps = depsPerStatement(ast, stmts);
  const imports = importsOf(ast);
  const srcRoot = 'src';
  const registry = exportsRegistry(srcRoot, path.resolve(to));

  const moving = stmts.filter((s) => s.names.some((n) => names.includes(n)));
  const missing = names.filter((n) => !moving.some((s) => s.names.includes(n)));
  if (missing.length) { console.error(`not found at top level of ${from}: ${missing.join(', ')}`); process.exit(2); }
  const movedNames = new Set(moving.flatMap((s) => s.names));

  // imports for the new file
  const need = new Map(); // local -> item
  const stays = new Set(stmts.filter((s) => !moving.includes(s)).flatMap((s) => s.names));
  const problems = [];
  moving.forEach((s) => deps[s.index].forEach((d) => {
    if (movedNames.has(d) || need.has(d)) return;
    if (imports.has(d)) {
      const im = imports.get(d);
      const source = im.source.startsWith('.') ? rel(to, path.resolve(path.dirname(from), im.source)) : im.source;
      need.set(d, { source, kind: im.kind, imported: im.imported, local: d });
    } else if (stays.has(d)) {
      if (registry.has(d)) need.set(d, { source: rel(to, registry.get(d)), kind: 'named', imported: d, local: d });
      else problems.push(`${s.names.join(',')} uses ${d}, which still lives in ${from}`);
    } else if (registry.has(d)) {
      need.set(d, { source: rel(to, registry.get(d)), kind: 'named', imported: d, local: d });
    }
  }));
  if (problems.length) { console.error(`cannot move:\n  ${problems.join('\n  ')}\nMove those first or in the same call.`); process.exit(1); }

  // new file
  const body = moving.map((s) => {
    const off = s.nodeStart - s.start;
    return `${s.text.slice(0, off)}export ${s.text.slice(off)}`;
  }).join('\n\n');
  const header = renderImports([...need.values()]);
  fs.mkdirSync(path.dirname(to), { recursive: true });
  fs.writeFileSync(to, `${header.join('\n')}${header.length ? '\n\n' : ''}${body}\n`);

  // origin: remove the statements, import what is still used, drop unused imports
  let out = src;
  [...moving].sort((a, b) => b.start - a.start).forEach((s) => {
    let end = s.end;
    while (out[end] === '\n') end += 1;
    out = out.slice(0, s.start) + out.slice(end);
  });
  const stillUsed = [...movedNames].filter((n) => stmts.some((s) => !moving.includes(s) && deps[s.index].has(n)));
  const ast2a = parse(out);
  const lastImport = ast2a.program.body.filter((n) => n.type === 'ImportDeclaration').pop();
  if (stillUsed.length) {
    const line = `import { ${stillUsed.join(', ')} } from '${rel(from, to)}';\n`;
    out = out.slice(0, lastImport.end + 1) + line + out.slice(lastImport.end + 1);
  }
  // prune unused import specifiers
  const ast3 = parse(out);
  const unused = new Set();
  traverse(ast3, {
    Program(p) {
      Object.values(p.scope.bindings).forEach((b) => {
        if (b.path.isImportSpecifier() || b.path.isImportDefaultSpecifier() || b.path.isImportNamespaceSpecifier()) {
          if (!b.referenced) unused.add(b.identifier.name);
        }
      });
    },
  });
  const edits = [];
  ast3.program.body.filter((n) => n.type === 'ImportDeclaration').forEach((n) => {
    const keep = n.specifiers.filter((s) => !unused.has(s.local.name));
    if (keep.length === n.specifiers.length) return;
    const items = keep.map((s) => ({ source: n.source.value, kind: s.type === 'ImportDefaultSpecifier' ? 'default' : s.type === 'ImportNamespaceSpecifier' ? 'namespace' : 'named', imported: s.imported ? s.imported.name : null, local: s.local.name }));
    edits.push({ start: n.start, end: n.end + (keep.length ? 0 : 1), text: keep.length ? renderImports(items)[0] : '' });
  });
  edits.sort((a, b) => b.start - a.start).forEach((e) => { out = out.slice(0, e.start) + e.text + out.slice(e.end); });
  fs.writeFileSync(from, out);
  console.log(`moved ${[...movedNames].join(', ')} -> ${to}` + (stillUsed.length ? ` (imported back into ${from}: ${stillUsed.join(', ')})` : ''));
  if (unused.size) console.log(`dropped now-unused imports from ${from}: ${[...unused].join(', ')}`);
}

main();
