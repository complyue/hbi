import asyncio
import inspect
import socket
import traceback
from typing import *

from .._details import *
from ..he import *
from ..log import *
from ..proto import *

__all__ = ["serve_socket", "dial_socket"]

logger = get_logger(__name__)


async def serve_socket(
    addr: Union[tuple, str],
    he_factory: Callable[[], HostingEnv],
    *,
    wire_buf_high=20 * 1024 * 1024,
    wire_buf_low=6 * 1024 * 1024,
    net_opts: Optional[dict] = None,
):
    if net_opts is None:
        net_opts = {}

    loop = asyncio.get_running_loop()

    if isinstance(addr, (str, bytes)):
        # UNIX domain socket
        server = await loop.create_unix_server(
            lambda: SocketWire(HBIC(he_factory()), wire_buf_high, wire_buf_low),
            path=addr,
            **net_opts,
        )
    else:
        # TCP socket
        if "family" not in net_opts:
            # default to IPv4 only
            net_opts["family"] = socket.AF_INET
        server = await loop.create_server(
            lambda: SocketWire(HBIC(he_factory()), wire_buf_high, wire_buf_low),
            host=addr.get("host", "127.0.0.1"),
            port=addr.get("port", 3232),
            **net_opts,
        )

    return server


async def dial_socket(
    addr: Union[tuple, str],
    he: Optional[HostingEnv] = None,
    *,
    wire_buf_high=20 * 1024 * 1024,
    wire_buf_low=6 * 1024 * 1024,
    net_opts: Optional[dict] = None,
) -> Tuple[PostingEnd, HostingEnd]:
    if net_opts is None:
        net_opts = {}

    loop = asyncio.get_running_loop()

    if isinstance(addr, (str, bytes)):
        # UNIX domain socket
        transport, wire = await loop.create_unix_connection(
            lambda: SocketWire(HBIC(he), wire_buf_high, wire_buf_low),
            path=addr,
            **net_opts,
        )
    else:  # TCP socket
        if "family" not in net_opts:
            # default to IPv4 only
            net_opts["family"] = socket.AF_INET
        transport, wire = await loop.create_connection(
            lambda: SocketWire(HBIC(he), wire_buf_high, wire_buf_low),
            host=addr.get("host", "127.0.0.1"),
            port=addr.get("port", 3232),
            **net_opts,
        )

    hbic = wire.hbic
    await hbic.wait_connected()

    return hbic.po, hbic.ho


class SocketWire(asyncio.Protocol):
    """
    HBI wire protocol over a Socket transport

    """

    __slots__ = (
        "hbic",
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
        self, hbic, wire_buf_high=20 * 1024 * 1024, wire_buf_low=6 * 1024 * 1024
    ):
        self.hbic = hbic
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

    def connection_made(self, transport):
        self.transport = transport
        self.hbic.wire_connected(self)

    def data_received(self, chunk):
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

        if not self.hbic.is_landing():
            # landing stopped, discard any data here
            # wire/transport should get disconnected elsewhere
            return

        # ctrl incoming flow regarding hwm/lwm
        buffered_amount = rcvb.nbytes
        if buffered_amount >= self.wire_buf_high:
            self.transport.pause_reading()
        elif buffered_amount <= self.wire_buf_low:
            self.transport.resume_reading()

        self.hbic.packet_available.set()

    def eof_received(self):
        # cb can prevent the transport from being closed by returning True
        return self.hbic.stop_landing()

    def connection_lost(self, exc):
        self.hbic.wire_disconnected(self, exc)

    def pause_writing(self):
        self.hbic.pause_sending()

    def resume_writing(self):
        self.hbic.resume_sending()

    def is_connected(self) -> bool:
        transport = self.transport
        if transport is None:
            return False
        return not transport.is_closing()

    def disconnect(self):
        if self.transport.is_closing():
            logger.warning(
                f"Repeating disconnection of {self.net_ident()!s}", exc_info=True
            )
        else:
            self.transport.close()

    def net_ident(self) -> str:
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

    def remote_addr(self) -> str:
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        peername = transport.get_extra_info("peername")
        if len(peername) in (2, 4):
            return ":".join(str(v) for v in peername)
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    def remote_host(self) -> str:
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        peername = transport.get_extra_info("peername")
        if len(peername) in (2, 4):
            return peername[0]
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    def remote_port(self) -> str:
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        peername = transport.get_extra_info("peername")
        if len(peername) in (2, 4):
            return peername[1]
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    def local_host(self) -> str:
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        sockname = transport.get_extra_info("sockname")
        if len(sockname) in (2, 4):
            return sockname[0]
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    def local_addr(self) -> str:
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        sockname = transport.get_extra_info("sockname")
        if len(sockname) in (2, 4):
            return ":".join(str(v) for v in sockname)
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    def local_port(self) -> str:
        transport = self.transport
        if transport is None:
            raise asyncio.InvalidStateError("Socket not wired!")

        sockname = transport.get_extra_info("sockname")
        if len(sockname) in (2, 4):
            return sockname[1]
        raise NotImplementedError(
            "Socket transport other than tcp4/tcp6 not supported yet."
        )

    def send_packet(
        self, payload: Union[bytes, bytearray, memoryview], wire_dir: Union[bytes, str]
    ):
        self.transport.writelines([b"[%d#%s]" % (len(payload), wire_dir), payload])

    def send_data(self, buf: Union[bytes, bytearray, memoryview]):
        self.transport.write(buf)

    def recv_packet(self) -> Optional[Tuple[str, str]]:
        while True:
            if self._recv_buffer.nbytes <= 0:
                # no single full packet can be read from buffer

                # make sure back pressure is released
                self.transport.resume_reading()

                # suspending further landing until new data arrived
                self.hbic.packet_available.clear()

                # no full packet available yet
                return None

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

                    return payload, wire_dir

    def begin_offload(self, sink):
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

    def end_offload(self, read_ahead=None, sink=None):
        if sink is not None and sink is not self._data_sink:
            raise RuntimeError("HBI resuming from wrong sink")
        self._data_sink = None
        if read_ahead:
            self._recv_buffer.appendleft(read_ahead)

    def pause_recv(self):
        # pause reading wrt lwm, when app queue is full
        buffered_amount = self._recv_buffer.nbytes
        if buffered_amount > self.wire_buf_low:
            self.transport.pause_reading()
            return True
        return False

    def resume_recv(self):
        # check resume reading on wire reading activities
        buffered_amount = self._recv_buffer.nbytes
        if buffered_amount <= self.wire_buf_low:
            self.transport.resume_reading()
            return True
        return False
