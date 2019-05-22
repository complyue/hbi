# Hosting Based Interface

## The Problem

### One Service On Behalf of Many

A human user thinks, between actions he/she will take to perform, and the thinking
usually takes previous action's result into account. Waiting for result/response is
a rather natural being in single user scenarios.

While most decent computer application systems comprise far more than a single service,
and even a lone service, are rather likely to be integrated with other systems/services
into larger solutions, to be useful today.

Such a **service** software typically act on behalf of many users concurrently, with main
line scenarios involve coordinated operations with other **service**s in reaction to
each user's each activity. This is largely different from a traditional **client**
software which assumes single user's activities only. If every user activity is to be
waited, there will be just too much waitings.

### Inter-Service Communication with request/response pattern

A classical implementation of the classic **request/response** pattern, say `HTTP/1.x`,
as the most typical example, is to pend subsequent out-bound transportation through
the underlying transport, wait until the expected response has been sent back from
peer endpoint, as in-bound transportation through the underlying transport, before
the next request is let-go to start off.

The result is that transports (tcp connections for HTTP), wasted too much time in
RTT (round-trip time). That's why **MANY** (yet **SMALL**) js/css files must be packed into
**FEW** (yet **LARGE**) ones for decent page loading time, while the total traffic amount is
almost the same.

Newer protocols like `HTTP/2` and [QUIC](https://en.wikipedia.org/wiki/QUIC)
(a.k.a [HTTP/3](https://www.zdnet.com/article/http-over-quic-to-be-renamed-http3/))
are addressing various issues,
especially including the above said one, but suffering from legacy burden for backward
compatibility with `HTTP/1.1`, they have gone not far.

## The Solution

Building new applications with **Hosting Based Interface** - HBI, the classic
**request/response** pattern can go naturally & very efficiently without imposing the
dreadful RTT (if done correctly, see Caveats).

### No Wait At Best, Hosting instead of Receiving

Active receiving is to wait, while hosting is **NO** wait.
It is best to be **hosting** rather than **receiving**.

### In Action

Checkout [HBI Chat](https://github.com/complyue/hbichat) and run it for fun.
That project can be considered an
[SSCCE](http://www.sscce.org/)
HBI application.

## What is HBI

### Throughput Oriented Communication Infrastructure

By [Pipelining](<https://en.wikipedia.org/wiki/Pipeline_(computing)>) the underlying transport
wire, though lantency of each single API call is not improved, the over system can process
largely more calls in a given window of time, i.e. optimal efficiency at overall throughput.

### API Defined Protocol

**HBI** is a _meta protocol_ for application systems (read **service** software components),
possibly implemented in different programming languages and/or base runtimes,
to establish communication channels between
[os process](<https://en.wikipedia.org/wiki/Process_(computing)>)es
(may or may not across computing nodes), as to communicate with _peer-scripting-code_ posted
to eachother's
**hosting environment**.

By providing a **hosting environment** which exposes necessary artifacts (various
**functions** in essense, see Mechanism) to accommodate the **landing** of the
_peer-scripting-code_ from the other end, a service process defines both its
[API](https://en.wikipedia.org/wiki/Application_programming_interface) and the effect
network protocol to access the API, at granted efficience.

Such network protocols are called **API Defined Protocol**s.

### Example - Download a File in a Room

You should see:

- There's no explicit network manipulations, just send/recv using the converstion object:
  - `co.send_obj()` / `co.recv_obj()`
  - `co.send_data()` / `co.recv_data()`
- Control info (i.e. request, accept/refuse msg, file size etc.) is straight forward
- Binary data streaming is straight forward too, i.e. no excessive memory used to hold full
  file data, and the buffer size can be arbitrarily chosen at either side
- The checksum of full data stream is calculated straight forward as well, no extra scan

And you should know:

- All network traffic is pipelined with a single tcp connection underlying
- The underlying tcp connection is shared with other service calls
- No service call will pend the tcp connection to block other calls, when it's not sending sth
- So the tcp connection is always at its max throughput possible (with RTT eliminated entirely)
- No sophiscated network protocol design & optimisation needed to achieve all above

Service API Implementation:

```python
    async def SendFile(self, room_id: str, fn: str):
        co = self.ho.co

        fpth = os.path.abspath(os.path.join("chat-server-files", room_id, fn))
        if not os.path.exists(fpth) or not os.path.isfile(fpth):
            # send negative file size, meaning download refused
            await co.send_obj(repr([-1, f"no such file"]))
            return

        s = os.stat(fpth)

        with open(fpth, "rb") as f:
            # get file data size
            f.seek(0, 2)
            fsz = f.tell()

            # send [file-size, msg] to peer, telling it the data size to receive and last
            # modification time of the file.
            msg = "last modified: " + datetime.fromtimestamp(s.st_mtime).strftime(
                "%F %T"
            )
            await co.send_obj(repr([fsz, msg]))

            # prepare to send file data from beginning, calculate checksum by the way
            f.seek(0, 0)
            chksum = 0

            def stream_file_data():  # a generator function is ideal for binary data streaming
                nonlocal chksum  # this is needed outer side, write to that var

                # nothing prevents the file from growing as we're sending, we only send
                # as much as glanced above, so count remaining bytes down,
                # send one 1-KB-chunk at max at a time.
                bytes_remain = fsz
                while bytes_remain > 0:
                    chunk = f.read(min(1024, bytes_remain))
                    assert len(chunk) > 0, "file shrunk !?!"
                    bytes_remain -= len(chunk)

                    yield chunk  # yield it so as to be streamed to client
                    chksum = crc32(chunk, chksum)  # update chksum

                assert bytes_remain == 0, "?!"

            # stream file data to consumer end
            await co.send_data(stream_file_data())

        # send chksum at last
        await co.send_obj(repr(chksum))
```

Consumer Usage:

```python
    async def _download_file(self, fn):
        lg = self.line_getter

        room_dir = os.path.abspath(f"chat-client-files/{self.in_room}")
        if not os.path.isdir(room_dir):
            self.line_getter.show(f"Making room dir [{room_dir}] ...")
            os.makedirs(room_dir, exist_ok=True)

        async with self.po.co() as co:  # start a new posting conversation

            # send out download request
            await co.send_code(
                rf"""
SendFile({self.in_room!r}, {fn!r})
"""
            )

        # receive response AFTER the posting conversation closed,
        # this is crucial for overall throughput.
        fsz, msg = await co.recv_obj()
        if fsz < 0:
            lg.show(f"Server refused file downlaod: {msg}")
            return
        elif msg is not None:
            lg.show(f"@@ Server: {msg}")

        fpth = os.path.join(room_dir, fn)

        with open(fpth, "wb") as f:
            total_kb = int(math.ceil(fsz / 1024))
            lg.show(f" Start downloading {total_kb} KB data ...")

            # prepare to recv file data from beginning, calculate checksum by the way
            chksum = 0

            def stream_file_data():  # a generator function is ideal for binary data streaming
                nonlocal chksum  # this is needed outer side, write to that var

                # receive 1 KB at most at a time
                buf = bytearray(1024)

                bytes_remain = fsz
                while bytes_remain > 0:

                    if len(buf) > bytes_remain:
                        buf = buf[:bytes_remain]

                    yield buf  # yield it so as to be streamed from client

                    f.write(buf)  # write received data to file

                    bytes_remain -= len(buf)

                    chksum = crc32(buf, chksum)  # update chksum

                    remain_kb = int(math.ceil(bytes_remain / 1024))
                    lg.show(  # overwrite line above prompt
                        f"\x1B[1A\r\x1B[0K {remain_kb:12d} of {total_kb:12d} KB remaining ..."
                    )

                assert bytes_remain == 0, "?!"

                # overwrite line above prompt
                lg.show(f"\x1B[1A\r\x1B[0K All {total_kb} KB received.")

            # receive data stream from server
            start_time = time.monotonic()
            await co.recv_data(stream_file_data())

        peer_chksum = await co.recv_obj()
        elapsed_seconds = time.monotonic() - start_time

        # overwrite line above
        lg.show(
            f"\x1B[1A\r\x1B[0K All {total_kb} KB downloaded in {elapsed_seconds:0.2f} second(s)."
        )
        # validate chksum calculated at peer side as it had all data sent
        if peer_chksum != chksum:
            lg.show(f"But checksum mismatch !?!")
        else:
            lg.show(
                rf"""
@@ downloaded {chksum:x} [{fn}]
"""
            )
```

### Mechanism

An `HBI` communication channel (wire) works Peer-to-Peer, each peer has 2 endpoints:
the `posting endpoint` and the `hosting endpoint`. A peer is said to be acting actively
when doing sending works with its `posting endpoint`, and is conversely said to be
acting passively when doing receiving (`landing`) works with its `hosting endpoint`.

At any time, either peer can initiate a `posting conversation` for active communication,
in response to that, a `hosting conversation` will be triggered at the other peer, for
passive communication.

Both posting and hosting conversations have 2 stages, the `send` and the `recv` stage,
a posting conversation starts out in `send` stage, while a hosting conversation starts out
in `recv` stage. The transition from initial stage to the other is controlled by application.
And only corresponding type of operations are allowed in each type of stage, i.e.

- In `send` stage:
  - send operations allowed
  - recv operations prohibited
- In `recv` stage:
  - recv operations allowed
  - send operations prohibited

Mixing of the 2 types of operation during either type of conversation is
[deaklock](https://en.wikipedia.org/wiki/Deadlock) prone, in the context that underlying
transport wire is shared among many conversations for overall throughput.

A `posting conversation` **SHOULD** transit to `recv` stage as soon as possible, the transition
is needed to release the underlying transport wire for other conversations to start sending.

A _FIRE-AND-FORGET_ posting conversation is closed without stage transition and any `recv`
operation, while in other cases the application transit a posting conversation to `recv`
stage, then perform `recv` operations with it to get the response in the form of a series of
value objects and/or data/stream, then finally close it.

As the other peer sees inbound traffic about the conversation, it establishes a
`hosting-conversation` to accommodate the `landing` of _peer-scripting-code_ received.

`landing` is simply the execution of textual scripting code, with the chosen programming
language / runtime, with the `hosting envrionment` as context. e.g.
[exec()](https://docs.python.org/3/library/functions.html#exec) is used with Python, and
[Anko interpreter](https://github.com/mattn/anko "Anko's Github Home") is used with Golang.

The `hosting environment` of one peer is openly accessible by the _peer-scripting-code_
sent from the other peer, to the extent the former peer is willing of exposure.

The _peer-scripting-code_ just executes as being `landed`, it scripts the hosting peer
for desired impacts by the posting peer.

    One extra optional responsibility of the _peer-scripting-code_ is that: if binary
    data/stream follows, the priori sent code _MUST_ receive or streamline the data
    properly, and bearing such responsibilities, it is then called `receiving-code`.

There normally occur subsequences as the hosting peer is being scripted to do anything,
e.g. in case the posting peer is a software agent in behalf of its user to start a live
video casting, all the user's subscribers should be notified of the starting of video
stream, and a streaming channel should be established to each ready subscriber, then the
broadcaster should be notified how many subscribers will be watching.

The _peer-scripting-code_ instructs about all those things as **WHAT** to do, and the
`hosting envirnoment` should expose enough artifacts implementing **HOW** to do each of those.

Theoretically every artifact exposed by the hosting environment is a **function**, which takes
specific number/type of arguments, generates side-effects, and returns specific number/type
of result (no return in case the number is zero).

While with Object-Oriented programming paradigm, there arose more types of **function** s that
carrying special semantics:

- `constructor` function:

  that creates a new (tho not strictly necessary newly allocated) object on each call

  Note:
  In HBI paradigm the `new` keyword should not appear for invocation of
  a `ctor` function. HBI peer script follows Python syntax and is different
  from Go/C++/Java/JavaScript syntax.

- `reactor method` function:

  that has a `reactor` object bound to it

  Meaning it has an implicit argument referencing the `reactor object`, in addition to its
  formal arguments.

The implementation of a **function** exposed by a `hosting environment`, normally (the exact
case that `response` is expected) does leverage the `hosting conversation` to send another
set of _peer-scripting-code_, optionally with binary data/stream (the `response`), back to
the posting peer, for the subsequences be realized at the posting site.

This set of _peer-scripting-code_ (plus data/stream if present), is `landed` (meaning
received & processed) during the `after-posting stage` of the posting peer's original
`posting conversation`, as mentioned in earlier paragrah of this section.

Additionally, the implementation can schedule more activities to happen later, and any
activity can then start new `posting conversation`s to the _OP_, i.e. communication in the
reverse direction.

Orchestration forms when multiple service/consumer nodes keep communicating with many others
through p2p connections.

### HBI over vanilla TCP

- Python 3.7+

  - Client

```python
import asyncio, hbi


async def say_hello_to(addr):

    po, ho = await hbi.dial_socket(addr, hbi.HostingEnv())

    async with po.co() as co:

        await co.send_code(
            f"""
my_name = 'Nick'
hello()
"""
        )

    msg_back = await co.recv_obj()

    print(msg_back)


asyncio.run(say_hello_to({"host": "127.0.0.1", "port": 3232}))
```

- Outut

```shell
Hello, HBI world!
Hello, Nick from 127.0.0.1:48784!
```

- Server

```python
import asyncio, hbi


def he_factory() -> hbi.HostingEnv:
    he = hbi.HostingEnv()

    he.expose_function(
        "__hbi_init__",  # callback on wire connected
        lambda po, ho: po.notif(
            f"""
print("Hello, HBI world!")
"""
        ),
    )

    he.expose_function(
        "hello",  # reacting function name
        lambda: he.ho.co.send_obj(
            repr(f"Hello, {he.get('my_name')} from {he.po.remote_addr}!")
        ),
    )

    return he


async def serve_hello():

    server = await hbi.serve_socket(
        {"host": "127.0.0.1", "port": 3232},  # listen address
        he_factory,  # factory for hosting environment
    )
    print("hello server listening:", server.sockets[0].getsockname())
    await server.wait_closed()


try:
    asyncio.run(serve_hello())
except KeyboardInterrupt:
    pass
```

- Go1

  - Client

```go
package main

import (
	"fmt"

	"github.com/complyue/hbi"
)

func main() {

	he := hbi.NewHostingEnv()

	he.ExposeFunction("print", fmt.Println)

	po, ho, err := hbi.DialTCP("localhost:3232", he)
	if err != nil {
		panic(err)
	}
	defer ho.Close()

	co, err := po.NewCo()
	if err != nil {
		panic(err)
	}
	func() {
		defer co.Close()

		if err = co.SendCode(`
my_name = "Nick"
hello()
`); err != nil {
			panic(err)
		}
	}()

	msgBack, err := co.RecvObj()
	if err != nil {
		panic(err)
	}
	fmt.Println(msgBack)
}
```

- Output

```shell
Hello, HBI world!
Hello, Nick from 127.0.0.1:48778!
```

- Server

```go
package main

import (
	"fmt"
	"net"

	"github.com/complyue/hbi"
)

func main() {

	hbi.ServeTCP("localhost:3232", func() *hbi.HostingEnv {
		he := hbi.NewHostingEnv()

		he.ExposeFunction("__hbi_init__", // callback on wire connected
			func(po *hbi.PostingEnd, ho *hbi.HostingEnd) {
				po.Notif(`
print("Hello, HBI world!")
`)
			})

		he.ExposeFunction("hello", func() {
			if err := he.Ho().Co().SendObj(hbi.Repr(fmt.Sprintf(
				`Hello, %s from %s!`,
				he.Get("my_name"), he.Po().RemoteAddr(),
			))); err != nil {
				panic(err)
			}
		})
		return he
	}, func(listener *net.TCPListener) {
		fmt.Println("hello server listening:", listener.Addr())
	})

}
```

- ES6

```js
// Coming later, if not sooner ...
```

### HBI over [QUIC](https://en.wikipedia.org/wiki/QUIC)

Concurrent conversations can work upon QUIC streams, coming later, if not sooner ...

## Caveats

### For Overall Throughput

- Do **NO** **receiving** at best, be **hosting** (i.e. **landing** peer scripts) instead,
- Decided to **receive**, **ONLY** do with a hosting conversation,
- Decided to **receive** with a posting conversation, **ONLY** do during the `after-posting stage`.

  Note: But you are not technically prevented to **receive** during the `posting stage`, well
  doing so will pend the underlying wire, stop pipelining of dataflow, thus _HARM A LOT_
  to overall throughput. And you might even be taking more chances to create
  [Deaklock](https://en.wikipedia.org/wiki/Deadlock)s
