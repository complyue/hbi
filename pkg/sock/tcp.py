import asyncio
import inspect
import socket
import traceback
from typing import *

from .._details import *
from ..log import *
from ..proto import *
from .sock import SocketWire

__all__ = ["serve_tcp", "dial_tcp"]

logger = get_logger(__name__)


async def serve_tcp(
    addr: Union[dict, str, Tuple[str, int]],
    he_factory: Callable[[], HostingEnv],
    *,
    wire_buf_high=20 * 1024 * 1024,
    wire_buf_low=6 * 1024 * 1024,
    net_opts: Optional[dict] = None,
):
    """
    serve_tcp takes a factory function (which will be asked to produce one hosting
    env for each incoming HBI consumer connection), and returns `asyncio.Server` object
    representing the listening tcp.

    """

    if isinstance(addr, str):
        host, *rest = addr.rsplit(":", 1)
        port = 3232
        if len(rest) > 0:
            port = int(rest[0])
        addr = {"host": host, "port": port}
    elif isinstance(addr, tuple):
        host, port = addr
        if not port:  # translate 0 port to default 3232
            port = 3232
        addr = {"host": host, "port": port}

    if net_opts is None:
        net_opts = {}

    loop = asyncio.get_running_loop()

    if "family" not in net_opts:
        # default to IPv4 only
        net_opts["family"] = socket.AF_INET

    server = await loop.create_server(
        lambda: SocketWire(HBIC(he_factory()), wire_buf_high, wire_buf_low),
        host=addr.get("host", "127.0.0.1"),
        port=addr.get("port", 3232),
        **net_opts,
    )

    return server


async def dial_tcp(
    addr: Union[dict, str, Tuple[str, int]],
    he: HostingEnv,
    *,
    wire_buf_high=20 * 1024 * 1024,
    wire_buf_low=6 * 1024 * 1024,
    net_opts: Optional[dict] = None,
) -> Tuple[PostingEnd, HostingEnd]:
    """
    dial_tcp takes a hosting env, establishes HBI connection to a service at `addr`,
    returns the posting and hosting endpoints pair.

    """

    if isinstance(addr, str):
        host, *rest = addr.rsplit(":", 1)
        port = 3232
        if len(rest) > 0:
            port = int(rest[0])
        addr = {"host": host, "port": port}
    elif isinstance(addr, tuple):
        host, port = addr
        if not port:  # translate 0 port to default 3232
            port = 3232
        addr = {"host": host, "port": port}

    if net_opts is None:
        net_opts = {}

    loop = asyncio.get_running_loop()

    if "family" not in net_opts:
        # default to IPv4 only
        net_opts["family"] = socket.AF_INET
    transport, wire = await loop.create_connection(
        lambda: SocketWire(HBIC(he), wire_buf_high, wire_buf_low),
        host=addr.get("host", "127.0.0.1"),
        port=addr.get("port", 3232),
        **net_opts,
    )

    hbic = wire.hbic
    await hbic.wait_connected()

    return hbic.po, hbic.ho

