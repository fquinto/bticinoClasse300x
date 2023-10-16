#!/usr/bin/env python
# -*- coding: utf-8 -*-
import sys
import time
from __future__ import print_function

for line in sys.stdin:
    if "##" in line:
        print(line[line.find('*'):], end="")
        sys.stdout.flush()
        time.sleep(0.2)
