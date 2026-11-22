import { Refusal } from './refusal.js';

/** The 32 symbols in value order; ASCII order equals value order. */
export const ALPHABET = '234567abcdefghijklmnopqrstuvwxyz';

/** Length of the canonical text. */
export const TEXT_LENGTH = 28;
/** Length of the binary form. */
export const BYTE_LENGTH = 16;
/** The largest millisecond, 2^48 - 1. */
export const TIME_MAX = 2 ** 48 - 1;
/** The largest writer number, 2^30 - 1. */
export const WRITER_MAX = 2 ** 30 - 1;
/** One more than the largest count, 2^18. */
export const COUNT_LIMIT = 2 ** 18;

/** A 15-bit code of three alphabet characters: a system code or a kind. */
export type Code = number;

/** The class of a system code, read from its first character. */
export type SystemClass = 'registered' | 'detached' | 'reserved';

const NOT_IN_ALPHABET = 0xff;
const VALUE = new Uint8Array(256).fill(NOT_IN_ALPHABET);
for (let i = 0; i < 32; i++) VALUE[ALPHABET.charCodeAt(i)] = i;

function symbolValue(c: number): number {
  return c < 256 ? (VALUE[c] ?? NOT_IN_ALPHABET) : NOT_IN_ALPHABET;
}

const encoder = new TextEncoder();

/** Parses three alphabet characters; uppercase and any other character give `undefined`. */
export function parseCode(s: string): Code | undefined {
  if (s.length !== 3) return undefined;
  let v = 0;
  for (let i = 0; i < 3; i++) {
    const x = symbolValue(s.charCodeAt(i));
    if (x === NOT_IN_ALPHABET) return undefined;
    v = (v << 5) | x;
  }
  return v;
}

/** The three characters of a code. */
export function codeToString(c: Code): string {
  return (
    ALPHABET.charAt((c >> 10) & 31) + ALPHABET.charAt((c >> 5) & 31) + ALPHABET.charAt(c & 31)
  );
}

/** The class of a code read as a system code: `2` is reserved, `3` to `7` detached, a letter registered. */
export function codeClass(c: Code): SystemClass {
  const first = (c >> 10) & 31;
  if (first === 0) return 'reserved';
  if (first <= 5) return 'detached';
  return 'registered';
}

/** Whether a code is a valid kind: three letters. */
export function isKind(c: Code): boolean {
  return ((c >> 10) & 31) >= 6 && ((c >> 5) & 31) >= 6 && (c & 31) >= 6;
}

/** The kind that names a system itself, in every system. */
export const SYS: Code = parseCode('sys') as Code;

/** The fields of an id, before it is packed. `system` and `kind` are three characters or a code. */
export interface Parts {
  system: string | Code;
  kind: string | Code;
  ms: number;
  writer: number;
  count: number;
}

function codeOf(v: string | Code, onBad: 'alphabet' | 'kind'): Code {
  if (typeof v === 'number') {
    if (!Number.isInteger(v) || v < 0 || v >= 1 << 15) throw new Refusal('range');
    return v;
  }
  const c = parseCode(v);
  if (c === undefined) throw new Refusal(onBad);
  return c;
}

function isUint(v: number, limit: number): boolean {
  return Number.isSafeInteger(v) && v >= 0 && v <= limit;
}

/** Packs checked fields into 16 bytes. */
export function pack(system: Code, kind: Code, ms: number, writer: number, count: number): Uint8Array {
  const b = new Uint8Array(BYTE_LENGTH);
  b[0] = system >> 7;
  b[1] = ((system & 127) << 1) | (kind >> 14);
  b[2] = (kind >> 6) & 255;
  b[3] = (kind & 63) << 2;
  const hi = Math.floor(ms / 4294967296);
  const lo = ms % 4294967296;
  b[4] = hi >> 8;
  b[5] = hi & 255;
  b[6] = (lo >>> 24) & 255;
  b[7] = (lo >>> 16) & 255;
  b[8] = (lo >>> 8) & 255;
  b[9] = lo & 255;
  b[10] = (writer >>> 22) & 255;
  b[11] = (writer >>> 14) & 255;
  const low = (writer & 0x3fff) * 262144 + count;
  b[12] = (low >>> 24) & 255;
  b[13] = (low >>> 16) & 255;
  b[14] = (low >>> 8) & 255;
  b[15] = low & 255;
  return b;
}

function textBytes(input: string | Uint8Array): Uint8Array {
  return typeof input === 'string' ? encoder.encode(input) : input;
}

/** Steps 1 to 4 of the strict parse; returns the 16 bytes, not yet checked for steps 5 to 7. */
function decodeText(t: Uint8Array): Uint8Array {
  if (t.length !== TEXT_LENGTH) throw new Refusal('length');
  if (t[3] !== 0x5f || t[7] !== 0x5f) throw new Refusal('separator');
  const out = new Uint8Array(BYTE_LENGTH);
  let acc = 0;
  let nbits = 0;
  let o = 0;
  let n = 0;
  let last = 0;
  for (let pos = 0; pos < TEXT_LENGTH; pos++) {
    if (pos === 3 || pos === 7) continue;
    const v = symbolValue(t[pos] ?? 0);
    if (v === NOT_IN_ALPHABET) throw new Refusal('alphabet');
    if (n < 25) {
      acc = (acc << 5) | v;
      nbits += 5;
      if (nbits >= 8) {
        out[o++] = (acc >> (nbits - 8)) & 255;
        nbits -= 8;
        acc &= (1 << nbits) - 1;
      }
    } else {
      last = v;
    }
    n++;
  }
  if ((last & 3) !== 0) throw new Refusal('pad');
  out[15] = ((acc << 3) | (last >> 2)) & 255;
  return out;
}

/** Encodes 16 bytes as the 28-character canonical text. */
function encodeText(b: Uint8Array): string {
  const groups: number[] = [];
  let acc = 0;
  let nbits = 0;
  for (const byte of b) {
    acc = (acc << 8) | byte;
    nbits += 8;
    while (nbits >= 5) {
      groups.push((acc >> (nbits - 5)) & 31);
      nbits -= 5;
      acc &= (1 << nbits) - 1;
    }
  }
  groups.push((acc << 2) & 31);
  let s = '';
  groups.forEach((g, i) => {
    if (i === 3 || i === 6) s += '_';
    s += ALPHABET.charAt(g);
  });
  return s;
}

/**
 * An Ecd ID: 16 bytes, big-endian. Comparing the bytes compares system, kind, time, writer and count.
 * Constructing one is binary ingress: exactly 16 bytes, then steps 5 to 7 of the strict parse.
 */
export class Eid {
  readonly #b: Uint8Array;

  /** Throws a `Refusal` of reason `length`, `reserved`, `system-reserved` or `kind`. */
  constructor(bytes: Uint8Array) {
    if (bytes.length !== BYTE_LENGTH) throw new Refusal('length');
    const b = Uint8Array.from(bytes);
    if (((b[3] ?? 0) & 3) !== 0) throw new Refusal('reserved');
    const system = (((b[0] ?? 0) << 7) | ((b[1] ?? 0) >> 1)) & 0x7fff;
    if (codeClass(system) === 'reserved') throw new Refusal('system-reserved');
    const kind = ((((b[1] ?? 0) & 1) << 14) | ((b[2] ?? 0) << 6) | ((b[3] ?? 0) >> 2)) & 0x7fff;
    if (!isKind(kind)) throw new Refusal('kind');
    this.#b = b;
  }

  /** The system's 15-bit code. */
  get systemCode(): Code {
    return ((((this.#b[0] ?? 0) << 7) | ((this.#b[1] ?? 0) >> 1)) & 0x7fff);
  }

  /** The kind's 15-bit code. */
  get kindCode(): Code {
    return (
      ((((this.#b[1] ?? 0) & 1) << 14) | ((this.#b[2] ?? 0) << 6) | ((this.#b[3] ?? 0) >> 2)) & 0x7fff
    );
  }

  /** The system that minted the id: three characters. */
  get system(): string {
    return codeToString(this.systemCode);
  }

  /** The kind: three letters. */
  get kind(): string {
    return codeToString(this.kindCode);
  }

  /** The two reserved bits; zero in every valid id. */
  get reserved(): number {
    return (this.#b[3] ?? 0) & 3;
  }

  /** Unix milliseconds stamped by the minter, below 2^48. */
  get ms(): number {
    const b = this.#b;
    const hi = (b[4] ?? 0) * 256 + (b[5] ?? 0);
    const lo = (((b[6] ?? 0) * 256 + (b[7] ?? 0)) * 256 + (b[8] ?? 0)) * 256 + (b[9] ?? 0);
    return hi * 4294967296 + lo;
  }

  /** The writer number, below 2^30. */
  get writer(): number {
    const b = this.#b;
    return (((b[10] ?? 0) * 256 + (b[11] ?? 0)) * 256 + (b[12] ?? 0)) * 64 + ((b[13] ?? 0) >> 2);
  }

  /** The count inside the writer's millisecond, below 2^18. */
  get count(): number {
    const b = this.#b;
    return ((b[13] ?? 0) & 3) * 65536 + (b[14] ?? 0) * 256 + (b[15] ?? 0);
  }

  /** The class of the id's system. */
  get class(): SystemClass {
    return codeClass(this.systemCode);
  }

  /** A copy of the 16 bytes. */
  get bytes(): Uint8Array {
    return Uint8Array.from(this.#b);
  }

  /** The canonical text. */
  toString(): string {
    return encodeText(this.#b);
  }

  toJSON(): string {
    return this.toString();
  }

  /** The byte-order comparison: negative, zero or positive. */
  compare(other: Eid): number {
    for (let i = 0; i < BYTE_LENGTH; i++) {
      const d = (this.#b[i] ?? 0) - (other.#b[i] ?? 0);
      if (d !== 0) return d < 0 ? -1 : 1;
    }
    return 0;
  }

  equals(other: Eid): boolean {
    return this.compare(other) === 0;
  }
}

/**
 * The strict parse of the canonical text (spec/id.md#strict-parse). A string is read as its UTF-8 bytes, so a
 * non-ASCII character fails as the specification says; a `Uint8Array` is read as it is. Throws a `Refusal`.
 */
export function parse(input: string | Uint8Array): Eid {
  return new Eid(decodeText(textBytes(input)));
}

/** Binary ingress: exactly 16 bytes, then steps 5 to 7 of the strict parse. Throws a `Refusal`. */
export function fromBytes(bytes: Uint8Array): Eid {
  return new Eid(bytes);
}

/** The canonical text of an id, or of 16 bytes that pass binary ingress. */
export function format(id: Eid | Uint8Array): string {
  return (id instanceof Eid ? id : new Eid(id)).toString();
}

/** Whether the input is a valid canonical text. */
export function valid(input: string | Uint8Array): boolean {
  try {
    parse(input);
    return true;
  } catch (e) {
    if (e instanceof Refusal) return false;
    throw e;
  }
}

/** Whether the 16 bytes pass binary ingress. */
export function validBinary(bytes: Uint8Array): boolean {
  try {
    fromBytes(bytes);
    return true;
  } catch (e) {
    if (e instanceof Refusal) return false;
    throw e;
  }
}

/**
 * Builds an id from its fields, checking each against the layout. Refuses with `system-reserved`, `kind` or
 * `range`; a system given as a string that is not three alphabet characters is refused with `alphabet`.
 */
export function fromParts(p: Parts): Eid {
  const system = codeOf(p.system, 'alphabet');
  if (codeClass(system) === 'reserved') throw new Refusal('system-reserved');
  const kind = codeOf(p.kind, 'kind');
  if (!isKind(kind)) throw new Refusal('kind');
  if (!isUint(p.ms, TIME_MAX) || !isUint(p.writer, WRITER_MAX) || !isUint(p.count, COUNT_LIMIT - 1)) {
    throw new Refusal('range');
  }
  return new Eid(pack(system, kind, p.ms, p.writer, p.count));
}

/** The system's own id: its code, kind `sys` and a zero local part. */
export function systemId(system: string | Code): Eid {
  return fromParts({ system, kind: SYS, ms: 0, writer: 0, count: 0 });
}

/** Compares two ids in byte order. */
export function compare(a: Eid, b: Eid): number {
  return a.compare(b);
}
