import asyncio
import inspect
from collections import deque
from typing import *

from ..log import *

__all__ = ["PostingEnd", "PoCo"]

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
        notif is shorthand to (implicitly) create a posting conversation, which is closed
        immediately after `code` is sent with it.

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
        notif_data is shorthand to (implicitly) create a posting conversation, which is closed
        immediately after `code` and `bufs` are sent with it.

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
        co = await self._hbic._new_po_co()

        self._co = co
        return co

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        hbic = self._hbic
        co = self._co

        if not co._send_done_fut.done():
            await hbic._po_co_finish_send(co)

        recv_done_fut = co._recv_done_fut
        if recv_done_fut is not None:
            recv_done_fut.set_result(None)

    def __enter__(self):
        raise TypeError("you'd use `async with po.co():` instead of `with po.co():` ?")


class PoCo:
    """
    PoCo is the active, posting conversation.

    A PoCo is created and used from application like:
        async with po.co() as co:
            await co.send_xxx(...)

            await co.start_recv()

            await co.recv_xxx(...)

    """

    __slots__ = (
        "_hbic",
        "_co_seq",
        "_send_done_fut",
        "_begin_acked_fut",
        "_recv_done_fut",
        "_end_acked_fut",
    )

    def __init__(self, hbic, co_seq: str):
        self._hbic = hbic
        self._co_seq = co_seq

        loop = asyncio.get_running_loop()
        self._send_done_fut = loop.create_future()
        self._begin_acked_fut = loop.create_future()
        self._recv_done_fut = None
        self._end_acked_fut = loop.create_future()

    @property
    def co_seq(self):
        """
        co_seq returns the sequence number of this conversation.

        The sequence number of a posting conversation is assigned by the posting endpoint created
        it, the value does not necessarily be unique across a long time period, but won't repeat
        among a lot of conversations per sent over a wire in line.

        """
        return self._co_seq

    async def send_code(self, code: str):
        """
        send_code sends `code` to peer's hosting endpoint for landing by its hosting environment.

        Only side effects are expected from landing of `code` at peer site.

        Note this can only be called in `send` stage, and from the aio task which created this
        conversation.

        """
        hbic = self._hbic

        if self._send_done_fut.done():
            raise asyncio.InvalidStateError("po co not in send stage")
        assert self is hbic._sender, "po co not current sender ?!"

        await hbic._send_packet(code)

    async def send_obj(self, code: str):
        """
        send_obj sends `code` to peer's hosting endpoint for landing by its hosting environment.
        
        The respective hosting conversation at peer site is expected to receive the result value
        from landing of `code`, by calling HoCo.recv_obj()
        
        Note this can only be called in `send` stage, and from the aio task which created this
        conversation.

        """

        hbic = self._hbic

        if self._send_done_fut.done():
            raise asyncio.InvalidStateError("po co not in send stage")
        assert self is hbic._sender, "po co not current sender ?!"

        await hbic._send_packet(code, b"co_recv")

    async def send_data(
        self,
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
        send_data sends a single chunk of binary data or a data stream in form of a series of 
        chunks to peer site.

        The respective hosting conversation at peer site is expected to receive the data by
        calling HoCo.recv_data()

        Note this can only be called in `send` stage, and from the aio task which created this
        conversation.

        """

        hbic = self._hbic

        if self._send_done_fut.done():
            raise asyncio.InvalidStateError("po co not in send stage")
        assert self is hbic._sender, "po co not current sender ?!"

        await hbic._send_data(bufs)

    async def start_recv(self):
        """
        start_recv transits this posting conversation from `send` stage to `recv` stage.

        Once in `recv` stage, no `send` operation can be performed any more with this conversation,
        the underlying wire is released for other posting conversation to start off.

        Note this can only be called in `send` stage, and from the aio task which created this
        conversation.

        """

        hbic = self._hbic

        if self._send_done_fut.done():
            raise asyncio.InvalidStateError("po co not in send stage")

        await hbic._po_co_finish_send(self)

        self._recv_done_fut = asyncio.get_running_loop().create_future()

        # wait begin of ho co ack
        await self._begin_acked_fut

        assert self is hbic._recver, "po co not current recver ?!"

    async def recv_obj(self):
        """
        recv_obj returns the landed result of a piece of back-script `code` sent with the triggered
        hosting conversation at remote site via HoCo.send_obj(code)

        Note this can only be called in `recv` stage, and from the aio task which created this
        conversation.

        """

        if not self._send_done_fut.done():
            raise asyncio.InvalidStateError("po co still in send stage")

        if self._recv_done_fut is None:
            raise asyncio.InvalidStateError("po co not in recv stage")

        assert self._begin_acked_fut.done(), "po co response not started ?!"

        hbic = self._hbic
        assert self is hbic._recver, "po co not current recver ?!"

        return await hbic._recv_one_obj()

    async def recv_data(
        self,
        bufs: Union[
            bytearray,
            memoryview,
            # or sequence of them, i.e. streaming on-the-fly
            Sequence[Union[bytearray, memoryview]],
        ],
    ):
        """
        recv_data receives the binary data/stream sent with the triggered hosting conversation at
        remote site via HoCo.send_data()

        Note this can only be called in `recv` stage, and from the aio task which created this
        conversation.

        """

        if not self._send_done_fut.done():
            raise asyncio.InvalidStateError("po co still in send stage")

        if self._recv_done_fut is None:
            raise asyncio.InvalidStateError("po co not in recv stage")

        assert self._begin_acked_fut.done(), "po co response not started ?!"

        hbic = self._hbic
        assert self is hbic._recver, "po co not current recver ?!"

        await hbic._recv_data(bufs)

    async def wait_completed(self):
        """
        wait_completed returns until this posting conversation has been fully processed with
        the triggered hosting conversation at remote site done, or asyncio.InvalidStateError
        will be raised if the underlying HBI connection is disconnected before that.

        Subsequent processes depending on the success of this conversation's completion can
        call this to await the signal of proceed.

        Successful return from this method can confirm the final success of
        this conversation, as well its `recv` stage. i.e. all peer-scripting-code and data/stream
        sent with this conversation has been landed by peer's hosting endpoint, with a triggered
        hosting conversation, and all back-scripts (plus data/stream if any) as the response
        from that hosting conversation has been landed by local hosting endpoint, and received
        with this posting conversation (if any recv ops involved).

        """

        await self._end_acked_fut  # propagate exception if any
