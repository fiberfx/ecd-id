"""The refusal of an input: an exception whose reason is the text of the specification and the vectors."""

from typing import Literal

#: The reasons a parse, a constructor or a conversion refuses an input; the strings are those of spec/id.md,
#: spec/stream.md, spec/lease.md, spec/registry.md#brd14 and the vectors.
Reason = Literal[
    "length",
    "separator",
    "alphabet",
    "pad",
    "reserved",
    "system-reserved",
    "kind",
    "range",
    "stream-syntax",
    "stream-range",
    "lease-syntax",
    "lease-system",
    "lease-writer",
    "lease-window",
    "brd14-length",
    "brd14-namespace",
    "brd14-alphabet",
    "brd14-range",
    "brd14-system",
    "brd14-time",
    "brd14-writer",
    "brd14-count",
    "brd14-kind",
]


class Refusal(ValueError):
    """An input the package refuses. ``reason`` holds the refusal reason, and the message is that same text."""

    reason: Reason

    def __init__(self, reason: Reason) -> None:
        super().__init__(reason)
        self.reason = reason
