import ast
from typing import *
from ..log import *

from .po import PostingEnd
from .ho import HostingEnd

__all__ = ["HostingEnv", "run_py"]

logger = get_logger(__name__)


class HostingEnv:
    """
    HostingEnv is the container of hbi artifacts, including:
      * functions
      * object constructors (special functions taking n args, returning 1 object)
      * value objects
      * reactor methods
    These artifacts need to be explicitly exposed to a hosting environment,
    to accomodate landing of peer scripting code.

    """

    __slots__ = ("_globals", "_locals", "_exposed_names", "_po", "_ho")

    def __init__(self, open_: bool = True):
        self._globals = {}
        self._locals = self._globals if open_ else None
        self._exposed_names = []
        self._po = None
        self._ho = None

    @property
    def po(self) -> "PostingEnd":
        return self._po

    @property
    def ho(self) -> "HostingEnd":
        return self._ho

    @property
    def globals(self) -> dict:
        return self._globals

    @property
    def locals(self) -> Optional[dict]:
        return self._locals

    @property
    def exposed_names(self) -> Tuple[str]:
        return tuple(self._exposed_names)

    def run_in_env(self, code: str) -> object:
        globals_ = self._globals
        locals_ = self._locals
        if locals_ is None:
            # locals should not be None, create one to fire-and-forget
            locals_ = {}

        return run_py(code, globals_, locals_, "<hbi-code>")

    def name_exposed(self, name: str) -> bool:
        return name in self._exposed_names

    def _validate_expose_name(self, name: str):
        if name in self._exposed_names:
            raise AttributeError(f"name `{name!s}` already exposed")

    def expose_function(self, name: str, func: Callable):
        if not callable(func):
            raise TypeError(f"not a function: {func!r}")
        if name is None:
            name = func.__name__
        self._validate_expose_name(name)
        self._exposed_names.append(name)
        self._globals[name] = func

    def expose_ctor(self, ctor: type):
        if not callable(ctor):
            raise TypeError(f"constructor not callable: {ctor!r}")
        try:
            name = ctor.__name__
        except AttributeError:
            raise TypeError(f"constructor has no name: {ctor!r}")
        self._validate_expose_name(name)
        self._exposed_names.append(name)
        self._globals[name] = ctor

    def expose_value(self, name: str, value: object):
        self._validate_expose_name(name)
        self._exposed_names.append(name)
        self._globals[name] = value

    def get(self, name: str) -> object:
        return self._globals.get(name, None)

    def expose_reactor(self, reactor: object, cls: type = None):

        try:
            names_to_expose = (
                cls.names_to_expose if cls is not None else reactor.names_to_expose
            )
        except AttributeError:
            logger.warning(
                f"Reactor type {cls or type(reactor)!r} has no exposure declaration,"
                " all methods not starting with underscore will be exposed.",
                exc_info=True,
            )

            # expose all public (name not started with underscore) artifacts
            for name, _ in (cls or type(reactor)).__dict__.items():
                assert len(name) > 0, "?!"
                if "_" == name[0]:  # started with underscore, consider private/internal
                    continue
                if name in self._exposed_names:
                    # name already exposed, not to overwrite more specific attrs with a more general one
                    continue
                self._exposed_names.append(name)
                self._globals[name] = getattr(reactor, name)

            if cls is not None:
                # finished exposing a specific super class, done
                return

            # try expose inherited artifacts from super classes
            for cls in type(reactor).__mro__[1:]:
                if "builtins" == cls.__module__:
                    continue  # no do for builtin types
                self.expose_reactor(reactor, cls)

            return  # done without an exposure declarartion

        # with an exposure declaration, expose whatever-and-only names declared in the list
        for name in names_to_expose:  # AttributeError following will raise through
            expose_val = getattr(reactor, name)
            if name in self._exposed_names:
                # name already exposed, not to overwrite more specific attrs with a more general one
                continue
            self._exposed_names.append(name)
            self._globals[name] = expose_val


def run_py(
    code: str, globals_: dict, locals_: dict = None, src_name="<py-code>"
) -> object:
    """
    Run arbitrary Python code in supplied globals/locals, return evaluated value of last statement.

    """
    try:
        ast_ = ast.parse(code, src_name, "exec")
        last_expr = None
        last_def_name = None
        for field_ in ast.iter_fields(ast_):
            if "body" != field_[0]:
                continue
            if len(field_[1]) > 0:
                le = field_[1][-1]
                if isinstance(le, ast.Expr):
                    last_expr = ast.Expression()
                    last_expr.body = field_[1].pop().value
                elif isinstance(le, (ast.FunctionDef, ast.ClassDef)):
                    last_def_name = le.name
        exec(compile(ast_, src_name, "exec"), globals_, locals_)
        if last_expr is not None:
            return eval(compile(last_expr, src_name, "eval"), globals_, locals_)
        elif last_def_name is not None:
            return defs[last_def_name]
        return None
    except Exception:
        logger.error(
            f"""Error running HBI code:
-=-{src_name}-=-
{code!s}
-=*{src_name}*=-
""",
            exc_info=True,
        )
        raise
