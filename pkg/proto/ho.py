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

    __slots__ = ("hbic", "env", "po", "local_addr", "_co")

    def __init__(self, hbic, env:, po):
        """
        App code should never create a hosting endpoint directly.

        """

        self.hbic:HBICI = hbic
        self.env: HostingEnv = env
        self.po: PostingEnd = po

        self.local_addr = "<unwired>"

        self._co: HoCo = None

    @property
    def co(self) -> HoCo:
        """
        co returns current hosting conversation or None if no one is present.

        """
        return self._co
