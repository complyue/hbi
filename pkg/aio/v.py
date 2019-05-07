import threading

__all__ = ["SyncVar"]


class SyncVar:
    """
    Synchronous variable to be set from another thread

    """

    def __init__(self):
        self.ready = threading.Event()
        self.val = None

    def set(self, val):
        self.val = val
        self.ready.set()

    def unset(self):
        self.val = None
        self.ready.clear()

    def get(self):
        self.ready.wait()
        return self.val

    def is_set(self):
        return self.ready.is_set()

