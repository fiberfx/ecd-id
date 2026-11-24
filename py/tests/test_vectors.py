"""Every data row of valid, invalid, binary_invalid, lease, stream and stream_invalid.tsv, both ways where they go both
ways."""

import unittest

from eid import (
    Eid,
    format,
    format_lease,
    format_stream_id,
    from_bytes,
    from_parts,
    from_stream_id,
    legacy_tail,
    parse,
    parse_lease,
    parse_stream_id,
    to_stream_id,
    valid,
    valid_binary,
    writer_for,
)

from .vectors import outcome, report_checked, rows, unhex


class ValidTest(unittest.TestCase):
    def test_every_id_parses_formats_converts_and_has_its_fields(self) -> None:
        data = rows("valid.tsv")
        checked = 0
        for text, hexed, system, kind, ms, writer, count, cls, stream in data:
            x = parse(text)
            self.assertEqual(x.bytes.hex(), hexed, text)
            self.assertEqual(str(x), text)
            self.assertEqual(str(from_bytes(unhex(hexed))), text)
            self.assertEqual(format(unhex(hexed)), text)
            self.assertEqual(parse(text.encode("ascii")), x)
            self.assertEqual(x.system, system)
            self.assertEqual(x.kind, kind)
            self.assertEqual(x.reserved, 0)
            self.assertEqual(str(x.ms), ms)
            self.assertEqual(str(x.writer), writer)
            self.assertEqual(str(x.count), count)
            self.assertEqual(x.class_, cls)
            self.assertEqual(format_stream_id(to_stream_id(x)), stream)
            self.assertEqual(from_parts(x.system, x.kind, x.ms, x.writer, x.count), x)
            self.assertTrue(valid(text))
            self.assertTrue(valid_binary(unhex(hexed)))
            checked += 1
        report_checked("valid.tsv", checked, len(data))
        self.assertGreater(len(data), 0)
        self.assertEqual(checked, len(data))

    def test_text_order_equals_byte_order_for_consecutive_rows(self) -> None:
        ids = [parse(r[0]) for r in rows("valid.tsv")]
        for a, b in zip(ids, ids[1:]):
            by_bytes = (a > b) - (a < b)
            by_text = (str(a) > str(b)) - (str(a) < str(b))
            self.assertEqual(by_text, by_bytes, (str(a), str(b)))


class InvalidTest(unittest.TestCase):
    def test_every_text_is_refused_with_the_reason_of_its_first_failing_step(self) -> None:
        data = rows("invalid.tsv")
        checked = 0
        seen = set()
        for name, input_hex, reason in data:
            b = unhex(input_hex)
            self.assertEqual(outcome(lambda: parse(b)), reason, name)
            self.assertFalse(valid(b), name)
            # A str is read as its UTF-8 bytes, so the same text as a str is refused the same way.
            try:
                s = b.decode("utf-8")
            except UnicodeDecodeError:
                pass
            else:
                self.assertEqual(outcome(lambda: parse(s)), reason, name)
            seen.add(reason)
            checked += 1
        report_checked("invalid.tsv", checked, len(data))
        for r in ("length", "separator", "alphabet", "pad", "reserved", "system-reserved", "kind"):
            self.assertIn(r, seen)
        self.assertGreater(len(data), 0)
        self.assertEqual(checked, len(data))


class BinaryInvalidTest(unittest.TestCase):
    def test_every_16_byte_input_is_refused_at_binary_ingress_with_its_reason(self) -> None:
        data = rows("binary_invalid.tsv")
        checked = 0
        for name, input_hex, reason in data:
            b = unhex(input_hex)
            self.assertEqual(outcome(lambda: from_bytes(b)), reason, name)
            self.assertEqual(outcome(lambda: Eid(b)), reason, name)
            self.assertEqual(outcome(lambda: format(b)), reason, name)
            self.assertFalse(valid_binary(b), name)
            checked += 1
        report_checked("binary_invalid.tsv", checked, len(data))
        self.assertGreater(len(data), 0)
        self.assertEqual(checked, len(data))


class LeaseTest(unittest.TestCase):
    def test_every_lease_text_parses_or_is_refused_and_writer_rows_give_the_writer(self) -> None:
        data = rows("lease.tsv")
        checked = 0
        for r in data:
            if r[0] == "writer":
                self.assertEqual(str(writer_for(int(r[1]), int(r[2]))), r[3], r)
            else:
                name, text, want = r
                got = outcome(lambda: parse_lease(text))
                self.assertEqual("ok" if got == "ok" else f"!{got}", want, name)
                if got == "ok":
                    self.assertEqual(format_lease(parse_lease(text)), text)
            checked += 1
        report_checked("lease.tsv", checked, len(data))
        self.assertGreater(len(data), 0)
        self.assertEqual(checked, len(data))


class StreamTest(unittest.TestCase):
    def test_every_id_maps_to_its_stream_id_and_legacy_tail_and_back(self) -> None:
        data = rows("stream.tsv")
        checked = 0
        for text, kind, stream, tail in data:
            x = parse(text)
            sid = to_stream_id(x)
            self.assertEqual(format_stream_id(sid), stream, text)
            self.assertEqual(str(legacy_tail(x)), tail, text)
            self.assertLess(sid.seq, 1 << 63)
            self.assertEqual(parse_stream_id(stream), sid)
            back = from_stream_id(kind, parse_stream_id(stream))
            self.assertEqual(str(back), text)
            self.assertEqual(back, x)
            checked += 1
        first4 = data[:4]
        self.assertEqual(len({r[2] for r in first4}), 4)
        self.assertEqual(len({r[3] for r in first4}), 1)
        report_checked("stream.tsv", checked, len(data))
        self.assertGreater(len(data), 0)
        self.assertEqual(checked, len(data))


class StreamInvalidTest(unittest.TestCase):
    def test_every_stream_id_is_refused_with_its_reason(self) -> None:
        data = rows("stream_invalid.tsv")
        checked = 0
        for name, kind, stream, reason in data:
            text = "" if stream == "-" else stream
            self.assertEqual(outcome(lambda: from_stream_id(kind, parse_stream_id(text))), reason, name)
            checked += 1
        report_checked("stream_invalid.tsv", checked, len(data))
        self.assertGreater(len(data), 0)
        self.assertEqual(checked, len(data))


if __name__ == "__main__":
    unittest.main()
