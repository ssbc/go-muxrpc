// SPDX-License-Identifier: MIT

var MRPC = require('muxrpc')
var pull = require('pull-stream')
var toPull = require('stream-to-pull-stream')

var api = {
  hello: 'async',
  stuff: 'source'
}

var client = MRPC(api, null)()
var a = client.createStream()

pull(a, toPull.sink(process.stdout))
pull(toPull.source(process.stdin), a)

var cnt = 0
setInterval(function () {
  client.hello('world' + cnt, function (err, value) {
    if (err) {
      console.error('hello:failed')
      return
    }
    console.error(value) // hello, world!
  })
  cnt++
}, 1000)

setInterval(function () {
  pull(client.stuff(), pull.drain(console.error))
}, 2000)
