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

Building new applications with **Hosting Based Interface** - HBI, the classic
**request/response** pattern can go naturally & very efficiently without imposing the
dreadful RTT (if done correctly, see Caveats).

### No Wait At Best, Hosting instead of Receiving

Active receiving is to wait, while hosting is **NO** wait.
It is best to be **hosting** rather than **receiving**.

## In Action

Checkout [HBI Chat](https://github.com/complyue/hbichat) and run it for fun.
That project can be considered an
[SSCCE](http://www.sscce.org/)
HBI application.

## Mechanism

An `HBI` communication channel (wire) works Peer-to-Peer, each peer has 2 endpoints:
the `posting endpoint` and the `hosting endpoint`. A peer is said to be acting actively
when doing sending works with its `posting endpoint`, and is conversely said to be
acting passively when doing receiving (`landing`) works with its `hosting endpoint`.

At any time, either peer can initiate a `posting conversation` for active communication,
in response to that, a `hosting conversation` will be triggered at the other peer, for
passive communication.

A `posting conversation` has 2 stages, -- the `posting stage` and the `after-posting stage`.
During the `posting stage`, _peer-scripting-code_, i.e. textual code meant to be `landed`
by peer's `hosting environment` (more on this later), optionally with binary data/stream,
are sent through the underlying transport/wire.

A `posting conversation` **SHOULD** be `closed` as earlier as possible once all its sending works
are done, this actually releases the underlying transport/wire for other `posting conversation`s
to start off. Upon `closed`, the `posting conversation` switches to its `after-posting stage`,
there ideally be no activity at all to happen within the `after-posting stage`, which makes it
a _FIRE-AND-FORGET_ conversation. But `posting stage` is only the `request` part of the
**request/response** pattern, `response`s are destined to be received and processed in many
real-world cases, and `response`, if expected, is to be received during the `after-posting stage`.

As the other peer sees inbound traffic about the conversation, it establishes a
`hosting-conversation` to accommodate the `landing` of _peer-scripting-code_ received.

A `hosting conversation` has just 1 stage, naming is not necessary but just call it
`hosting stage`. During the `hosting stage`, the hosting peer uses its `hosting environment`
to `land` whatever textual code sent by the posting peer.

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

Python 3.7+

```python
# Coming sooner than later ...
```

Go1

```go
// Coming sooner than later ...
```

ES6

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
