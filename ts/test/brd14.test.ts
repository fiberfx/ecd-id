import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  BRD14_EPOCH,
  type Eid,
  fromBrd14,
  fromBrd14Registered,
  parse,
  toBrd14,
  toBrd14Registered,
} from '../src/index.js';
import { outcome, reportChecked, rows, unhex } from './vectors.js';

const BASE62 = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz';
const REASONS = [
  'brd14-length',
  'brd14-namespace',
  'brd14-alphabet',
  'brd14-range',
  'brd14-system',
  'brd14-time',
  'brd14-writer',
  'brd14-count',
  'brd14-kind',
];

/** The kinds that registry/kinds.tsv gives the system brd. */
function brdKinds(): Set<string> {
  const text = readFileSync(new URL('../../registry/kinds.tsv', import.meta.url), 'utf8');
  const lines = text.split('\n').filter((l) => l !== '' && !l.startsWith('#'));
  expect(lines[0]).toBe('system\tkind\trole\tstate\tserved_by\tmeaning');
  const cells = lines.slice(1).map((l) => l.split('\t'));
  return new Set(cells.filter((c) => c[0] === 'brd').map((c) => c[1]!));
}

/** The text of what a call returns, or `!reason` when it refuses. */
function answer(f: () => Eid | string): string {
  let out = '';
  const r = outcome(() => {
    out = String(f());
  });
  return r === 'ok' ? out : `!${r}`;
}

function cmp(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

describe('vectors/brd14.tsv', () => {
  const data = rows('brd14.tsv');
  const kinds = brdKinds();

  test('every row both ways, pure and through the registry layer, or refused with its reason', () => {
    let checked = 0;
    const seen = new Set<string>();
    for (const [name, op, input, want, wantRegistry] of data) {
      let got: string;
      let gotRegistry: string;
      if (op === 'from') {
        const b = unhex(input!);
        got = answer(() => fromBrd14(b));
        gotRegistry = answer(() => fromBrd14Registered(b, kinds));
      } else if (op === 'to') {
        const id = parse(input!);
        got = answer(() => toBrd14(id));
        gotRegistry = answer(() => toBrd14Registered(id, kinds));
      } else {
        throw new Error(`case ${name}: op ${op}`);
      }
      expect([name, op, got, gotRegistry]).toEqual([name, op, want, wantRegistry]);
      for (const a of [want!, wantRegistry!]) if (a.startsWith('!')) seen.add(a.slice(1));
      checked++;
    }
    for (const r of REASONS) expect(seen.has(r)).toBe(true);
    reportChecked('brd14.tsv', checked, data.length);
    expect(data.length).toBeGreaterThan(0);
    expect(checked).toBe(data.length);
  });

  test('byte order of the embedded ids equals the order of the brd14 texts; random ids return', () => {
    const pairs = data
      .filter((r) => r[1] === 'to' && !r[3]!.startsWith('!'))
      .map((r) => [r[3]!, parse(r[2]!)] as const);
    expect(pairs.length).toBeGreaterThan(50);
    let bad = 0;
    for (const [a, x] of pairs) for (const [b, y] of pairs) if (cmp(a, b) !== x.compare(y)) bad++;
    expect(bad).toBe(0);
    // splitmix64 over BigInt, seeded as the Go and Rust tests are
    let s = 14n;
    const M = (1n << 64n) - 1n;
    const next = (): bigint => {
      s = (s + 0x9e3779b97f4a7c15n) & M;
      let z = s;
      z = ((z ^ (z >> 30n)) * 0xbf58476d1ce4e5b9n) & M;
      z = ((z ^ (z >> 27n)) * 0x94d049bb133111ebn) & M;
      return z ^ (z >> 31n);
    };
    let prev: [string, Eid] | undefined;
    const n = 10000;
    for (let k = 0; k < n; k++) {
      let text = '';
      for (let i = 0; i < 3; i++) text += String.fromCharCode(65 + Number(next() % 26n));
      let v = next() >> 1n;
      const fields = [Number(v >> 22n), Number((v >> 12n) & 1023n), Number(v & 4095n)];
      let digits = '';
      for (let i = 0; i < 11; i++) {
        digits = BASE62.charAt(Number(v % 62n)) + digits;
        v /= 62n;
      }
      text += digits;
      const id = fromBrd14(text);
      expect([id.ms, id.writer, id.count]).toEqual([BRD14_EPOCH + fields[0]!, fields[1]!, fields[2]!]);
      expect(toBrd14(id)).toBe(text);
      if (prev !== undefined) expect(prev[1].compare(id)).toBe(cmp(prev[0], text));
      prev = [text, id];
    }
    console.log(`BRD14 ${pairs.length} vector ids keep brd14 order pairwise; ${n} random ids return and keep order`);
  });
});
