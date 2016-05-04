// a simple RPC server for client tests
var MRPC = require('muxrpc')
var pull = require('pull-stream')
var toPull = require('stream-to-pull-stream')

var api = {
  hello: 'async',
  stuff: 'source'
}

var server = MRPC(null, api)({
  hello: function (name, cb) {
    console.error('hello:ok')
    cb(null, 'hello, ' + name + '!')
  },
  stuff: function () {
    console.error('stuff called')
    return pull.values([{"ohai":1}, {"omg":2}, {"wtf":3}, {"obj":4}])
  }
})

var a = server.createStream()
pull(a, toPull.sink(process.stdout))
pull(toPull.source(process.stdin), a)
