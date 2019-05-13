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

    __slots__ = ("hbic", "remote_addr")

    def __init__(self, hbic):
        self.hbic = hbic

        self.remote_addr = "<unwired>"

    def co(self) -> PoCo:
        """
        co creates and returns a posting conversation object, normally to be used
        as an `async with` context manager, or its `begin()` and `end()` should be
        called before and after using its send/recv methods.

        """
        return self.hbic.co()

    async def notif(self, code: Union[str, Sequence[str]]):
        """
        notif is short hand to push a piece/sequence of code to peer for landing,
        within a posting conversation.

        """

        hbic = self.hbic
        async with hbic.co():
            await hbic._send_text(code)

    async def notif_data(
        self,
        code: Union[str, Sequence[str]],
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
        hbic = self.hbic
        async with hbic.co():
            await hbic._send_text(code)
            await hbic._send_data(bufs)
