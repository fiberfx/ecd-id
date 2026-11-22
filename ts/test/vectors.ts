import { readFileSync } from 'node:fs';

/** The data rows of a vector file: comment lines and empty lines dropped, cells split on tabs. */
export function rows(name: string): string[][] {
  const text = readFileSync(new URL(`../../vectors/${name}`, import.meta.url), 'utf8');
  return text
    .split('\n')
    .filter((l) => l !== '' && !l.startsWith('#'))
    .map((l) => l.split('\t'));
}

/** Hex to bytes; `-` is the empty input. */
export function unhex(s: string): Uint8Array {
  if (s === '-') return new Uint8Array(0);
  if (s.length % 2 !== 0 || !/^[0-9a-f]*$/.test(s)) throw new Error(`bad hex ${s}`);
  const out = new Uint8Array(s.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(s.slice(2 * i, 2 * i + 2), 16);
  return out;
}

export function hex(b: Uint8Array): string {
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
}

/** The refusal reason of a call, or `ok` when it returns. */
export function outcome(f: () => unknown): string {
  try {
    f();
    return 'ok';
  } catch (e) {
    if (e instanceof Error && e.name === 'Refusal') return (e as Error & { reason: string }).reason;
    throw e;
  }
}

/** Prints the checked count in a line the rows script reads, and fails when it differs from the file's rows. */
export function reportChecked(file: string, checked: number, total: number): void {
  console.log(`VECTORS ${file} checked ${checked} of ${total}`);
}
