from __future__ import annotations

import codecs
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "msvc_link_response", Path(__file__).with_name("msvc_link_response.py"))
link_response = importlib.util.module_from_spec(spec)
spec.loader.exec_module(link_response)


class MsvcLinkResponseTests(unittest.TestCase):
    def test_long_line_preserves_every_non_newline_byte(self):
        data = b'"object with spaces.obj"\n' + b'"library with spaces.lib" ' * 2000
        self.assertGreater(max(map(len, data.splitlines())), 32768)
        normalized = link_response.normalize_newlines(data)
        self.assertEqual(normalized.replace(b"\r\n", b"\n"), data)
        self.assertEqual(normalized.splitlines(), data.splitlines())

    def test_crlf_is_unchanged_and_mixed_newlines_are_idempotent(self):
        self.assertEqual(link_response.normalize_newlines(b"a\r\nb\r\n"), b"a\r\nb\r\n")
        mixed = b"a\r\nb\nc\r\n"
        normalized = link_response.normalize_newlines(mixed)
        self.assertEqual(normalized, b"a\r\nb\r\nc\r\n")
        self.assertEqual(link_response.normalize_newlines(normalized), normalized)

    def test_unicode_byte_order_and_bom_are_preserved(self):
        for bom, encoding in ((codecs.BOM_UTF16_LE, "utf-16-le"),
                              (codecs.BOM_UTF16_BE, "utf-16-be")):
            with self.subTest(encoding=encoding):
                source = bom + '"caf\u00e9.obj"\n"\u6587.lib"\n'.encode(encoding)
                expected = bom + '"caf\u00e9.obj"\r\n"\u6587.lib"\r\n'.encode(encoding)
                self.assertEqual(link_response.normalize_newlines(source), expected)
        utf8 = codecs.BOM_UTF8 + '"caf\u00e9.obj"\n'.encode("utf-8")
        self.assertEqual(link_response.normalize_newlines(utf8), utf8.replace(b"\n", b"\r\n"))

    def test_child_sees_normalized_copy_original_and_exit_code_survive(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "response with spaces.rsp"
            original = b'"object with spaces.obj"\n/NODEFAULTLIB\n'
            source.write_bytes(original)
            output = root / "child.json"
            child = root / "child.py"
            child.write_text(
                "from pathlib import Path\nimport json,sys\n"
                "p=Path(sys.argv[2][1:])\n"
                "Path(sys.argv[1]).write_text(json.dumps({'path':str(p),'bytes':list(p.read_bytes()),'tail':sys.argv[3:]}))\n"
                "sys.exit(17)\n", encoding="utf-8")
            status = link_response.run([sys.executable, str(child), str(output),
                                        "@" + str(source), '/EXPORT:quoted name'])
            self.assertEqual(status, 17)
            record = json.loads(output.read_text())
            self.assertEqual(bytes(record["bytes"]), original.replace(b"\n", b"\r\n"))
            self.assertEqual(record["tail"], ['/EXPORT:quoted name'])
            self.assertEqual(source.read_bytes(), original)
            self.assertFalse(Path(record["path"]).exists())

    def test_missing_response_is_forwarded(self):
        status = link_response.run([sys.executable, "-c",
                                    "import sys;sys.exit(0 if sys.argv[1]=='@missing.rsp' else 1)",
                                    "@missing.rsp"])
        self.assertEqual(status, 0)

    def test_empty_command_is_an_error(self):
        self.assertEqual(link_response.run([]), 2)


if __name__ == "__main__":
    unittest.main()
