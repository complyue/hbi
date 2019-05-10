import asyncio
import traceback
from typing import *

from ..log import *

__all__ = ["Conver"]

logger = get_logger(__name__)


class Conver:
    """
    Abstract Conversation

    """

    __slots__ = ()

    @property
    def co_seq(self) -> str:
        return self._co_seq

    async def send_code(self, code):
        raise NotImplementedError

    async def send_obj(self, code):
        raise NotImplementedError

    async def send_data(self, bufs):
        raise NotImplementedError

    async def recv_obj(self):
        raise NotImplementedError

    async def recv_data(self, bufs):
        raise NotImplementedError

    def is_ended(self):
        raise NotImplementedError

    async def wait_ended(self):
        await self._send_done_fut


class HoCo(Conver):
    """
    Hosting Conversation

    """

    __slots__ = ("ho", "_co_seq", "_send_done_fut")

    def __init__(self, ho, co_seq):
        self.ho = ho
        self._co_seq = co_seq

        self._send_done_fut = asyncio.get_running_loop().create_future()

    async def send_code(self, code):
        ho = self.ho
        if self is not ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")
        po = ho.po

        await po._send_code(code)

    async def send_obj(self, code):
        ho = self.ho
        if self is not ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")
        po = ho.po

        await po._send_code(code, b"co_recv")

    async def send_data(self, bufs):
        ho = self.ho
        if self is not ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")
        po = ho.po

        await po._send_data(bufs)

    async def recv_obj(self):
        ho = self.ho
        if self is not ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")

        return await ho._recv_obj()

    async def recv_data(self, bufs):
        ho = self.ho
        if self is not ho.co:
            raise asyncio.InvalidStateError("Hosting conversation ended already!")

        await ho._recv_data(bufs)

    def is_ended(self):
        return self._send_done_fut.done()

    async def wait_ended(self):
        await self._send_done_fut


class PoCo(Conver):
    """
    Posting Conversation

    """

    __slots__ = (
        "po",
        "_co_seq",
        "_send_done_fut",
        "_begin_acked_fut",
        "_end_acked_fut",
    )

    def __init__(self, po, co_seq):
        self.po = po
        self._co_seq = co_seq

        self._send_done_fut = asyncio.get_running_loop().create_future()
        self._begin_acked_fut = None
        self._end_acked_fut = None

    async def __aenter__(self):
        await self.begin()
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.end()

    async def begin(self):
        if self._begin_acked_fut is not None:
            raise asyncio.InvalidStateError("co_begin sent already!")

        po = self.po
        coq = po._coq
        while coq:
            tail_co = coq[-1]
            if tail_co.is_ended():
                break
            await tail_co.wait_ended()

        self._begin_acked_fut = asyncio.get_running_loop().create_future()

        coq.append(self)

        try:
            await po._send_text(self.co_seq, b"co_begin")
        except Exception as exc:
            self._begin_acked_fut.set_exception(exc)

            err_msg = "Error sending co_begin: " + str(exc)
            err_stack = "".join(traceback.format_exc())
            err_reason = err_msg + "\n" + err_stack
            await po.disconnect(err_reason)
            raise

    async def end(self):
        try:
            if self._begin_acked_fut is None:
                raise asyncio.InvalidStateError("co_begin not sent yet!")
            if self._end_acked_fut is not None:
                raise asyncio.InvalidStateError("co_end sent already!")

            po = self.po
            assert self is po._coq[-1], "co not tail of po's coq ?!"

            self._end_acked_fut = asyncio.get_running_loop().create_future()

            if not po.is_connected():
                exc = RuntimeError("Co End After Disconnected")
                self._end_acked_fut.set_exception(exc)
                self._send_done_fut.set_exception(exc)
                raise exc

            try:
                await po._send_text(self.co_seq, b"co_end")

                self._send_done_fut.set_result(self.co_seq)
            except Exception as exc:
                self._end_acked_fut.set_exception(exc)

                if not self._send_done_fut.done():
                    self._send_done_fut.set_exception(exc)

                err_msg = "Error sending co_end: " + str(exc)
                err_stack = "".join(traceback.format_exc())
                err_reason = err_msg + "\n" + err_stack
                await po.disconnect(err_reason)
                raise
        finally:
            if not self._send_done_fut.done():
                self._send_done_fut.set_exception(RuntimeError("Abnormal Co End"))

    def is_ended(self):
        return self._send_done_fut.done()

    async def wait_ended(self):
        await self._send_done_fut

    def _begin_acked(self, co_seq):
        if self.co_seq != co_seq:
            raise asyncio.InvalidStateError("co_seq mismatch ?!")

        fut = self._begin_acked_fut
        if fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")
        ended = self._end_acked_fut
        if ended is not None and ended.done():
            raise asyncio.InvalidStateError("co_end acked already!")

        fut.set_result(co_seq)

    def _end_acked(self, co_seq):
        if self.co_seq != co_seq:
            raise asyncio.InvalidStateError("co_seq mismatch ?!")

        fut = self._end_acked_fut
        if fut is None:
            raise asyncio.InvalidStateError("co_end not sent yet!")

        fut.set_result(co_seq)

    async def response_begin(self):
        fut = self._begin_acked_fut
        if fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")
        await fut

    async def response_end(self):
        fut = self._end_acked_fut
        if fut is None:
            raise asyncio.InvalidStateError("co_end not sent yet!")
        await fut

    async def send_code(self, code):
        if self._begin_acked_fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")

        po = self.po
        assert self is po._coq[-1], "co not tail of po's coq ?!"

        await po._send_code(code)

    async def send_obj(self, code):
        if self._begin_acked_fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")

        po = self.po
        assert self is po._coq[-1], "co not tail of po's coq ?!"

        await po._send_code(code, b"co_recv")

    async def send_data(self, bufs):
        if self._begin_acked_fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")

        po = self.po
        assert self is po._coq[-1], "co not tail of po's coq ?!"

        await po._send_data(bufs)

    async def recv_obj(self):
        if self._begin_acked_fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")
        await self.response_begin()

        po = self.po
        assert self is po._coq[0], "co not head of po's coq ?!"

        return await po._wire.ho._recv_obj()

    async def recv_data(self, bufs):
        if self._begin_acked_fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")
        await self.response_begin()

        po = self.po
        assert self is po._coq[0], "co not head of po's coq ?!"

        await po._wire.ho._recv_data(bufs)

    async def get_obj(self, code):
        if self._begin_acked_fut is None:
            raise asyncio.InvalidStateError("co_begin not sent yet!")

        po = self.po
        assert self is po._coq[0], "co not head of po's coq ?!"
        await po._send_text(code, b"co_send")

        return await self.recv_obj()
