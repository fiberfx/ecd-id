"""One valid and one refused case for each function where the vectors leave a gap, and the Python surface of an Eid."""

import copy
import pickle
import unittest

from eid import (
    ALPHABET,
    SYS,
    Eid,
    Lease,
    Refusal,
    StreamId,
    code_class,
    code_to_string,
    compare,
    format,
    format_lease,
    format_stream_id,
    from_bytes,
    from_parts,
    from_stream_id,
    is_kind,
    legacy_tail,
    new_lease,
    parse,
    parse_code,
    parse_lease,
    parse_stream_id,
    stream_less,
    system_id,
    to_stream_id,
    valid,
    valid_binary,
    valkey_accepts,
    writer_for,
)

from .vectors import outcome, unhex

WORKED = "emq_job_23obzcbapvupu2272222"
WORKED_HEX = "54acfa1c01a1fe839abbd57400140000"
WRITER_MAX = 2**30 - 1


class CodesTest(unittest.TestCase):
    def test_parse_code_and_code_to_string_round_trip_and_classes_follow_the_first_character(self) -> None:
        self.assertEqual(code_to_string(parse_code("emq")), "emq")
        self.assertEqual(parse_code("222"), 0)
        self.assertEqual(parse_code("zzz"), 32767)
        self.assertEqual(code_class(parse_code("emq")), "registered")
        self.assertEqual(code_class(parse_code("3ab")), "detached")
        self.assertEqual(code_class(parse_code("7zz")), "detached")
        self.assertEqual(code_class(parse_code("2ab")), "reserved")
        self.assertTrue(is_kind(parse_code("job")))
        self.assertEqual(code_to_string(SYS), "sys")

    def test_parse_code_refuses_uppercase_wrong_length_0_1_and_non_ascii(self) -> None:
        for s in ("EMQ", "em", "emqq", "", "e0q", "e1q", "eé", "e\u0000q", "emq\n", "eāq"):
            self.assertIsNone(parse_code(s), repr(s))

    def test_is_kind_refuses_a_digit_in_any_position(self) -> None:
        for s in ("2ob", "j2b", "jo3", "234"):
            self.assertFalse(is_kind(parse_code(s)), s)

    def test_the_alphabet_is_32_distinct_symbols_in_ascii_order(self) -> None:
        self.assertEqual(len(ALPHABET), 32)
        self.assertEqual("".join(sorted(set(ALPHABET))), ALPHABET)


class EidTest(unittest.TestCase):
    def test_fields_of_the_worked_example(self) -> None:
        x = parse(WORKED)
        self.assertEqual(x.system, "emq")
        self.assertEqual(x.kind, "job")
        self.assertEqual(x.reserved, 0)
        self.assertEqual(x.ms, 1795271400123)
        self.assertEqual(x.writer, 895287301)
        self.assertEqual(x.count, 0)
        self.assertEqual(x.class_, "registered")
        self.assertEqual(x.bytes.hex(), WORKED_HEX)
        self.assertEqual(bytes(x), unhex(WORKED_HEX))
        self.assertEqual(x.system_code, parse_code("emq"))
        self.assertEqual(x.kind_code, parse_code("job"))
        self.assertEqual(str(x), WORKED)
        self.assertEqual(f"{x}", WORKED)
        self.assertEqual(repr(x), f"eid.parse('{WORKED}')")
        self.assertEqual(parse("emq_job_23obzcbapvupu2272226").count, 1)

    def test_a_str_is_read_as_utf8_and_bytes_as_they_are(self) -> None:
        self.assertEqual(str(parse(WORKED.encode("ascii"))), WORKED)
        self.assertEqual(str(parse(bytearray(WORKED, "ascii"))), WORKED)
        self.assertEqual(str(parse(memoryview(WORKED.encode("ascii")))), WORKED)
        # 27 characters but 28 UTF-8 bytes: the alphabet step refuses, not the length step
        self.assertEqual(outcome(lambda: parse("emq_job_23obébapvupu2272222")), "alphabet")
        # 28 characters but 29 UTF-8 bytes
        self.assertEqual(outcome(lambda: parse("emq_job_23obécbapvupu2272222")), "length")
        # a lone surrogate is three bytes, as U+FFFD is in other implementations
        self.assertEqual(outcome(lambda: parse(WORKED[:12] + "\ud800" + WORKED[15:])), "alphabet")
        self.assertEqual(outcome(lambda: parse(WORKED[:12] + "\ud800" + WORKED[14:])), "length")

    def test_a_non_text_input_is_a_type_error_not_a_refusal(self) -> None:
        with self.assertRaises(TypeError):
            parse(28)  # type: ignore[arg-type]
        with self.assertRaises(TypeError):
            Eid(16)  # type: ignore[arg-type]
        with self.assertRaises(TypeError):
            Eid(WORKED)  # type: ignore[arg-type]
        with self.assertRaises(TypeError):
            valid(None)  # type: ignore[arg-type]

    def test_an_eid_is_immutable_hashable_and_ordered(self) -> None:
        a = parse(WORKED)
        b = parse("emq_job_23obzcbapvupu2272226")
        with self.assertRaises(AttributeError):
            a.ms = 0  # type: ignore[misc]
        with self.assertRaises(AttributeError):
            a._b = b""  # type: ignore[misc]
        with self.assertRaises(AttributeError):
            del a._b
        self.assertEqual(len({a, parse(WORKED), from_bytes(unhex(WORKED_HEX))}), 1)
        self.assertEqual({a: 1}[parse(WORKED)], 1)
        self.assertTrue(a < b and a <= b and b > a and b >= a and a != b)
        self.assertFalse(a < a or a > a)
        self.assertEqual(sorted([b, a]), [a, b])
        self.assertNotEqual(a, unhex(WORKED_HEX))
        self.assertNotEqual(a, WORKED)
        with self.assertRaises(TypeError):
            _ = a < WORKED  # type: ignore[operator]

    def test_an_eid_survives_pickle_and_copy(self) -> None:
        a = parse(WORKED)
        self.assertEqual(pickle.loads(pickle.dumps(a)), a)
        self.assertEqual(copy.copy(a), a)
        self.assertEqual(copy.deepcopy(a), a)

    def test_the_constructor_copies_a_mutable_input(self) -> None:
        data = bytearray(unhex(WORKED_HEX))
        x = Eid(data)
        data[0] = 0
        self.assertEqual(x.bytes.hex(), WORKED_HEX)

    def test_a_set_reserved_bit_is_refused_not_repaired(self) -> None:
        bad = bytearray(unhex(WORKED_HEX))
        bad[3] |= 1
        self.assertEqual(outcome(lambda: from_bytes(bad)), "reserved")

    def test_format_accepts_an_id_or_checked_bytes_and_refuses_unchecked_ones(self) -> None:
        self.assertEqual(format(parse(WORKED)), WORKED)
        self.assertEqual(format(unhex(WORKED_HEX)), WORKED)
        self.assertEqual(outcome(lambda: format(bytes(16))), "system-reserved")
        self.assertEqual(outcome(lambda: format(bytes(3))), "length")

    def test_valid_and_valid_binary_answer_true_for_good_input_and_false_for_refused_input(self) -> None:
        self.assertTrue(valid(WORKED))
        self.assertFalse(valid(WORKED.upper()))
        self.assertFalse(valid(""))
        self.assertTrue(valid_binary(unhex(WORKED_HEX)))
        self.assertFalse(valid_binary(bytes(16)))
        self.assertFalse(valid_binary(b""))

    def test_a_refusal_is_a_value_error_carrying_its_reason_as_message_and_field(self) -> None:
        with self.assertRaises(Refusal) as caught:
            parse("")
        self.assertIsInstance(caught.exception, ValueError)
        self.assertEqual(caught.exception.reason, "length")
        self.assertEqual(str(caught.exception), "length")
        self.assertEqual(caught.exception.args, ("length",))
        again = pickle.loads(pickle.dumps(caught.exception))
        self.assertEqual(again.reason, "length")

    def test_a_detached_id_has_the_detached_class(self) -> None:
        self.assertEqual(from_parts("3ab", "evt", 1, 2, 3).class_, "detached")

    def test_compare_follows_byte_order_system_kind_time_writer_count(self) -> None:
        a = parse("emq_job_23obzcbapvupu2272222")
        b = parse("emq_job_23obzcbapvupu2272226")
        self.assertEqual(compare(a, b), -1)
        self.assertEqual(compare(b, a), 1)
        self.assertEqual(compare(a, parse(WORKED)), 0)
        hi = from_parts("emq", "job", 2**32, 0, 0)
        lo = from_parts("emq", "job", 2**32 - 1, WRITER_MAX, 262143)
        self.assertEqual(compare(lo, hi), -1)


class FromPartsTest(unittest.TestCase):
    def test_the_extremes_pack_and_unpack_exactly(self) -> None:
        x = from_parts(system="zzz", kind="zzz", ms=2**48 - 1, writer=WRITER_MAX, count=262143)
        self.assertEqual(str(x), "zzz_zzz_bzzzzzzzzzzzzzzzzzzw")
        self.assertEqual((x.ms, x.writer, x.count), (2**48 - 1, WRITER_MAX, 262143))
        self.assertEqual(from_parts("a22", "aaa", 0, 0, 0).ms, 0)

    def test_a_system_or_kind_may_be_given_as_a_code(self) -> None:
        x = from_parts(parse_code("emq"), parse_code("job"), 1, 1, 1)
        self.assertEqual((x.system, x.kind), ("emq", "job"))

    def test_refuses_a_reserved_system_a_bad_kind_an_out_of_range_field_and_a_malformed_system(self) -> None:
        ok = {"system": "emq", "kind": "job", "ms": 1, "writer": 1, "count": 1}
        cases = [
            ({"system": "2ab"}, "system-reserved"),
            ({"kind": "j2b"}, "kind"),
            ({"kind": "JOB"}, "kind"),
            ({"kind": "jo"}, "kind"),
            ({"system": "EMQ"}, "alphabet"),
            ({"system": 32768}, "range"),
            ({"system": -1}, "range"),
            ({"ms": 2**48}, "range"),
            ({"ms": -1}, "range"),
            ({"ms": 1.5}, "range"),
            ({"ms": float("nan")}, "range"),
            ({"ms": True}, "range"),
            ({"writer": 2**30}, "range"),
            ({"count": 262144}, "range"),
            ({"count": -1}, "range"),
        ]
        self.assertEqual(outcome(lambda: from_parts(**ok)), "ok")
        for change, reason in cases:
            self.assertEqual(outcome(lambda: from_parts(**{**ok, **change})), reason, change)

    def test_system_id_is_the_system_with_kind_sys_and_a_zero_local_part(self) -> None:
        self.assertEqual(str(system_id("emq")), "emq_sys_22222222222222222222")
        self.assertEqual(str(system_id(parse_code("kan"))), "kan_sys_22222222222222222222")
        self.assertEqual(outcome(lambda: system_id("2ab")), "system-reserved")


class StreamIdTest(unittest.TestCase):
    def test_parse_and_format_round_trip_including_the_largest_64_bit_numbers(self) -> None:
        s = parse_stream_id("18446744073709551615-0")
        self.assertEqual(s.ms, 2**64 - 1)
        self.assertEqual(format_stream_id(s), "18446744073709551615-0")
        self.assertEqual(format_stream_id(parse_stream_id("0-0")), "0-0")

    def test_parse_refuses_sign_leading_zero_extra_dashes_spaces_and_overflow(self) -> None:
        for s in (
            "",
            "-",
            "1",
            "1-",
            "-1",
            "1-2-3",
            "+1-2",
            "-1-2",
            "01-2",
            "1-02",
            "1 -2",
            "1-2 ",
            "1-2\n",
            "1.5-2",
            "18446744073709551616-0",
            "0-18446744073709551616",
            "1e3-2",
            "０-1",
            "١-2",
            "1_0-2",
            "9" * 5000 + "-1",
        ):
            self.assertEqual(outcome(lambda: parse_stream_id(s)), "stream-syntax", repr(s[:40]))

    def test_stream_less_orders_by_ms_then_seq(self) -> None:
        a = StreamId(1, 5)
        self.assertTrue(stream_less(a, StreamId(1, 6)))
        self.assertTrue(stream_less(a, StreamId(2, 0)))
        self.assertFalse(stream_less(a, a))
        self.assertFalse(stream_less(StreamId(2, 0), a))

    def test_to_stream_id_and_legacy_tail_of_the_worked_example(self) -> None:
        x = parse(WORKED)
        self.assertEqual(format_stream_id(to_stream_id(x)), "1795271400123-3050860491784323072")
        self.assertEqual(legacy_tail(x), 895287301 * 262144)

    def test_from_stream_id_inverts_and_refuses_a_reserved_system_a_bad_kind_and_a_range_error(self) -> None:
        x = parse(WORKED)
        self.assertEqual(from_stream_id("job", to_stream_id(x)), x)
        # the kind is not in the stream id: another kind gives another id
        self.assertNotEqual(from_stream_id("evt", to_stream_id(x)), x)
        self.assertEqual(outcome(lambda: from_stream_id("job", StreamId(1, 0))), "system-reserved")
        self.assertEqual(outcome(lambda: from_stream_id("j0b", StreamId(1, 1 << 48))), "kind")
        self.assertEqual(outcome(lambda: from_stream_id("job", StreamId(1 << 48, 1 << 48))), "stream-range")
        self.assertEqual(outcome(lambda: from_stream_id("job", StreamId(1, 1 << 63))), "stream-range")
        self.assertEqual(outcome(lambda: from_stream_id("job", StreamId(-1, 1 << 58))), "stream-range")
        self.assertEqual(outcome(lambda: from_stream_id("job", StreamId(1, -1))), "stream-range")

    def test_valkey_accepts_an_entry_greater_than_the_top_and_not_0_0(self) -> None:
        top = StreamId(10, 5)
        self.assertTrue(valkey_accepts(None, StreamId(1, 0)))
        self.assertTrue(valkey_accepts(top, StreamId(10, 6)))
        self.assertTrue(valkey_accepts(top, StreamId(11, 0)))
        self.assertFalse(valkey_accepts(top, top))
        self.assertFalse(valkey_accepts(top, StreamId(10, 4)))
        self.assertFalse(valkey_accepts(None, StreamId(0, 0)))


class LeaseTest(unittest.TestCase):
    def test_new_lease_builds_a_lease_and_format_lease_writes_it_back(self) -> None:
        lease = new_lease("emq", 895287301, 1795271400000, 1795357800000)
        self.assertEqual(format_lease(lease), "emq:895287301:1795271400000:1795357800000")
        self.assertEqual(parse_lease(format_lease(lease)), lease)
        self.assertEqual(lease, Lease("emq", 895287301, 1795271400000, 1795357800000))
        self.assertEqual(new_lease(parse_code("kan"), 0, 1, 1).system, "kan")
        with self.assertRaises(AttributeError):
            lease.writer = 0  # type: ignore[misc]

    def test_new_lease_refuses_by_the_same_reasons_as_the_text_form(self) -> None:
        cases = [
            (("2ab", 1, 1000, 2000), "lease-system"),
            (("EMQ", 1, 1000, 2000), "lease-syntax"),
            ((32768, 1, 1000, 2000), "lease-syntax"),
            (("emq", -1, 1000, 2000), "lease-syntax"),
            (("emq", 1.5, 1000, 2000), "lease-syntax"),
            (("emq", 1, 1000, 2**64), "lease-syntax"),
            (("emq", 2**30, 1000, 2000), "lease-writer"),
            (("emq", 1, 0, 2000), "lease-window"),
            (("emq", 1, 2000, 1000), "lease-window"),
            (("emq", 1, 1000, 1000 + 86_400_000), "ok"),
            (("emq", 1, 1000, 1001 + 86_400_000), "lease-window"),
            (("emq", 1, 1000, 2**48), "lease-window"),
            (("emq", 1, 1000, 2**64 - 1), "lease-window"),
        ]
        for args, reason in cases:
            self.assertEqual(outcome(lambda: new_lease(*args)), reason, args)
            # the text form gives the same answer wherever the arguments have one
            if isinstance(args[0], str) and all(isinstance(n, int) and n >= 0 for n in args[1:]):
                if args[3] < 2**64:
                    text = ":".join(str(a) for a in args)
                    self.assertEqual(outcome(lambda: parse_lease(text)), reason, text)

    def test_parse_lease_refuses_21_digits_2_64_plus_signs_fractions_and_empty_text(self) -> None:
        for s in (
            "",
            "emq",
            "emq:1:1000:18446744073709551616",
            "emq:+1:1000:2000",
            "emq:1.0:1000:2000",
            "emq:1:1000:00001",
            "emq:1:100000000000000000000:2000000000000000000000",
            "emq:١:1000:2000",
            "emq:1:1000:" + "9" * 5000,
        ):
            self.assertEqual(outcome(lambda: parse_lease(s)), "lease-syntax", repr(s[:40]))
        self.assertEqual(outcome(lambda: parse_lease("emq:1:1000:18446744073709551615")), "lease-window")

    def test_writer_for_is_the_second_mod_2_17_times_2_13_plus_the_index_and_refuses_bad_input(self) -> None:
        self.assertEqual(writer_for(1795271400, 5), 895287301)
        self.assertEqual(writer_for(131072 + 3, 8192 + 7), 3 * 8192 + 7)
        self.assertEqual(writer_for(2**70, 0), 0)
        self.assertEqual(outcome(lambda: writer_for(-1, 0)), "range")
        self.assertEqual(outcome(lambda: writer_for(1, 0.5)), "range")  # type: ignore[arg-type]
        self.assertEqual(outcome(lambda: writer_for(True, 0)), "range")


class SeededRandomTest(unittest.TestCase):
    def test_text_and_bytes_round_trip_and_sorting_texts_sorts_ids(self) -> None:
        state = 0x2545F491

        def next_(n: int) -> int:
            nonlocal state
            state = (state * 1664525 + 1013904223) & 0xFFFFFFFF
            return state % n

        letters = "abcdefghijklmnopqrstuvwxyz"

        def word(symbols: str, first: str) -> str:
            return first[next_(len(first))] + symbols[next_(len(symbols))] + symbols[next_(len(symbols))]

        ids = [
            from_parts(
                system=word(ALPHABET, letters + "34567"),
                kind=word(letters, letters),
                ms=next_(2**20) * 2**28 + next_(2**28),
                writer=next_(2**15) * 2**15 + next_(2**15),
                count=next_(262144),
            )
            for _ in range(2000)
        ]
        for x in ids:
            self.assertEqual(parse(str(x)), x)
            self.assertEqual(str(from_bytes(x.bytes)), str(x))
            self.assertEqual(from_stream_id(x.kind, to_stream_id(x)), x)
        self.assertEqual([str(x) for x in sorted(ids, key=str)], [str(x) for x in sorted(ids)])


if __name__ == "__main__":
    unittest.main()
