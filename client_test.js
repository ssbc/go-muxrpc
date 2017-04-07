// a simple RPC server for client tests
var MRPC = require('muxrpc')
var pull = require('pull-stream')
var toPull = require('stream-to-pull-stream')

var api = {
  hello: 'async',
  stuff: 'source'
}

var server = MRPC(null, api)({
  hello: function (name, name2, cb) {
    console.error('hello:ok')
    cb(null, 'hello, ' + name + ' and ' + name2 + '!')
  },
  stuff: function () {
    console.error('stuff called')
    return pull.values([{"a":1}, {"a":2}, {"a":3}, {"a":4}])
  }
})

var a = server.createStream()
pull(a, toPull.sink(process.stdout))
pull(toPull.source(process.stdin), a)
