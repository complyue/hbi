import signal
import sys
import traceback

from ..log import *
from .d import *

__all__ = ["handle_signals"]

logger = get_logger(__name__)


def log_and_ignore(signum, frame):
    logger.error(f"Broken pipe!")
    traceback.print_stack(frame, file=sys.stderr)


def dump_and_quit(signum, frame):
    dump_aio_task_stacks()
    sys.exit(3)


def handle_signals():
    try:
        signal.signal(signal.SIGPIPE, log_and_ignore)
    except AttributeError:
        pass

    try:
        signal.signal(signal.SIGQUIT, dump_and_quit)
    except AttributeError:
        pass
