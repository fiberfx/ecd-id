import { describe, expect, test } from 'bun:test';
import {
  Eid,
  formatLease,
  formatStreamId,
  fromBytes,
  fromParts,
  fromStreamId,
  legacyTail,
  parse,
  parseLease,
  parseStreamId,
  toStreamId,
  valid,
  validBinary,
  writerFor,
} from '../src/index.js';
import { hex, outcome, reportChecked, rows, unhex } from './vectors.js';

describe('vectors/valid.tsv', () => {
  const data = rows('valid.tsv');
  test('every id parses, formats, converts to bytes and back, and has the stated fields', () => {
    let checked = 0;
    for (const [text, bytes, system, kind, ms, writer, count, cls, stream] of data) {
      const id = parse(text!);
      expect(hex(id.bytes)).toBe(bytes!);
      expect(id.toString()).toBe(text!);
      expect(fromBytes(unhex(bytes!)).toString()).toBe(text!);
      expect(id.system).toBe(system!);
      expect(id.kind).toBe(kind!);
      expect(id.reserved).toBe(0);
      expect(String(id.ms)).toBe(ms!);
      expect(String(id.writer)).toBe(writer!);
      expect(String(id.count)).toBe(count!);
      expect(id.class).toBe(cls! as 'registered' | 'detached');
      expect(formatStreamId(toStreamId(id))).toBe(stream!);
      const rebuilt = fromParts({ system: id.system, kind: id.kind, ms: id.ms, writer: id.writer, count: id.count });
      expect(rebuilt.equals(id)).toBe(true);
      expect(valid(text!)).toBe(true);
      expect(validBinary(unhex(bytes!))).toBe(true);
      checked++;
    }
    reportChecked('valid.tsv', checked, data.length);
    expect(data.length).toBeGreaterThan(0);
    expect(checked).toBe(data.length);
  });

  test('text order equals byte order for consecutive rows', () => {
    const ids = data.map((r) => parse(r[0]!));
    for (let i = 1; i < ids.length; i++) {
      const byBytes = Math.sign(ids[i - 1]!.compare(ids[i]!));
      const a = ids[i - 1]!.toString();
      const b = ids[i]!.toString();
      expect(Math.sign(a < b ? -1 : a > b ? 1 : 0)).toBe(byBytes);
    }
  });
});

describe('vectors/invalid.tsv', () => {
  const data = rows('invalid.tsv');
  test('every text is refused with the reason of its first failing step', () => {
    let checked = 0;
    const seen = new Set<string>();
    for (const [, inputHex, reason] of data) {
      expect(outcome(() => parse(unhex(inputHex!)))).toBe(reason!);
      expect(valid(unhex(inputHex!))).toBe(false);
      seen.add(reason!);
      checked++;
    }
    for (const r of ['length', 'separator', 'alphabet', 'pad', 'reserved', 'system-reserved', 'kind']) {
      expect(seen.has(r)).toBe(true);
    }
    reportChecked('invalid.tsv', checked, data.length);
    expect(data.length).toBeGreaterThan(0);
    expect(checked).toBe(data.length);
  });
});

describe('vectors/binary_invalid.tsv', () => {
  const data = rows('binary_invalid.tsv');
  test('every 16-byte input is refused at binary ingress with its reason', () => {
    let checked = 0;
    for (const [, inputHex, reason] of data) {
      expect(outcome(() => fromBytes(unhex(inputHex!)))).toBe(reason!);
      expect(outcome(() => new Eid(unhex(inputHex!)))).toBe(reason!);
      expect(validBinary(unhex(inputHex!))).toBe(false);
      checked++;
    }
    reportChecked('binary_invalid.tsv', checked, data.length);
    expect(data.length).toBeGreaterThan(0);
    expect(checked).toBe(data.length);
  });
});

describe('vectors/lease.tsv', () => {
  const data = rows('lease.tsv');
  test('every row: lease texts parse or are refused with their reason; writer rows give the writer number', () => {
    let checked = 0;
    for (const r of data) {
      if (r[0] === 'writer') {
        expect(String(writerFor(Number(r[1]), Number(r[2])))).toBe(r[3]!);
      } else {
        const text = r[1]!;
        const expect_ = r[2]!;
        const got = outcome(() => parseLease(text));
        expect(got === 'ok' ? 'ok' : `!${got}`).toBe(expect_);
        if (got === 'ok') expect(formatLease(parseLease(text))).toBe(text);
      }
      checked++;
    }
    reportChecked('lease.tsv', checked, data.length);
    expect(data.length).toBeGreaterThan(0);
    expect(checked).toBe(data.length);
  });
});

describe('vectors/stream.tsv', () => {
  const data = rows('stream.tsv');
  test('every id maps to its stream id and legacy tail, and the stream id returns the id', () => {
    let checked = 0;
    for (const [text, kind, stream, tail] of data) {
      const id = parse(text!);
      const sid = toStreamId(id);
      expect(formatStreamId(sid)).toBe(stream!);
      expect(String(legacyTail(id))).toBe(tail!);
      expect(sid.seq < 1n << 63n).toBe(true);
      const back = fromStreamId(kind!, parseStreamId(stream!));
      expect(back.toString()).toBe(text!);
      expect(back.equals(id)).toBe(true);
      checked++;
    }
    const first4 = data.slice(0, 4);
    expect(new Set(first4.map((r) => r[2])).size).toBe(4);
    expect(new Set(first4.map((r) => r[3])).size).toBe(1);
    reportChecked('stream.tsv', checked, data.length);
    expect(data.length).toBeGreaterThan(0);
    expect(checked).toBe(data.length);
  });
});

describe('vectors/stream_invalid.tsv', () => {
  const data = rows('stream_invalid.tsv');
  test('every stream id is refused with its reason', () => {
    let checked = 0;
    for (const [, kind, stream, reason] of data) {
      const text = stream === '-' ? '' : stream!;
      expect(outcome(() => fromStreamId(kind!, parseStreamId(text)))).toBe(reason!);
      checked++;
    }
    reportChecked('stream_invalid.tsv', checked, data.length);
    expect(data.length).toBeGreaterThan(0);
    expect(checked).toBe(data.length);
  });
});
