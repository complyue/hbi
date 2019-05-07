# Hosting Based Interface

## Server Paradigm:

```python

import asyncio, hbi

POOR_THROUGHPUT = False

if '__job_context__' == __name__:

    po2peer: hbi.PostingEnd = None
    ho4peer: hbi.HostingEnd = None

    # get called when an hbi connection is made
    async def __hbi_init__(po, ho):
        global po2peer: hbi.PostingEnd
        global ho4peer: hbi.HostingEnd

        po2peer, ho4peer = po, ho

    # this is the service method doing its job
    # it's called by service consumer as scripted through hbi wire
    async def do_job(action, data_len):
        # responding within the hosting endpoint's current conversation
        co = ho4peer.co

        # this method expects binary input in addition to vanilla args (the `action` arg e.g.)
        #   * which can be received as plain blob
        job_data = bytearray(data_len)
        #   * or numpy array with shape&dtype infered from data_len
        #shape, dtype = data_len
        #job_data = np.empty(shape, dtype)
        #   * or any form of a binary stream

        # hbi wire can receive binary streams very efficiently,
        # a single binary buffer or an iterator of binary buffers can be passed to
        # `co.recv_data()` so long as their summed bytes count matches that sent by
        # the peer endpoint
        await co.recv_data(job_data)

        # use all inputs, i.e. action/job_data
        job_result = ...

        # !! try best to avoid such synchronous service calls !!
        if POOR_THROUGHPUT:
            # send back the code snippet of result, which will be landed by peer hosting endpoint,
            # and the eval result received by consumer application via `await co.recv_obj()`
            await co.send_obj(repr(job_result))
            # !! this holds down throughput REALLY !!
        else:
            # it's best for throughput to send asynchronous notification back
            # to the service consumer context
            await co.send_code(rf'''
job_done({job_result!r})
''')

elif '__main__' == __name__:

    serving_addr = {'host': '127.0.0.1', 'port': 3232}

    async def serve_jobs():
        await hbi.HBIS(
            # listening IP address
            serving_addr,
            # the service context factory function,
            # create an isolated context for each consumer connection
            lambda po, ho: runpy.run_module(
                # reuse this module file for both service context and `python -m` entry point
                mod_name = __package__, # this module file needs to be `some/py_pkg/__main__.py`
                # telling the module init purpose via run_name, i.e. the global __name__ value
                run_name = '__job_context__',
            ),
        ).serve_until_closed()

    asyncio.run(serve_jobs())

```
