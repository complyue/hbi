import asyncio
import inspect
from typing import *

from .._details import *
from ..aio import *
from ..he import *
from ..log import *
from .co import HoCo

__all__ = ["HostingEnd"]

logger = get_logger(__name__)


class HostingEnd:
    """
    HBI hosting endpoint

    """

    __slots__ = (
        "po",
        "_wire",
        "net_ident",
        "local_addr",
        "ctx",
        "open_ctx",
        "_conn_fut",
        "_disc_fut",
        "_hoth",
        "_landing",
        "_hott",
        "_horq",
        "co",
    )

    def __init__(self, po, open_ctx=True, app_queue_size: int = 100):
        self.po = po
        self.open_ctx = open_ctx

        self.ctx = None

        self._wire = None
        self.net_ident = "<unwired>"
        self.local_addr = "<unwired>"

        self._conn_fut = asyncio.get_running_loop().create_future()
        self._disc_fut = None

        # hosting green-thread
        self._hoth = None
        # the event indicating landing should keep proceding
        self._landing = asyncio.Event()
        # the hosting task that is scheduled to run
        self._hott = None

        # queue for received objects
        self._horq = CancellableQueue(app_queue_size)

        # current hosting conversation
        self.co = None

    def is_connected(self):
        wire = self._wire
        if wire is None:
            return False
        if self._disc_fut is not None:
            return False
        return wire.connected

    async def wait_connected(self):
        await self._conn_fut

    async def _recv_obj(self):
        landing = self._landing
        wire = self._wire
        horq = self._horq
        while True:

            try:
                # try return from receiving queue
                return horq.get_nowait()
            except asyncio.QueueEmpty:
                # no object in queue, proceed to land one packet
                pass
            while not wire._land_one():
                # no complete packet on wire atm, wait more data to arrive
                await landing

    async def disconnect(self, err_reason=None, try_send_peer_err=True):
        _wire = self._wire
        if _wire is None:
            raise asyncio.InvalidStateError(
                f"HBI {self.net_ident} hosting endpoint not wired yet!"
            )

        disc_fut = self._disc_fut
        if disc_fut is not None:
            if err_reason is not None:
                logger.error(
                    rf"""
HBI {self.net_ident} repeated disconnection due to error:
{err_reason}
""",
                    stack_info=True,
                )
            await disc_fut
            return

        if err_reason is not None:
            logger.error(
                rf"""
HBI {self.net_ident} disconnecting due to error:
{err_reason}
""",
                stack_info=True,
            )

        disc_fut = self._disc_fut = asyncio.get_running_loop().create_future()

        if self.po is not None:
            # close posting endpoint (i.e. write eof) before closing the socket
            await self.po.disconnect(err_reason, try_send_peer_err)

        elif err_reason is not None and try_send_peer_err:
            logger.warning(
                f"HBI {self.net_ident} not sending peer error {err_reason!s} as no posting endpoint.",
                exc_info=True,
            )

        try:
            _wire.transport.close()
        except OSError:
            # may already be invalid
            pass
        # connection_lost will be called by asyncio loop after out-going packets flushed

        await disc_fut

    async def disconnected(self):
        disc_fut = self._disc_fut
        if disc_fut is None:
            raise asyncio.InvalidStateError(f"HBI {self.net_ident} not disconnecting")
        await disc_fut

    # should be called by wire protocol
    def _connected(self):
        wire = self._wire
        self.net_ident = wire.net_ident
        self.local_addr = wire.local_addr

        conn_fut = self._conn_fut
        assert conn_fut is not None, "?!"
        if conn_fut.done():
            assert fut.exception() is None and fut.result() is None, "?!"
        else:
            conn_fut.set_result(None)

        assert self._hoth is None, "hosting green-thread created already ?!"
        self._landing.set()
        self._hoth = asyncio.create_task(self._ho_thread())

    async def _ho_thread(self):
        """
        Use a green-thread to guarantee serial execution of packets transfered over the wire.

        Though the code from a packet can spawn new green-threads by calling `asyncio.create_task()`
        during landing.

        """

        landing = self._landing
        wire = self._wire
        while True:
            try:
                await landing.wait()
            except asyncio.CancelledError:
                if self._disc_fut is None:
                    logger.warning(
                        "hosting thread cancelled while not disconnecting ?!"
                    )
                break

            if not wire._land_one():
                assert (
                    not landing.is_set()
                ), "`landing` not cleared upon no packet landed ?!"
                # no full packet from wire, wait for more data to arrive
                continue

            coro = self._hott
            if coro is None:  # no coroutine to run from last packet landing
                # proceed to land next packet
                continue
            # the coroutine is taken off to local var `coro`, clear the member field
            self._hott = None

            try:

                await coro  # run the coroutine by awaiting it

            except asyncio.CancelledError:
                if self._disc_fut is not None:
                    # disconnecting, stop hosting thread
                    break

                logger.warning(
                    f"HBI {self.net_ident!s} a hosted task cancelled: {coro}",
                    exc_info=True,
                )
            except Exception as exc:
                logger.error(
                    f"HBI {self.net_ident!s} a hosted task failed: {coro}",
                    exc_info=True,
                )
                # discard all following data on the wire
                await self.disconnect(exc)
                return

    async def _ack_co_begin(self, co_seq: str):
        if self.co is not None:
            raise asyncio.InvalidStateError("Unclean co_begin!")

        po = self.po
        coq = po._coq
        while coq:
            tail_co = coq[-1]
            if tail_co.is_ended():
                break
            await tail_co.wait_ended()

        co = HoCo(self, co_seq)
        coq.append(co)
        self.co = co
        await po._send_text(co_seq, b"co_ack_begin")

    async def _ack_co_end(self, co_seq: str):
        co = self.co
        if co.co_seq != co_seq:
            raise asyncio.InvalidStateError("Mismatch co_end!")

        po = self.po
        coq = po._coq
        tail_co = coq.pop()
        assert co is tail_co, "ho co not tail of po's coq ?!"

        co._send_done_fut.set_result(co_seq)
        await po._send_text(co_seq, b"co_ack_end")
        self.co = None

    async def _co_begin_acked(self, co_seq: str):
        co = self.po._coq[0]
        if co.co_seq != co_seq:
            raise asyncio.InvalidStateError(f"Mismatch co_seq!")
        co._begin_acked(co_seq)

    async def _co_end_acked(self, co_seq: str):
        co = self.po._coq.popleft()
        if co.co_seq != co_seq:
            raise asyncio.InvalidStateError(f"Mismatch co_seq!")
        co._end_acked(co_seq)

    async def _co_send_back(self, obj):
        if inspect.isawaitable(obj):
            obj = await obj
        await self._send_text(repr(obj), b"co_recv")

    async def _co_recv_landed(self, obj):
        if self.co is None:
            raise asyncio.InvalidStateError("No conversation for recv!")

        if self._horq.full():
            self._wire._check_pause()

        if inspect.isawaitable(obj):
            obj = await obj

        await self._horq.put(obj)

    def _land_packet(self, code, wire_dir) -> Optional[tuple]:
        assert self._hott is None, "landing new packet while next ho task scheduled ?!"
        assert self._landing.is_set(), "_landing not set on landing new packet ?!"

        if "" == wire_dir:

            landed = self._land_code(code)
            if inspect.iscoroutine(landed):
                self._hott = landed
            elif inspect.isawaitable(landed):
                logger.warning(
                    f"Possible bug: landed object type=[{type(landed)!s}], being awaitable but not a coroutine: {landed!r}"
                )

        elif "co_begin" == wire_dir:

            self._hott = self._ack_co_begin(code)

        elif "co_recv" == wire_dir:
            # peer is sending a result object to be received by this end

            landed = self._land_code(code)
            self._hott = self._co_recv_landed(landed)

        elif "co_send" == wire_dir:
            # peer is requesting this end to send landed result back

            landed = self._land_code(code)
            self._hott = self._co_send_back(landed)

        elif "co_end" == wire_dir:

            self._hott = self._ack_co_end(code)

        elif "co_ack_begin" == wire_dir:

            self._hott = self._co_begin_acked(code)

        elif "co_ack_end" == wire_dir:

            self._hott = self._co_end_acked(code)

        elif "err" == wire_dir:
            # peer error

            self._handle_peer_error(code)

        else:

            self._handle_landing_error(
                RuntimeError(f"HBI unknown wire directive [{wire_dir}]")
            )

    def _land_code(self, code):
        # allow customization of code landing
        lander = self.ctx.get("__hbi_land__", None)
        if lander is not None:
            assert callable(lander), "non-callable __hbi_land__ defined ?!"
            try:

                landed = lander(code)
                if NotImplemented is not landed:
                    # custom lander can return `NotImplemented` to proceed standard landing
                    return landed

            except Exception as exc:
                logger.debug(
                    rf"""
HBI {self.net_info}, error custom landing code:
--CODE--
{code!s}
--====--
""",
                    exc_info=True,
                )
                self._handle_landing_error(exc)
                raise

        # standard landing
        defs = self.ctx if self.open_ctx else {}
        try:

            landed = run_in_env(code, self.ctx, defs)
            return landed

        except Exception as exc:
            logger.error(
                rf"""
HBI {self.net_ident}, error landing code:
--CODE--
{code!s}
--====--
""",
                exc_info=True,
            )
            self._handle_landing_error(exc)
            raise

    async def _recv_data(self, bufs):
        wire = self._wire
        wire._check_resume()

        fut = asyncio.get_running_loop().create_future()

        # use a generator function to pull all buffers from hierarchy
        def pull_from(boc):
            b = cast_to_tgt_buffer(
                boc
            )  # this static method can be overridden by subclass
            if b:
                yield b
            else:
                for boc1 in boc:
                    yield from pull_from(boc1)

        pos = 0
        buf = None

        buf_puller = pull_from(bufs)

        def data_sink(chunk):
            nonlocal pos
            nonlocal buf

            if chunk is None:
                if not fut.done():
                    fut.set_exception(RuntimeError("HBI disconnected"))
                wire._end_offload(None, data_sink)

            try:
                while True:
                    if buf is not None:
                        assert pos < len(buf)
                        # current buffer not filled yet
                        if not chunk or len(chunk) <= 0:
                            # data expected by buf, and none passed in to this call,
                            # return and expect next call into here
                            wire.transport.resume_reading()
                            return
                        available = len(chunk)
                        needs = len(buf) - pos
                        if available < needs:
                            # add to current buf
                            new_pos = pos + available
                            buf[pos:new_pos] = chunk.data()
                            pos = new_pos
                            # all data in this chunk has been exhausted while current buffer not filled yet
                            # return now and expect succeeding data chunks to come later
                            wire.transport.resume_reading()
                            return
                        # got enough or more data in this chunk to filling current buf
                        buf[pos:] = chunk.data(0, needs)
                        # slice chunk to remaining data
                        chunk.consume(needs)
                        # clear current buffer pointer
                        buf = None
                        pos = 0
                        # continue to process rest data in chunk, even chunk is empty now, still need to proceed for
                        # finish condition check

                    # pull next buf to fill
                    try:
                        buf = next(buf_puller)
                        pos = 0
                        if len(buf) == 0:  # special case for some empty data
                            buf = None
                    except StopIteration as ret:
                        # all buffers in hierarchy filled, finish receiving
                        wire._end_offload(chunk, data_sink)
                        # resolve the future
                        if not fut.done():
                            fut.set_result(bufs)
                        # and done
                        return
                    except Exception as exc:
                        raise RuntimeError(
                            "HBI buffer source raised exception"
                        ) from exc
            except Exception as exc:
                self._handle_wire_error(exc)
                if not fut.done():
                    fut.set_exception(exc)

        wire._begin_offload(data_sink)

        return await fut

    # should be called by wire protocol
    def _peer_eof(self):
        if self.co is not None:
            exc = asyncio.InvalidStateError(
                "Premature peer EOF before hosting conversation ended."
            )
            # trigger disconnection
            asyncio.create_task(self.disconnect(exc))
            return False

        # returning True here to prevent the socket from being closed automatically
        peer_done_cb = self.ctx.get("hbi_peer_done", None)
        if peer_done_cb is not None:
            maybe_coro = peer_done_cb()
            if inspect.iscoroutine(maybe_coro):
                # the callback is a coroutine, assuming the socket should not be closed on peer eof
                asyncio.get_running_loop().create_task(maybe_coro)
                return True
            else:
                # the callback is not coroutine, its return value should reflect its intent
                return maybe_coro
        return False  # let the socket be closed automatically

    def _handle_landing_error(self, exc):
        asyncio.create_task(self.disconnect(exc))

    def _handle_peer_error(self, err_reaon):
        asyncio.create_task(self.disconnect(f"HBI peer error: {err_reaon}", False))

    # should be called by wire protocol
    def _disconnected(self, exc=None):
        if exc is not None:
            logger.warning(
                f"HBI {self.net_ident} connection unwired due to error: {exc}"
            )

        disc_fut = self._disc_fut
        if disc_fut is None:
            disc_fut = self._disc_fut = asyncio.get_running_loop().create_future()
        if not disc_fut.done():
            disc_fut.set_result(exc)

        self._horq.cancel_all(exc)

        hoth = self._hoth
        if hoth is not None:
            hoth.cancel()

        cleanup_cb = self.ctx.get("__hbi_cleanup__", None)
        if cleanup_cb is not None:

            async def call_cleanup():
                try:
                    maybe_coro = cleanup_cb(exc)
                    if inspect.iscoroutine(maybe_coro):
                        await maybe_coro
                except Exception:
                    logger.warning(
                        f"HBI {self.net_ident} cleanup callback failure ignored.",
                        exc_info=True,
                    )

            asyncio.get_running_loop().create_task(call_cleanup())
