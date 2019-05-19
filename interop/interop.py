import json

import hbi

from collections import OrderedDict

__all__ = ["JSONObj", "JSONArray", "JSONStr", "expose_interop_values"]


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


def expose_interop_values(he: "hbi.HostingEnv"):
    globals_ = he.globals

    exec(
        rf"""
from math import nan
NaN = nan

nil = None
true = True
false = False
null = None
""",
        globals_,
        globals_,
    )

    globals_["JSONObj"] = JSONObj
    globals_["JSONArray"] = JSONArray
    globals_["JSONStr"] = JSONStr
