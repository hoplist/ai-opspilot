#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import math


def percentile(values, p):
    if not values:
        return 0.0
    values = sorted(values)
    if p <= 0:
        return values[0]
    if p >= 1:
        return values[-1]
    k = (len(values) - 1) * p
    f = math.floor(k)
    c = math.ceil(k)
    if f == c:
        return values[int(k)]
    return values[f] + (values[c] - values[f]) * (k - f)
