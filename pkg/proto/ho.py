import asyncio
import inspect
from typing import *

from ..he import *
from ..log import *
from .co import *
from .po import *
from .conn import *

__all__ = ["HostingEnd"]

logger = get_logger(__name__)


class HostingEnd:
    """
    HostingEnd is the application programming interface of an HBI hosting endpoint.

    """

    __slots__ = ("_hbic", "_env", "_local_addr", "_co")

    def __init__(self, hbic, env):
        """
        HBI applications should never create a hosting endpoint directly.

        """

        self._hbic: HBICI = hbic
        self._env: HostingEnv = env

        self._local_addr = "<unwired>"

        self._co: HoCo = None

    @property
    def po(self):
        return self._hbic.po

    @property
    def env(self):
        return self._env

    @property
    def local_addr(self):
        return self._local_addr

    @property
    def net_ident(self):
        return self._hbic.net_ident

    @property
    def co(self) -> HoCo:
        """
        co returns current hosting conversation or None if no one is present.

        """
        return self._co

    def is_connected(self) -> bool:
        return self._hbic.is_connected()

    async def disconnect(
        self, err_reason: Optional[str] = None, try_send_peer_err: bool = True
    ):
        await self._hbic.disconnect(err_reason, try_send_peer_err)

    async def wait_disconnected(self):
        await self._hbic.wait_disconnected()
