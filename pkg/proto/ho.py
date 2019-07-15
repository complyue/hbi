import asyncio
import inspect
from typing import *

from ..log import *
from .conn import *
from .he import *
from .po import *

__all__ = ["HostingEnd", "HoCo"]

logger = get_logger(__name__)


class HostingEnd:
    """
    HostingEnd is the application programming interface of a hosting endpoint.

    """

    __slots__ = ("_hbic", "_env")

    def __init__(self, hbic, env):
        """
        HBI applications should never create a hosting endpoint directly.

        """

        self._hbic: HBICI = hbic
        self._env: HostingEnv = env

    @property
    def env(self):
        return self._env

    @property
    def net_ident(self):
        return self._hbic.net_ident

    @property
    def local_addr(self):
        return self._hbic.wire.local_addr()

    @property
    def local_host(self):
        return self._hbic.wire.local_host()

    @property
    def local_port(self):
        return self._hbic.wire.local_port()

    def co(self) -> "HoCo":
        """
        co returns the current hosting conversation in `recv` stage.

        """

        recver = self._hbic._recver
        if isinstance(recver, HoCo):
            return recver
        return None

    def is_connected(self) -> bool:
        return self._hbic.is_connected()

    async def disconnect(
        self, disc_reason: Optional[str] = None, try_send_peer_err: bool = True
    ):
        await self._hbic.disconnect(disc_reason, try_send_peer_err)

    async def wait_disconnected(self):
        await self._hbic.wait_disconnected()


class HoCo:
    """
    HoCo is the passive, hosting conversation.

    A HoCo is triggered by a PoCo from peer's posting endpoint, it is automatically available to
    application, obtained by calling HostingEnd.co()

    """

    __slots__ = ("_hbic", "_co_seq", "_recv_done_fut", "_send_done_fut")

    def __init__(self, hbic, co_seq):
        self._hbic = hbic
        self._co_seq = co_seq

        loop = asyncio.get_running_loop()
        self._recv_done_fut = loop.create_future()
        self._send_done_fut = None

    @property
    def co_seq(self):
        """
        co_seq returns the sequence number of this conversation.

        The sequence of a hosting conversation is always the same as the peer's posting conversation
        that triggered it.

        """
        return self._co_seq

    async def recv_obj(self):
        """
        recv_obj returns the landed result of a piece of peer-scripting-code sent by calling
        PoCo.send_obj() with the remote posting conversation which triggered this ho co.

        Note this can only be called in `recv` stage, and from the dedicated hosting aio task, i.e.
        from functions exposed to the hosting environment and called by the peer-scripting-code from
        the remote posting conversation which triggered this ho co.

        """

        if self._recv_done_fut.done():
            raise asyncio.InvalidStateError("ho co not in recv stage")

        hbic = self._hbic
        if self is not hbic._recver:
            raise asyncio.InvalidStateError("ho co not current recver")

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
        recv_data receives the binary data/stream sent by calling PoCo.send_data()
        with the remote posting conversation which triggered this ho co.

        Note this can only be called in `recv` stage, and from the dedicated hosting aio task, i.e.
        from functions exposed to the hosting environment and called by the peer-scripting-code from
        the remote posting conversation which triggered this ho co.

        """

        if self._recv_done_fut.done():
            raise asyncio.InvalidStateError("ho co not in recv stage")

        hbic = self._hbic
        if self is not hbic._recver:
            raise asyncio.InvalidStateError("ho co not current recver")

        await hbic._recv_data(bufs)

    async def finish_recv(self):
        """
        finish_recv transits this hosting conversation from `recv` to `work` stage.

        As soon as all recv operations done, if some time-consuming work should be carried out to
        prepare the response to be sent back, a hosting conversation should transit to `work` stage
        by calling `finish_recv()`; a hosting conversion should be closed directly, if nothing is
        supposed to be sent back.

        As soon as no further back-script and/or data/stream is to be sent with a hosting conversation,
        it should close to release the underlying HBI transport wire for the next posting conversation
        to start off or next send-ready hosting conversation to start sending.

        Note this can only be called from the dedicated hosting aio task, i.e. from functions
        exposed to the hosting environment and called by the peer-scripting-code from the remote
        posting conversation which triggered this ho co.

        """

        if self._recv_done_fut.done():
            raise asyncio.InvalidStateError("ho co not in recv stage")

        hbic = self._hbic

        await hbic._ho_co_finish_recv(self)

    async def start_send(self):
        """
        start_send transits this hosting conversation from `recv` or `work` stage to `send` stage.

        As soon as all recv operations done, if no time-consuming work needs to be carried out to
        prepare the response to be sent back, a hosting conversation should transit to `send` stage
        by calling `start_send()`; a hosting conversion should be closed directly, if nothing is
        supposed to be sent back.

        As soon as no further back-script and/or data/stream is to be sent with a hosting conversation,
        it should close to release the underlying HBI transport wire for the next posting conversation
        to start off or next send-ready hosting conversation to start sending.

        Note this can only be called from the dedicated hosting aio task, i.e. from functions
        exposed to the hosting environment and called by the peer-scripting-code from the remote
        posting conversation which triggered this ho co.

        """

        hbic = self._hbic

        if not self._recv_done_fut.done():
            await hbic._ho_co_finish_recv(self)

        await hbic._ho_co_start_send(self)

    async def send_code(self, code: str):
        """
        send_code sends `code` as back-script to peer's hosting endpoint for landing by its hosting
        environment. Only side effects are expected from landing of `code` at peer site.

        Note this can only be called in `send` stage, and from the dedicated hosting aio task, i.e.
        from functions exposed to the hosting environment and called by the peer-scripting-code from
        the remote posting conversation which triggered this ho co.

        """

        if self._send_done_fut is None or self._send_done_fut.done():
            raise asyncio.InvalidStateError("ho co not in send stage")

        hbic = self._hbic
        if self is not hbic._sender:
            raise asyncio.InvalidStateError("ho co not current sender")

        await hbic._send_packet(code)

    async def send_obj(self, code: str):
        """
        send_obj sends `code` to peer's hosting endpoint for landing by its hosting environment, and
        the landed value to be received by calling PoCo.recv_obj() with the remote posting conversation
        which triggered this ho co.

        Note this can only be called in `send` stage, and from the dedicated hosting aio task, i.e.
        from functions exposed to the hosting environment and called by the peer-scripting-code from
        the remote posting conversation which triggered this ho co.

        """

        if self._send_done_fut is None or self._send_done_fut.done():
            raise asyncio.InvalidStateError("ho co not in send stage")

        hbic = self._hbic
        if self is not hbic._sender:
            raise asyncio.InvalidStateError("ho co not current sender")

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
        send_data sends a single chunk of binary data or a data/stream to peer site, to be received
        with the remote posting conversation which triggered this ho co, by calling PoCo.recv_data()
        
        Note this can only be called in `send` stage, and from the dedicated hosting aio task, i.e.
        from functions exposed to the hosting environment and called by the peer-scripting-code from
        the remote posting conversation which triggered this ho co.

        """

        if self._send_done_fut is None or self._send_done_fut.done():
            raise asyncio.InvalidStateError("ho co not in send stage")

        hbic = self._hbic
        if self is not hbic._sender:
            raise asyncio.InvalidStateError("ho co not current sender")

        await hbic._send_data(bufs)

    async def close(self):
        """
        close closes this hosting conversation, neither send nor recv operation can be performed
        with a closed hosting conversation.

        Note this can only be called from the dedicated hosting aio task, i.e. from functions
        exposed to the hosting environment and called by the peer-scripting-code from the remote
        posting conversation which triggered this ho co.

        """
        hbic = self._hbic
        if not hbic.is_connected():
            raise asyncio.InvalidStateError(
                f"hbic disconnected due to: {hbic.disc_reason!s}"
            )

        if not self._recv_done_fut.done():
            await hbic._ho_co_finish_recv(self)

        await hbic._ho_co_finish_send(self)

    # this must be spawned as a dedicated aio task
    async def _hosting_thread(self):
        hbic = self._hbic
        assert self is hbic._recver, "ho co not start out as current recver ?!"
        assert not self._recv_done_fut.done(), "ho co started out w/ recv done ?!"
        he = hbic.ho.env

        disc_reason = None
        try_send_peer_err = True

        try:
            while disc_reason is None:

                pkt = await hbic._recv_packet()
                if pkt is None:
                    assert not wire.is_connected()
                    break
                # got a packet
                payload, wire_dir = pkt

                if "" == wire_dir:
                    # peer is pushing the textual code for side-effect of its landing

                    landed = he.run_in_env(payload)
                    if inspect.iscoroutine(landed):
                        landed = await landed

                    if self._recv_done_fut.done():
                        # recv actively finished by the exposed reacting function

                        # finish send if still connected
                        if hbic.is_connected():
                            await hbic._ho_co_finish_send(self)

                        # terminate this hosting thread anyway
                        return

                elif "co_send" == wire_dir:
                    # peer is requesting this end to push landed result (in textual repr code) back

                    landed = he.run_in_env(payload)
                    if inspect.iscoroutine(landed):
                        landed = await landed

                    await hbic._send_packet(repr(landed), b"co_recv")

                elif "co_end" == wire_dir:
                    # done with this hosting conversation

                    if payload != self._co_seq:
                        disc_reason = "co seq mismatch on co_end"

                    assert (
                        not self._recv_done_fut.done()
                    ), "recv finished by reacting func without co_end swallowed ?!"

                    self._recv_done_fut.set_result(None)

                    await hbic._ho_co_finish_send(self)

                    return

                elif "co_recv" == wire_dir:
                    # pushing obj to a ho co

                    disc_reason = "co_recv without priori receiving code under landing"
                    break

                elif "err" == wire_dir:
                    # peer error

                    disc_reason = f"peer error: {payload!s}"
                    try_send_peer_err = False
                    break

                else:

                    disc_reason = f"HO unexpected packet: [#{wire_dir}]{payload!s}"
                    break

        except asyncio.CancelledError as exc:
            # capture the `disc_reason` passed to `disconnect()`,
            # can be None for normal disconnection.
            disc_reason = hbic.disc_reason
        except Exception as exc:
            disc_reason = traceback.print_exc()

        await hbic.disconnect(disc_reason, try_send_peer_err)
