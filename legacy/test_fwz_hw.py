"""Run: python3 -m unittest legacy/test_fwz_hw.py  (or: cd legacy && python3 -m unittest)."""
import unittest

from fwz_hw import set_req_hw_ver

XML = '<?xml version="1.0"?>\n<fwz>\n<name>C300X</name>\n</fwz>\n'


class ReqHwVer(unittest.TestCase):
    def test_adds_as_last_child(self):
        out = set_req_hw_ver(XML, '02.00.00')
        self.assertIn('</name>\n<req_hw_ver>02.00.00</req_hw_ver>\n</fwz>', out)

    def test_idempotent(self):
        once = set_req_hw_ver(XML, '02.00.00')
        self.assertEqual(once, set_req_hw_ver(once, '02.00.00'))

    def test_other_value_refused(self):
        with self.assertRaises(ValueError):
            set_req_hw_ver(set_req_hw_ver(XML, '03.00.00'), '02.00.00')

    def test_no_fwz_end(self):
        with self.assertRaises(ValueError):
            set_req_hw_ver('<x/>', '02.00.00')


if __name__ == '__main__':
    unittest.main()
