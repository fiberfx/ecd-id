/** The reasons a parse or a constructor refuses an input; the strings are those of spec/id.md and the vectors. */
export type Reason =
  | 'length'
  | 'separator'
  | 'alphabet'
  | 'pad'
  | 'reserved'
  | 'system-reserved'
  | 'kind'
  | 'range'
  | 'stream-syntax'
  | 'stream-range'
  | 'lease-syntax'
  | 'lease-system'
  | 'lease-writer'
  | 'lease-window';

/** An error whose `reason` is the refusal reason, and whose message is that same text. */
export class Refusal extends Error {
  readonly reason: Reason;

  constructor(reason: Reason) {
    super(reason);
    this.name = 'Refusal';
    this.reason = reason;
  }
}
