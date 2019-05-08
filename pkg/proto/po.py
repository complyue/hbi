import asyncio
import inspect
from collections import deque
from typing import *

from .._details import *
from .._details import SendCtrl
from ..log import *
from .co import PoCo

__all__ = ["PostingEnd"]

logger = get_logger(__name__)


class PostingEnd:
    """
    HBI posting endpoint

    """

    __slots__ = (
        "_wire",
        "net_ident",
        "remote_addr",
        "_conn_fut",
        "_disc_fut",
        "_send_ctrl",
        "_coq",
    )

    def __init__(self):
        self._wire = None
        self.net_ident = "<unwired>"
        self.remote_addr = "<unwired>"

        self._conn_fut = asyncio.get_running_loop().create_future()
        self._disc_fut = None

        self._send_ctrl = SendCtrl()
        self._coq = deque()

    @property
    def _connected_wire(self):
        wire = self._wire
        if wire is None:
            raise asyncio.InvalidStateError("HBI posting endpoint not wired!")
        if not wire.connected:
            raise asyncio.InvalidStateError("HBI posting endpoint not connected!")
        return wire

    def is_connected(self):
        wire = self._wire
        if wire is None:
            return False
        if self._disc_fut is not None:
            return False
        return wire.connected

    async def wait_connected(self):
        await self._conn_fut

    async def notif(self, code):
        async with self.co():
            await self._send_code(code)

    async def notif_data(self, code, bufs):
        async with self.co():
            await self._send_code(code)
            await self._send_data(bufs)

    def co(self):
        co = PoCo(self)
        return co

    async def _send_code(self, code, wire_dir=b""):
        # use a generator function to pull code from hierarchy
        def pull_code(container):
            for mc in container:
                if inspect.isgenerator(mc):
                    yield from pull_code(mc)
                else:
                    yield mc

        if inspect.isgenerator(code):
            for c in pull_code(code):
                await self._send_text(c, wire_dir)
        else:
            await self._send_text(code, wire_dir)

    async def _send_data(self, bufs):
        assert bufs is not None

        # use a generator function to pull all buffers from hierarchy

        def pull_from(boc):
            b = cast_to_src_buffer(
                boc
            )  # this static method can be overridden by subclass
            if b is not None:
                yield b
                return
            for boc1 in boc:
                yield from pull_from(boc1)

        for buf in pull_from(bufs):
            # take receiving high watermark as reference for max chunk size
            max_chunk_size = self._wire.wire_buf_high
            remain_size = len(buf)
            send_from_idx = 0
            while remain_size > max_chunk_size:
                await self._send_buffer(
                    buf[send_from_idx : send_from_idx + max_chunk_size]
                )
                send_from_idx += max_chunk_size
                remain_size -= max_chunk_size
            if remain_size > 0:
                await self._send_buffer(buf[send_from_idx:])

    async def _send_text(self, code, wire_dir=b""):
        if isinstance(code, bytes):
            payload = code
        elif isinstance(code, str):
            payload = code.encode("utf-8")
        else:
            # try convert to json and send
            payload = json.dumps(code).encode("utf-8")

        # check connected & wait for flowing for each code packet
        wire = self._connected_wire
        await self._send_ctrl.flowing()
        wire.transport.writelines([b"[%d#%s]" % (len(payload), wire_dir), payload])

    async def _send_buffer(self, buf):
        # check connected & wait for flowing for each single buffer
        wire = self._connected_wire
        await self._send_ctrl.flowing()
        wire.transport.write(buf)

    async def disconnect(self, err_reason=None, try_send_peer_err=True):
        wire = self._wire
        if wire is None:
            raise asyncio.InvalidStateError(
                f"HBI {self.net_ident} posting endpoint not wired yet!"
            )

        disc_fut = self._disc_fut
        if disc_fut is not None:
            if err_reason is not None:
                logger.error(
                    rf"""
HBI {self.net_ident} repeated disconnection of due to error:
{err_reason}
""",
                    stack_info=True,
                )
            return await disc_fut

        if err_reason is not None:
            logger.error(
                rf"""
HBI {self.net_ident} disconnecting due to error:
{err_reason}
""",
                stack_info=True,
            )

        disc_fut = self._disc_fut = asyncio.get_running_loop().create_future()

        try:
            if wire.transport.is_closing():
                logger.warning(
                    f"HBI {self.net_ident} transport already closing.", stack_info=True
                )

                if err_reason is not None and try_send_peer_err:
                    logger.warning(
                        f"Not sending peer error as transport is closing.\n >>> {err_reason!s}",
                        exc_info=True,
                    )
            else:
                if err_reason is not None and try_send_peer_err:
                    try:
                        await self._send_text(str(err_reason), b"err")
                    except Exception:
                        logger.warning(
                            "HBI {self.net_ident} failed sending peer error",
                            exc_info=True,
                        )

                # close outgoing channel
                # This method can raise NotImplementedError if the transport (e.g. SSL) doesn’t support half-closed connections.
                # TODO handle SSL cases once supported
                wire.transport.write_eof()

            disc_fut.set_result(None)
        except Exception as exc:
            logger.warning(
                "HBI {self.net_ident} failed closing posting endpoint.", exc_info=True
            )
            disc_fut.set_exception(exc)

        assert disc_fut.done()
        await disc_fut

    async def disconnected(self):
        disc_fut = self._disc_fut
        if disc_fut is None:
            raise asyncio.InvalidStateError("Not disconnecting")
        await disc_fut

    # should be called by wire protocol
    def _connected(self):
        wire = self._wire
        self.net_ident = wire.net_ident
        self.remote_addr = wire.remote_addr

        self._send_ctrl.startup()

        conn_fut = self._conn_fut
        assert conn_fut is not None, "?!"
        if conn_fut.done():
            assert fut.exception() is None and fut.result() is None, "?!"
        else:
            conn_fut.set_result(None)

    # should be called by wire protocol
    def _disconnected(self, exc=None):
        if exc is not None:
            logger.warning(
                f"HBI {self.net_ident} connection unwired due to error: {exc}"
            )

        conn_fut = self._conn_fut
        if conn_fut is not None and not conn_fut.done():
            conn_fut.set_exception(
                exc
                or asyncio.InvalidStateError(
                    f"HBI {self.net_ident} disconnected due to {exc!s}"
                )
            )

        self._send_ctrl.shutdown(exc)

        disc_fut = self._disc_fut
        if disc_fut is None:
            disc_fut = self._disc_fut = asyncio.get_running_loop().create_future()

        if not disc_fut.done():
            disc_fut.set_result(exc)
        elif disc_fut.result() is not exc:
            logger.warning(
                f"HBI {self.net_ident} disconnection exception not reported to application: {exc!s}"
            )