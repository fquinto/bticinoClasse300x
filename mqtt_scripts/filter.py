#!/usr/bin/env python
# -*- coding: utf-8 -*-
from __future__ import print_function
import sys
import time

for line in sys.stdin:
    if "##" in line:
        print(line[line.find('*'):], end="")
        sys.stdout.flush()
        time.sleep(0.2)
