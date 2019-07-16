import asyncio
import inspect
import socket
import traceback
from typing import *

from .._details import *
from ..log import *
from ..proto import *
from .sock import SocketWire

__all__ = ["serve_unix", "dial_unix"]

logger = get_logger(__name__)


async def serve_unix(
    addr: str,
    he_factory: Callable[[], HostingEnv],
    *,
    wire_buf_high=20 * 1024 * 1024,
    wire_buf_low=6 * 1024 * 1024,
    net_opts: Optional[dict] = None,
):
    """
    serve_unix takes a factory function (which will be asked to produce one hosting
    env for each incoming HBI consumer connection), and returns `asyncio.Server` object
    representing the listening socket.

    """

    if net_opts is None:
        net_opts = {}

    loop = asyncio.get_running_loop()

    server = await loop.create_unix_server(
        lambda: SocketWire(HBIC(he_factory()), wire_buf_high, wire_buf_low),
        path=addr,
        **net_opts,
    )

    return server


async def dial_unix(
    addr: str,
    he: HostingEnv,
    *,
    wire_buf_high=20 * 1024 * 1024,
    wire_buf_low=6 * 1024 * 1024,
    net_opts: Optional[dict] = None,
) -> Tuple[PostingEnd, HostingEnd]:
    """
    dial_unix takes a hosting env, establishes HBI connection to a service at `addr`,
    returns the posting and hosting endpoints pair.

    """

    if net_opts is None:
        net_opts = {}

    loop = asyncio.get_running_loop()

    transport, wire = await loop.create_unix_connection(
        lambda: SocketWire(HBIC(he), wire_buf_high, wire_buf_low), path=addr, **net_opts
    )

    hbic = wire.hbic
    await hbic.wait_connected()

    return hbic.po, hbic.ho
