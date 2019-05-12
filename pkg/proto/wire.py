from typing import *

__all__ = ["HBIWire"]


class HBIWire:
    """
    HBIWire is the abstractinterface an HBI wire should implement
    
    """

    def is_connected() -> bool:
        raise NotImplementedError

    def disconnect():
        raise NotImplementedError

    def send_packet(
        payload: Union[bytes, bytearray, memoryview], wire_dir: Union[bytes, str]
    ):
        raise NotImplementedError

    def send_data(buf: Union[bytes, bytearray, memoryview]):
        raise NotImplementedError

    def recv_packet() -> Optional[Tuple[str, str]]:
        raise NotImplementedError

    def begin_offload(self, data_sink):
        raise NotImplementedError

    def end_offload(self, read_ahead: bytes = None, sink=None):
        raise NotImplementedError

    def pause_recv():
        raise NotImplementedError

    def resume_recv():
        raise NotImplementedError

    def net_ident() -> str:
        raise NotImplementedError

    def remote_addr() -> str:
        raise NotImplementedError

    def remote_host() -> str:
        raise NotImplementedError

    def remote_port() -> str:
        raise NotImplementedError

    def local_host() -> str:
        raise NotImplementedError

    def local_addr() -> str:
        raise NotImplementedError

    def local_port() -> str:
        raise NotImplementedError
