import { Eid, TIME_MAX, WRITER_MAX, COUNT_LIMIT, codeClass, isKind, pack, parseCode } from './id.js';
import { Refusal } from './refusal.js';

/** A Valkey stream entry id: two unsigned 64-bit numbers, ordered by `ms`, then `seq`. */
export interface StreamId {
  readonly ms: bigint;
  readonly seq: bigint;
}

const U64 = 1n << 64n;
const DECIMAL = /^(?:0|[1-9][0-9]*)$/;

/** The text `<ms>-<seq>`. */
export function formatStreamId(s: StreamId): string {
  return `${s.ms}-${s.seq}`;
}

/** Whether `a` orders before `b`. */
export function streamLess(a: StreamId, b: StreamId): boolean {
  return a.ms < b.ms || (a.ms === b.ms && a.seq < b.seq);
}

function streamDecimal(p: string): bigint {
  if (!DECIMAL.test(p)) throw new Refusal('stream-syntax');
  const v = BigInt(p);
  if (v >= U64) throw new Refusal('stream-syntax');
  return v;
}

/**
 * Parses `<ms>-<seq>`: two decimal numbers below 2^64, without sign or leading zero, joined by one `-`.
 * Throws a `Refusal` of reason `stream-syntax`.
 */
export function parseStreamId(s: string): StreamId {
  const cut = s.indexOf('-');
  if (cut < 0) throw new Refusal('stream-syntax');
  return { ms: streamDecimal(s.slice(0, cut)), seq: streamDecimal(s.slice(cut + 1)) };
}

/** The stream entry id of an id: time, then `system << 48 | writer << 18 | count`. */
export function toStreamId(id: Eid): StreamId {
  return { ms: BigInt(id.ms), seq: (BigInt(id.systemCode) << 48n) | BigInt(id.writer * COUNT_LIMIT + id.count) };
}

/**
 * The 48-bit tail without the system (bytes 10 to 15). It is not unique across systems and exists only as the
 * control that shows why the stream id carries the system.
 */
export function legacyTail(id: Eid): number {
  return id.writer * COUNT_LIMIT + id.count;
}

/**
 * Rebuilds an id from a stream entry id and the kind the stream carries. Refuses with `kind`, `stream-range`
 * (`ms` at least 2^48 or `seq` at least 2^63, or either negative) or `system-reserved`.
 */
export function fromStreamId(kind: string, sid: StreamId): Eid {
  const k = parseCode(kind);
  if (k === undefined || !isKind(k)) throw new Refusal('kind');
  if (sid.ms < 0n || sid.ms > BigInt(TIME_MAX) || sid.seq < 0n || sid.seq >= 1n << 63n) {
    throw new Refusal('stream-range');
  }
  const system = Number(sid.seq >> 48n);
  if (codeClass(system) === 'reserved') throw new Refusal('system-reserved');
  const writer = Number((sid.seq >> 18n) & BigInt(WRITER_MAX));
  const count = Number(sid.seq & BigInt(COUNT_LIMIT - 1));
  return new Eid(pack(system, k, Number(sid.ms), writer, count));
}

/**
 * Valkey's rule for XADD with an explicit id: the id is greater than the stream's top entry and is not 0-0.
 * `top` is `undefined` for an empty stream.
 */
export function valkeyAccepts(top: StreamId | undefined, entry: StreamId): boolean {
  if (entry.ms === 0n && entry.seq === 0n) return false;
  return top === undefined || streamLess(top, entry);
}
