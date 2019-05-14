import asyncio
import inspect
from collections import deque
from typing import *

from ..log import *
from .co import *

__all__ = ["PostingEnd"]

logger = get_logger(__name__)


class PostingEnd:
    """
    PostingEnd is the application programming interface of an HBI posting endpoint.

    """

    __slots__ = ("_hbic", "_remote_addr")

    def __init__(self, hbic):
        """
        HBI applications should never create a posting endpoint directly.

        """

        self._hbic = hbic

        self._remote_addr = "<unwired>"

    @property
    def ho(self):
        return self._hbic.ho

    @property
    def remote_addr(self):
        return self._remote_addr

    @property
    def net_ident(self):
        return self._hbic.net_ident

    def co(self) -> PoCo:
        """
        co creates and returns a posting conversation object, normally to be used
        as an `async with` context manager, or its `begin()` and `end()` should be
        called before and after using its send/recv methods.

        """
        return self._hbic.co()

    async def notif(self, code: str):
        """
        notif is short hand to push a piece/sequence of code to peer for landing,
        within a posting conversation.

        """

        hbic = self._hbic
        async with hbic.co():
            await hbic._send_packet(code)

    async def notif_data(
        self,
        code: str,
        bufs: Union[
            bytes,
            bytearray,
            memoryview,
            # or sequence of them, i.e. streaming on-the-fly,
            # normally with a generator function call
            Sequence[Union[bytes, bytearray, memoryview]],
        ],
    ):
        """
        notif_data is short hand to push a piece/sequence of code to peer for landing,
        with binary data/stream following, within a posting conversation.

        """
        hbic = self._hbic
        async with hbic.co():
            await hbic._send_packet(code)
            await hbic._send_data(bufs)

    def is_connected(self) -> bool:
        return self._hbic.is_connected()

    async def disconnect(
        self, err_reason: Optional[str] = None, try_send_peer_err: bool = True
    ):
        await self._hbic.disconnect(err_reason, try_send_peer_err)

    async def wait_disconnected(self):
        await self._hbic.wait_disconnected()
