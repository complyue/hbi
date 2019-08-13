import asyncio
import inspect
import traceback
from typing import *

from .._details import *
from ..aio import *
from ..log import *
from .he import *
from .ho import *
from .po import *

__all__ = ["HBIC"]

logger = get_logger(__name__)


class HBIC:
    """
    HBIC is designed to interface with HBI wire protocol implementations,
    HBI applications should not use HBIC directly.

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
        "_ppc",
        "_next_co_seq",
        "_sender",
        "_recver",
        "packet_available",
        "_cok_task",
    )

    def __init__(self, he: "HostingEnv"):
        self.po = PostingEnd(self)
        self.ho = HostingEnd(self, he)
        he._po, he._ho = self.po, self.ho

        loop = asyncio.get_running_loop()
        self._conn_fut = loop.create_future()
        self._disc_fut = loop.create_future()
        self.disc_reason = None

        self.wire = None
        self.net_ident = "<unwired>"

        # out-going flow ctrl with high/low watermarks
        self._send_ctrl = SendCtrl()

        # pending posting conversations
        self._ppc = {}

        self._next_co_seq = MIN_CO_SEQ

        # current sending coversation
        self._sender = None

        # current receiving conversation
        self._recver = None

        # the event indicating data pending on the wire to be read for landing
        self.packet_available = asyncio.Event()

        # the co keeper aio task
        self._cok_task = None

    def is_connected(self) -> bool:
        if self._disc_fut.done():
            return False
        wire = self.wire
        if wire is None:
            return False
        return wire.is_connected()

    async def wait_connected(self):
        await self._conn_fut

    async def wait_disconnected(self):
        await self._disc_fut
        wire = self.wire
        assert wire is None or not wire.is_connected()

    async def disconnect(
        self, disc_reason: Optional[str] = None, try_send_peer_err: bool = True
    ):
        if disc_reason is not None:
            if self.disc_reason is None:
                self.disc_reason = disc_reason
                logger.error(
                    rf"""
HBIC {self.net_ident} disconnecting due to error:
{disc_reason}
""",
                    stack_info=True,
                )
            elif self.disc_reason != disc_reason:
                logger.error(
                    f"HBIC {self.net_ident!s} sees another reason to disconnect: {disc_reason}",
                    stack_info=True,
                )

        wire = self.wire
        if wire is None:
            raise asyncio.InvalidStateError("HBIC not wired yet!")

        disc_fut = self._disc_fut
        if disc_fut.done():  # already disconnected
            assert not wire.is_connected(), "disc_fut done but wire still connected ?!"
            return await disc_fut  # propagate the exception if one occurred

        cokt = self._cok_task
        if cokt is not None and not cokt.done():
            cokt.cancel()

        try:
            if disc_reason is not None and try_send_peer_err:
                if wire.is_connected():
                    try:
                        wire.send_packet(str(disc_reason).encode("utf-8"), b"err")
                    except Exception:
                        logger.warning(
                            f"HBIC {self.net_ident} failed sending peer error",
                            exc_info=True,
                        )
                else:
                    logger.warning(
                        f"Not sending peer error as unwired:\n{disc_reason!s}",
                        stack_info=True,
                    )

            if wire.is_connected():
                wire.disconnect()
        except Exception as exc:
            logger.warning(
                "HBIC {self.net_ident} failed closing posting endpoint.", exc_info=True
            )
            assert not disc_fut.done(), "?!"
            disc_fut.set_exception(exc)

        # wait for the wire to finish disconnection process
        await disc_fut  # propagate the exception if one occurred

    def is_landing(self):
        cokt = self._cok_task
        return cokt is not None and not cokt.done()

    def wire_connected(self, wire):
        self.wire = wire
        self.net_ident = wire.net_ident()

        self._send_ctrl.startup()

        self.packet_available.set()

        loop = asyncio.get_running_loop()
        init_done = loop.create_future()

        def on_init_done(fut):
            conn_fut = self._conn_fut
            if conn_fut.done():
                return
            init_exc = fut.exception()
            if init_exc is not None:
                self._conn_fut.set_exception(init_exc)
            else:
                self._conn_fut.set_result(None)

        init_done.add_done_callback(on_init_done)

        self._cok_task = asyncio.create_task(self._co_keeper(init_done))

        def on_cok_done(fut):
            cok_exc = fut.exception()
            if cok_exc is not None:
                logger.error(f"Unexpected failure in co_keeper: {cok_exc!s}")
            if not self._conn_fut.done():
                self._conn_fut.set_exception(
                    cok_exc or asyncio.InvalidStateError("co_keeper exit")
                )
            if not self._disc_fut.done():
                self._disc_fut.set_result(None)
            if wire.is_connected():
                wire.disconnect()

        self._cok_task.add_done_callback(on_cok_done)

    def wire_disconnected(self, wire, exc=None):
        assert wire is self.wire, "wire mismatch ?!"

        if exc is not None:
            logger.error(f"HBIC {self.net_ident} wire error: {exc}")
            disc_reason = f"wire error: {exc}"
            if self.disc_reason is None:
                self.disc_reason = disc_reason

        else:
            disc_reason = self.disc_reason
            exc = asyncio.InvalidStateError(
                f"HBIC {self.net_ident} disconnected - {disc_reason}"
            )

        conn_fut = self._conn_fut
        if not conn_fut.done():
            conn_fut.set_exception(exc)

        self._send_ctrl.shutdown(exc)

        cokt = self._cok_task
        if cokt is not None and not cokt.done():
            cokt.cancel()

        disc_fut = self._disc_fut
        if not disc_fut.done():
            disc_fut.set_result(disc_reason)

        # break waiting for more packets
        self.packet_available.set()

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
            disc_reason = traceback.print_exc()
            await self.disconnect(disc_reason, False)
            raise

    async def _send_buffer(self, buf):
        wire = self.wire
        try:
            await self._send_ctrl.flowing()
            wire.send_data(buf)
        except Exception:
            disc_reason = traceback.print_exc()
            await self.disconnect(disc_reason, False)
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

    async def _recv_packet(self):
        wire = self.wire
        pkt_available = self.packet_available

        try:
            while wire.is_connected():
                wire.resume_recv()

                await pkt_available.wait()

                pkt = wire.recv_packet()
                if pkt is not None:
                    return pkt

                # no or only partial packet arrived
                # clear the event to wait until new data arrived
                pkt_available.clear()
        except asyncio.CancelledError:
            # disconnection most probabily
            return None

        return None

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

    async def _ho_co_start_send(self, co: HoCo):
        assert co._send_done_fut is None, "ho co starting send twice ?!"
        co._send_done_fut = asyncio.get_event_loop().create_future()

        # wait current sender done
        disc_fut = self._disc_fut
        sender = self._sender
        while sender is not None:
            done, pending = await asyncio.wait(
                [sender._send_done_fut, disc_fut], return_when=asyncio.FIRST_COMPLETED
            )
            if disc_fut in done:  # disconnected already
                raise asyncio.InvalidStateError("hbic disconnected")
            # the first awaken aio task sees current sender being None, as it winning the race,
            # it's thus obliged to set a new sender, putting rest waiters loop more iterations awaiting.
            sender = self._sender

        self._sender = co
        await self._send_packet(co._co_seq, b"co_ack_begin")

    async def _new_po_co(self, he) -> PoCo:
        # wait current sender done
        disc_fut = self._disc_fut
        sender = self._sender
        while sender is not None:
            done, pending = await asyncio.wait(
                [sender._send_done_fut, disc_fut], return_when=asyncio.FIRST_COMPLETED
            )
            if disc_fut in done:  # disconnected already
                raise asyncio.InvalidStateError("hbic disconnected")
            # the first awaken aio task sees current sender being None, as it winning the race,
            # it's thus obliged to set a new sender, putting rest waiters loop more iterations awaiting.
            sender = self._sender

        next_co_seq = self._next_co_seq
        while True:
            co_seq = str(next_co_seq)
            next_co_seq += 1
            if next_co_seq > MAX_CO_SEQ:
                next_co_seq = MIN_CO_SEQ
            if co_seq not in self._ppc:
                break  # the new coSeq must not be occupied by a pending co
        self._next_co_seq = next_co_seq

        co = PoCo(he, self, co_seq)
        self._ppc[co_seq] = co
        self._sender = co
        await self._send_packet(co_seq, b"co_begin")
        return co

    async def _ho_co_finish_recv(self, co: HoCo):
        assert co is self._recver, "ho co not current recver ?!"
        assert not co._recv_done_fut.done(), "ho co finishing recv twice ?!"

        pkt = await self._recv_packet()

        if pkt is None:
            raise asyncio.InvalidStateError("hbic disconnected")
        payload, wire_dir = pkt

        if wire_dir != "co_end":
            raise asyncio.InvalidStateError(
                f"Extra packet not landed by ho co before leaving recv stage: [{payload!s}#{wire_dir!s}]"
            )
        if payload != co._co_seq:
            raise asyncio.InvalidStateError("co seq mismatch on co_end")

        # signal co_keeper to start receiving next co
        co._recv_done_fut.set_result(None)

    async def _ho_co_finish_send(self, co: HoCo):
        if co._send_done_fut is not None:
            if co._send_done_fut.done():
                return  # already finished

            assert co is self._sender, "ho co not current sender ?!"

            await self._send_packet(co._co_seq, b"co_ack_end")

            co._send_done_fut.set_result(None)

            self._sender = None

            return

        # ho co never started sending
        # wait opportunity to send, and send an empty co_ack_begin/co_ack_end pair

        # wait current sender done
        disc_fut = self._disc_fut
        sender = self._sender
        while sender is not None:
            done, pending = await asyncio.wait(
                [sender._send_done_fut, disc_fut], return_when=asyncio.FIRST_COMPLETED
            )
            if disc_fut in done:  # disconnected already
                raise asyncio.InvalidStateError("hbic disconnected")
            # the first awaken aio task sees current sender being None, as it winning the race,
            # it's thus obliged to set a new sender, putting rest waiters loop more iterations awaiting.
            sender = self._sender

        # we don't need to set a sender here,
        # just send a pair of co_ack_begin/co_ack_end pair, telling the po co that ho co has completed
        await self._send_packet(co._co_seq, b"co_ack_begin")
        await self._send_packet(co._co_seq, b"co_ack_end")

        fut = asyncio.get_running_loop().create_future()
        fut.set_result(None)
        co._send_done_fut = fut

    async def _po_co_finish_send(self, co: PoCo):
        assert co is self._sender, "po co not current sender ?!"

        await self._send_packet(co._co_seq, b"co_end")

        co._send_done_fut.set_result(None)

        self._sender = None

    async def _co_keeper(self, init_done: asyncio.Future):
        he = self.ho.env
        ppc = self._ppc

        disc_reason = None
        try_send_peer_err = True
        disc_exc = None

        try:
            # call init magic
            init_magic = he.get("__hbi_init__")
            if init_magic is not None:
                maybe_coro = init_magic(po=self.po, ho=self.ho)
                if inspect.iscoroutine(maybe_coro):
                    await maybe_coro
            init_done.set_result(None)
        except Exception as exc:
            disc_exc = exc
            init_done.set_exception(exc)
            disc_reason = f"init failed: {exc!s}"
            logger.error(f"HBIC {self.net_ident!s} init failed.", exc_info=True)

        try:
            while disc_reason is None:

                pkt = await self._recv_packet()
                if pkt is None:
                    # disconnected or disconnecting
                    break
                # got a packet
                payload, wire_dir = pkt

                if "co_begin" == wire_dir:

                    # start a new hosting conversation to accommodate peer's posting conversation

                    ho_co = HoCo(self, payload)

                    self._recver = ho_co

                    asyncio.create_task(ho_co._hosting_thread())

                    await ho_co._recv_done_fut

                    self._recver = None

                elif "" == wire_dir:

                    # back-script to a po co, just land it for side-effects

                    co = self._recver
                    eff_he = co.he
                    if eff_he is None:
                        eff_he = he

                    landed = eff_he.run_in_env(payload)
                    if inspect.iscoroutine(landed):
                        landed = await landed

                elif "co_ack_begin" == wire_dir:

                    # start of response to a local po co, pull it out from ppc and set as current recver

                    try:
                        po_co = ppc.pop(payload)
                    except KeyError as exc:
                        raise RuntimeError("lost po co to ack begin ?!") from exc

                    po_co._begin_acked_fut.set_result(None)

                    # set po co as current receiver
                    self._recver = po_co

                    recv_done_fut = po_co._recv_done_fut
                    if recv_done_fut is not None:
                        await recv_done_fut

                elif "co_ack_end" == wire_dir:

                    # end of response (i.e. completion) of the local po co, should be current recver

                    po_co = self._recver
                    assert isinstance(
                        po_co, PoCo
                    ), "po co not current recver on ack end ?!"
                    assert payload == po_co._co_seq, "po co seq mismatch on ack end ?!"

                    po_co._end_acked_fut.set_result(None)

                    self._recver = None

                elif "co_recv" == wire_dir:

                    disc_reason = (
                        f"No active conversation to receive object:\n{payload!s}"
                    )
                    break

                elif "err" == wire_dir:
                    # peer error

                    try_send_peer_err = False
                    disc_reason = f"peer error: {payload!s}"
                    break

                else:

                    disc_reason = (
                        f"COKEEPER unexpected packet: [#{wire_dir}]{payload!s}"
                    )
                    break

        except asyncio.CancelledError as exc:
            # capture the `disc_reason` passed to `disconnect()`,
            # can be None for normal disconnection.
            disc_reason = self.disc_reason
            disc_exc = exc
        except Exception as exc:
            disc_reason = traceback.print_exc()
            disc_exc = exc

        for co_seq, co in ppc.items():
            if not co._begin_acked_fut.done():
                if disc_exc is None:
                    disc_exc = asyncio.InvalidStateError("hbic disconnected")
                co._begin_acked_fut.set_exception(disc_exc)
                # this future is not always awaited by user, retrive the exc here,
                # let asyncio shutup about this case.
                co._begin_acked_fut.exception()

            if not co._end_acked_fut.done():
                if disc_exc is None:
                    disc_exc = asyncio.InvalidStateError("hbic disconnected")
                co._end_acked_fut.set_exception(disc_exc)
                # this future is not always awaited by user, retrive the exc here,
                # let asyncio shutup about this case.
                co._end_acked_fut.exception()

        # the co keeper task is terminating, disconnect if not already
        if not self._disc_fut.done():
            # but don't await the `disconnect()` here, it cancels this thread as part of the
            # disconnecting procedure.
            asyncio.create_task(self.disconnect(disc_reason, try_send_peer_err))

        # call cleanup magic
        cleanup_magic = he.get("__hbi_cleanup__")
        if cleanup_magic is not None:
            try:
                maybe_coro = cleanup_magic(
                    po=self.po, ho=self.ho, disc_reason=disc_reason
                )
                if inspect.iscoroutine(maybe_coro):
                    await maybe_coro
            except Exception:
                logger.error(f"HBIC {self.net_ident!s} cleanup failed.", exc_info=True)

    async def _recv_one_obj(self, he=None) -> object:
        if he is None:
            he = self.ho.env

        disc_reason = None
        try_send_peer_err = True

        while disc_reason is None:

            pkt = await self._recv_packet()
            if pkt is None:
                raise asyncio.InvalidStateError("hbic disconnected")

            # got a packet
            payload, wire_dir = pkt

            if "co_recv" == wire_dir:
                # the very expected packet

                landed = he.run_in_env(payload)
                if inspect.iscoroutine(landed):
                    landed = await landed
                return landed

            elif "" == wire_dir:
                # some code to execute preceding code for obj to be received.
                # todo this harmful and be explicitly disallowed ?

                landed = he.run_in_env(payload)
                if inspect.iscoroutine(landed):
                    landed = await landed

            elif "err" == wire_dir:
                # peer error

                disc_reason = f"peer error: {payload!s}"
                try_send_peer_err = False

            elif "co_send" == wire_dir:

                disc_reason = "issued co_send before sending an object expected by prior receiving-code"

            else:

                disc_reason = f"RECV unexpected packet: [#{wire_dir}]{payload!s}"

        await self.disconnect(disc_reason, try_send_peer_err)

        raise asyncio.InvalidStateError("hbic disconnected")

    def stop_landing(self) -> bool:  # return whether to keep wire for sending
        # disconnect wire after the sender (if any) finished sending
        async def disc_after_send_done():
            disc_reason = None
            try:

                if self._recver is not None:
                    disc_reason = f"Receiver {self._recver!r} not fulfilled"

                while self._sender is not None:
                    await self._sender._send_done_fut

            except Exception:
                disc_reason = traceback.format_exc()
            await self.disconnect(disc_reason, True)

        asyncio.create_task(disc_after_send_done())
        return True  # keep wire for out sending
