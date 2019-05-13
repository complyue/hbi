import asyncio
import traceback
from typing import *

from ..log import *

__all__ = ["Conver", "PoCo", "HoCo"]

logger = get_logger(__name__)


class Conver:
    """
    Abstract Conversation

    All conversations can:
      * push code to peer for landing
        * the landed result obj can optionally be received by peer conversation
      * push data/stream to peer for receiving by peer conversation

    """

    __slots__ = ()

    @property
    def co_seq(self) -> str:
        return self._co_seq

    async def send_code(self, code: str):
        raise NotImplementedError

    async def send_obj(self, code: str):
        raise NotImplementedError

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
        raise NotImplementedError

    async def recv_obj(self) -> object:
        raise NotImplementedError

    async def recv_data(
        self,
        bufs: Union[
            bytearray,
            memoryview,
            # or sequence of them, i.e. streaming on-the-fly
            Sequence[Union[bytearray, memoryview]],
        ],
    ):
        raise NotImplementedError

    def is_closed(self):
        return self._send_done_fut.done()

    async def wait_closed(self):
        await self._send_done_fut


class PoCo(Conver):
    """
    Posting Conversation

    """

    __slots__ = (
        "hbic",
        "_co_seq",
        "_send_done_fut",
        "_begin_acked_fut",
        "_end_acked_fut",
        "_roq",
        "_rdq",
    )

    def __init__(self, hbic, co_seq):
        self.hbic = hbic
        self._co_seq = co_seq

        self._send_done_fut = asyncio.get_running_loop().create_future()
        self._begin_acked_fut = None
        self._end_acked_fut = None

        # obj receiving queue
        self._roq = asyncio.Queue()
        # data receiving queue
        self._rdq = asyncio.Queue()

    async def __aenter__(self):
        await self.begin()
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.end()

    async def begin(self):
        if self._begin_acked_fut is not None:
            raise asyncio.InvalidStateError("co_begin sent already!")

        hbic = self.hbic
        coq = hbic._coq
        while coq:
            tail_co = coq[-1]
            if tail_co.is_closed():
                break
            await tail_co.wait_closed()

        self._begin_acked_fut = asyncio.get_running_loop().create_future()

        coq.append(self)

        try:
            await hbic._send_text(self.co_seq, b"co_begin")
        except Exception as exc:
            self._begin_acked_fut.set_exception(exc)
            raise

    async def end(self):
        try:
            if self._begin_acked_fut is None:
                raise asyncio.InvalidStateError("co_begin not sent yet!")
            if self._end_acked_fut is not None:
                raise asyncio.InvalidStateError("co_end sent already!")

            hbic = self.hbic
            assert self is hbic._coq[-1], "co not current sender?!"

            self._end_acked_fut = asyncio.get_running_loop().create_future()

            try:
                await hbic._send_text(self.co_seq, b"co_end")

                self._send_done_fut.set_result(self.co_seq)
            except Exception as exc:
                self._end_acked_fut.set_exception(exc)

                if not self._send_done_fut.done():
                    self._send_done_fut.set_exception(exc)

                raise
        finally:
            if not self._send_done_fut.done():
                self._send_done_fut.set_exception(
                    asyncio.IncompleteReadError("abnormal co end")
                )

    def _begin_acked(self, co_seq):
        if self.co_seq != co_seq:
            raise asyncio.InvalidStateError("co_seq mismatch ?!")

        fut = self._begin_acked_fut
        if fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")

        fut.set_result(co_seq)

    def _end_acked(self, co_seq):
        if self.co_seq != co_seq:
            raise asyncio.InvalidStateError("co_seq mismatch ?!")

        fut = self._end_acked_fut
        if fut is None:
            raise asyncio.InvalidStateError("co_end not sent yet!")

        fut.set_result(co_seq)

    async def wait_ack_begin(self):
        fut = self._begin_acked_fut
        if fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")
        await fut

    async def wait_ack_end(self):
        fut = self._end_acked_fut
        if fut is None:
            raise asyncio.InvalidStateError("co_end not sent yet!")
        await fut

    async def send_code(self, code: str):
        if self._begin_acked_fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")

        hbic = self.hbic
        assert self is hbic._coq[-1], "co not current sender?!"

        await hbic._send_text(code)

    async def send_obj(self, code: str):
        if self._begin_acked_fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")

        hbic = self.hbic
        assert self is hbic._coq[-1], "co not current sender?!"

        await hbic._send_text(code, b"co_recv")

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
        if self._begin_acked_fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")

        hbic = self.hbic
        assert self is hbic._coq[-1], "co not current sender?!"

        await hbic._send_data(bufs)

    async def recv_obj(self):

        recv_fut = asyncio.ensure_future(self._roq.get())

        disc_fut = self.hbic._disc_fut
        done, pending = await asyncio.wait(
            (disc_fut, recv_fut), return_when=asyncio.FIRST_COMPLETED
        )
        if recv_fut.done():
            return await recv_fut  # exception will be propagated if ever raised
        # the done one must be disc_fut
        raise disc_fut.exception() or asyncio.InvalidStateError(f"hbic disconnected")

    async def recv_data(
        self,
        bufs: Union[
            bytearray,
            memoryview,
            # or sequence of them, i.e. streaming on-the-fly
            Sequence[Union[bytearray, memoryview]],
        ],
    ):

        recv_fut = asyncio.get_running_loop().create_future()
        await self._rdq.put((bufs, recv_fut))

        disc_fut = self.hbic._disc_fut
        done, pending = await asyncio.wait(
            (disc_fut, recv_fut), return_when=asyncio.FIRST_COMPLETED
        )
        if recv_fut.done():
            await recv_fut  # exception will be propagated if ever raised
            return
        # the done one must be disc_fut
        raise disc_fut.exception() or asyncio.InvalidStateError(f"hbic disconnected")


class HoCo(Conver):
    """
    Hosting Conversation

    """

    __slots__ = ("hbic", "_co_seq", "_send_done_fut")

    def __init__(self, hbic, co_seq):
        self.hbic = hbic
        self._co_seq = co_seq

        self._send_done_fut = asyncio.get_running_loop().create_future()

    async def send_code(self, code: str):
        hbic = self.hbic
        if self is not hbic.ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")
        assert self is hbic._coq[-1], "co not current sender?!"

        await hbic._send_text(code)

    async def send_obj(self, code: str):
        hbic = self.hbic
        if self is not hbic.ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")
        assert self is hbic._coq[-1], "co not current sender?!"

        await hbic._send_text(code, b"co_recv")

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
        hbic = self.hbic
        if self is not hbic.ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")
        assert self is hbic._coq[-1], "co not current sender?!"

        await hbic._send_text(self.co_seq, b"po_data")
        await hbic._send_data(bufs)

    async def recv_obj(self):
        hbic = self.hbic
        if self is not hbic.ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")

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
        hbic = self.hbic
        if self is not hbic.ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")

        await hbic._recv_data(bufs)
