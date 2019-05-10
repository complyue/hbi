import asyncio
import inspect
import socket
import traceback
from typing import *

from .._details import *
from ..log import *
from ..proto.ho import *
from ..proto.po import *

__all__ = ["HBIC", "HBIS"]

logger = get_logger(__name__)


async def _call_init_magic(init_magic, po, ho):
    try:
        maybe_coro = init_magic(po=po, ho=ho)
        if inspect.iscoroutine(maybe_coro):
            await maybe_coro
    except Exception as exc:
        logger.error(f"Error calling __hbi_init__()", exc_info=True)

        err_reason = traceback.format_exc()
        await ho.disconnect(err_reason)


class HBIC:
    """
    HBI client over a socket

    """

    __slots__ = (
        "addr",
        "ctx",
        "app_queue_size",
        "wire_buf_high",
        "wire_buf_low",
        "net_opts",
        "_wire",
    )

    def __init__(
        self,
        addr,
        ctx,
        *,
        app_queue_size: int = 200,
        wire_buf_high=50 * 1024 * 1024,
        wire_buf_low=10 * 1024 * 1024,
        net_opts: Optional[dict] = None,
    ):
        self.addr = addr
        self.ctx = ctx

        self.app_queue_size = app_queue_size
        self.wire_buf_high = wire_buf_high
        self.wire_buf_low = wire_buf_low
        self.net_opts = net_opts if net_opts is not None else {}

        if isinstance(self.addr, (str, bytes)):
            # UNIX domain socket
            pass
        else:  # TCP socket
            if "family" not in self.net_opts:
                # default to IPv4 only
                self.net_opts["family"] = socket.AF_INET

        self._wire = None

    async def __aenter__(self):
        wire = await self.connect()

        return wire.po, wire.ho

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        if exc_type is None:
            err_reason = None
        else:
            err_msg = str(exc_type) + ": " + str(exc_val)
            err_stack = "".join(traceback.format_exception(exc_type, exc_val, exc_tb))
            err_reason = err_msg + "\n" + err_stack

        await self.disconnect(err_reason, try_send_peer_err=True)

    async def connect(self):
        wire = self._wire
        if wire is not None:
            if wire.connected:
                return wire
            wire = None

        loop = asyncio.get_running_loop()
        init_coro = None

        def ProtocolFactory():
            nonlocal init_coro

            po = PostingEnd()
            if self.ctx is None:  # posting only
                ho = None
            else:
                ho = HostingEnd(po, self.app_queue_size)
                ho.ctx = self.ctx

                init_magic = ho.ctx.get("__hbi_init__", None)
                if init_magic is not None:
                    init_coro = _call_init_magic(init_magic, po, ho)

            return SocketWire(po, ho, self.wire_buf_high, self.wire_buf_low)

        if isinstance(self.addr, (str, bytes)):
            # UNIX domain socket
            transport, wire = await loop.create_unix_connection(
                ProtocolFactory, path=self.addr, **self.net_opts
            )
        else:
            # TCP socket
            transport, wire = await loop.create_connection(
                ProtocolFactory,
                host=self.addr.get("host", "127.0.0.1"),
                port=self.addr.get("port", 3232),
                **self.net_opts,
            )
        self._wire = wire

        if init_coro is not None:
            await init_coro

        return wire

    async def disconnect(self, err_reason=None, try_send_peer_err=True):
        wire = self._wire
        if wire is not None:
            if wire.connected:
                await wire.disconnect(err_reason, try_send_peer_err)

    def __str__(self):
        wire = self._wire
        if wire is not None:
            return f"[HBIC#{self.addr!s}@{wire.net_ident!s}]"
        return f"[HBIC#{self.addr!s}]"


class HBIS:
    """
    HBI server over sockets

    """

    __slots__ = (
        "addr",
        "context_factory",
        "app_queue_size",
        "wire_buf_high",
        "wire_buf_low",
        "net_opts",
        "_server",
    )

    def __init__(
        self,
        addr,
        context_factory,
        *,
        app_queue_size: int = 100,
        wire_buf_high=20 * 1024 * 1024,
        wire_buf_low=6 * 1024 * 1024,
        net_opts: Optional[dict] = None,
    ):
        self.addr = addr
        self.context_factory = context_factory

        self.app_queue_size = app_queue_size
        self.wire_buf_high = wire_buf_high
        self.wire_buf_low = wire_buf_low
        self.net_opts = net_opts if net_opts is not None else {}

        if isinstance(self.addr, (str, bytes)):
            # UNIX domain socket
            pass
        else:  # TCP socket
            if "family" not in self.net_opts:
                # default to IPv4 only
                self.net_opts["family"] = socket.AF_INET

        self._server = None

    async def server(self):
        if self._server is not None:
            return self._server

        loop = asyncio.get_running_loop()

        def ProtocolFactory():
            try:
                po = PostingEnd()
                ho = HostingEnd(po, self.app_queue_size)
                ho.ctx = self.context_factory(po=po, ho=ho)
                wire = SocketWire(po, ho, self.wire_buf_high, self.wire_buf_low)

                init_magic = ho.ctx.get("__hbi_init__", None)
                if init_magic is not None:
                    loop.call_soon(
                        loop.create_task, _call_init_magic(init_magic, po, ho)
                    )

                return wire
            except Exception:
                logger.error(f"Error establishing HBI wire.", exc_info=True)

        if isinstance(self.addr, (str, bytes)):
            # UNIX domain socket
            self._server = await loop.create_unix_server(
                ProtocolFactory, path=self.addr, **self.net_opts
            )
        else:
            # TCP socket
            self._server = await loop.create_server(
                ProtocolFactory,
                host=self.addr.get("host", "127.0.0.1"),
                port=self.addr.get("port", 3232),
                **self.net_opts,
            )

        return self._server

    async def serve_until_closed(self):
        server = await self.server()
        await server.wait_closed()

    def __str__(self):
        server = self._server
        if server is not None:
            return f"[HBIS#{self.addr!s}@{server.get_extra_info('sockname')!s}]"
        return f"[HBIS#{self.addr!s}]"


class SocketWire(asyncio.Protocol):
    """
    HBI wire protocol over a Socket transport

    """

    __slots__ = (
        "po",
        "ho",
        "wire_buf_high",
        "wire_buf_low",
        "transport",
        "_hdr_buf",
        "_hdr_got",
        "_bdy_buf",
        "bdy_got",
        "_wire_dir",
        "_recv_buffer",
        "_data_sink",
    )

    def __init__(
        self, po, ho, wire_buf_high=20 * 1024 * 1024, wire_buf_low=6 * 1024 * 1024
    ):
        self.po = po
        self.ho = ho
        self.wire_buf_high = wire_buf_high
        self.wire_buf_low = wire_buf_low

        self.transport = None

        self._hdr_buf = bytearray(PACK_HEADER_MAX)
        self._hdr_got = 0
        self._bdy_buf = None
        self._bdy_got = 0
        self._wire_dir = None

        # for packet parsing and data pumping
        self._recv_buffer = BufferList()
        self._data_sink = None

        if po is not None:
            po._wire = self
        if ho is not None:
            ho._wire = self

    def connection_made(self, transport):
        logger.debug(f"HBI connection made: {transport!r}")

        self.transport = transport

        po = self.po
        if po is not None:
            assert po._wire is self, "wire mismatch ?!"
            po._connected()

        ho = self.ho
        if ho is not None:
            assert ho._wire is self, "wire mismatch ?!"
            ho._connected()

    def data_received(self, chunk):
        ho = self.ho
        if ho is None:
            raise RuntimeError(f"Posting only connection received data ?!")
        assert ho._wire is self, "wire mismatch ?!"
        self._take_data(chunk)

    def eof_received(self):
        logger.debug(f"HBI connection eof: {self.transport!r}")

        ho = self.ho
        if ho is None:  # posting only connection
            return True  # don't let the transport close itself on peer eof
        # hosting endpoint can prevent the transport from being closed by returning True
        return ho._peer_eof()

    def connection_lost(self, exc):
        logger.debug(f"HBI connection lost: {self.transport!r} exc={exc}")

        po = self.po
        if po is not None:
            assert po._wire is self, "wire mismatch ?!"
            po._disconnected(exc)
        ho = self.ho
        if ho is not None:
            assert ho._wire is self, "wire mismatch ?!"
            ho._disconnected(exc)

    def pause_writing(self):
        self.po._send_mutex.obstruct()

    def resume_writing(self):
        self.po._send_mutex.unleash()

    @property
    def connected(self):
        transport = self.transport
        if transport is None:
            return False
        return not transport.is_closing()

    async def disconnect(self, err_reason=None, try_send_peer_err=True):
        po, ho = self.po, self.ho
        if ho is not None:
            # hosting endpoint closes posting endpoint on its own closing
            await ho.disconnect(err_reason, try_send_peer_err)
        elif po is not None:
            # a posting only wire
            await po.disconnect(err_reason, try_send_peer_err)
        else:
            assert False, "neither po nor ho ?!"

    @property
    def net_ident(self):
        transport = self.transport
        if transport is None:
            return "<unwired>"

        try:
            addr_info = f"{transport.get_extra_info('sockname')}<=>{transport.get_extra_info('peername')}"
            if transport.is_closing():
                return f"@closing@{addr_info}"
            return addr_info
        except Exception as exc:
            return f"<HBI bogon wire: {exc!s}>"

    @property
    def remote_addr(self):
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        peername = transport.get_extra_info("peername")
        if len(peername) in (2, 4):
            return ":".join(str(v) for v in peername)
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    @property
    def remote_host(self):
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        peername = transport.get_extra_info("peername")
        if len(peername) in (2, 4):
            return peername[0]
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    @property
    def remote_port(self):
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        peername = transport.get_extra_info("peername")
        if len(peername) in (2, 4):
            return peername[1]
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    @property
    def local_host(self):
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        sockname = transport.get_extra_info("sockname")
        if len(sockname) in (2, 4):
            return sockname[0]
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    @property
    def local_addr(self):
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        sockname = transport.get_extra_info("sockname")
        if len(sockname) in (2, 4):
            return ":".join(str(v) for v in sockname)
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    @property
    def local_port(self):
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        sockname = transport.get_extra_info("sockname")
        if len(sockname) in (2, 4):
            return sockname[1]
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    def _take_data(self, chunk):
        rcvb = self._recv_buffer

        # push to buffer
        if chunk:
            rcvb.append(BytesBuffer(chunk))

        # feed as much buffered data as possible to data sink if one present
        while self._data_sink:
            # make sure data keep flowing in regarding lwm
            if rcvb.nbytes <= self.wire_buf_low:
                self.transport.resume_reading()

            if rcvb is None:
                # unexpected disconnect
                self._data_sink(None)
                return

            chunk = rcvb.popleft()
            if not chunk:
                # no more buffered data, wire is empty, return
                return
            self._data_sink(chunk)

        if self.ho._disc_fut is not None:
            return  # disconnecting, nop

        # ctrl incoming flow regarding hwm/lwm
        buffered_amount = rcvb.nbytes
        if buffered_amount >= self.wire_buf_high:
            self.transport.pause_reading()
        elif buffered_amount <= self.wire_buf_low:
            self.transport.resume_reading()

        self.ho._landing.set()

    def _check_pause(self):
        # pause reading wrt lwm, when app queue is full
        buffered_amount = self._recv_buffer.nbytes
        if buffered_amount > self.wire_buf_low:
            self.transport.pause_reading()
            return True
        return False

    def _check_resume(self):
        # check resume reading on wire reading activities
        buffered_amount = self._recv_buffer.nbytes
        if buffered_amount <= self.wire_buf_low:
            self.transport.resume_reading()
            return True
        return False

    def _land_one(self) -> bool:
        while True:
            if self._recv_buffer.nbytes <= 0:
                # no single full packet can be read from buffer

                # make sure back pressure is released
                self.transport.resume_reading()

                # suspending further landing until new data arrived
                self.ho._landing.clear()

                return False

            chunk = self._recv_buffer.popleft()
            if not chunk:  # ignore empty buf in the buffer queue
                continue

            while True:
                if self._bdy_buf is None:
                    # packet header not fully received yet
                    pe_pos = chunk.find(b"]")
                    if pe_pos < 0:
                        # still not enough for packet header
                        if len(chunk) + self._hdr_got >= PACK_HEADER_MAX:
                            raise RuntimeError(
                                f"No packet header within first {len(chunk) + self._hdr_got} bytes."
                            )
                        hdr_got = self._hdr_got + len(chunk)
                        self._hdr_buf[self._hdr_got : hdr_got] = chunk.data()
                        self._hdr_got = hdr_got
                        break  # proceed to next chunk in buffer
                    hdr_got = self._hdr_got + pe_pos
                    self._hdr_buf[self._hdr_got : hdr_got] = chunk.data(0, pe_pos)
                    self._hdr_got = hdr_got
                    chunk.consume(pe_pos + 1)
                    header_pl = self._hdr_buf[: self._hdr_got]
                    if not header_pl.startswith(b"["):
                        rpt_len = len(header_pl)
                        rpt_hdr = header_pl[: min(self._hdr_got, 30)]
                        rpt_net = self.net_ident
                        raise RuntimeError(
                            f"Invalid packet start in header: len: {rpt_len}, peer: {rpt_net}, head: [{rpt_hdr}]"
                        )
                    ple_pos = header_pl.find(b"#", 1)
                    if ple_pos <= 0:
                        raise RuntimeError(f"No packet length in header: [{header_pl}]")
                    pack_len = int(header_pl[1:ple_pos])
                    self._wire_dir = header_pl[ple_pos + 1 :].decode("utf-8")
                    self._hdr_got = 0
                    self._bdy_buf = bytearray(pack_len)
                    self._bdy_got = 0
                else:
                    # packet body not fully received yet
                    needs = len(self._bdy_buf) - self._bdy_got
                    if len(chunk) < needs:
                        # still not enough for packet body
                        bdy_got = self._bdy_got + len(chunk)
                        self._bdy_buf[self._bdy_got : bdy_got] = chunk.data()
                        self._bdy_got = bdy_got
                        break  # proceed to next chunk in buffer
                    # body can be filled now
                    self._bdy_buf[self._bdy_got :] = chunk.data(0, needs)
                    if (
                        len(chunk) > needs
                    ):  # the other case is equal, means exactly consumed
                        # put back extra data to buffer
                        self._recv_buffer.appendleft(chunk.consume(needs))
                    payload = self._bdy_buf.decode("utf-8")
                    self._bdy_buf = None
                    self._bdy_got = 0
                    wire_dir = self._wire_dir
                    self._wire_dir = None

                    self.ho._land_packet(payload, wire_dir)
                    return True

    def _begin_offload(self, sink):
        rcvb = self._recv_buffer

        if self._data_sink is not None:
            raise RuntimeError("HBI already offloading data")
        if not callable(sink):
            raise RuntimeError(
                "HBI sink to offload data must be a function accepting data chunks"
            )
        self._data_sink = sink
        if rcvb.nbytes > 0:
            # having buffered data, dump to sink
            while self._data_sink is sink:
                chunk = rcvb.popleft()
                # make sure data keep flowing in regarding lwm
                if rcvb.nbytes <= self.wire_buf_low:
                    self.transport.resume_reading()
                if not chunk:
                    break
                sink(chunk)
        else:
            sink(b"")

    def _end_offload(self, read_ahead=None, sink=None):
        if sink is not None and sink is not self._data_sink:
            raise RuntimeError("HBI resuming from wrong sink")
        self._data_sink = None
        if read_ahead:
            self._recv_buffer.appendleft(read_ahead)
