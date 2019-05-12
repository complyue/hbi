"""
this module meant to be imported by modules serving as HBI contexts,
to provide adaptive behaviors across different languages/runtimes
of the hosting environment

"""
from .interop import *

__all__ = [

    # exports from .interop
    'JSONObj', 'JSONArray', 'JSONStr', 'expose_interop_values',

]
