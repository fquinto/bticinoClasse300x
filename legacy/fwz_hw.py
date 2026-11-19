"""Optional <req_hw_ver> tag for the fwz.xml manifest (pure, no dependencies)."""
import re

_TAG = re.compile(r'<req_hw_ver>\s*([^<]*?)\s*</req_hw_ver>')


def set_req_hw_ver(xml, ver):
    """Return xml with <req_hw_ver>ver</req_hw_ver> as the last child of <fwz>.

    Idempotent: same value already present -> unchanged. A different value
    already present -> ValueError (we never overwrite the vendor's choice).
    """
    found = _TAG.search(xml)
    if found:
        if found.group(1) != ver:
            raise ValueError(f'fwz.xml already requires hardware {found.group(1)}, not {ver}')
        return xml
    head, sep, tail = xml.rpartition('</fwz>')
    if not sep:
        raise ValueError('fwz.xml has no closing </fwz>')
    return f'{head}<req_hw_ver>{ver}</req_hw_ver>\n</fwz>{tail}'
