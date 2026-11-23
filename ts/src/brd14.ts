import { Eid, pack, parseCode, type Code } from './id.js';
import { Refusal } from './refusal.js';

/** The length of a brd14 id in bytes: a namespace of three bytes `A`-`Z`, then 11 Base62 digits. */
export const BRD14_LENGTH = 14;
/** The brd14 epoch, 2024-01-01T00:00:00Z, in Unix milliseconds. */
export const BRD14_EPOCH = 1704067200000;

const BASE62 = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz';
/** The payload of 2^63 - 1. */
const MAX_PAYLOAD = 'AzL8n0Y58m7';
const NOT_BASE62 = 0xff;
const DIGIT = new Uint8Array(256).fill(NOT_BASE62);
for (let i = 0; i < 62; i++) DIGIT[BASE62.charCodeAt(i)] = i;
const BRD = parseCode('brd') as Code;

const encoder = new TextEncoder();

/**
 * Embeds a brd14 id into an id of the system `brd` (spec/registry.md#brd14): the kind is the namespace in lowercase,
 * the time is `BRD14_EPOCH` plus the snowflake's milliseconds, the writer its node and the count its sequence. A string
 * is read as its UTF-8 bytes. It reads no registry. Throws a `Refusal` of reason `brd14-length`, `brd14-namespace`,
 * `brd14-alphabet` or `brd14-range`.
 */
export function fromBrd14(input: string | Uint8Array): Eid {
  const b = typeof input === 'string' ? encoder.encode(input) : input;
  if (b.length !== BRD14_LENGTH) throw new Refusal('brd14-length');
  let ns = '';
  for (let i = 0; i < 3; i++) {
    const c = b[i] ?? 0;
    if (c < 0x41 || c > 0x5a) throw new Refusal('brd14-namespace');
    ns += String.fromCharCode(c + 0x20);
  }
  let payload = '';
  for (let i = 3; i < BRD14_LENGTH; i++) {
    const c = b[i] ?? 0;
    if (DIGIT[c] === NOT_BASE62) throw new Refusal('brd14-alphabet');
    payload += String.fromCharCode(c);
  }
  // Fixed width over an alphabet in ASCII order: byte order is numeric order.
  if (payload > MAX_PAYLOAD) throw new Refusal('brd14-range');
  let snow = 0n;
  for (let i = 3; i < BRD14_LENGTH; i++) snow = snow * 62n + BigInt(DIGIT[b[i] ?? 0] ?? 0);
  const kind = parseCode(ns) as Code;
  const ms = BRD14_EPOCH + Number(snow >> 22n);
  return new Eid(pack(BRD, kind, ms, Number((snow >> 12n) & 1023n), Number(snow & 4095n)));
}

/**
 * The inverse of `fromBrd14`. It reads no registry. Throws a `Refusal` of reason `brd14-system`, `brd14-time`,
 * `brd14-writer` or `brd14-count`.
 */
export function toBrd14(id: Eid): string {
  if (id.systemCode !== BRD) throw new Refusal('brd14-system');
  const ts = id.ms - BRD14_EPOCH;
  if (ts < 0 || ts >= 2 ** 41) throw new Refusal('brd14-time');
  if (id.writer >= 1024) throw new Refusal('brd14-writer');
  if (id.count >= 4096) throw new Refusal('brd14-count');
  let snow = (BigInt(ts) << 22n) | (BigInt(id.writer) << 12n) | BigInt(id.count);
  let digits = '';
  for (let i = 0; i < 11; i++) {
    digits = BASE62.charAt(Number(snow % 62n)) + digits;
    snow /= 62n;
  }
  return id.kind.toUpperCase() + digits;
}

/**
 * The registry layer of `fromBrd14`. The package reads no registry: `brdKinds` holds the kinds that
 * registry/kinds.tsv gives the system `brd`, in any state, and any other kind is refused with `brd14-kind`.
 */
export function fromBrd14Registered(input: string | Uint8Array, brdKinds: ReadonlySet<string>): Eid {
  const id = fromBrd14(input);
  if (!brdKinds.has(id.kind)) throw new Refusal('brd14-kind');
  return id;
}

/** The registry layer of `toBrd14`, with `brdKinds` as for `fromBrd14Registered`. */
export function toBrd14Registered(id: Eid, brdKinds: ReadonlySet<string>): string {
  const text = toBrd14(id);
  if (!brdKinds.has(id.kind)) throw new Refusal('brd14-kind');
  return text;
}
