import { TIME_MAX, WRITER_MAX, codeClass, codeToString, parseCode, type Code } from './id.js';
import { Refusal } from './refusal.js';

/** The longest lease window, 24 hours in milliseconds. */
export const LEASE_TERM_MAX = 86_400_000;
/** A writer number's second comes round after this many seconds (36.4 hours). */
export const SECOND_SLOTS = 1 << 17;
/** Grants per second per system. */
export const INDEX_SLOTS = 1 << 13;

/** An exclusive right to one writer number and the id-times from `notBefore` to `notAfter`, inclusive. */
export interface Lease {
  readonly system: string;
  readonly writer: number;
  readonly notBefore: number;
  readonly notAfter: number;
}

const DECIMAL = /^(?:0|[1-9][0-9]*)$/;
const U64 = 1n << 64n;

function window(notBefore: bigint, notAfter: bigint): boolean {
  return (
    notBefore !== 0n &&
    notAfter >= notBefore &&
    notAfter <= BigInt(TIME_MAX) &&
    notAfter - notBefore <= BigInt(LEASE_TERM_MAX)
  );
}

/**
 * A lease, checked as the text form is checked. Refuses with `lease-syntax` (a system that is not three
 * alphabet characters, a number that is not an integer from 0 to 2^64 - 1), `lease-system`, `lease-writer` or
 * `lease-window`.
 */
export function newLease(
  system: string | Code,
  writer: number,
  notBefore: number,
  notAfter: number,
): Lease {
  const code = typeof system === 'number' ? system : parseCode(system);
  if (code === undefined || !Number.isInteger(code) || code < 0 || code >= 1 << 15) {
    throw new Refusal('lease-syntax');
  }
  // A number from 2^53 is not exact, but every one fails the writer or the window check, as the text form does.
  for (const n of [writer, notBefore, notAfter]) {
    if (!Number.isInteger(n) || n < 0 || n >= 2 ** 64) throw new Refusal('lease-syntax');
  }
  if (codeClass(code) === 'reserved') throw new Refusal('lease-system');
  if (writer > WRITER_MAX) throw new Refusal('lease-writer');
  if (!window(BigInt(notBefore), BigInt(notAfter))) throw new Refusal('lease-window');
  return { system: codeToString(code), writer, notBefore, notAfter };
}

function leaseDecimal(p: string): bigint {
  if (p.length > 20 || !DECIMAL.test(p)) throw new Refusal('lease-syntax');
  const v = BigInt(p);
  if (v >= U64) throw new Refusal('lease-syntax');
  return v;
}

/** The strict parse of `system:writer:not_before:not_after`. Throws a `Refusal` of a `lease-` reason. */
export function parseLease(s: string): Lease {
  const parts = s.split(':');
  if (parts.length !== 4) throw new Refusal('lease-syntax');
  const system = parseCode(parts[0] ?? '');
  if (system === undefined) throw new Refusal('lease-syntax');
  const n = [parts[1], parts[2], parts[3]].map((p) => leaseDecimal(p ?? ''));
  const [writer = 0n, notBefore = 0n, notAfter = 0n] = n;
  if (codeClass(system) === 'reserved') throw new Refusal('lease-system');
  if (writer > BigInt(WRITER_MAX)) throw new Refusal('lease-writer');
  if (!window(notBefore, notAfter)) throw new Refusal('lease-window');
  return newLease(system, Number(writer), Number(notBefore), Number(notAfter));
}

/** The text form of a lease. */
export function formatLease(l: Lease): string {
  return `${l.system}:${l.writer}:${l.notBefore}:${l.notAfter}`;
}

/** `(second mod 2^17) * 2^13 + index mod 2^13`: the writer number of the grant of an allocator. */
export function writerFor(second: number, index: number): number {
  if (!Number.isSafeInteger(second) || second < 0 || !Number.isSafeInteger(index) || index < 0) {
    throw new Refusal('range');
  }
  return (second % SECOND_SLOTS) * INDEX_SLOTS + (index % INDEX_SLOTS);
}
