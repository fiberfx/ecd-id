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
  | 'lease-window'
  | 'brd14-length'
  | 'brd14-namespace'
  | 'brd14-alphabet'
  | 'brd14-range'
  | 'brd14-system'
  | 'brd14-time'
  | 'brd14-writer'
  | 'brd14-count'
  | 'brd14-kind';

/** An error whose `reason` is the refusal reason, and whose message is that same text. */
export class Refusal extends Error {
  readonly reason: Reason;

  constructor(reason: Reason) {
    super(reason);
    this.name = 'Refusal';
    this.reason = reason;
  }
}
