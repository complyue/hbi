"""
Send and Receive support for numpy arrays

"""

import numpy as np

import hbi

__all__ = ["expose_numpy_helpers", "send_ndarray", "recv_ndarray"]


def expose_numpy_helpers(he: "hbi.HostingEnv"):
    he.expose_ctor(np.dtype)


async def send_ndarray(a: np.ndarray, co: Union["hbi.HoCo", "hbi.PoCo"]):
    await co.send_obj(repr(list(a.shape)))
    await co.send_obj(repr(a.dtype))
    if a.nbytes > 0:
        await co.send_data(a)


async def recv_ndarray(co: Union["hbi.HoCo", "hbi.PoCo"]):
    shape = await co.recv_obj()
    dtype = await co.recv_obj()
    a = np.zeros(shape, dtype)
    if a.nbytes > 0:
        await co.recv_data(a)
    return a

