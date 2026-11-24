import { describe, expect, test } from 'bun:test';
import {
  ALPHABET,
  Eid,
  Refusal,
  SYS,
  codeClass,
  codeToString,
  compare,
  format,
  formatLease,
  formatStreamId,
  fromBytes,
  fromParts,
  fromStreamId,
  isKind,
  legacyTail,
  newLease,
  parse,
  parseCode,
  parseLease,
  parseStreamId,
  streamLess,
  systemId,
  toStreamId,
  valid,
  validBinary,
  valkeyAccepts,
  writerFor,
} from '../src/index.js';
import { hex, outcome, unhex } from './vectors.js';

const WORKED = 'emq_job_23obzcbapvupu2272222';
const WORKED_HEX = '54acfa1c01a1fe839abbd57400140000';
const WRITERMAX = 2 ** 30 - 1;

describe('codes', () => {
  test('parseCode and codeToString round trip; classes and kinds follow the first characters', () => {
    expect(parseCode('emq')).toBe(parseCode('emq') as number);
    expect(codeToString(parseCode('emq') as number)).toBe('emq');
    expect(parseCode('222')).toBe(0);
    expect(parseCode('zzz')).toBe(32767);
    expect(codeClass(parseCode('emq') as number)).toBe('registered');
    expect(codeClass(parseCode('3ab') as number)).toBe('detached');
    expect(codeClass(parseCode('7zz') as number)).toBe('detached');
    expect(codeClass(parseCode('2ab') as number)).toBe('reserved');
    expect(isKind(parseCode('job') as number)).toBe(true);
    expect(codeToString(SYS)).toBe('sys');
  });

  test('parseCode refuses uppercase, wrong length, 0, 1 and non-ASCII', () => {
    for (const s of ['EMQ', 'em', 'emqq', '', 'e0q', 'e1q', 'eé', 'e\u0000q', 'emq\n']) {
      expect(parseCode(s)).toBeUndefined();
    }
  });

  test('isKind refuses a digit in any position', () => {
    for (const s of ['2ob', 'j2b', 'jo3', '234']) expect(isKind(parseCode(s) as number)).toBe(false);
  });

  test('the alphabet is 32 distinct symbols in ASCII order', () => {
    expect(ALPHABET.length).toBe(32);
    expect([...ALPHABET].sort().join('')).toBe(ALPHABET);
  });
});

describe('Eid, parse, fromBytes, format', () => {
  test('fields of the worked example', () => {
    const id = parse(WORKED);
    expect(id.system).toBe('emq');
    expect(id.kind).toBe('job');
    expect(id.reserved).toBe(0);
    expect(id.ms).toBe(1795271400123);
    expect(id.writer).toBe(895287301);
    expect(id.count).toBe(0);
    expect(id.class).toBe('registered');
    expect(hex(id.bytes)).toBe(WORKED_HEX);
    expect(JSON.stringify({ id })).toBe(`{"id":"${WORKED}"}`);
    expect(parse('emq_job_23obzcbapvupu2272226').count).toBe(1);
  });

  test('a string is read as UTF-8 and bytes as they are', () => {
    const bytes = new TextEncoder().encode(WORKED);
    expect(parse(bytes).toString()).toBe(WORKED);
    // 27 characters but 28 UTF-8 bytes: the alphabet step refuses, not the length step
    expect(outcome(() => parse('emq_job_23ob\u00e9bapvupu2272222'))).toBe('alphabet');
    // 28 characters but 29 UTF-8 bytes
    expect(outcome(() => parse('emq_job_23ob\u00e9cbapvupu2272222'))).toBe('length');
  });

  test('bytes returns a copy, and the constructor copies its input', () => {
    const input = unhex(WORKED_HEX);
    const id = new Eid(input);
    input[0] = 0;
    id.bytes[0] = 0;
    expect(hex(id.bytes)).toBe(WORKED_HEX);
  });

  test('a set reserved bit is refused, not repaired', () => {
    const bad = unhex(WORKED_HEX);
    bad[3] = (bad[3] ?? 0) | 1;
    expect(outcome(() => fromBytes(bad))).toBe('reserved');
  });

  test('format accepts an id or checked bytes and throws on unchecked ones', () => {
    expect(format(parse(WORKED))).toBe(WORKED);
    expect(format(unhex(WORKED_HEX))).toBe(WORKED);
    expect(outcome(() => format(new Uint8Array(16)))).toBe('system-reserved');
    expect(outcome(() => format(new Uint8Array(3)))).toBe('length');
  });

  test('valid and validBinary answer true for good input and false for refused input', () => {
    expect(valid(WORKED)).toBe(true);
    expect(valid(WORKED.toUpperCase())).toBe(false);
    expect(valid('')).toBe(false);
    expect(validBinary(unhex(WORKED_HEX))).toBe(true);
    expect(validBinary(new Uint8Array(16))).toBe(false);
    expect(validBinary(new Uint8Array(0))).toBe(false);
  });

  test('a Refusal carries its reason as name, message and field', () => {
    try {
      parse('');
      throw new Error('not refused');
    } catch (e) {
      expect(e).toBeInstanceOf(Refusal);
      expect((e as Refusal).reason).toBe('length');
      expect((e as Refusal).message).toBe('length');
    }
  });

  test('a detached id has the detached class', () => {
    expect(fromParts({ system: '3ab', kind: 'evt', ms: 1, writer: 2, count: 3 }).class).toBe('detached');
  });

  test('compare and equals follow byte order, and order is lexicographic by system, kind, time, writer, count', () => {
    const a = parse('emq_job_23obzcbapvupu2272222');
    const b = parse('emq_job_23obzcbapvupu2272226');
    expect(compare(a, b)).toBe(-1);
    expect(compare(b, a)).toBe(1);
    expect(compare(a, parse(WORKED))).toBe(0);
    expect(a.equals(parse(WORKED))).toBe(true);
    expect(a.equals(b)).toBe(false);
    const hi = fromParts({ system: 'emq', kind: 'job', ms: 2 ** 32, writer: 0, count: 0 });
    const lo = fromParts({ system: 'emq', kind: 'job', ms: 2 ** 32 - 1, writer: WRITERMAX, count: 262143 });
    expect(compare(lo, hi)).toBe(-1);
  });
});


describe('fromParts and systemId', () => {
  test('the extremes pack and unpack exactly', () => {
    const id = fromParts({ system: 'zzz', kind: 'zzz', ms: 2 ** 48 - 1, writer: WRITERMAX, count: 262143 });
    expect(id.toString()).toBe('zzz_zzz_bzzzzzzzzzzzzzzzzzzw');
    expect(id.ms).toBe(2 ** 48 - 1);
    expect(id.writer).toBe(WRITERMAX);
    expect(id.count).toBe(262143);
    expect(fromParts({ system: 'a22', kind: 'aaa', ms: 0, writer: 0, count: 0 }).ms).toBe(0);
  });

  test('a system or kind may be given as a code', () => {
    const id = fromParts({ system: parseCode('emq') as number, kind: parseCode('job') as number, ms: 1, writer: 1, count: 1 });
    expect(id.system).toBe('emq');
    expect(id.kind).toBe('job');
  });

  test('refuses a reserved system, a bad kind, an out-of-range field and a malformed system', () => {
    const ok = { system: 'emq', kind: 'job', ms: 1, writer: 1, count: 1 };
    expect(outcome(() => fromParts({ ...ok, system: '2ab' }))).toBe('system-reserved');
    expect(outcome(() => fromParts({ ...ok, kind: 'j2b' }))).toBe('kind');
    expect(outcome(() => fromParts({ ...ok, kind: 'JOB' }))).toBe('kind');
    expect(outcome(() => fromParts({ ...ok, kind: 'jo' }))).toBe('kind');
    expect(outcome(() => fromParts({ ...ok, system: 'EMQ' }))).toBe('alphabet');
    expect(outcome(() => fromParts({ ...ok, system: 32768 }))).toBe('range');
    expect(outcome(() => fromParts({ ...ok, ms: 2 ** 48 }))).toBe('range');
    expect(outcome(() => fromParts({ ...ok, ms: -1 }))).toBe('range');
    expect(outcome(() => fromParts({ ...ok, ms: 1.5 }))).toBe('range');
    expect(outcome(() => fromParts({ ...ok, ms: Number.NaN }))).toBe('range');
    expect(outcome(() => fromParts({ ...ok, writer: 2 ** 30 }))).toBe('range');
    expect(outcome(() => fromParts({ ...ok, count: 262144 }))).toBe('range');
    expect(outcome(() => fromParts({ ...ok, count: -1 }))).toBe('range');
  });

  test('systemId is the system with kind sys and a zero local part; a reserved system has none', () => {
    expect(systemId('emq').toString()).toBe('emq_sys_22222222222222222222');
    expect(systemId(parseCode('kan') as number).toString()).toBe('kan_sys_22222222222222222222');
    expect(outcome(() => systemId('2ab'))).toBe('system-reserved');
  });
});

describe('stream ids', () => {
  test('parseStreamId and formatStreamId round trip, including the largest 64-bit numbers', () => {
    const s = parseStreamId('18446744073709551615-0');
    expect(s.ms).toBe(2n ** 64n - 1n);
    expect(formatStreamId(s)).toBe('18446744073709551615-0');
    expect(formatStreamId(parseStreamId('0-0'))).toBe('0-0');
  });

  test('parseStreamId refuses sign, leading zero, extra dashes, spaces and overflow', () => {
    for (const s of ['', '-', '1', '1-', '-1', '1-2-3', '+1-2', '-1-2', '01-2', '1-02', '1 -2', '1-2 ', '1-2\n', '1.5-2', '18446744073709551616-0', '0-18446744073709551616', '1e3-2', '０-1']) {
      expect(outcome(() => parseStreamId(s))).toBe('stream-syntax');
    }
  });

  test('streamLess orders by ms then seq', () => {
    const a = { ms: 1n, seq: 5n };
    expect(streamLess(a, { ms: 1n, seq: 6n })).toBe(true);
    expect(streamLess(a, { ms: 2n, seq: 0n })).toBe(true);
    expect(streamLess(a, a)).toBe(false);
    expect(streamLess({ ms: 2n, seq: 0n }, a)).toBe(false);
  });

  test('toStreamId and legacyTail of the worked example', () => {
    const id = parse(WORKED);
    expect(formatStreamId(toStreamId(id))).toBe('1795271400123-3050860491784323072');
    expect(legacyTail(id)).toBe(895287301 * 262144);
  });

  test('fromStreamId inverts toStreamId and refuses a reserved system, a bad kind and a range error', () => {
    const id = parse(WORKED);
    expect(fromStreamId('job', toStreamId(id)).equals(id)).toBe(true);
    // the kind is not in the stream id: another kind gives another id
    expect(fromStreamId('evt', toStreamId(id)).equals(id)).toBe(false);
    expect(outcome(() => fromStreamId('job', { ms: 1n, seq: 0n }))).toBe('system-reserved');
    expect(outcome(() => fromStreamId('j0b', { ms: 1n, seq: 1n << 48n }))).toBe('kind');
    expect(outcome(() => fromStreamId('job', { ms: 1n << 48n, seq: 1n << 48n }))).toBe('stream-range');
    expect(outcome(() => fromStreamId('job', { ms: 1n, seq: 1n << 63n }))).toBe('stream-range');
    expect(outcome(() => fromStreamId('job', { ms: -1n, seq: 1n << 58n }))).toBe('stream-range');
    expect(outcome(() => fromStreamId('job', { ms: 1n, seq: -1n }))).toBe('stream-range');
  });

  test('valkeyAccepts: greater than the top and not 0-0', () => {
    const top = { ms: 10n, seq: 5n };
    expect(valkeyAccepts(undefined, { ms: 1n, seq: 0n })).toBe(true);
    expect(valkeyAccepts(top, { ms: 10n, seq: 6n })).toBe(true);
    expect(valkeyAccepts(top, { ms: 11n, seq: 0n })).toBe(true);
    expect(valkeyAccepts(top, top)).toBe(false);
    expect(valkeyAccepts(top, { ms: 10n, seq: 4n })).toBe(false);
    expect(valkeyAccepts(undefined, { ms: 0n, seq: 0n })).toBe(false);
  });
});

describe('leases', () => {
  test('newLease builds a lease and formatLease writes it back', () => {
    const l = newLease('emq', 895287301, 1795271400000, 1795357800000);
    expect(formatLease(l)).toBe('emq:895287301:1795271400000:1795357800000');
    expect(parseLease(formatLease(l))).toEqual(l);
    expect(newLease(parseCode('kan') as number, 0, 1, 1).system).toBe('kan');
  });

  test('newLease refuses by the same reasons as the text form', () => {
    expect(outcome(() => newLease('2ab', 1, 1000, 2000))).toBe('lease-system');
    expect(outcome(() => newLease('EMQ', 1, 1000, 2000))).toBe('lease-syntax');
    expect(outcome(() => newLease('emq', -1, 1000, 2000))).toBe('lease-syntax');
    expect(outcome(() => newLease('emq', 1.5, 1000, 2000))).toBe('lease-syntax');
    expect(outcome(() => newLease('emq', 2 ** 30, 1000, 2000))).toBe('lease-writer');
    expect(outcome(() => newLease('emq', 1, 0, 2000))).toBe('lease-window');
    expect(outcome(() => newLease('emq', 1, 2000, 1000))).toBe('lease-window');
    expect(outcome(() => newLease('emq', 1, 1000, 1000 + 86_400_000))).toBe('ok');
    expect(outcome(() => newLease('emq', 1, 1000, 1001 + 86_400_000))).toBe('lease-window');
    expect(outcome(() => newLease('emq', 1, 1000, 2 ** 48))).toBe('lease-window');
    // from 2^53 a number is refused by the writer or the window, as in the text form; 2^64 - 2048 is the largest below 2^64
    expect(outcome(() => newLease('emq', 2 ** 53, 1000, 2000))).toBe('lease-writer');
    expect(outcome(() => newLease('emq', 1, 1000, 2 ** 53))).toBe('lease-window');
    expect(outcome(() => newLease('emq', 1, 2 ** 60, 2 ** 60))).toBe('lease-window');
    expect(outcome(() => newLease('emq', 1, 1000, 2 ** 64 - 2048))).toBe('lease-window');
    expect(outcome(() => newLease('emq', 1, 1000, 2 ** 64))).toBe('lease-syntax');
  });

  test('parseLease refuses numbers of 21 digits, 2^64, plus signs, fractions and empty text', () => {
    for (const s of ['', 'emq', 'emq:1:1000:18446744073709551616', 'emq:+1:1000:2000', 'emq:1.0:1000:2000', 'emq:1:1000:00001', 'emq:1:100000000000000000000:2000000000000000000000']) {
      expect(outcome(() => parseLease(s))).toBe('lease-syntax');
    }
    expect(outcome(() => parseLease('emq:1:1000:18446744073709551615'))).toBe('lease-window');
  });

  test('writerFor: the second modulo 2^17 times 2^13 plus the index; refuses negatives and fractions', () => {
    expect(writerFor(1795271400, 5)).toBe(895287301);
    expect(writerFor(131072 + 3, 8192 + 7)).toBe(3 * 8192 + 7);
    expect(outcome(() => writerFor(-1, 0))).toBe('range');
    expect(outcome(() => writerFor(1, 0.5))).toBe('range');
  });
});

describe('seeded random ids', () => {
  let s = 0x2545f491;
  const next = (n: number): number => {
    s = (Math.imul(s, 1664525) + 1013904223) >>> 0;
    return s % n;
  };
  const letters = 'abcdefghijklmnopqrstuvwxyz';
  const any = ALPHABET;
  const word = (set: string, first: string) =>
    first.charAt(next(first.length)) + set.charAt(next(set.length)) + set.charAt(next(set.length));

  test('text and bytes round trip, and sorting texts sorts ids', () => {
    const ids: Eid[] = [];
    for (let i = 0; i < 2000; i++) {
      ids.push(
        fromParts({
          system: word(any, letters + '34567'),
          kind: word(letters, letters),
          ms: next(2 ** 20) * 2 ** 28 + next(2 ** 28),
          writer: next(2 ** 15) * 2 ** 15 + next(2 ** 15),
          count: next(262144),
        }),
      );
    }
    for (const id of ids) {
      expect(parse(id.toString()).equals(id)).toBe(true);
      expect(fromBytes(id.bytes).toString()).toBe(id.toString());
      expect(fromStreamId(id.kind, toStreamId(id)).equals(id)).toBe(true);
    }
    const byText = [...ids].sort((a, b) => (a.toString() < b.toString() ? -1 : a.toString() > b.toString() ? 1 : 0));
    const byBytes = [...ids].sort(compare);
    expect(byText.map(String)).toEqual(byBytes.map(String));
  });
});
