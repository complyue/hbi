import json
from math import nan

from collections import OrderedDict

__all__ = [
    "nil",
    "true",
    "false",
    "null",
    "nan",
    "NaN",
    "JSONObj",
    "JSONArray",
    "JSONStr",
]

nil = None
true = True
false = False
null = None
NaN = nan


class JSONObj(OrderedDict):
    """
    A subclass of `collections.OrderedDict`, for JSON object with proper repr and str value in json form.

    """

    def __repr__(self):
        return json.dumps(self)

    __str__ = __repr__


class JSONArray(list):
    """
    A subclass of standard python list, for JSON array with proper repr and str value in json form.

    """

    def __repr__(self):
        return json.dumps(self)

    __str__ = __repr__


class JSONStr(str):
    """
    A subclass of standard python str, for JSON string with proper repr in json form.

    """

    def __repr__(self):
        return json.dumps(self)

    def __str__(self):
        return self
