import asyncio
import inspect
import traceback
from collections import deque
from typing import *

from .._details import *
from ..aio import *
from ..he import *
from ..log import *
from .co import *
from .ho import *
from .po import *

__all__ = ["HBIC"]

logger = get_logger(__name__)


class HBIC:
    """
    HBIC is connection object regardless of the underlying transporting mechanism.

    """

    __slots__ = (
        "po",
        "ho",
        "_conn_fut",
        "_disc_fut",
        "disc_reason",
        "wire",
        "net_ident",
        "_send_ctrl",
        "_coq",
        "_next_co_seq",
        "packet_available",
        "_lath",
        "_hott",
    )

    def __init__(self, he: HostingEnv):
        self.po = PostingEnd(self)
        self.ho = HostingEnd(self, he)

        # the wire underlying for data transportation
        self.wire = None
        self.net_ident = "<unwired>"

        loop = asyncio.get_running_loop()
        self._conn_fut = loop.create_future()
        self._disc_fut = loop.create_future()
        self.disc_reason = None

        self._send_ctrl = SendCtrl()
        self._coq = deque()
        self._next_co_seq = MIN_CO_SEQ

        # the event indicating data pending on the wire to be read for landing
        self.packet_available = asyncio.Event()
        # the landing thread (it's an asyncio task but think a green thread of it)
        self._lath = None
        # the hosting task that is scheduled to/already be running in landing thread
        self._hott = None

    def co(self) -> PoCo:
        next_co_seq = self._next_co_seq
        co_seq = str(next_co_seq)
        next_co_seq += 1
        if next_co_seq > MAX_CO_SEQ:
            self._next_co_seq = MIN_CO_SEQ
        co = PoCo(self, co_seq)
        return co

    def is_connected(self):
        if self._disc_fut.done():
            return False
        wire = self.wire
        if wire is None:
            return False
        return wire.is_connected()

    async def wait_connected(self):
        await self._conn_fut
        wire = self.wire
        assert wire.is_connected()

    async def wait_disconnected(self):
        await self._disc_fut
        wire = self.wire
        assert wire is None or not wire.is_connected()

    def pause_sending(self):
        self._send_ctrl.obstruct()

    def resume_sending(self):
        self._send_ctrl.unleash()

    async def _send_packet(self, payload, wire_dir=b""):
        if isinstance(payload, str):
            # textual code
            payload = payload.encode("utf-8")
        elif isinstance(payload, (bytes, bytearray)):
            # raw bytes of textual code
            pass
        else:
            # send its repr as textual code
            payload = repr(payload).encode("utf-8")

        wire = self.wire
        try:
            await self._send_ctrl.flowing()
            wire.send_packet(payload, wire_dir)
        except Exception:
            err_reason = traceback.print_exc()
            await self.disconnect(err_reason, False)
            raise

    async def _send_buffer(self, buf):
        wire = self.wire
        try:
            await self._send_ctrl.flowing()
            wire.send_data(buf)
        except Exception:
            err_reason = traceback.print_exc()
            await self.disconnect(err_reason, False)
            raise

    async def _send_data(
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
        assert bufs is not None

        # use a generator function to pull all buffers from hierarchy

        def pull_from(boc):
            b = cast_to_src_buffer(boc)
            if b is not None:
                yield b
                return
            for boc1 in boc:
                yield from pull_from(boc1)

        for buf in pull_from(bufs):
            await self._send_buffer(buf)

    async def disconnect(
        self, err_reason: Optional[str] = None, try_send_peer_err: bool = True
    ):
        if err_reason is not None:
            if self.disc_reason is None:
                self.disc_reason = err_reason
                logger.error(
                    rf"""
HBIC {self.net_ident} disconnecting due to error:
{err_reason}
""",
                    stack_info=True,
                )
            elif self.disc_reason != err_reason:
                logger.error(
                    f"HBIC {self.net_ident!s} sees another reason to disconnect: {err_reason}",
                    stack_info=True,
                )

        wire = self.wire
        if wire is None:
            raise asyncio.InvalidStateError("HBIC not wired yet!")

        disc_fut = self._disc_fut
        if disc_fut.done():  # already disconnected
            assert not wire.is_connected(), "disc_fut done but wire still connected ?!"
            return

        lath = self._lath
        if lath is not None and not lath.done():
            lath.cancel()

        try:
            if err_reason is not None and try_send_peer_err:
                if wire.is_connected():
                    try:
                        await wire.send_packet(str(err_reason).encode("utf-8"), b"err")
                    except Exception:
                        logger.warning(
                            "HBIC {self.net_ident} failed sending peer error",
                            exc_info=True,
                        )
                else:
                    logger.warning(
                        f"Not sending peer error as unwired:\n{err_reason!s}",
                        stack_info=True,
                    )

            if wire.is_connected():
                wire.disconnect()

            disc_fut.set_result(err_reason)
        except Exception as exc:
            logger.warning(
                "HBIC {self.net_ident} failed closing posting endpoint.", exc_info=True
            )
            assert not disc_fut.done(), "?!"
            disc_fut.set_exception(exc)

        assert disc_fut.done()
        await disc_fut  # propagate the exception if one occurred

    def is_landing(self):
        lath = self._lath
        return lath is not None and not lath.done()

    def wire_connected(self, wire):
        self.wire = wire
        self.net_ident = wire.net_ident()
        self.po.remote_addr = wire.remote_addr()
        self.ho.local_addr = wire.local_addr()

        self._send_ctrl.startup()

        assert self._lath is None, "landing thread created already ?!"
        self.packet_available.set()

        async def run_lath():
            try:
                await self._landing_thread()
            except Exception as exc:
                logger.error(
                    f"HBIC {self.net_ident!s} unexpected error in landing thread.",
                    exc_info=True,
                )
            if wire.is_connected():
                wire.disconnect()

        lath = self._lath = asyncio.create_task(run_lath())

        self._conn_fut.set_result(None)

    def wire_disconnected(self, wire, exc=None):
        assert wire is self.wire, "wire mismatch ?!"

        if exc is not None:
            logger.error(f"HBIC {self.net_ident} wire error: {exc}")

            disc_reason = f"wire error: {exc}"
            if self.disc_reason is None:
                self.disc_reason = disc_reason

        else:
            exc = asyncio.InvalidStateError(
                f"HBIC {self.net_ident} wire forcefully disconnected."
            )

            disc_reason = self.disc_reason
            if disc_reason is None:
                disc_reason = "wire forcefully disconnected"

        conn_fut = self._conn_fut
        if not conn_fut.done():
            conn_fut.set_exception(exc)

        self._send_ctrl.shutdown(exc)

        lath = self._lath
        if lath is not None and not lath.done():
            lath.cancel()

        disc_fut = self._disc_fut
        if not disc_fut.done():
            disc_fut.set_result(disc_reason)

    async def _landing_thread(self):
        """
        Use a green-thread to guarantee serial landing of packets pushed over the wire.

        Though the code from a packet can spawn new coroutines for concurrent execution
        by calling `asyncio.create_task()` during landing.

        """

        wire = self.wire

        po = self.po
        coq = self._coq
        ho = self.ho
        he = ho.env

        disc_reason = None
        try_send_peer_err = True

        # call init magic
        init_magic = he.get("__hbi_init__")
        if init_magic is not None:
            try:
                maybe_coro = init_magic(po=po, ho=ho)
                if inspect.iscoroutine(maybe_coro):
                    await maybe_coro
            except Exception as exc:
                disc_reason = f"init failed: {exc!s}"
                logger.error(f"HBIC {self.net_ident!s} init failed.", exc_info=True)

        pkt_available = self.packet_available
        pkt = None
        while disc_reason is None:
            hott = self._hott
            if hott is not None:  # clear last hosting task
                self._hott = None

            try:
                await pkt_available.wait()
            except asyncio.CancelledError as exc:

                # capture the `err_reason` passed to `disconnect()`,
                # can be None for normal disconnection.
                disc_reason = self.disc_reason

                break

            pkt = wire.recv_packet()
            if pkt is None:  # no (or only partial) packet received

                if not wire.is_connected():
                    break  # wire disconnected by peer, break landing loop

                # clear the event to wait until new data arrived
                pkt_available.clear()
                continue

            # got a packet, land it
            payload, wire_dir = pkt

            if "co_begin" == wire_dir:

                assert ho._co is None, "unclean co_begin ?!"
                co_seq = payload
                while coq:
                    tail_co = coq[-1]
                    if tail_co.is_closed():
                        break
                    await tail_co.wait_closed()

                co = HoCo(self, co_seq)
                coq.append(co)
                ho._co = co
                await self._send_packet(co_seq, b"co_ack_begin")

            elif "" == wire_dir:

                async def land_code():
                    landed = he.run_in_env(payload)
                    if inspect.iscoroutine(landed):
                        landed = await landed

                self._hott = land_code()

            elif "co_send" == wire_dir:

                # peer is requesting this end to push landed result (in textual code) back

                async def sendback_to_poco():
                    landed = he.run_in_env(payload)
                    if inspect.iscoroutine(landed):
                        landed = await landed

                    await self._send_packet(landed, b"co_recv")

                self._hott = sendback_to_poco()

            elif "co_recv" == wire_dir:

                if ho._co is not None:  # pushing obj to a ho co
                    disc_reason = "co_recv without priori receiving code in landing"
                    break

                if not coq:  # nor a po co to recv the pushed obj
                    disc_reason = "no conversation to receive object"
                    break

                # pushing obj to a po co
                co = coq[0]

                async def recv_to_poco():
                    landed = he.run_in_env(payload)
                    if inspect.iscoroutine(landed):
                        landed = await landed

                    await co._roq.put(landed)

                self._hott = recv_to_poco()

            elif "co_end" == wire_dir:

                co_seq = payload
                co = ho._co
                assert co is not None, "ho co mismatch ?!"
                if co.co_seq != co_seq:
                    raise asyncio.InvalidStateError("mismatch co_end")

                tail_co = coq.pop()
                assert co is tail_co, "ho co not tail of coq ?!"

                co._send_done_fut.set_result(co_seq)
                await self._send_packet(co_seq, b"co_ack_end")
                ho._co = None

            elif "co_ack_begin" == wire_dir:

                co_seq = payload
                co = coq[0]
                if co.co_seq != co_seq:
                    raise asyncio.InvalidStateError(f"mismatch co_seq")

                co._begin_acked(co_seq)

            elif "po_data" == wire_dir:

                if ho._co is not None:  # pushing data to a ho co
                    disc_reason = "po_data to a ho co ?!"
                    break

                if not coq:  # nor a po co to recv the pushed data
                    disc_reason = "no po co to receive data"
                    break

                # pushing data to a po co
                co_seq = payload
                co = coq[0]
                if co.co_seq != co_seq:
                    raise asyncio.InvalidStateError(f"mismatch co_seq")

                async def pump_po_co_data():
                    bufs, fut = await co._rdq.get()
                    try:
                        await self._recv_data(bufs)
                        fut.set_result(None)
                    except Exception as exc:
                        if not fut.done():
                            fut.set_exception(exc)
                        raise

                self._hott = pump_po_co_data()

            elif "co_ack_end" == wire_dir:

                co_seq = payload
                co = coq.popleft()
                if co.co_seq != co_seq:
                    raise asyncio.InvalidStateError(f"mismatch co_seq!")
                co._end_acked(co_seq)

            elif "err" == wire_dir:
                # peer error

                try_send_peer_err = False
                disc_reason = f"peer error: {payload!s}"

            else:

                disc_reason = f"HBIC unexpected packet: [#{wire_dir}]{payload!s}"

            if disc_reason is not None:
                break  # error occurred, break landing loop

            hott = self._hott
            if hott is None:  # no coroutine to run from last packet landing
                continue  # proceed to land next packet

            try:

                await hott  # run the coroutine by awaiting it

            except asyncio.CancelledError:

                disc_reason = self.disc_reason
                if disc_reason is None:
                    # cancelled due to other reasons than disconnection

                    disc_reason = f"landing task cancelled:\n{traceback.print_exc()!s}"

                    logger.error(
                        f"HBIC {self.net_ident!s} unexpected cancellation in landing task: {hott!r}",
                        exc_info=True,
                    )

                break

            except Exception:

                disc_reason = f"landing task failed:\n{traceback.print_exc()!s}"

                logger.error(
                    f"HBIC {self.net_ident!s} unexpected error in landing task: {hott!r}",
                    exc_info=True,
                )

                break

        # the landing thread is terminating, disconnect if not already
        if not self._disc_fut.done():
            # but don't await the `disconnect()` here, it cancels this thread as part of the
            # disconnecting procedure.
            asyncio.create_task(self.disconnect(disc_reason, try_send_peer_err))

        # call cleanup magic
        cleanup_magic = he.get("__hbi_cleanup__")
        if cleanup_magic is not None:
            try:
                maybe_coro = cleanup_magic(po=po, ho=ho, err_reason=disc_reason)
                if inspect.iscoroutine(maybe_coro):
                    await maybe_coro
            except Exception:
                logger.error(f"HBIC {self.net_ident!s} cleanup failed.", exc_info=True)

    async def _recv_one_obj(self) -> object:
        receiving_hott = self._hott
        if receiving_hott is None:
            raise asyncio.InvalidStateError(
                "_recv_one_obj() not called from the receiving code ?!"
            )

        wire = self.wire
        wire.resume_recv()

        # use up stack hott as a marker to indicate the desired obj not received yet,
        # this'll be set to the received obj once final result landed from a co_recv pkt.
        # can not use None here coz the obj to recv can verbatimly be None, any object later
        # comparable is okay.
        obj2hoco = receiving_hott

        he = ho.env

        disc_reason = None
        try_send_peer_err = True

        pkt_available = self.packet_available
        pkt = None
        while disc_reason is None:
            hott = self._hott
            if hott is not receiving_hott:  # restore up stack hosting task
                assert hott.done(), "last hosting task still pending ?!"
                self._hott = receiving_hott

            try:
                await pkt_available.wait()
            except asyncio.CancelledError:
                # this should capture the `err_reason` passed to an active `disconnect()` call
                disc_reason = exc.args[0]
                break

            pkt = wire.recv_packet()
            if pkt is None:  # no (or only partial) packet received

                if not wire.is_connected():
                    raise asyncio.InvalidStateError("wire disconnected")

                # clear the event to wait until new data arrived
                pkt_available.clear()
                continue

            # got a packet, land it
            payload, wire_dir = pkt

            if "" == wire_dir:

                # some code to execute preceding code for obj to be received.
                # todo this harmful and be explicitly disallowed ?

                async def land_code():
                    landed = he.run_in_env(payload)
                    if inspect.iscoroutine(landed):
                        landed = await landed

                self._hott = land_code()

            elif "co_recv" == wire_dir:

                # the very expected packet

                async def recv_for_hoco():
                    nonlocal obj2hoco

                    landed = he.run_in_env(payload)
                    if inspect.iscoroutine(landed):
                        landed = await landed

                    obj2hoco = landed

                self._hott = recv_for_hoco()

            elif "err" == wire_dir:
                # peer error

                try_send_peer_err = False
                disc_reason = f"peer error: {payload!s}"

            else:

                disc_reason = f"HBIC unexpected packet: [#{wire_dir}]{payload!s}"

            if disc_reason is not None:
                break  # error occurred, break landing loop

            hott = self._hott
            if hott is receiving_hott:  # no coroutine to run from last packet landing
                continue  # proceed to land next packet

            assert hott is not None, "?!"

            try:

                await hott  # run the coroutine by awaiting it

            except Exception as exc:

                raise

            if obj2hoco is not receiving_hott:
                return obj2hoco

        # this should get the disconnected exception as is called from landing thread
        await self.disconnect(disc_reason, try_send_peer_err)

    def stop_landing(self) -> bool:  # return whether to keep wire for sending
        ho = self.ho
        if ho._co is not None:
            # trigger disconnection
            asyncio.create_task(
                self.disconnect(
                    "Premature EOF before hosting conversation ended.", True
                )
            )
            return True

        # disconnect wire after all po co finished sending
        async def disc_after_po_done():
            coq = self._coq
            while coq:
                tail_co = coq[-1]
                if tail_co.is_closed():
                    break
                await tail_co.wait_closed()
            await self.disconnect(None, False)

        asyncio.create_task(disc_after_po_done())
        return True

    async def _recv_data(self, bufs):
        wire = self.wire
        wire.resume_recv()

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
                    fut.set_exception(asyncio.InvalidStateError("hbic disconnected"))
                wire.end_offload(None, data_sink)

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
                        wire.end_offload(chunk, data_sink)
                        # resolve the future
                        if not fut.done():
                            fut.set_result(None)
                        # and done
                        return
                    except Exception as exc:
                        raise RuntimeError(
                            "HBIC buffer source raised exception"
                        ) from exc
            except Exception as exc:
                asyncio.create_task(self.disconnect(traceback.print_exc(), True))
                if not fut.done():
                    fut.set_exception(exc)

        wire.begin_offload(data_sink)

        await fut
