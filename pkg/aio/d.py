import asyncio
import sys

__all__ = ["dump_aio_task_stacks"]


def dump_aio_task_stacks():
    for aiot in asyncio.Task.all_tasks():
        print("", file=sys.stderr)
        aiot.print_stack(file=sys.stderr)
