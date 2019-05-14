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

    def co(self):
        """
        co creates a context manager, meant to be `async with`, for a fresh new posting conversation.

        """
        return _PoCoCtx(self._hbic)

    async def notif(self, code: str):
        """
        notif is short hand to push a piece/sequence of code to peer for landing,
        within a posting conversation.

        """

        hbic = self._hbic
        async with self.co():
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
        async with self.co():
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


class _PoCoCtx:
    __slots__ = ("_hbic", "_co")

    def __init__(self, hbic):
        self._hbic = hbic
        self._co = None

    async def __aenter__(self):
        co = await self._hbic.new_po_co()
        assert co._begin_acked_fut is not None

        self._co = co
        return co

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        hbic = self._hbic
        co = self._co

        try:
            if co._begin_acked_fut is None:
                raise asyncio.InvalidStateError("co_begin not sent yet!")
            if co._end_acked_fut is not None:
                raise asyncio.InvalidStateError("co_end sent already!")

            assert co is hbic._coq[-1], "co not current sender?!"

            co._end_acked_fut = asyncio.get_running_loop().create_future()

            try:
                await hbic._send_packet(co.co_seq, b"co_end")

                co._send_done_fut.set_result(co.co_seq)
            except Exception as exc:
                co._end_acked_fut.set_exception(exc)

                if not co._send_done_fut.done():
                    co._send_done_fut.set_exception(exc)

                raise
        finally:
            if not co._send_done_fut.done():
                co._send_done_fut.set_exception(
                    asyncio.IncompleteReadError("abnormal co end")
                )

    def __enter__(self):
        raise TypeError(
            "do you mean `async with po.co():` instead of `with po.co():` ?"
        )
